package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/k8s-device-plugin/util"
	v1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"strings"
	"time"
)

type PatchType string

const (
	JSONPatchType           PatchType = "application/json-patch+json"
	MergePatchType          PatchType = "application/merge-patch+json"
	StrategicMergePatchType PatchType = "application/strategic-merge-patch+json"
	ApplyPatchType          PatchType = "application/apply-patch+yaml"
)

// container只会使用独占或者mig的GPU，不会同时使用
func (plugin *NvidiaDevicePlugin) GetAnnotation(containerId int, deviceIds []string) map[string]string {
	devices := plugin.Devices()
	fmt.Println("111111111111111all device ids:", devices.GetIDs())
	var (
		gpuIdx    string
		gpuPcieId string
		gpuMod    string
	)

	gpuModKey := fmt.Sprintf("inspur.com/gpu-mod-idx-%d", containerId)
	gpuIdxKey := fmt.Sprintf("inspur.com/gpu-index-idx-%d", containerId)
	gpuPciKey := fmt.Sprintf("inspur.com/gpu-gpuPcieId-%d", containerId)

	for _, deviceId := range deviceIds {
		fmt.Println("222222222222222222request ids : ", deviceId)
		reqDevice := devices.GetByIndex(deviceId)

		fmt.Println("333333333333333333request uuid : ", reqDevice.GetUUID())

		fmt.Println("44444444444444444is mig ?", reqDevice.IsMigDevice())
		if reqDevice.IsMigDevice() {

			// handle mig
			gpuMod = "mig"

		} else {
			// handle gpu
			nvmlDevice, _ := nvml.DeviceGetHandleByUUID(reqDevice.GetUUID())
			pcieInfo, _ := nvml.DeviceGetPciInfo(nvmlDevice)

			fmt.Printf("555555555555555555555gpu index: %s, gpu uuid : %s \n", reqDevice.Index, reqDevice.GetUUID())
			fmt.Printf("6666666666666666666pcie info : %+v \n", pcieInfo)

			if gpuPcieId == "" {
				gpuPcieId = fmt.Sprintf("%d", pcieInfo.PciDeviceId)
			} else {
				gpuPcieId += "-" + fmt.Sprintf("%d", pcieInfo.PciDeviceId)
			}
			if gpuIdx == "" {
				gpuIdx = reqDevice.Index
			} else {
				gpuIdx += "-" + reqDevice.Index
			}

			gpuMod = "nvidia"
			fmt.Println("777777777777777777777")
		}
	}
	fmt.Println("888888888888888888888888")
	return map[string]string{
		gpuModKey: gpuMod,
		gpuIdxKey: gpuIdx,
		gpuPciKey: gpuPcieId,
	}
}

func ParserAnnotation(anotation map[string]string, containerId int) []string {
	var deviceIds []string
	gpuIdxKey := fmt.Sprintf("inspur.com/gpu-index-idx-%d", containerId)
	for key, value := range anotation {
		if strings.Contains(key, gpuIdxKey) {
			tmp := strings.Split(value, "-")
			deviceIds = append(deviceIds, tmp...)
		}
	}

	return deviceIds
}

func PacthPodAnnotation(annotation map[string]string, pod *v1.Pod) error {
	client, _, _ := util.GetClientAndHostName()
	waitTimeout := 10 * time.Second

	// update annotations by patching to the pod
	type patchMetadata struct {
		Annotations map[string]string `json:"annotations"`
	}
	type patchPod struct {
		Metadata patchMetadata `json:"metadata"`
	}
	payload := patchPod{
		Metadata: patchMetadata{
			Annotations: annotation,
		},
	}

	payloadBytes, _ := json.Marshal(payload)
	err := wait.PollImmediate(time.Second, waitTimeout, func() (bool, error) {
		_, err := client.CoreV1().Pods(pod.Namespace).
			Patch(context.Background(), pod.Name, types.PatchType(StrategicMergePatchType), payloadBytes, metav1.PatchOptions{})
		if err == nil {
			return true, nil
		}
		if ShouldRetry(err) {
			return false, nil
		}

		return false, err
	})
	if err != nil {
		msg := fmt.Sprintf("failed to add annotation %v to pod %s due to %s",
			annotation, pod.UID, err.Error())
		klog.Infof(msg)
		return fmt.Errorf(msg)
	}
	return nil
}
func ShouldRetry(err error) bool {
	return apierr.IsConflict(err) || apierr.IsServerTimeout(err)
}
