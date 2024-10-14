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
package v1

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	clusterv1 "github.com/alauda/redis-operator/api/cluster/v1alpha1"
	"github.com/alauda/redis-operator/api/core"
	redisfailoverv1 "github.com/alauda/redis-operator/api/databases/v1"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestRedis_Default(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "3.14 enable nodeport",
			data: `apiVersion: middleware.alauda.io/v1
kind: Redis
metadata:
  annotations:
    middleware.instance/autoUpgrade: "false"
  name: s6-notupgrade-nodeport
spec:
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/component
            operator: In
            values:
            - redis
          - key: redisfailovers.databases.spotahome.com/name
            operator: In
            values:
            - s6-notupgrade-nodeport
        topologyKey: kubernetes.io/hostname
  arch: sentinel
  backup: {}
  customConfig: {}
  exporter:
    enabled: true
  expose:
    enableNodePort: true
  passwordSecret: redis-password-np
  persistent:
    storageClassName: sc-topolvm
  persistentSize: 1Gi
  replicas:
    sentinel:
      master: 1
      slave: 1
  resources:
    limits:
      cpu: 300m
      memory: 300Mi
    requests:
      cpu: 300m
      memory: 300Mi
  sentinelCustomConfig: {}
  sentinel:
    affinity:
      podAntiAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchExpressions:
            - key: redisfailovers.databases.spotahome.com/name
              operator: In
              values:
              - s6-notupgrade-nodeport
            - key: app.kubernetes.io/component
              operator: In
              values:
              - redis
          topologyKey: kubernetes.io/hostname
  upgradeOption:
    autoUpgrade: false
    crVersion: 3.14.50
  version: "6.0"`,
			want: `apiVersion: middleware.alauda.io/v1
kind: Redis
metadata:
  annotations:
    middleware.instance/autoUpgrade: "false"
  name: s6-notupgrade-nodeport
spec:
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/component
            operator: In
            values:
            - redis
          - key: redisfailovers.databases.spotahome.com/name
            operator: In
            values:
            - s6-notupgrade-nodeport
        topologyKey: kubernetes.io/hostname
  arch: sentinel
  backup: {}
  customConfig:
    repl-backlog-size: "3145728"
  exporter:
    enabled: true
    resources:
      limits:
       cpu: 100m
       memory: 384Mi
      requests:
       cpu: 50m
       memory: 128Mi
  expose:
    enableNodePort: true
    type: NodePort
  passwordSecret: redis-password-np
  persistent:
    storageClassName: sc-topolvm
  persistentSize: 1Gi
  replicas:
    sentinel:
      master: 1
      slave: 1
  resources:
    limits:
      cpu: 300m
      memory: 300Mi
    requests:
      cpu: 300m
      memory: 300Mi
  sentinelCustomConfig: {}
  sentinel:
    replicas: 3
    resources:
      limits:
        cpu: 100m
        memory: 128Mi
      requests:
        cpu: 100m
        memory: 128Mi
    expose:
      type: NodePort
    affinity:
      podAntiAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchExpressions:
            - key: redisfailovers.databases.spotahome.com/name
              operator: In
              values:
              - s6-notupgrade-nodeport
            - key: app.kubernetes.io/component
              operator: In
              values:
              - redis
          topologyKey: kubernetes.io/hostname
  upgradeOption:
    autoUpgrade: false
    crVersion: 3.14.50
  version: "6.0"`,
		},
		{
			name: "3.14 enable nodeport with custom port",
			data: `apiVersion: middleware.alauda.io/v1
kind: Redis
metadata:
  annotations:
    middleware.instance/autoUpgrade: "false"
  name: s6-notupgrade-nodeport
spec:
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/component
            operator: In
            values:
            - redis
          - key: redisfailovers.databases.spotahome.com/name
            operator: In
            values:
            - s6-notupgrade-nodeport
        topologyKey: kubernetes.io/hostname
  arch: sentinel
  backup: {}
  customConfig:
    repl-backlog-size: "3145728"
  exporter:
    enabled: true
    resources:
      limits:
       cpu: 100m
       memory: 384Mi
      requests:
       cpu: 50m
       memory: 128Mi
  expose:
    enableNodePort: true
    accessPort: 31002
    dataStorageNodePortMap:
      "0": 31000
      "1": 31001
  passwordSecret: redis-password-np
  persistent:
    storageClassName: sc-topolvm
  persistentSize: 1Gi
  replicas:
    sentinel:
      master: 1
      slave: 1
  resources:
    limits:
      cpu: 300m
      memory: 300Mi
    requests:
      cpu: 300m
      memory: 300Mi
  sentinelCustomConfig: {}
  sentinel:
    affinity:
      podAntiAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchExpressions:
            - key: redisfailovers.databases.spotahome.com/name
              operator: In
              values:
              - s6-notupgrade-nodeport
            - key: app.kubernetes.io/component
              operator: In
              values:
              - redis
          topologyKey: kubernetes.io/hostname
  upgradeOption:
    autoUpgrade: false
    crVersion: 3.14.50
  version: "6.0"`,
			want: `apiVersion: middleware.alauda.io/v1
kind: Redis
metadata:
  annotations:
    middleware.instance/autoUpgrade: "false"
  name: s6-notupgrade-nodeport
spec:
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/component
            operator: In
            values:
            - redis
          - key: redisfailovers.databases.spotahome.com/name
            operator: In
            values:
            - s6-notupgrade-nodeport
        topologyKey: kubernetes.io/hostname
  arch: sentinel
  backup: {}
  customConfig:
    repl-backlog-size: "3145728"
  exporter:
    enabled: true
    resources:
      limits:
       cpu: 100m
       memory: 384Mi
      requests:
       cpu: 50m
       memory: 128Mi
  expose:
    enableNodePort: true
    type: NodePort
    accessPort: 31002
    dataStorageNodePortMap:
      "0": 31000
      "1": 31001
    dataStorageNodePortSequence: 31000,31001
  passwordSecret: redis-password-np
  persistent:
    storageClassName: sc-topolvm
  persistentSize: 1Gi
  replicas:
    sentinel:
      master: 1
      slave: 1
  resources:
    limits:
      cpu: 300m
      memory: 300Mi
    requests:
      cpu: 300m
      memory: 300Mi
  sentinelCustomConfig: {}
  sentinel:
    replicas: 3
    resources:
      limits:
        cpu: 100m
        memory: 128Mi
      requests:
        cpu: 100m
        memory: 128Mi
    expose:
      type: NodePort
      accessPort: 31002
    affinity:
      podAntiAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchExpressions:
            - key: redisfailovers.databases.spotahome.com/name
              operator: In
              values:
              - s6-notupgrade-nodeport
            - key: app.kubernetes.io/component
              operator: In
              values:
              - redis
          topologyKey: kubernetes.io/hostname
  upgradeOption:
    autoUpgrade: false
    crVersion: 3.14.50
  version: "6.0"`,
		},
		{
			name: "3.18 enable nodeport",
			data: `apiVersion: middleware.alauda.io/v1
kind: Redis
metadata:
  annotations:
    middleware.instance/autoUpgrade: "false"
  name: s6-notupgrade-nodeport
spec:
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/component
            operator: In
            values:
            - redis
          - key: redisfailovers.databases.spotahome.com/name
            operator: In
            values:
            - s6-notupgrade-nodeport
        topologyKey: kubernetes.io/hostname
  arch: sentinel
  backup: {}
  customConfig: {}
  exporter:
    enabled: true
  expose:
    type: NodePort
  passwordSecret: redis-password-np
  persistent:
    storageClassName: sc-topolvm
  persistentSize: 1Gi
  replicas:
    sentinel:
      master: 1
      slave: 1
  resources:
    limits:
      cpu: 300m
      memory: 300Mi
    requests:
      cpu: 300m
      memory: 300Mi
  sentinelCustomConfig: {}
  sentinel:
    affinity:
      podAntiAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchExpressions:
            - key: redisfailovers.databases.spotahome.com/name
              operator: In
              values:
              - s6-notupgrade-nodeport
            - key: app.kubernetes.io/component
              operator: In
              values:
              - redis
          topologyKey: kubernetes.io/hostname
  upgradeOption:
    autoUpgrade: false
  version: "6.0"`,
			want: `apiVersion: middleware.alauda.io/v1
kind: Redis
metadata:
  annotations:
    middleware.instance/autoUpgrade: "false"
  name: s6-notupgrade-nodeport
spec:
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/component
            operator: In
            values:
            - redis
          - key: redisfailovers.databases.spotahome.com/name
            operator: In
            values:
            - s6-notupgrade-nodeport
        topologyKey: kubernetes.io/hostname
  arch: sentinel
  backup: {}
  customConfig:
    repl-backlog-size: "3145728"
  exporter:
    enabled: true
    resources:
      limits:
       cpu: 100m
       memory: 384Mi
      requests:
       cpu: 50m
       memory: 128Mi
  expose:
    type: NodePort
  passwordSecret: redis-password-np
  persistent:
    storageClassName: sc-topolvm
  persistentSize: 1Gi
  replicas:
    sentinel:
      master: 1
      slave: 1
  resources:
    limits:
      cpu: 300m
      memory: 300Mi
    requests:
      cpu: 300m
      memory: 300Mi
  sentinelCustomConfig: {}
  sentinel:
    replicas: 3
    resources:
      limits:
        cpu: 100m
        memory: 128Mi
      requests:
        cpu: 100m
        memory: 128Mi
    expose:
      type: NodePort
    affinity:
      podAntiAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchExpressions:
            - key: redissentinels.databases.spotahome.com/name
              operator: In
              values:
              - s6-notupgrade-nodeport
            - key: app.kubernetes.io/component
              operator: In
              values:
              - sentinel
          topologyKey: kubernetes.io/hostname
  upgradeOption:
    autoUpgrade: false
  version: "6.0"`,
		},
		{
			name: "3.18 enable nodeport with custom port",
			data: `apiVersion: middleware.alauda.io/v1
kind: Redis
metadata:
  annotations:
    middleware.instance/autoUpgrade: "false"
  name: s6-notupgrade-nodeport
spec:
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/component
            operator: In
            values:
            - redis
          - key: redisfailovers.databases.spotahome.com/name
            operator: In
            values:
            - s6-notupgrade-nodeport
        topologyKey: kubernetes.io/hostname
  arch: sentinel
  backup: {}
  customConfig:
    repl-backlog-size: "3145728"
  exporter:
    enabled: true
    resources:
      limits:
       cpu: 100m
       memory: 384Mi
      requests:
       cpu: 50m
       memory: 128Mi
  expose:
    type: NodePort
    accessPort: 31002
    dataStorageNodePortMap:
      "0": 31000
      "1": 31001
  passwordSecret: redis-password-np
  persistent:
    storageClassName: sc-topolvm
  persistentSize: 1Gi
  replicas:
    sentinel:
      master: 1
      slave: 1
  resources:
    limits:
      cpu: 300m
      memory: 300Mi
    requests:
      cpu: 300m
      memory: 300Mi
  sentinelCustomConfig: {}
  sentinel:
    affinity:
      podAntiAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchExpressions:
            - key: redisfailovers.databases.spotahome.com/name
              operator: In
              values:
              - s6-notupgrade-nodeport
            - key: app.kubernetes.io/component
              operator: In
              values:
              - redis
          topologyKey: kubernetes.io/hostname
  upgradeOption:
    autoUpgrade: false
  version: "6.0"`,
			want: `apiVersion: middleware.alauda.io/v1
kind: Redis
metadata:
  annotations:
    middleware.instance/autoUpgrade: "false"
  name: s6-notupgrade-nodeport
spec:
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/component
            operator: In
            values:
            - redis
          - key: redisfailovers.databases.spotahome.com/name
            operator: In
            values:
            - s6-notupgrade-nodeport
        topologyKey: kubernetes.io/hostname
  arch: sentinel
  backup: {}
  customConfig:
    repl-backlog-size: "3145728"
  exporter:
    enabled: true
    resources:
      limits:
       cpu: 100m
       memory: 384Mi
      requests:
       cpu: 50m
       memory: 128Mi
  expose:
    type: NodePort
    accessPort: 31002
    dataStorageNodePortMap:
      "0": 31000
      "1": 31001
    dataStorageNodePortSequence: 31000,31001
  passwordSecret: redis-password-np
  persistent:
    storageClassName: sc-topolvm
  persistentSize: 1Gi
  replicas:
    sentinel:
      master: 1
      slave: 1
  resources:
    limits:
      cpu: 300m
      memory: 300Mi
    requests:
      cpu: 300m
      memory: 300Mi
  sentinelCustomConfig: {}
  sentinel:
    replicas: 3
    resources:
      limits:
        cpu: 100m
        memory: 128Mi
      requests:
        cpu: 100m
        memory: 128Mi
    expose:
      type: NodePort
      accessPort: 31002
    affinity:
      podAntiAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchExpressions:
            - key: redissentinels.databases.spotahome.com/name
              operator: In
              values:
              - s6-notupgrade-nodeport
            - key: app.kubernetes.io/component
              operator: In
              values:
              - sentinel
          topologyKey: kubernetes.io/hostname
  upgradeOption:
    autoUpgrade: false
  version: "6.0"`,
		},
		{
			name: "failover setup default affinity",
			data: `apiVersion: middleware.alauda.io/v1
kind: Redis
metadata:
  annotations:
    middleware.instance/autoUpgrade: "false"
  name: s6-notupgrade-nodeport
spec:
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/component
            operator: In
            values:
            - redis
          - key: redisfailovers.databases.spotahome.com/name
            operator: In
            values:
            - s6-notupgrade-nodeport
        topologyKey: kubernetes.io/hostname
  arch: sentinel
  backup: {}
  customConfig:
    repl-backlog-size: "3145728"
  exporter:
    enabled: true
    resources:
      limits:
       cpu: 100m
       memory: 384Mi
      requests:
       cpu: 50m
       memory: 128Mi
  expose:
    type: NodePort
    accessPort: 31002
    dataStorageNodePortMap:
      "0": 31000
      "1": 31001
  passwordSecret: redis-password-np
  persistent:
    storageClassName: sc-topolvm
  persistentSize: 1Gi
  replicas:
    sentinel:
      master: 1
      slave: 1
  resources:
    limits:
      cpu: 300m
      memory: 300Mi
    requests:
      cpu: 300m
      memory: 300Mi
  sentinelCustomConfig: {}
  sentinel:
    affinity:
      podAntiAffinity:
        requiredDuringSchedulingIgnoredDuringExecution: []
  upgradeOption:
    autoUpgrade: false
  version: "6.0"`,
			want: `apiVersion: middleware.alauda.io/v1
kind: Redis
metadata:
  annotations:
    middleware.instance/autoUpgrade: "false"
  name: s6-notupgrade-nodeport
spec:
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/component
            operator: In
            values:
            - redis
          - key: redisfailovers.databases.spotahome.com/name
            operator: In
            values:
            - s6-notupgrade-nodeport
        topologyKey: kubernetes.io/hostname
  arch: sentinel
  backup: {}
  customConfig:
    repl-backlog-size: "3145728"
  exporter:
    enabled: true
    resources:
      limits:
       cpu: 100m
       memory: 384Mi
      requests:
       cpu: 50m
       memory: 128Mi
  expose:
    type: NodePort
    accessPort: 31002
    dataStorageNodePortMap:
      "0": 31000
      "1": 31001
    dataStorageNodePortSequence: 31000,31001
  passwordSecret: redis-password-np
  persistent:
    storageClassName: sc-topolvm
  persistentSize: 1Gi
  replicas:
    sentinel:
      master: 1
      slave: 1
  resources:
    limits:
      cpu: 300m
      memory: 300Mi
    requests:
      cpu: 300m
      memory: 300Mi
  sentinelCustomConfig: {}
  sentinel:
    replicas: 3
    resources:
      limits:
        cpu: 100m
        memory: 128Mi
      requests:
        cpu: 100m
        memory: 128Mi
    expose:
      type: NodePort
      accessPort: 31002
    affinity:
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
        - podAffinityTerm:
            labelSelector:
              matchExpressions:
              - key: redissentinels.databases.spotahome.com/name
                operator: In
                values:
                - s6-notupgrade-nodeport
              - key: app.kubernetes.io/component
                operator: In
                values:
                - sentinel
            topologyKey: kubernetes.io/hostname
          weight: 100
  upgradeOption:
    autoUpgrade: false
  version: "6.0"`,
		},
		{
			name: "standalone setup default",
			data: `apiVersion: middleware.alauda.io/v1
kind: Redis
metadata:
  annotations:
    middleware.instance/autoUpgrade: "false"
  name: s6-notupgrade-nodeport
spec:
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/component
            operator: In
            values:
            - redis
          - key: redisfailovers.databases.spotahome.com/name
            operator: In
            values:
            - s6-notupgrade-nodeport
        topologyKey: kubernetes.io/hostname
  arch: standalone
  backup: {}
  exporter:
    enabled: true
    resources:
      limits:
       cpu: 100m
       memory: 384Mi
      requests:
       cpu: 50m
       memory: 128Mi
  expose:
    type: NodePort
    dataStorageNodePortMap:
      "0": 31000
  passwordSecret: redis-password-np
  persistent:
    storageClassName: sc-topolvm
  persistentSize: 1Gi
  resources:
    limits:
      cpu: 300m
      memory: 300Mi
    requests:
      cpu: 300m
      memory: 300Mi
  upgradeOption:
    autoUpgrade: false
  version: "6.0"`,
			want: `apiVersion: middleware.alauda.io/v1
kind: Redis
metadata:
  annotations:
    middleware.instance/autoUpgrade: "false"
  name: s6-notupgrade-nodeport
spec:
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/component
            operator: In
            values:
            - redis
          - key: redisfailovers.databases.spotahome.com/name
            operator: In
            values:
            - s6-notupgrade-nodeport
        topologyKey: kubernetes.io/hostname
  arch: standalone
  backup: {}
  customConfig:
    repl-backlog-size: "3145728"
  exporter:
    enabled: true
    resources:
      limits:
       cpu: 100m
       memory: 384Mi
      requests:
       cpu: 50m
       memory: 128Mi
  expose:
    type: NodePort
    dataStorageNodePortMap:
      "0": 31000
    dataStorageNodePortSequence: "31000"
  passwordSecret: redis-password-np
  persistent:
    storageClassName: sc-topolvm
  persistentSize: 1Gi
  replicas:
    sentinel:
      master: 1
  resources:
    limits:
      cpu: 300m
      memory: 300Mi
    requests:
      cpu: 300m
      memory: 300Mi
  upgradeOption:
    autoUpgrade: false
  version: "6.0"`,
		},
		{
			name: "default rename config",
			data: `apiVersion: middleware.alauda.io/v1
kind: Redis
metadata:
  annotations:
    middleware.instance/autoUpgrade: "false"
  name: s5
spec:
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/component
            operator: In
            values:
            - redis
          - key: redisfailovers.databases.spotahome.com/name
            operator: In
            values:
            - s6-notupgrade-nodeport
        topologyKey: kubernetes.io/hostname
  arch: standalone
  backup: {}
  exporter:
    enabled: true
    resources:
      limits:
       cpu: 100m
       memory: 384Mi
      requests:
       cpu: 50m
       memory: 128Mi
  expose:
    type: NodePort
    dataStorageNodePortMap:
      "0": 31000
  passwordSecret: redis-password-np
  persistent:
    storageClassName: sc-topolvm
  persistentSize: 1Gi
  resources:
    limits:
      cpu: 300m
      memory: 300Mi
    requests:
      cpu: 300m
      memory: 300Mi
  upgradeOption:
    autoUpgrade: false
  version: "5.0"`,
			want: `apiVersion: middleware.alauda.io/v1
kind: Redis
metadata:
  annotations:
    middleware.instance/autoUpgrade: "false"
  name: s5
spec:
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/component
            operator: In
            values:
            - redis
          - key: redisfailovers.databases.spotahome.com/name
            operator: In
            values:
            - s6-notupgrade-nodeport
        topologyKey: kubernetes.io/hostname
  arch: standalone
  backup: {}
  customConfig:
    repl-backlog-size: "3145728"
    rename-command: flushall "" flushdb ""
  exporter:
    enabled: true
    resources:
      limits:
       cpu: 100m
       memory: 384Mi
      requests:
       cpu: 50m
       memory: 128Mi
  expose:
    type: NodePort
    dataStorageNodePortMap:
      "0": 31000
    dataStorageNodePortSequence: "31000"
  passwordSecret: redis-password-np
  persistent:
    storageClassName: sc-topolvm
  persistentSize: 1Gi
  replicas:
    sentinel:
      master: 1
  resources:
    limits:
      cpu: 300m
      memory: 300Mi
    requests:
      cpu: 300m
      memory: 300Mi
  upgradeOption:
    autoUpgrade: false
  version: "5.0"`,
		},
		{
			name: "pause",
			data: `apiVersion: middleware.alauda.io/v1
kind: Redis
metadata:
  annotations:
    middleware.instance/autoUpgrade: "false"
  name: s6
spec:
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/component
            operator: In
            values:
            - redis
          - key: redisfailovers.databases.spotahome.com/name
            operator: In
            values:
            - s6
        topologyKey: kubernetes.io/hostname
  arch: sentinel
  backup: {}
  expose:
    type: NodePort
  passwordSecret: redis-password-np
  persistent:
    storageClassName: sc-topolvm
  persistentSize: 1Gi
  resources:
    limits:
      cpu: 300m
      memory: 300Mi
    requests:
      cpu: 300m
      memory: 300Mi
  pause: true
  upgradeOption:
    autoUpgrade: false
  version: "6.0"`,
			want: `apiVersion: middleware.alauda.io/v1
kind: Redis
metadata:
  annotations:
    middleware.instance/autoUpgrade: "false"
  name: s6
spec:
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/component
            operator: In
            values:
            - redis
          - key: redisfailovers.databases.spotahome.com/name
            operator: In
            values:
            - s6
        topologyKey: kubernetes.io/hostname
  arch: sentinel
  backup: {}
  customConfig:
    repl-backlog-size: "3145728"
  exporter:
    enabled: true
    resources:
      limits:
       cpu: 100m
       memory: 384Mi
      requests:
       cpu: 50m
       memory: 128Mi
  expose:
    type: NodePort
  passwordSecret: redis-password-np
  podAnnotations:
    app.cpaas.io/pause-timestamp: "2024-08-15T16:24:01+08:00"
  persistent:
    storageClassName: sc-topolvm
  persistentSize: 1Gi
  replicas:
    sentinel:
      master: 1
  resources:
    limits:
      cpu: 300m
      memory: 300Mi
    requests:
      cpu: 300m
      memory: 300Mi
  pause: true
  sentinel:
    replicas: 3
    resources:
      limits:
        cpu: 100m
        memory: 128Mi
      requests:
        cpu: 100m
        memory: 128Mi
    expose:
      type: NodePort
    affinity:
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
        - podAffinityTerm:
            labelSelector:
              matchExpressions:
              - key: redissentinels.databases.spotahome.com/name
                operator: In
                values:
                - s6
              - key: app.kubernetes.io/component
                operator: In
                values:
                - sentinel
            topologyKey: kubernetes.io/hostname
          weight: 100
  upgradeOption:
    autoUpgrade: false
  version: "6.0"`,
		},
		{
			name: "update options",
			data: `apiVersion: middleware.alauda.io/v1
kind: Redis
metadata:
  annotations:
    middleware.instance/autoUpgrade: "false"
    middleware.upgrade.crVersion: "3.18.0"
    middleware.upgrade.component.version: "6.0.20"
  name: s6
spec:
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/component
            operator: In
            values:
            - redis
          - key: redisfailovers.databases.spotahome.com/name
            operator: In
            values:
            - s6
        topologyKey: kubernetes.io/hostname
  arch: sentinel
  backup: {}
  expose:
    type: NodePort
  passwordSecret: redis-password-np
  persistent:
    storageClassName: sc-topolvm
  persistentSize: 1Gi
  resources:
    limits:
      cpu: 300m
      memory: 300Mi
    requests:
      cpu: 300m
      memory: 300Mi
  pause: true
  version: "6.0"`,
			want: `apiVersion: middleware.alauda.io/v1
kind: Redis
metadata:
  annotations:
    middleware.instance/autoUpgrade: "false"
  name: s6
spec:
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/component
            operator: In
            values:
            - redis
          - key: redisfailovers.databases.spotahome.com/name
            operator: In
            values:
            - s6
        topologyKey: kubernetes.io/hostname
  arch: sentinel
  backup: {}
  customConfig:
    repl-backlog-size: "3145728"
  exporter:
    enabled: true
    resources:
      limits:
       cpu: 100m
       memory: 384Mi
      requests:
       cpu: 50m
       memory: 128Mi
  expose:
    type: NodePort
  passwordSecret: redis-password-np
  podAnnotations:
    app.cpaas.io/pause-timestamp: "2024-08-15T16:24:01+08:00"
  persistent:
    storageClassName: sc-topolvm
  persistentSize: 1Gi
  replicas:
    sentinel:
      master: 1
  resources:
    limits:
      cpu: 300m
      memory: 300Mi
    requests:
      cpu: 300m
      memory: 300Mi
  pause: true
  sentinel:
    replicas: 3
    resources:
      limits:
        cpu: 100m
        memory: 128Mi
      requests:
        cpu: 100m
        memory: 128Mi
    expose:
      type: NodePort
    affinity:
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
        - podAffinityTerm:
            labelSelector:
              matchExpressions:
              - key: redissentinels.databases.spotahome.com/name
                operator: In
                values:
                - s6
              - key: app.kubernetes.io/component
                operator: In
                values:
                - sentinel
            topologyKey: kubernetes.io/hostname
          weight: 100
  upgradeOption: {}
  version: "6.0"`,
		},
		{
			name: "failover default persistent size",
			data: `apiVersion: middleware.alauda.io/v1
kind: Redis
metadata:
  annotations:
    middleware.upgrade.crVersion: "3.18.0"
    middleware.upgrade.component.version: "6.0.20"
  name: s6
spec:
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/component
            operator: In
            values:
            - redis
          - key: redisfailovers.databases.spotahome.com/name
            operator: In
            values:
            - s6
        topologyKey: kubernetes.io/hostname
  arch: sentinel
  backup: {}
  expose:
    type: NodePort
  passwordSecret: redis-password-np
  persistent:
    storageClassName: sc-topolvm
  resources:
    limits:
      cpu: 300m
      memory: 300Mi
    requests:
      cpu: 300m
      memory: 300Mi
  pause: true
  version: "6.0"`,
			want: `apiVersion: middleware.alauda.io/v1
kind: Redis
metadata:
  annotations: {}
  name: s6
spec:
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/component
            operator: In
            values:
            - redis
          - key: redisfailovers.databases.spotahome.com/name
            operator: In
            values:
            - s6
        topologyKey: kubernetes.io/hostname
  arch: sentinel
  backup: {}
  customConfig:
    repl-backlog-size: "3145728"
  exporter:
    enabled: true
    resources:
      limits:
       cpu: 100m
       memory: 384Mi
      requests:
       cpu: 50m
       memory: 128Mi
  expose:
    type: NodePort
  passwordSecret: redis-password-np
  podAnnotations:
    app.cpaas.io/pause-timestamp: "2024-08-15T16:24:01+08:00"
  persistent:
    storageClassName: sc-topolvm
  persistentSize: 600Mi
  replicas:
    sentinel:
      master: 1
  resources:
    limits:
      cpu: 300m
      memory: 300Mi
    requests:
      cpu: 300m
      memory: 300Mi
  pause: true
  sentinel:
    replicas: 3
    resources:
      limits:
        cpu: 100m
        memory: 128Mi
      requests:
        cpu: 100m
        memory: 128Mi
    expose:
      type: NodePort
    affinity:
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
        - podAffinityTerm:
            labelSelector:
              matchExpressions:
              - key: redissentinels.databases.spotahome.com/name
                operator: In
                values:
                - s6
              - key: app.kubernetes.io/component
                operator: In
                values:
                - sentinel
            topologyKey: kubernetes.io/hostname
          weight: 100
  upgradeOption: {}
  version: "6.0"`,
		},
		{
			name: "cluster default persistent size",
			data: `apiVersion: middleware.alauda.io/v1
kind: Redis
metadata:
  annotations:
    middleware.upgrade.crVersion: "3.18.0"
    middleware.upgrade.component.version: "6.0.20"
  name: c6
spec:
  arch: cluster
  backup: {}
  replicas:
    cluster:
      shard: 3
      slave: 1
  expose:
    type: NodePort
  passwordSecret: redis-password-np
  persistent:
    storageClassName: sc-topolvm
  resources:
    limits:
      cpu: 300m
      memory: 300Mi
    requests:
      cpu: 300m
      memory: 300Mi
  pause: true
  version: "6.0"`,
			want: `apiVersion: middleware.alauda.io/v1
kind: Redis
metadata:
  annotations:
    middleware.alauda.io/storage_size: "{\"0\":\"600Mi\",\"1\":\"600Mi\",\"2\":\"600Mi\"}"
  name: c6
spec:
  arch: cluster
  backup: {}
  customConfig:
    repl-backlog-size: "3145728"
  exporter:
    enabled: true
    resources:
      limits:
       cpu: 100m
       memory: 384Mi
      requests:
       cpu: 50m
       memory: 128Mi
  expose:
    type: NodePort
  passwordSecret: redis-password-np
  podAnnotations:
    app.cpaas.io/pause-timestamp: "2024-08-15T16:24:01+08:00"
  persistent:
    storageClassName: sc-topolvm
  persistentSize: 600Mi
  replicas:
    cluster:
      shard: 3
      slave: 1
  resources:
    limits:
      cpu: 300m
      memory: 300Mi
    requests:
      cpu: 300m
      memory: 300Mi
  pause: true
  upgradeOption: {}
  version: "6.0"`,
		},
	}

	var fieldDiffCheck func(t *testing.T, r, w any) (bool, any, any, []string)
	fieldDiffCheck = func(t *testing.T, r, w any) (bool, any, any, []string) {
		if r == nil && w != nil || r != nil && w == nil {
			return false, r, w, nil
		}

		var fields []string
		if reflect.DeepEqual(r, w) {
			return true, nil, nil, nil
		}
		switch rVal := r.(type) {
		case map[string]any:
			wVal := w.(map[string]any)
			for key, val := range rVal {
				fields = append(fields, key)
				if equal, rDiff, wDiff, subfs := fieldDiffCheck(t, val, wVal[key]); !equal {
					fields = append(fields, subfs...)
					return false, rDiff, wDiff, fields
				}
			}
		}
		if reflect.DeepEqual(r, w) {
			return true, nil, nil, nil
		}
		return false, r, w, nil
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				r Redis
				w Redis
			)
			if err := yaml.Unmarshal([]byte(tt.data), &r); err != nil {
				t.Errorf("yaml.Unmarshal() raw error = %v", err)
			}
			if err := yaml.Unmarshal([]byte(tt.want), &w); err != nil {
				t.Errorf("yaml.Unmarshal() wanted error = %v", err)
			}
			r.Default()

			if !reflect.DeepEqual(r.ObjectMeta, w.ObjectMeta) {
				rData, _ := json.Marshal(r.ObjectMeta)
				wData, _ := json.Marshal(w.ObjectMeta)
				t.Errorf("Redis.Default().metadata= %s, <<<<<<<>>>>>>> want %s", rData, wData)
			}

			rPauseAnnVal, rPauseOk := r.Spec.PodAnnotations[PauseAnnotationKey]
			wPauseAnnVal, wPauseOk := w.Spec.PodAnnotations[PauseAnnotationKey]
			if rPauseOk != wPauseOk {
				t.Errorf("Redis.Default().spec.podAnnotations.%s = %v, <<<<<<<>>>>>>> want %v", PauseAnnotationKey, rPauseAnnVal, wPauseAnnVal)
			}
			delete(r.Spec.PodAnnotations, PauseAnnotationKey)
			delete(w.Spec.PodAnnotations, PauseAnnotationKey)

			rJsonData, _ := json.Marshal(r.Spec)
			wJsonData, _ := json.Marshal(w.Spec)
			var (
				rData = map[string]any{}
				wData = map[string]any{}
			)
			if err := json.Unmarshal(rJsonData, &rData); err != nil {
				t.Errorf("json.Unmarshal() raw error = %v", err)
			}
			if err := json.Unmarshal(wJsonData, &wData); err != nil {
				t.Errorf("json.Unmarshal() wanted error = %v", err)
			}

			keys := lo.Keys(rData)
			for key := range wData {
				if !slices.Contains(keys, key) {
					keys = append(keys, key)
				}
			}
			for _, key := range keys {
				if !reflect.DeepEqual(rData[key], wData[key]) {
					if equal, rDiff, wDiff, fields := fieldDiffCheck(t, rData[key], wData[key]); !equal {
						t.Errorf("Redis.Default().spec.%s.%v = %v, <<<<<<<>>>>>>> want %v", key, strings.Join(fields, "."), rDiff, wDiff)
					} else {
						t.Errorf("Redis.Default().spec.%v = %v, <<<<<<<>>>>>>> want %v", key, rData[key], wData[key])
					}
				}
			}
		})
	}
}

func TestRedis_ValidateCreateCluster(t *testing.T) {
	tests := []struct {
		name      string
		redis     *Redis
		wantErr   error
		wantWarns admission.Warnings
	}{
		{
			name: "cluster nil replcas",
			redis: &Redis{
				Spec: RedisSpec{
					Arch:     core.RedisCluster,
					Replicas: nil,
				},
			},
			wantErr:   fmt.Errorf("instance replicas not specified"),
			wantWarns: nil,
		},
		{
			name: "cluster enabled clusterip",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeClusterIP,
						},
					},
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
		{
			name: "cluster enabled nodeport",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
		{
			name: "cluster enabled lb",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeLoadBalancer,
						},
					},
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
		{
			name: "cluster matched nodeport count",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType:      corev1.ServiceTypeNodePort,
							NodePortSequence: "30000,30001,30002-30004,30005",
						},
					},
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
		{
			name: "cluster not matched nodeport count",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType:      corev1.ServiceTypeNodePort,
							NodePortSequence: "30000,30002-30004,30005",
						},
					},
				},
			},
			wantErr:   fmt.Errorf("expected 6 nodes, but got 5 ports in node port sequence"),
			wantWarns: nil,
		},
		{
			name: "cluster with matched specified shards",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
							Shards: []clusterv1.ClusterShardConfig{
								{Slots: "0-5461"},
								{Slots: "5462-10922"},
								{Slots: "10923-16383"},
							},
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
		{
			name: "cluster with not matched specified shards",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
							Shards: []clusterv1.ClusterShardConfig{
								{Slots: "0-10000"},
							},
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
				},
			},
			wantErr:   fmt.Errorf("specified shard slots list length not not match shards count"),
			wantWarns: nil,
		},
		{
			name: "cluster specified shards with invalid slots",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
							Shards: []clusterv1.ClusterShardConfig{
								{Slots: "0-5461"},
								{Slots: "5462-10922"},
								{Slots: "10923-16433"},
							},
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
				},
			},
			wantErr:   fmt.Errorf("failed to load shard slots: invalid slot 16384"),
			wantWarns: nil,
		},
		{
			name: "cluster specified shards with invalid slots2",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
							Shards: []clusterv1.ClusterShardConfig{
								{Slots: "aaaaa"},
								{Slots: "5462-10922"},
								{Slots: "10923-16433"},
							},
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
				},
			},
			wantErr:   fmt.Errorf("failed to load shard slots: invalid range slot aaaaa"),
			wantWarns: nil,
		},
		{
			name: "cluster shards specified slots not fullfilled",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
							Shards: []clusterv1.ClusterShardConfig{
								{Slots: "0-5461"},
								{Slots: "5462-10921"},
								{Slots: "10923-16383"},
							},
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
				},
			},
			wantErr:   fmt.Errorf("specified shard slots not fullfilled"),
			wantWarns: nil,
		},
		{
			name: "cluster shards specified slots duplicated",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
							Shards: []clusterv1.ClusterShardConfig{
								{Slots: "0-5462"},
								{Slots: "5462-10923"},
								{Slots: "10923-16383"},
							},
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
				},
			},
			wantErr:   fmt.Errorf("specified shard slots duplicated"),
			wantWarns: nil,
		},
		{
			name: "cluster enabled activeredis without serviceID",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
							Shards: []clusterv1.ClusterShardConfig{
								{Slots: "0-5461"},
								{Slots: "5462-10922"},
								{Slots: "10923-16383"},
							},
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
					EnableActiveRedis: true,
				},
			},
			wantErr:   fmt.Errorf("activeredis is enabled but serviceID is not valid"),
			wantWarns: nil,
		},
		{
			name: "cluster enabled activeredis",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
							Shards: []clusterv1.ClusterShardConfig{
								{Slots: "0-5461"},
								{Slots: "5462-10922"},
								{Slots: "10923-16383"},
							},
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
					EnableActiveRedis: true,
					ServiceID:         pointer.Int32(1),
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
		{
			name: "cluster enabled tls for v5",
			redis: &Redis{
				Spec: RedisSpec{
					Arch:    core.RedisCluster,
					Version: "5.0",
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
						},
					},
					EnableTLS: true,
				},
			},
			wantErr:   fmt.Errorf("tls not supported in version 5.0"),
			wantWarns: nil,
		},
		{
			name: "cluster enabled tls",
			redis: &Redis{
				Spec: RedisSpec{
					Arch:    core.RedisCluster,
					Version: "6.0",
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
						},
					},
					EnableTLS: true,
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warns, err := tt.redis.ValidateCreate()
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("ValidateCreate() error = %v, wantErr %v", err, tt.wantErr)
				} else if err.Error() != tt.wantErr.Error() {
					t.Errorf("ValidateCreate() error = %v, wantErr %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("ValidateCreate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(warns, tt.wantWarns) {
				t.Errorf("ValidateCreate() warns = %v, wantWarns %v", warns, tt.wantWarns)
			}
		})
	}
}

func TestRedis_ValidateCreateFailover(t *testing.T) {
	tests := []struct {
		name      string
		redis     *Redis
		wantErr   error
		wantWarns admission.Warnings
	}{
		{
			name: "failover nil replcas",
			redis: &Redis{
				Spec: RedisSpec{
					Arch:     core.RedisSentinel,
					Replicas: nil,
				},
			},
			wantErr:   fmt.Errorf("instance replicas not specified"),
			wantWarns: nil,
		},
		{
			name: "failover not enable nodeport",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{},
					Sentinel: &redisfailoverv1.SentinelSettings{
						RedisSentinelSpec: redisfailoverv1.RedisSentinelSpec{
							Replicas: 1,
						},
					},
				},
			},
			wantErr:   fmt.Errorf("sentinel replicas must be odd and greater >= 3"),
			wantWarns: nil,
		},
		{
			name: "failover not enable nodeport, with invalid sentinel replicas",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{},
					Sentinel: &redisfailoverv1.SentinelSettings{
						RedisSentinelSpec: redisfailoverv1.RedisSentinelSpec{
							Replicas: 4,
						},
					},
				},
			},
			wantErr:   fmt.Errorf("sentinel replicas must be odd and greater >= 3"),
			wantWarns: nil,
		},
		{
			name: "failover enabled nodeport",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
					Sentinel: &redisfailoverv1.SentinelSettings{},
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
		{
			name: "failover matched nodeport count",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(2),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType:      corev1.ServiceTypeNodePort,
							NodePortSequence: "30000,30001-30002",
						},
					},
					Sentinel: &redisfailoverv1.SentinelSettings{
						RedisSentinelSpec: redisfailoverv1.RedisSentinelSpec{
							Replicas: 3,
							Expose: core.InstanceAccess{
								InstanceAccessBase: core.InstanceAccessBase{
									ServiceType:      corev1.ServiceTypeNodePort,
									NodePortSequence: "31000,31001-31002",
								},
							},
						},
					},
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
		{
			name: "failover nodeport not match--data node",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(2),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType:      corev1.ServiceTypeNodePort,
							NodePortSequence: "30000,30001",
						},
					},
					Sentinel: &redisfailoverv1.SentinelSettings{
						RedisSentinelSpec: redisfailoverv1.RedisSentinelSpec{
							Replicas: 3,
							Expose: core.InstanceAccess{
								InstanceAccessBase: core.InstanceAccessBase{
									ServiceType:      corev1.ServiceTypeNodePort,
									NodePortSequence: "31000,31001-31002",
								},
							},
						},
					},
				},
			},
			wantErr:   fmt.Errorf("expected 3 nodes, but got 2 ports in node port sequence"),
			wantWarns: nil,
		},
		{
			name: "failover nodeport not match--sentinel node",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(2),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType:      corev1.ServiceTypeNodePort,
							NodePortSequence: "30000,30001-30002",
						},
					},
					Sentinel: &redisfailoverv1.SentinelSettings{
						RedisSentinelSpec: redisfailoverv1.RedisSentinelSpec{
							Replicas: 3,
							Expose: core.InstanceAccess{
								InstanceAccessBase: core.InstanceAccessBase{
									ServiceType:      corev1.ServiceTypeNodePort,
									NodePortSequence: "31001-31002",
								},
							},
						},
					},
				},
			},
			wantErr:   fmt.Errorf("expected 3 nodes, but got 2 ports in node port sequence"),
			wantWarns: nil,
		},
		{
			name: "failover duplicate nodeport",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(2),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType:      corev1.ServiceTypeNodePort,
							NodePortSequence: "30000,30001-30002",
						},
					},
					Sentinel: &redisfailoverv1.SentinelSettings{
						RedisSentinelSpec: redisfailoverv1.RedisSentinelSpec{
							Replicas: 3,
							Expose: core.InstanceAccess{
								InstanceAccessBase: core.InstanceAccessBase{
									ServiceType:      corev1.ServiceTypeNodePort,
									NodePortSequence: "30000,31001-31002",
								},
							},
						},
					},
				},
			},
			wantErr:   fmt.Errorf("port 30000 has assigned to spec.expose.dataStorageNodePortSequence"),
			wantWarns: nil,
		},
		{
			name: "failover duplicate nodeport with accessPort",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(2),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType:      corev1.ServiceTypeNodePort,
							NodePortSequence: "30000,30001-30002",
						},
					},
					Sentinel: &redisfailoverv1.SentinelSettings{
						RedisSentinelSpec: redisfailoverv1.RedisSentinelSpec{
							Replicas: 3,
							Expose: core.InstanceAccess{
								InstanceAccessBase: core.InstanceAccessBase{
									ServiceType:      corev1.ServiceTypeNodePort,
									AccessPort:       31001,
									NodePortSequence: "31000,31001-31002",
								},
							},
						},
					},
				},
			},
			wantErr:   fmt.Errorf("port 31001 has assigned to spec.sentinel.expose.accessPort"),
			wantWarns: nil,
		},
		{
			name: "failover enabled activeredis without serviceID",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(2),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
					Sentinel: &redisfailoverv1.SentinelSettings{
						RedisSentinelSpec: redisfailoverv1.RedisSentinelSpec{
							Replicas: 3,
							Expose: core.InstanceAccess{
								InstanceAccessBase: core.InstanceAccessBase{
									ServiceType: corev1.ServiceTypeNodePort,
								},
							},
						},
					},
					EnableActiveRedis: true,
				},
			},
			wantErr:   fmt.Errorf("activeredis is enabled but serviceID is not valid"),
			wantWarns: nil,
		},
		{
			name: "failover enabled activeredis with invalid serviceID",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(2),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
					Sentinel: &redisfailoverv1.SentinelSettings{
						RedisSentinelSpec: redisfailoverv1.RedisSentinelSpec{
							Replicas: 3,
							Expose: core.InstanceAccess{
								InstanceAccessBase: core.InstanceAccessBase{
									ServiceType: corev1.ServiceTypeNodePort,
								},
							},
						},
					},
					EnableActiveRedis: true,
					ServiceID:         pointer.Int32(-1),
				},
			},
			wantErr:   fmt.Errorf("activeredis is enabled but serviceID is not valid"),
			wantWarns: nil,
		},
		{
			name: "failover enabled activeredis with invalid serviceID",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(2),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
					Sentinel: &redisfailoverv1.SentinelSettings{
						RedisSentinelSpec: redisfailoverv1.RedisSentinelSpec{
							Replicas: 3,
							Expose: core.InstanceAccess{
								InstanceAccessBase: core.InstanceAccessBase{
									ServiceType: corev1.ServiceTypeNodePort,
								},
							},
						},
					},
					EnableActiveRedis: true,
					ServiceID:         pointer.Int32(16),
				},
			},
			wantErr:   fmt.Errorf("activeredis is enabled but serviceID is not valid"),
			wantWarns: nil,
		},
		{
			name: "failover enabled activeredis",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(2),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
					Sentinel: &redisfailoverv1.SentinelSettings{
						RedisSentinelSpec: redisfailoverv1.RedisSentinelSpec{
							Replicas: 3,
							Expose: core.InstanceAccess{
								InstanceAccessBase: core.InstanceAccessBase{
									ServiceType: corev1.ServiceTypeNodePort,
								},
							},
						},
					},
					EnableActiveRedis: true,
					ServiceID:         pointer.Int32(1),
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
		{
			name: "failover enabled tls for v5",
			redis: &Redis{
				Spec: RedisSpec{
					Arch:    core.RedisCluster,
					Version: "5.0",
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
						},
					},
					EnableTLS: true,
				},
			},
			wantErr:   fmt.Errorf("tls not supported in version 5.0"),
			wantWarns: nil,
		},
		{
			name: "failover enabled tls",
			redis: &Redis{
				Spec: RedisSpec{
					Arch:    core.RedisCluster,
					Version: "6.0",
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
						},
					},
					EnableTLS: true,
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warns, err := tt.redis.ValidateCreate()
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("ValidateCreateFailover() error = %v, wantErr %v", err, tt.wantErr)
				} else if err.Error() != tt.wantErr.Error() {
					t.Errorf("ValidateCreateFailover() error = %v, wantErr %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("ValidateCreateFailover() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(warns, tt.wantWarns) {
				t.Errorf("ValidateCreateFailover() warns = %v, wantWarns %v", warns, tt.wantWarns)
			}
		})
	}
}

func TestRedis_ValidateCreateStandalone(t *testing.T) {
	tests := []struct {
		name      string
		redis     *Redis
		wantErr   error
		wantWarns admission.Warnings
	}{
		{
			name: "standalone nil replcas",
			redis: &Redis{
				Spec: RedisSpec{
					Arch:     core.RedisStandalone,
					Replicas: nil,
				},
			},
			wantErr:   fmt.Errorf("spec.replicas.sentinel.master must be 1"),
			wantWarns: nil,
		},
		{
			name: "standalone ok",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisStandalone,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{},
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
		{
			name: "standalone enabled nodeport",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisStandalone,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
		{
			name: "standalone enable nodeport with custom port",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisStandalone,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType:      corev1.ServiceTypeNodePort,
							NodePortSequence: "30000",
						},
					},
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
		{
			name: "standalone enabled activeredis without serviceID",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisStandalone,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
					EnableActiveRedis: true,
				},
			},
			wantErr:   fmt.Errorf("activeredis is enabled but serviceID is not valid"),
			wantWarns: nil,
		},
		{
			name: "standalone enabled activeredis with invalid serviceID",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisStandalone,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
					EnableActiveRedis: true,
					ServiceID:         pointer.Int32(-1),
				},
			},
			wantErr:   fmt.Errorf("activeredis is enabled but serviceID is not valid"),
			wantWarns: nil,
		},
		{
			name: "standalone enabled activeredis with invalid serviceID",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisStandalone,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
					EnableActiveRedis: true,
					ServiceID:         pointer.Int32(16),
				},
			},
			wantErr:   fmt.Errorf("activeredis is enabled but serviceID is not valid"),
			wantWarns: nil,
		},
		{
			name: "standalone enabled activeredis",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisStandalone,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
					EnableActiveRedis: true,
					ServiceID:         pointer.Int32(1),
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warns, err := tt.redis.ValidateCreate()
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("ValidateCreateStandalone() error = %v, wantErr %v", err, tt.wantErr)
				} else if err.Error() != tt.wantErr.Error() {
					t.Errorf("ValidateCreateStandalone() error = %v, wantErr %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("ValidateCreateStandalone() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(warns, tt.wantWarns) {
				t.Errorf("ValidateCreateStandalone() warns = %v, wantWarns %v", warns, tt.wantWarns)
			}
		})
	}
}

func TestRedis_ValidateUpdateCluster(t *testing.T) {
	tests := []struct {
		name      string
		redis     *Redis
		wantErr   error
		wantWarns admission.Warnings
	}{
		{
			name: "cluster nil replcas",
			redis: &Redis{
				Spec: RedisSpec{
					Arch:     core.RedisCluster,
					Replicas: nil,
				},
			},
			wantErr:   fmt.Errorf("instance replicas not specified"),
			wantWarns: nil,
		},
		{
			name: "cluster enabled nodeport",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
		{
			name: "cluster matched nodeport count",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType:      corev1.ServiceTypeNodePort,
							NodePortSequence: "30000,30001,30002-30004,30005",
						},
					},
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
		{
			name: "cluster not matched nodeport count",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType:      corev1.ServiceTypeNodePort,
							NodePortSequence: "30000,30002-30004,30005",
						},
					},
				},
			},
			wantErr:   fmt.Errorf("expected 6 nodes, but got 5 ports in node port sequence"),
			wantWarns: nil,
		},
		{
			name: "cluster with matched specified shards",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
							Shards: []clusterv1.ClusterShardConfig{
								{Slots: "0-5461"},
								{Slots: "5462-10922"},
								{Slots: "10923-16383"},
							},
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
		{
			name: "cluster with not matched specified shards#not work for update",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
							Shards: []clusterv1.ClusterShardConfig{
								{Slots: "0-10000"},
							},
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
		{
			name: "cluster specified shards with invalid slots",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
							Shards: []clusterv1.ClusterShardConfig{
								{Slots: "0-5461"},
								{Slots: "5462-10922"},
								{Slots: "10923-16433"},
							},
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
				},
			},
			wantErr:   fmt.Errorf("failed to load shard slots: invalid slot 16384"),
			wantWarns: nil,
		},
		{
			name: "cluster specified shards with invalid slots2",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
							Shards: []clusterv1.ClusterShardConfig{
								{Slots: "aaaaa"},
								{Slots: "5462-10922"},
								{Slots: "10923-16433"},
							},
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
				},
			},
			wantErr:   fmt.Errorf("failed to load shard slots: invalid range slot aaaaa"),
			wantWarns: nil,
		},
		{
			name: "cluster shards specified slots not fullfilled",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
							Shards: []clusterv1.ClusterShardConfig{
								{Slots: "0-5461"},
								{Slots: "5462-10921"},
								{Slots: "10923-16383"},
							},
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
				},
			},
			wantErr:   fmt.Errorf("specified shard slots not fullfilled"),
			wantWarns: nil,
		},
		{
			name: "cluster shards specified slots duplicated",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
							Shards: []clusterv1.ClusterShardConfig{
								{Slots: "0-5462"},
								{Slots: "5462-10923"},
								{Slots: "10923-16383"},
							},
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
				},
			},
			wantErr:   fmt.Errorf("specified shard slots duplicated"),
			wantWarns: nil,
		},
		{
			name: "cluster enabled activeredis without serviceID",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
							Shards: []clusterv1.ClusterShardConfig{
								{Slots: "0-5461"},
								{Slots: "5462-10922"},
								{Slots: "10923-16383"},
							},
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
					EnableActiveRedis: true,
				},
			},
			wantErr:   fmt.Errorf("activeredis is enabled but serviceID is not valid"),
			wantWarns: nil,
		},

		{
			name: "cluster enabled activeredis",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisCluster,
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
							Shards: []clusterv1.ClusterShardConfig{
								{Slots: "0-5461"},
								{Slots: "5462-10922"},
								{Slots: "10923-16383"},
							},
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
					EnableActiveRedis: true,
					ServiceID:         pointer.Int32(1),
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
		{
			name: "cluster enabled tls for v5",
			redis: &Redis{
				Spec: RedisSpec{
					Arch:    core.RedisCluster,
					Version: "5.0",
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
						},
					},
					EnableTLS: true,
				},
			},
			wantErr:   fmt.Errorf("tls not supported in version 5.0"),
			wantWarns: nil,
		},
		{
			name: "cluster enabled tls",
			redis: &Redis{
				Spec: RedisSpec{
					Arch:    core.RedisCluster,
					Version: "6.0",
					Replicas: &RedisReplicas{
						Cluster: &ClusterReplicas{
							Shard: pointer.Int32(3),
							Slave: pointer.Int32(1),
						},
					},
					EnableTLS: true,
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warns, err := tt.redis.ValidateUpdate(nil)
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("ValidateUpdate() error = %v, wantErr %v", err, tt.wantErr)
				} else if err.Error() != tt.wantErr.Error() {
					t.Errorf("ValidateUpdate() error = %v, wantErr %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("ValidateCreate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(warns, tt.wantWarns) {
				t.Errorf("ValidateUpdate() warns = %v, wantWarns %v", warns, tt.wantWarns)
			}
		})
	}
}

func TestRedis_ValidateUpdateFailover(t *testing.T) {
	tests := []struct {
		name      string
		redis     *Redis
		wantErr   error
		wantWarns admission.Warnings
	}{
		{
			name: "failover nil replcas",
			redis: &Redis{
				Spec: RedisSpec{
					Arch:     core.RedisSentinel,
					Replicas: nil,
				},
			},
			wantErr:   fmt.Errorf("instance replicas not specified"),
			wantWarns: nil,
		},
		{
			name: "failover not enable nodeport",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{},
					Sentinel: &redisfailoverv1.SentinelSettings{
						RedisSentinelSpec: redisfailoverv1.RedisSentinelSpec{
							Replicas: 1,
						},
					},
				},
			},
			wantErr:   fmt.Errorf("sentinel replicas must be odd and greater >= 3"),
			wantWarns: nil,
		},
		{
			name: "failover not enable nodeport, with invalid sentinel replicas",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{},
					Sentinel: &redisfailoverv1.SentinelSettings{
						RedisSentinelSpec: redisfailoverv1.RedisSentinelSpec{
							Replicas: 4,
						},
					},
				},
			},
			wantErr:   fmt.Errorf("sentinel replicas must be odd and greater >= 3"),
			wantWarns: nil,
		},
		{
			name: "failover enabled nodeport",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
					Sentinel: &redisfailoverv1.SentinelSettings{},
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
		{
			name: "failover matched nodeport count",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(2),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType:      corev1.ServiceTypeNodePort,
							NodePortSequence: "30000,30001-30002",
						},
					},
					Sentinel: &redisfailoverv1.SentinelSettings{
						RedisSentinelSpec: redisfailoverv1.RedisSentinelSpec{
							Replicas: 3,
							Expose: core.InstanceAccess{
								InstanceAccessBase: core.InstanceAccessBase{
									ServiceType:      corev1.ServiceTypeNodePort,
									NodePortSequence: "31000,31001-31002",
								},
							},
						},
					},
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
		{
			name: "failover nodeport not match--data node",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(2),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType:      corev1.ServiceTypeNodePort,
							NodePortSequence: "30000,30001",
						},
					},
					Sentinel: &redisfailoverv1.SentinelSettings{
						RedisSentinelSpec: redisfailoverv1.RedisSentinelSpec{
							Replicas: 3,
							Expose: core.InstanceAccess{
								InstanceAccessBase: core.InstanceAccessBase{
									ServiceType:      corev1.ServiceTypeNodePort,
									NodePortSequence: "31000,31001-31002",
								},
							},
						},
					},
				},
			},
			wantErr:   fmt.Errorf("expected 3 nodes, but got 2 ports in node port sequence"),
			wantWarns: nil,
		},
		{
			name: "failover nodeport not match--sentinel node",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(2),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType:      corev1.ServiceTypeNodePort,
							NodePortSequence: "30000,30001-30002",
						},
					},
					Sentinel: &redisfailoverv1.SentinelSettings{
						RedisSentinelSpec: redisfailoverv1.RedisSentinelSpec{
							Replicas: 3,
							Expose: core.InstanceAccess{
								InstanceAccessBase: core.InstanceAccessBase{
									ServiceType:      corev1.ServiceTypeNodePort,
									NodePortSequence: "31001-31002",
								},
							},
						},
					},
				},
			},
			wantErr:   fmt.Errorf("expected 3 nodes, but got 2 ports in node port sequence"),
			wantWarns: nil,
		},
		{
			name: "failover duplicate nodeport",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(2),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType:      corev1.ServiceTypeNodePort,
							NodePortSequence: "30000,30001-30002",
						},
					},
					Sentinel: &redisfailoverv1.SentinelSettings{
						RedisSentinelSpec: redisfailoverv1.RedisSentinelSpec{
							Replicas: 3,
							Expose: core.InstanceAccess{
								InstanceAccessBase: core.InstanceAccessBase{
									ServiceType:      corev1.ServiceTypeNodePort,
									NodePortSequence: "30000,31001-31002",
								},
							},
						},
					},
				},
			},
			wantErr:   fmt.Errorf("port 30000 has assigned to spec.expose.dataStorageNodePortSequence"),
			wantWarns: nil,
		},
		{
			name: "failover duplicate nodeport with accessPort",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(2),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType:      corev1.ServiceTypeNodePort,
							NodePortSequence: "30000,30001-30002",
						},
					},
					Sentinel: &redisfailoverv1.SentinelSettings{
						RedisSentinelSpec: redisfailoverv1.RedisSentinelSpec{
							Replicas: 3,
							Expose: core.InstanceAccess{
								InstanceAccessBase: core.InstanceAccessBase{
									ServiceType:      corev1.ServiceTypeNodePort,
									AccessPort:       31001,
									NodePortSequence: "31000,31001-31002",
								},
							},
						},
					},
				},
			},
			wantErr:   fmt.Errorf("port 31001 has assigned to spec.sentinel.expose.accessPort"),
			wantWarns: nil,
		},
		{
			name: "failover enabled activeredis without serviceID",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(2),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
					Sentinel: &redisfailoverv1.SentinelSettings{
						RedisSentinelSpec: redisfailoverv1.RedisSentinelSpec{
							Replicas: 3,
							Expose: core.InstanceAccess{
								InstanceAccessBase: core.InstanceAccessBase{
									ServiceType: corev1.ServiceTypeNodePort,
								},
							},
						},
					},
					EnableActiveRedis: true,
				},
			},
			wantErr:   fmt.Errorf("activeredis is enabled but serviceID is not valid"),
			wantWarns: nil,
		},
		{
			name: "failover enabled activeredis with invalid serviceID",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(2),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
					Sentinel: &redisfailoverv1.SentinelSettings{
						RedisSentinelSpec: redisfailoverv1.RedisSentinelSpec{
							Replicas: 3,
							Expose: core.InstanceAccess{
								InstanceAccessBase: core.InstanceAccessBase{
									ServiceType: corev1.ServiceTypeNodePort,
								},
							},
						},
					},
					EnableActiveRedis: true,
					ServiceID:         pointer.Int32(-1),
				},
			},
			wantErr:   fmt.Errorf("activeredis is enabled but serviceID is not valid"),
			wantWarns: nil,
		},
		{
			name: "failover enabled activeredis with invalid serviceID",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(2),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
					Sentinel: &redisfailoverv1.SentinelSettings{
						RedisSentinelSpec: redisfailoverv1.RedisSentinelSpec{
							Replicas: 3,
							Expose: core.InstanceAccess{
								InstanceAccessBase: core.InstanceAccessBase{
									ServiceType: corev1.ServiceTypeNodePort,
								},
							},
						},
					},
					EnableActiveRedis: true,
					ServiceID:         pointer.Int32(16),
				},
			},
			wantErr:   fmt.Errorf("activeredis is enabled but serviceID is not valid"),
			wantWarns: nil,
		},
		{
			name: "failover enabled activeredis",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisSentinel,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
							Slave:  pointer.Int32(2),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
					Sentinel: &redisfailoverv1.SentinelSettings{
						RedisSentinelSpec: redisfailoverv1.RedisSentinelSpec{
							Replicas: 3,
							Expose: core.InstanceAccess{
								InstanceAccessBase: core.InstanceAccessBase{
									ServiceType: corev1.ServiceTypeNodePort,
								},
							},
						},
					},
					EnableActiveRedis: true,
					ServiceID:         pointer.Int32(1),
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warns, err := tt.redis.ValidateUpdate(nil)
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("ValidateUpdateFailover() error = %v, wantErr %v", err, tt.wantErr)
				} else if err.Error() != tt.wantErr.Error() {
					t.Errorf("ValidateUpdatekFailover() error = %v, wantErr %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("ValidateUpdateFailover() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(warns, tt.wantWarns) {
				t.Errorf("ValidateUpdateFailover() warns = %v, wantWarns %v", warns, tt.wantWarns)
			}
		})
	}
}

func TestRedis_ValidateUpdateStandalone(t *testing.T) {
	tests := []struct {
		name      string
		redis     *Redis
		wantErr   error
		wantWarns admission.Warnings
	}{
		{
			name: "standalone nil replcas",
			redis: &Redis{
				Spec: RedisSpec{
					Arch:     core.RedisStandalone,
					Replicas: nil,
				},
			},
			wantErr:   fmt.Errorf("spec.replicas.sentinel.master must be 1"),
			wantWarns: nil,
		},
		{
			name: "standalone ok",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisStandalone,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{},
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
		{
			name: "standalone enabled nodeport",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisStandalone,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
		{
			name: "standalone enable nodeport with custom port",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisStandalone,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType:      corev1.ServiceTypeNodePort,
							NodePortSequence: "30000",
						},
					},
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
		{
			name: "standalone enabled activeredis without serviceID",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisStandalone,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
					EnableActiveRedis: true,
				},
			},
			wantErr:   fmt.Errorf("activeredis is enabled but serviceID is not valid"),
			wantWarns: nil,
		},
		{
			name: "standalone enabled activeredis with invalid serviceID",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisStandalone,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
					EnableActiveRedis: true,
					ServiceID:         pointer.Int32(-1),
				},
			},
			wantErr:   fmt.Errorf("activeredis is enabled but serviceID is not valid"),
			wantWarns: nil,
		},
		{
			name: "standalone enabled activeredis with invalid serviceID",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisStandalone,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
					EnableActiveRedis: true,
					ServiceID:         pointer.Int32(16),
				},
			},
			wantErr:   fmt.Errorf("activeredis is enabled but serviceID is not valid"),
			wantWarns: nil,
		},
		{
			name: "standalone enabled activeredis",
			redis: &Redis{
				Spec: RedisSpec{
					Arch: core.RedisStandalone,
					Replicas: &RedisReplicas{
						Sentinel: &SentinelReplicas{
							Master: pointer.Int32(1),
						},
					},
					Expose: InstanceAccess{
						InstanceAccessBase: core.InstanceAccessBase{
							ServiceType: corev1.ServiceTypeNodePort,
						},
					},
					EnableActiveRedis: true,
					ServiceID:         pointer.Int32(1),
				},
			},
			wantErr:   nil,
			wantWarns: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warns, err := tt.redis.ValidateUpdate(nil)
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("ValidateUpdateStandalone() error = %v, wantErr %v", err, tt.wantErr)
				} else if err.Error() != tt.wantErr.Error() {
					t.Errorf("ValidateCreateStandalone() error = %v, wantErr %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("ValidateUpdateStandalone() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(warns, tt.wantWarns) {
				t.Errorf("ValidateUpdateStandalone() warns = %v, wantWarns %v", warns, tt.wantWarns)
			}
		})
	}
}

func TestRedis_ValidateDelete(t *testing.T) {
	redis := &Redis{}
	_, err := redis.ValidateDelete()
	if err != nil {
		t.Errorf("ValidateDelete() error = %v, wantErr %v", err, false)
	}
}

func Test_getNodeCountByArch(t *testing.T) {
	type args struct {
		arch     core.Arch
		replicas *RedisReplicas
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			name: "cluster 3-0",
			args: args{
				arch: core.RedisCluster,
				replicas: &RedisReplicas{
					Cluster: &ClusterReplicas{
						Shard: pointer.Int32(3),
					},
				},
			},
			want: 3,
		},
		{
			name: "cluster 3-1",
			args: args{
				arch: core.RedisCluster,
				replicas: &RedisReplicas{
					Cluster: &ClusterReplicas{
						Shard: pointer.Int32(3),
						Slave: pointer.Int32(1),
					},
				},
			},
			want: 6,
		},
		{
			name: "cluster 3-2",
			args: args{
				arch: core.RedisCluster,
				replicas: &RedisReplicas{
					Cluster: &ClusterReplicas{
						Shard: pointer.Int32(3),
						Slave: pointer.Int32(2),
					},
				},
			},
			want: 9,
		},
		{
			name: "sentinel 1",
			args: args{
				arch: core.RedisSentinel,
				replicas: &RedisReplicas{
					Sentinel: &SentinelReplicas{
						Master: pointer.Int32(1),
					},
				},
			},
			want: 1,
		},
		{
			name: "sentinel 2",
			args: args{
				arch: core.RedisSentinel,
				replicas: &RedisReplicas{
					Sentinel: &SentinelReplicas{
						Master: pointer.Int32(1),
						Slave:  pointer.Int32(1),
					},
				},
			},
			want: 2,
		},
		{
			name: "sentinel 3",
			args: args{
				arch: core.RedisSentinel,
				replicas: &RedisReplicas{
					Sentinel: &SentinelReplicas{
						Master: pointer.Int32(1),
						Slave:  pointer.Int32(2),
					},
				},
			},
			want: 3,
		},
		{
			name: "standalone 0",
			args: args{
				arch:     core.RedisStandalone,
				replicas: &RedisReplicas{},
			},
			want: 1,
		},
		{
			name: "standalone 1",
			args: args{
				arch: core.RedisStandalone,
				replicas: &RedisReplicas{
					Sentinel: &SentinelReplicas{
						Master: pointer.Int32(1),
					},
				},
			},
			want: 1,
		},
		{
			name: "standalone any",
			args: args{
				arch: core.RedisStandalone,
				replicas: &RedisReplicas{
					Sentinel: &SentinelReplicas{
						Master: pointer.Int32(10),
						Slave:  pointer.Int32(3),
					},
				},
			},
			want: 1,
		},
		{
			name: "empty",
			args: args{},
			want: 0,
		},
		{
			name: "empty2",
			args: args{
				replicas: &RedisReplicas{},
			},
			want: 0,
		},
		{
			name: "empty cluster",
			args: args{
				arch:     core.RedisCluster,
				replicas: &RedisReplicas{},
			},
			want: 0,
		},
		{
			name: "empty sentinel",
			args: args{
				arch:     core.RedisSentinel,
				replicas: &RedisReplicas{},
			},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getNodeCountByArch(tt.args.arch, tt.args.replicas); got != tt.want {
				t.Errorf("getNodeCountByArch() = %v, want %v", got, tt.want)
			}
		})
	}
}
