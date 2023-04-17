package util

import (
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"regexp"
	"strconv"
	"strings"

	"k8s.io/klog/v2"
	"os"
	"time"
)

const (
	NVIDIAAnnotation              = "nvidia.com/gpu"
	VCoreAnnotation               = "tencent.com/vcuda-core"
	PreStartContainerCheckErrMsg  = "PreStartContainer check failed"
	PreStartContainerCheckErrType = "PreStartContainerCheckErr"
	UnexpectedAdmissionErrType    = "UnexpectedAdmissionError"
	PredicateTimeAnnotation       = "tencent.com/predicate-time"
	PredicateGPUIndexPrefix       = "tencent.com/predicate-gpu-idx-"
	GPUAssigned                   = "tencent.com/gpu-assigned"
)

const (
	NvidiaCtlDevice    = "/dev/nvidiactl"
	NvidiaUVMDevice    = "/dev/nvidia-uvm"
	NvidiaFullpathRE   = `^/dev/nvidia([0-9]*)$`
	NvidiaDevicePrefix = "/dev/nvidia"
)

func GetCandidatePods(client kubernetes.Interface, hostname string) ([]*v1.Pod, error) {
	candidatePods := []*v1.Pod{}
	allPods, err := getPodsOnNode(client, hostname, string(v1.PodPending))
	if err != nil {
		return candidatePods, err
	}
	for _, pod := range allPods {
		current := pod
		if IsGPURequiredPod(&current) && !ShouldDelete(&current) {
			candidatePods = append(candidatePods, &current)
		}
	}

	if klog.V(4).Enabled() {
		for _, pod := range candidatePods {
			klog.Infof("candidate pod %s in ns %s  is found.",
				pod.Name,
				pod.Namespace,
			)
		}
	}

	return candidatePods, nil
}

func getPodsOnNode(client kubernetes.Interface, hostname string, phase string) ([]v1.Pod, error) {
	if len(hostname) == 0 {
		hostname, _ = os.Hostname()
	}
	var (
		selector fields.Selector
		pods     []v1.Pod
	)

	if phase != "" {
		selector = fields.SelectorFromSet(fields.Set{"spec.nodeName": hostname, "status.phase": phase})
	} else {
		selector = fields.SelectorFromSet(fields.Set{"spec.nodeName": hostname})
	}
	var (
		podList *v1.PodList
		err     error
	)

	err = wait.PollImmediate(time.Second, time.Minute, func() (bool, error) {
		podList, err = client.CoreV1().Pods(v1.NamespaceAll).List(context.Background(), metav1.ListOptions{
			FieldSelector: selector.String(),
			LabelSelector: labels.Everything().String(),
		})
		if err != nil {
			return false, err
		}
		return true, nil
	})
	if err != nil {
		return pods, fmt.Errorf("failed to get Pods on node %s because: %v", hostname, err)
	}

	klog.V(9).Infof("all pods on this node: %v", podList.Items)
	for _, pod := range podList.Items {
		pods = append(pods, pod)
	}

	return pods, nil
}
func IsGPURequiredPod(pod *v1.Pod) bool {
	gpu := GetGPUResourceOfPod(pod, NVIDIAAnnotation)

	// Check if pod request for GPU resource
	if gpu <= 0 {
		klog.V(4).Infof("Pod %s in namespace %s does not Request for NVIDIA GPU resource",
			pod.Name,
			pod.Namespace)
		return false
	}

	return true
}

func IsGPURequiredContainer(c *v1.Container) bool {
	klog.V(4).Infof("Determine if the container %s needs NVIDIA GPU resource", c.Name)

	gpu := GetGPUResourceOfContainer(c, NVIDIAAnnotation)

	// Check if container request for GPU resource
	if gpu <= 0 {
		klog.V(4).Infof("Container %s does not Request for NVIDIA GPU resource", c.Name)
		return false
	}

	return true
}

func GetGPUResourceOfPod(pod *v1.Pod, resourceName v1.ResourceName) uint {
	var total uint
	containers := pod.Spec.Containers
	for _, container := range containers {
		if val, ok := container.Resources.Limits[resourceName]; ok {
			total += uint(val.Value())
		}
	}
	return total
}
func ShouldDelete(pod *v1.Pod) bool {
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Waiting != nil &&
			strings.Contains(status.State.Waiting.Message, PreStartContainerCheckErrMsg) {
			return true
		}
	}
	if pod.Status.Reason == UnexpectedAdmissionErrType {
		return true
	}
	return false
}
func GetGPUResourceOfContainer(container *v1.Container, resourceName v1.ResourceName) uint {
	var count uint
	if val, ok := container.Resources.Limits[resourceName]; ok {
		count = uint(val.Value())
	}
	return count
}

func GetVCudaDevice(client kubernetes.Interface, hostname string) ([]string, error) {
	allPods, err := getPodsOnNode(client, hostname, string(v1.PodRunning))
	if err != nil {
		return nil, err
	}
	devMap := make(map[string]struct{}, 0)
	for _, pod := range allPods {
		for i, _ := range pod.Spec.Containers {
			if idxStr, ok := pod.ObjectMeta.Annotations[PredicateGPUIndexPrefix+strconv.Itoa(i)]; ok {
				if _, err := strconv.Atoi(idxStr); err != nil {
					return nil, fmt.Errorf("predicate idx %s invalid for pod %s ", idxStr, pod.UID)
				}
				devStr := NvidiaDevicePrefix + idxStr
				if !IsValidGPUPath(devStr) {
					return nil, fmt.Errorf("predicate idx %s invalid", devStr)
				}
				if _, ok := devMap[idxStr]; !ok {
					devMap[idxStr] = struct{}{}
				}
			}
		}
	}
	devList := []string{}
	for dev, _ := range devMap {
		devList = append(devList, dev)
	}
	return devList, nil
}

// IsValidGPUPath checks if path is valid Nvidia GPU device path
func IsValidGPUPath(path string) bool {
	return regexp.MustCompile(NvidiaFullpathRE).MatchString(path)
}

func GetContainer(deviceIds []string) (found bool, candidatePod *v1.Pod, candidateContainer *v1.Container, candidateContainerIdx int, err error) {

	k8sclient, hostname, err := GetClientAndHostName()
	// 2. 获取CandidatePods
	candidatePods, err := GetCandidatePods(k8sclient, hostname)
	if err != nil {
		fmt.Println("GetCandidatePods err", err)
		return
	}
	fmt.Println("11111111111111111111  candidatePods")
	for _, pod := range candidatePods {
		fmt.Println(pod.Name)
	}

	//3. 获取pod，container
	for _, pod := range candidatePods {
		if found {
			break
		}
		for i, c := range pod.Spec.Containers {
			gpuReq := GetGPUResourceOfContainer(&c, NVIDIAAnnotation)
			if gpuReq == 0 || len(deviceIds) != int(gpuReq) {
				continue
			}

			//if _, err := GetNvidiaPredicateIdxOfContainer(pod, i); err == nil {
			//	continue
			//}

			candidatePod = pod
			candidateContainer = &candidatePod.Spec.Containers[i]
			candidateContainerIdx = i
			found = true
			break
		}
	}
	return
}
