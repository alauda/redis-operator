/*
Copyright 2023 The RedisOperator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cluster

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

// RetryGet
func RetryGet(f func() error, steps ...int) error {
	step := 5
	if len(steps) > 0 && steps[0] > 0 {
		step = steps[0]
	}
	return retry.OnError(wait.Backoff{
		Steps:    step,
		Duration: 400 * time.Millisecond,
		Factor:   2.0,
		Jitter:   2,
	}, func(err error) bool {
		return errors.IsInternalError(err) || errors.IsServerTimeout(err) || errors.IsServiceUnavailable(err) ||
			errors.IsTimeout(err) || errors.IsTooManyRequests(err)
	}, f)
}

// GetStatefulSet
func GetStatefulSet(ctx context.Context, client *kubernetes.Clientset, namespace, name string) (*appv1.StatefulSet, error) {
	var sts *appv1.StatefulSet
	if err := RetryGet(func() (err error) {
		sts, err = client.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		return err
	}); err != nil {
		return nil, err
	}
	return sts, nil
}

// GetPod
func GetPod(ctx context.Context, client *kubernetes.Clientset, namespace, name string, logger logr.Logger) (*corev1.Pod, error) {
	var pod *corev1.Pod
	if err := RetryGet(func() (err error) {
		logger.Info("get pods ip", "namespace", namespace, "name", name)
		if pod, err = client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{}); err != nil {
			logger.Error(err, "get pods failed")
			return err
		} else if pod.Status.PodIP == "" {
			return errors.NewTimeoutError("pod have not assigied pod ip", 0)
		} else if pod.Status.HostIP == "" {
			return errors.NewTimeoutError("pod have not assigied host ip", 0)
		}
		return
	}, 20); err != nil {
		return nil, err
	}
	return pod, nil
}

// ExposeNodePort
func ExposeNodePort(ctx context.Context, client *kubernetes.Clientset, namespace, podName, ipfamily string, nodeportEnabled bool, customPort bool, logger logr.Logger) error {
	logger.Info("Info", "nodeport", nodeportEnabled, "ipfamily", ipfamily)
	pod, err := GetPod(ctx, client, namespace, podName, logger)
	if err != nil {
		logger.Error(err, "get pods failed", "namespace", namespace, "name", podName)
		return err
	}
	if pod.Status.HostIP == "" {
		return fmt.Errorf("pod not found or pod with invalid hostIP")
	}

	index := strings.LastIndex(podName, "-")
	sts, err := GetStatefulSet(ctx, client, namespace, podName[0:index])
	if err != nil {
		return err
	}

	var (
		announceIp         = pod.Status.PodIP
		announcePort int32 = 6379
		// announceIPort is this port necessary ?
		announceIPort int32 = 16379
	)
	if nodeportEnabled {
		// get svc
		podSvc, err := RetryGetService(client, namespace, podName, true, 1)
		if customPort {
			podSvc, err = RetryGetService(client, namespace, podName, true, 20)
		}
		if errors.IsNotFound(err) {
			newSvc := newPodService(namespace, podName, pod.GetLabels(), sts.OwnerReferences, ipfamily)
			if podSvc, err = client.CoreV1().Services(namespace).Create(ctx, newSvc, metav1.CreateOptions{}); err != nil {
				logger.Error(err, "create pod service failed")
				return err
			}
		} else if err != nil {
			logger.Error(err, "get service failed", "target", fmt.Sprintf("%s/%s", namespace, podName))
			return err
		}
		for _, v := range podSvc.Spec.Ports {
			if v.Name == "client" {
				announcePort = v.NodePort
			}
			if v.Name == "gossip" {
				announceIPort = v.NodePort
			}
		}

		node, err := client.CoreV1().Nodes().Get(context.TODO(), pod.Spec.NodeName, metav1.GetOptions{})
		if err != nil {
			logger.Error(err, "get nodes err", "node", node.Name)
			return err
		}
		logger.Info("get nodes success", "Name", node.Name)
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeExternalIP || addr.Type == corev1.NodeInternalIP {
				if addr.Address == "" {
					continue
				}
				logger.Info("Parsing node ", "IP", addr.Address)
				ip, err := netip.ParseAddr(addr.Address)
				if err != nil {
					logger.Error(err, "parse address err", "address", addr.Address)
					return err
				}
				if ipfamily == "IPv6" && ip.Is6() {
					announceIp = addr.Address
					break
				} else if ipfamily != "IPv6" && ip.Is4() {
					announceIp = addr.Address
					break
				}
			}
		}
	} else {
		for _, addr := range pod.Status.PodIPs {
			ip, err := netip.ParseAddr(addr.IP)
			if err != nil {
				return err
			}
			if ipfamily == "IPv6" && ip.Is6() {
				announceIp = addr.IP
				break
			} else if ipfamily != "IPv6" && ip.Is4() {
				announceIp = addr.IP
				break
			}
		}
	}

	format_announceIp := strings.Replace(announceIp, ":", "-", -1)
	labelPatch := fmt.Sprintf(`[{"op":"add","path":"/metadata/labels/%s","value":"%s"},{"op":"add","path":"/metadata/labels/%s","value":"%s"},{"op":"add","path":"/metadata/labels/%s","value":"%s"}]`,
		strings.Replace("middleware.alauda.io/announce_ip", "/", "~1", -1), format_announceIp,
		strings.Replace("middleware.alauda.io/announce_port", "/", "~1", -1), strconv.Itoa(int(announcePort)),
		strings.Replace("middleware.alauda.io/announce_iport", "/", "~1", -1), strconv.Itoa(int(announceIPort)))

	logger.Info(labelPatch)
	_, err = client.CoreV1().Pods(pod.Namespace).Patch(ctx, podName, types.JSONPatchType, []byte(labelPatch), metav1.PatchOptions{})
	if err != nil {
		logger.Error(err, "patch pod label failed")
		return err
	}
	configContent := fmt.Sprintf(`cluster-announce-ip %s
cluster-announce-port %d
cluster-announce-bus-port %d`, announceIp, announcePort, announceIPort)

	return os.WriteFile("/data/announce.conf", []byte(configContent), 0644)
}

func newPodService(namespace, name string, labels map[string]string, ownerRef []metav1.OwnerReference, ipfamily string) *corev1.Service {
	ptype := corev1.IPFamilyPolicySingleStack
	protocol := []corev1.IPFamily{}
	if ipfamily == "IPv6" {
		protocol = append(protocol, corev1.IPv6Protocol)
	} else {
		protocol = append(protocol, corev1.IPv4Protocol)
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerRef,
		},
		Spec: corev1.ServiceSpec{
			Selector:       map[string]string{"statefulset.kubernetes.io/pod-name": name},
			Type:           corev1.ServiceTypeNodePort,
			IPFamilies:     protocol,
			IPFamilyPolicy: &ptype,
			Ports: []corev1.ServicePort{
				{
					Name:       "client",
					Port:       6379,
					TargetPort: intstr.FromInt(6379),
					Protocol:   "TCP",
				},
				{
					Name:       "gossip",
					Port:       16379,
					TargetPort: intstr.FromInt(16379),
					Protocol:   "TCP",
				},
			},
		},
	}
}

func RetryGetService(clientset *kubernetes.Clientset, svcNamespace, svcName string, isCluster bool, count int) (*corev1.Service, error) {
	fmt.Println("RetryGetService", "NS", svcNamespace, "Name", svcName, "count", count)
	svc, err := clientset.CoreV1().Services(svcNamespace).Get(context.TODO(), svcName, metav1.GetOptions{})
	if count <= 1 {
		return svc, err
	}
	if errors.IsNotFound(err) {
		fmt.Println("waiting for svc create")
		time.Sleep(time.Second * 5)
		svc, err = RetryGetService(clientset, svcNamespace, svcName, isCluster, count-1)
	}
	if err == nil && isCluster {
		if len(svc.Spec.Ports) < 2 {
			fmt.Println("waiting for svc bus port update")
			time.Sleep(time.Second * 3)
			svc, err = RetryGetService(clientset, svcNamespace, svcName, isCluster, count-1)
		} else {
			for _, port := range svc.Spec.Ports {
				if port.NodePort == 0 {
					time.Sleep(time.Second * 3)
					fmt.Println("wait for node port allocation")
					svc, err = RetryGetService(clientset, svcNamespace, svcName, isCluster, count-1)
				}
			}
		}

	}
	return svc, err
}

func DefaultOwnerReferences(pod *corev1.Pod) []metav1.OwnerReference {
	or := metav1.NewControllerRef(&pod.ObjectMeta, corev1.SchemeGroupVersion.WithKind("Pod"))
	or.BlockOwnerDeletion = nil
	or.Controller = nil
	return []metav1.OwnerReference{*or}
}
