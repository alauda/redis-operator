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

package clusterbuilder

import (
	"fmt"
	"os"
	"strings"

	redisv1alpha1 "github.com/alauda/redis-operator/api/redis.kun/v1alpha1"
	smv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	DefaultScrapInterval = "60s"
	DefaultScrapeTimeout = "10s"
)

func getDefaultRegex(regex []string) string {
	return fmt.Sprintf("(%s)", strings.Join(uniqueArr(regex), "|"))
}

func uniqueArr(m []string) []string {
	d := make([]string, 0)
	result := make(map[string]bool, len(m))
	for _, v := range m {
		if !result[v] {
			result[v] = true
			d = append(d, v)
		}
	}
	return d
}

var regexArr = []string{
	"redis_instance_info",
	"redis_slave_info",
	"redis_connected_clients",
	"redis_uptime_in_seconds",
	"redis_commands_processed_total",
	"redis_commands_total",
	"redis_db_keys",
	"redis_pubsub_channels",
	"redis_pubsub_patterns",
	"redis_slowlog_length",
	"redis_evicted_keys_total",
	"redis_expired_keys_total",
	"redis_keyspace_hits_total",
	"redis_keyspace_misses_total",
	"redis_blocked_clients",
	"redis_commands_duration_seconds_total",
	"redis_net_input_bytes_total",
	"redis_net_output_bytes_total",
	"redis_cpu_user_seconds_total",
	"redis_cpu_user_children_seconds_total",
	"redis_cpu_sys_seconds_total",
	"redis_cpu_sys_children_seconds_total",
	"redis_memory_used_bytes",
	"redis_memory_used_rss_bytes",
	"redis_memory_used_peak_bytes",
	"redis_memory_max_bytes",
	"redis_aof_rewrite_in_progress",
	"redis_rdb_bgsave_in_progress",
	"redis_aof_last_rewrite_duration_sec",
	"redis_rdb_last_save_timestamp_seconds",
	"redis_rdb_last_bgsave_duration_sec",
	"redis_rdb_last_cow_size_bytes",
	"redis_rdb_last_bgsave_status",
	"redis_mem_fragmentation_ratio",
	"redis_mem_fragmentation_bytes",
	"redis_master_repl_offset",
	"redis_lazyfree_pending_objects",
	"redis_latest_fork_seconds",
	"redis_defrag_misses",
	"redis_defrag_key_misses",
	"redis_defrag_key_hits",
	"redis_connections_received_total",
	"redis_connected_slaves",
	"redis_connected_slave_lag_seconds",
	"redis_connected_slave_offset_bytes",
	"redis_aof_last_cow_size_bytes",
	"redis_aof_last_bgrewrite_status",
	"redis_memory_used_lua_bytes",
	"redis_config_maxmemory",
	"redis_cluster_.*",
}

func GetPodOwnerReferences() metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: "v1",
		Kind:       "Pod",
		Name:       os.Getenv("POD_NAME"),
		UID:        types.UID(os.Getenv("POD_UID")),
	}
}

const (
	RedisClusterServiceMonitorName = "redis-cluster"
)

func NewServiceMonitorForCR(cluster *redisv1alpha1.DistributedRedisCluster) *smv1.ServiceMonitor {
	labels := map[string]string{
		"managed-by": "redis-cluster-operator",
	}

	interval := DefaultScrapInterval
	scrapeTimeout := DefaultScrapeTimeout

	configs := []*smv1.RelabelConfig{{
		Action:       "keep",
		Regex:        getDefaultRegex(regexArr),
		SourceLabels: []smv1.LabelName{"__name__"},
	}}

	if cluster != nil {
		if cluster.Spec.ServiceMonitor.Interval != "" {
			interval = cluster.Spec.ServiceMonitor.Interval
		}
		if cluster.Spec.ServiceMonitor.ScrapeTimeout != "" {
			scrapeTimeout = cluster.Spec.ServiceMonitor.ScrapeTimeout
		}
		if cluster.Spec.ServiceMonitor.CustomMetricRelabelings {
			configs = cluster.Spec.ServiceMonitor.MetricRelabelConfigs
		}
	}

	sm := &smv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name: RedisClusterServiceMonitorName,
			Labels: map[string]string{
				"prometheus": "kube-prometheus",
			},
		},
		Spec: smv1.ServiceMonitorSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: labels,
			},
			NamespaceSelector: smv1.NamespaceSelector{
				Any: true,
			},
			Endpoints: []smv1.Endpoint{
				// for sentinel metrics
				{
					HonorLabels:          true,
					Port:                 "http-metrics",
					Path:                 "/metrics",
					Interval:             smv1.Duration(interval),
					ScrapeTimeout:        smv1.Duration(scrapeTimeout),
					MetricRelabelConfigs: configs,
				},
				{
					HonorLabels:          true,
					Port:                 "metrics",
					Path:                 "/metrics",
					Interval:             smv1.Duration(interval),
					ScrapeTimeout:        smv1.Duration(scrapeTimeout),
					MetricRelabelConfigs: configs,
				},
				// for cluster metrics
				{
					HonorLabels:          true,
					Port:                 "prom-http",
					Path:                 "/metrics",
					Interval:             smv1.Duration(interval),
					ScrapeTimeout:        smv1.Duration(scrapeTimeout),
					MetricRelabelConfigs: configs,
				},
			},
			TargetLabels: []string{"redisarch"},
		},
	}
	return sm
}
