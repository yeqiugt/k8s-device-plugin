package util

import (
	"fmt"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"os"
)

func Contains(ids []string, id string) bool {
	for _, i := range ids {
		if i == id {
			return true
		}
	}
	return false
}

func GetClientAndHostName() (*kubernetes.Clientset, string, error) {
	// 1. 获取client
	config, err := rest.InClusterConfig()
	if err != nil {
		fmt.Println("get incluster config err")
		return &kubernetes.Clientset{}, "", err
	}
	k8sclient, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Println("getConfig err ", err)
		return &kubernetes.Clientset{}, "", err
	}
	hostname, _ := os.Hostname()
	return k8sclient, hostname, nil

}
