package plugin

import (
	"fmt"
	"github.com/NVIDIA/k8s-device-plugin/util"
)

func GetInUseDevice(plugin *NvidiaDevicePlugin) map[string]bool {
	fmt.Println("22222222222222222 all device List ")
	devUsage := make(map[string]bool, 0)
	deviceList := plugin.Devices()
	for _, d := range deviceList {
		devUsage[d.Index] = false
		fmt.Println(d.Index)
		fmt.Println(d.GetUUID())
	}

	// 4. 获取vcuda占用的设备
	k8sclient, hostname, err := util.GetClientAndHostName()
	inUsedDev, err := GetVCudaDevice(k8sclient, hostname)
	if err != nil {
		fmt.Println("GetVCudaDevice err", err)
	}
	fmt.Println("4444444444444 vcuda in use device", inUsedDev)
	for _, dev := range inUsedDev {
		devUsage[dev] = true
	}
	return devUsage

}
func GetAllocDevice(found bool, devUsage map[string]bool, reqDeviceIds []string) ([]string, error) {
	var devAlloc []string
	if found {
		//
		for _, dev := range reqDeviceIds {
			if inUse, ok := devUsage[dev]; ok {
				if !inUse {
					devAlloc = append(devAlloc, dev)
				}
			}
		}
		num := len(reqDeviceIds) - len(devAlloc)
		for dev, inUse := range devUsage {
			if num <= 0 {
				break
			}
			if inUse {
				continue
			}
			devAlloc = append(devAlloc, dev)
			devUsage[dev] = true
			num--
		}
	}
	if len(reqDeviceIds) != len(devAlloc) {
		fmt.Println("6666666666666666alloc err ")
	}
	fmt.Println("66666666666666666", devAlloc)
	return devAlloc, nil
}
