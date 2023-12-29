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
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/alauda/redis-operator/cmd/redis-tools/pkg/redis"
	"github.com/go-logr/logr"
	"github.com/urfave/cli/v2"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var (
	IsPersistentEnabled = (os.Getenv("PERSISTENT_ENABLED") == "true")
	IsNodePortEnaled    = (os.Getenv("NODEPORT_ENABLED") == "true")
	IsTLSEnabled        = (os.Getenv("TLS_ENABLED") == "true")
)

const (
	operatorPasswordMountPath = "/account/password"
	injectedPasswordPath      = "/tmp/newpass"
)

// Heal heal may fail when updated password for redis 4,5
func Heal(ctx context.Context, c *cli.Context, client *kubernetes.Clientset, logger logr.Logger) error {
	podName := c.String("pod-name")

	logger.Info(fmt.Sprintf("check cluster status before pod %s startup", podName))
	nodes, err := getClusterNodes(c, ctx, client, true, logger)
	if err != nil {
		logger.Error(err, "update nodes.conf with unknown error")
		return err
	}
	if nodes == nil {
		logger.Info("no nodes found")
		return nil
	}
	nodesInfo, _ := nodes.Marshal()
	logger.Info("get nodes", "nodes", string(nodesInfo))

	var (
		self     = nodes.Self()
		replicas = nodes.Replicas(self.ID)
	)
	if !self.IsJoined() || len(replicas) == 0 {
		return nil
	}

	authInfo, err := getRedisAuthInfo(c)
	if err != nil {
		logger.Error(err, "load redis operator user info failed")
		return err
	}

	masterExists := false
	// NOTE: when node is in importing state, if do force failover
	//       some slots will missing, or there will be multi master in cluster
	if slots := self.Slots(); !slots.IsImporting() {
		pods, _ := getPodsOfShard(ctx, c, client, logger)
		for _, pod := range pods {
			logger.Info(fmt.Sprintf("check pod %s", pod.GetName()))
			if pod.GetName() == podName {
				continue
			}
			if pod.GetDeletionTimestamp() != nil {
				continue
			}

			if err := func() error {
				addr := net.JoinHostPort(pod.Status.PodIP, "6379")
				announceIP := strings.ReplaceAll(pod.Labels["middleware.alauda.io/announce_ip"], "-", ":")
				announcePort := pod.Labels["middleware.alauda.io/announce_port"]
				if announceIP != "" && announcePort != "" {
					addr = net.JoinHostPort(announceIP, announcePort)
				}
				logger.Info("connect to redis", "addr", addr)
				redisClient := redis.NewClient(addr, *authInfo)
				defer redisClient.Close()

				nodes, err := redisClient.Nodes(ctx)
				if err != nil {
					logger.Error(err, "get nodes info failed")
					return err
				}
				currentNode := nodes.Self()
				if !currentNode.IsJoined() {
					logger.Info("unjoined node")
					return fmt.Errorf("unjoined node")
				}
				if currentNode.Role == redis.MasterRole {
					// this shard has got one new master
					// clean and start
					logger.Info("master nodes exists")
					masterExists = true
					return nil
				}

				currentMaster := nodes.Get(currentNode.MasterID)
				if currentMaster != nil && (currentMaster.ID != self.ID &&
					!strings.Contains(currentMaster.Flags, "fail") &&
					!strings.Contains(currentMaster.Flags, "noaddr")) {
					masterExists = true
					return nil
				}
				if err := doRedisFailover(ctx, redisClient, ForceFailoverAction, logger); err != nil {
					return err
				}
				return nil
			}(); err != nil {
				continue
			}
			break
		}
	}

	if masterExists {
		// check current rdb and aof
		var (
			rdbFile       = "/data/dump.rdb"
			aofFile       = "/data/appendonly.aof"
			oldestModTime = time.Now().Add(time.Second * -3600)
		)
		if info, _ := os.Stat(rdbFile); info != nil {
			if info.ModTime().Before(oldestModTime) {
				// clean this file
				logger.Info("clean old dump.rdb")
				os.Remove(rdbFile)
			}
		}
		logger.Info("clean appendonly.aof")
		os.Remove(aofFile)
	}
	return nil
}

type FailoverAction string

const (
	NoFailoverAction       FailoverAction = ""
	ForceFailoverAction                   = "FORCE"
	TakeoverFailoverAction                = "TAKEOVER"
)

func doRedisFailover(ctx context.Context, cli redis.Client, action FailoverAction, logger logr.Logger) (err error) {
	args := []interface{}{"FAILOVER"}
	if action != "" {
		args = append(args, action)
	}
	if _, err := cli.Do(ctx, "CLUSTER", args...); err != nil {
		logger.Error(err, "do failover failed", "action", action)
		return err
	}

	logger.Info("wait 5s for failover")
	time.Sleep(time.Second * 5)

	nodes, err := cli.Nodes(ctx)
	if err != nil {
		logger.Error(err, "fetch cluster nodes failed")
		return err
	}
	self := nodes.Self()
	if self == nil {
		return fmt.Errorf("get nodes info failed, as if the nodes.conf is broken")
	}
	if self.Role == redis.MasterRole {
		logger.Info("failover succeed")
		return nil
	}
	if action == ForceFailoverAction {
		return doRedisFailover(ctx, cli, TakeoverFailoverAction, logger)
	}
	return fmt.Errorf("do manual failover failed")
}

func getPodsOfShard(ctx context.Context, c *cli.Context, client *kubernetes.Clientset, logger logr.Logger) (pods []corev1.Pod, err error) {
	var (
		namespace = c.String("namespace")
		podName   = c.String("pod-name")
	)
	if podName == "" {
		podName, _ = os.Hostname()
	}
	if podName == "" {
		return nil, nil
	}
	splitIndex := strings.LastIndex(podName, "-")
	stsName := podName[0:splitIndex]

	labels := map[string]string{
		"middleware.instance/type": "distributed-redis-cluster",
		"statefulSet":              stsName,
	}

	if err = RetryGet(func() error {
		if resp, err := client.CoreV1().Pods(namespace).List(ctx, v1.ListOptions{
			LabelSelector: v1.FormatLabelSelector(&v1.LabelSelector{MatchLabels: labels}),
			Limit:         5,
		}); err != nil {
			return err
		} else {
			pods = resp.Items
		}
		return nil
	}, 3); err != nil {
		logger.Error(err, "list statefulset pods failed")
		return nil, err
	}
	return
}

func getRedisAuthInfo(c *cli.Context) (*redis.AuthInfo, error) {
	var (
		operatorName = c.String("operator-username")
		passwordPath = operatorPasswordMountPath
		isTLSEnabled = c.Bool("tls")
		tlsKeyFile   = c.String("tls-key-file")
		tlsCertFile  = c.String("tls-cert-file")
	)

	info := redis.AuthInfo{}
	info.Username = operatorName
	if operatorName == "" || operatorName == "default" {
		if _, err := os.Stat(injectedPasswordPath); err == nil {
			passwordPath = injectedPasswordPath
		}
	}
	if data, err := os.ReadFile(passwordPath); err != nil && !os.IsNotExist(err) {
		return nil, err
	} else {
		info.Password = strings.TrimSpace(string(data))
	}

	if !isTLSEnabled {
		return &info, nil
	}
	if tlsKeyFile == "" || tlsCertFile == "" {
		return nil, fmt.Errorf("require tls key and cert")
	}

	cert, err := tls.LoadX509KeyPair(tlsCertFile, tlsKeyFile)
	if err != nil {
		return nil, err
	}
	info.TLSConf = &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,
	}
	return &info, nil
}

func getClusterNodes(c *cli.Context, ctx context.Context, client *kubernetes.Clientset, overwrite bool, logger logr.Logger) (redis.Nodes, error) {
	var (
		namespace = c.String("namespace")
		podName   = c.String("pod-name")
		workspace = c.String("workspace")
		target    = c.String("node-config-name")
		prefix    = c.String("prefix")
	)
	if namespace == "" {
		return nil, fmt.Errorf("require namespace")
	}
	if podName == "" {
		return nil, fmt.Errorf("require podname")
	}
	if workspace == "" {
		workspace = "/data"
	}
	if target == "" {
		target = "nodes.conf"
	}

	nodeFile := path.Join(workspace, target)
	data, err := os.ReadFile(nodeFile)
	if err != nil {
		if os.IsNotExist(err) {
			// sync configmap to local
			configName := strings.Join([]string{strings.TrimSuffix(prefix, "-"), podName}, "-")
			if data, err = SyncToLocal(ctx, client, namespace, configName, workspace, target, logger); err != nil {
				logger.Error(err, "sync nodes.conf from configmap to local failed")
				return nil, err
			}
		} else {
			logger.Error(err, "get nodes.conf failed")
			return nil, err
		}
	}

	// new node startup
	if len(data) == 0 {
		return nil, nil
	}

	var (
		lines     []string
		epochLine string
	)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "vars") {
			if epochLine == "" {
				epochLine = line
			}
			continue
		}
		fields := strings.Fields(line)
		// NOTE: here ignore the wrong line, this may caused by redis-server crash
		if !strings.Contains(line, "connect") || len(fields) < 8 {
			continue
		}
		lines = append(lines, line)
	}
	if epochLine != "" {
		// format: vars currentEpoch 105 lastVoteEpoch 105
		fields := strings.Fields(epochLine)
		if len(fields) == 5 {
			lines = append(lines, epochLine)
		} else if len(fields) < 5 {
			if len(fields) == 4 && fields[3] == "lastVoteEpoch" {
				fields = append(fields, "0")
				lines = append(lines, strings.Join(fields, " "))
			} else if len(fields) == 3 && fields[1] == "currentEpoch" {
				fields = append(fields, "lastVoteEpoch", "0")
				lines = append(lines, strings.Join(fields, " "))
			}
		}
	}

	newData := strings.Join(lines, "\n")
	nodes := redis.ParseNodes(newData)
	if overwrite {
		nodeFileBak := nodeFile + ".bak"
		if nodes.Self() == nil {
			// nodes.conf error with self node id which is not irreparable, remove this node as new node
			if err := os.Rename(nodeFile, nodeFileBak); err != nil {
				logger.Error(err, "remove nodes.conf failed")
			}
			logger.Info("no self node record found, clean this node as new node")
			return nil, nil
		} else if newData != string(data) {
			// back the nodes.conf to nodes.conf.bak
			_ = os.Rename(nodeFile, nodeFileBak)

			tmpFile := path.Join(workspace, "tmp-"+target)
			if err := os.WriteFile(tmpFile, []byte(newData), 0644); err != nil {
				logger.Error(err, "update nodes.conf failed")
			} else if err := os.Rename(tmpFile, nodeFile); err != nil {
				logger.Error(err, "rename tmp-nodes.conf to nodes.conf failed")
			}
		}
	}
	return nodes, nil
}

func getBindableAddresses(logger logr.Logger) []string {
	var bindAddrs []string
	// generate ip list for startup
	if addrs, err := net.InterfaceAddrs(); err != nil {
		logger.Error(err, "load ips of container failed")

		if ips := os.Getenv("POD_IPS"); ips != "" {
			bindAddrs = strings.Split(ips, ",")
		} else if podIp := os.Getenv("POD_IP"); podIp != "" {
			bindAddrs = append(bindAddrs, podIp)
		}
	} else {
		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				if v.IP.IsLoopback() {
					continue
				}
				bindAddrs = append(bindAddrs, v.IP.String())
			default:
				logger.Info("WARNING: unsupported interface %s", addr.String())
			}
		}
	}

	foundIPv6 := false
	foundIPv4 := false
	for _, addr := range bindAddrs {
		if strings.Contains(addr, ":") {
			foundIPv6 = true
		} else {
			foundIPv4 = true
		}
	}

	// make sure 127.0.0.1 and ::1 at the end of ip list
	if foundIPv4 {
		bindAddrs = append(bindAddrs, "127.0.0.1")
	}
	if foundIPv6 {
		bindAddrs = append(bindAddrs, "::1")
	}
	return bindAddrs
}
