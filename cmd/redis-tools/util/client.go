package util

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func NewClient() (*kubernetes.Clientset, error) {
	var (
		err  error
		conf *rest.Config
	)

	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" && port == "" {
		if fp := os.Getenv("KUBE_CONFIG"); fp != "" {
			if conf, err = clientcmd.BuildConfigFromFlags("", fp); err != nil {
				return nil, fmt.Errorf("load config from $KUBE_CONFIG failed, error=%s", err)
			}
		} else {
			if home := homedir.HomeDir(); home != "" {
				fp := filepath.Join(home, ".kube", "config")
				if conf, err = clientcmd.BuildConfigFromFlags("", fp); err != nil {
					return nil, fmt.Errorf("load config from local .kube/config failed, error=%s", err)
				}
			} else {
				return nil, fmt.Errorf("no local config found")
			}
		}
	} else {
		conf, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	}
	return kubernetes.NewForConfig(conf)
}
