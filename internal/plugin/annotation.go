package plugin

import (
	"fmt"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"strings"
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
		reqDevice := devices.GetByID(deviceId)

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

func ParserAnnotation(anotation map[string]string) []string {
	var deviceIds []string

	for key, value := range anotation {
		if strings.Contains(key, "inspur.com/gpu-index-idx-") {
			tmp := strings.Split(value, "-")
			deviceIds = append(deviceIds, tmp...)
		}
	}

	return deviceIds
}
