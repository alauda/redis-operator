//go:build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1

import (
	"github.com/alauda/redis-operator/api/core"
	corev1 "k8s.io/api/core/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AuthSettings) DeepCopyInto(out *AuthSettings) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AuthSettings.
func (in *AuthSettings) DeepCopy() *AuthSettings {
	if in == nil {
		return nil
	}
	out := new(AuthSettings)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Authorization) DeepCopyInto(out *Authorization) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Authorization.
func (in *Authorization) DeepCopy() *Authorization {
	if in == nil {
		return nil
	}
	out := new(Authorization)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MonitorStatus) DeepCopyInto(out *MonitorStatus) {
	*out = *in
	if in.Nodes != nil {
		in, out := &in.Nodes, &out.Nodes
		*out = make([]SentinelMonitorNode, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MonitorStatus.
func (in *MonitorStatus) DeepCopy() *MonitorStatus {
	if in == nil {
		return nil
	}
	out := new(MonitorStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisExporter) DeepCopyInto(out *RedisExporter) {
	*out = *in
	in.Resources.DeepCopyInto(&out.Resources)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisExporter.
func (in *RedisExporter) DeepCopy() *RedisExporter {
	if in == nil {
		return nil
	}
	out := new(RedisExporter)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisFailover) DeepCopyInto(out *RedisFailover) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisFailover.
func (in *RedisFailover) DeepCopy() *RedisFailover {
	if in == nil {
		return nil
	}
	out := new(RedisFailover)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RedisFailover) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisFailoverDetailedStatus) DeepCopyInto(out *RedisFailoverDetailedStatus) {
	*out = *in
	if in.Nodes != nil {
		in, out := &in.Nodes, &out.Nodes
		*out = make([]core.RedisDetailedNode, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisFailoverDetailedStatus.
func (in *RedisFailoverDetailedStatus) DeepCopy() *RedisFailoverDetailedStatus {
	if in == nil {
		return nil
	}
	out := new(RedisFailoverDetailedStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisFailoverList) DeepCopyInto(out *RedisFailoverList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]RedisFailover, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisFailoverList.
func (in *RedisFailoverList) DeepCopy() *RedisFailoverList {
	if in == nil {
		return nil
	}
	out := new(RedisFailoverList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RedisFailoverList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisFailoverSpec) DeepCopyInto(out *RedisFailoverSpec) {
	*out = *in
	in.Redis.DeepCopyInto(&out.Redis)
	if in.Sentinel != nil {
		in, out := &in.Sentinel, &out.Sentinel
		*out = new(SentinelSettings)
		(*in).DeepCopyInto(*out)
	}
	out.Auth = in.Auth
	if in.LabelWhitelist != nil {
		in, out := &in.LabelWhitelist, &out.LabelWhitelist
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.ServiceID != nil {
		in, out := &in.ServiceID, &out.ServiceID
		*out = new(int32)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisFailoverSpec.
func (in *RedisFailoverSpec) DeepCopy() *RedisFailoverSpec {
	if in == nil {
		return nil
	}
	out := new(RedisFailoverSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisFailoverStatus) DeepCopyInto(out *RedisFailoverStatus) {
	*out = *in
	in.Instance.DeepCopyInto(&out.Instance)
	out.Master = in.Master
	if in.Nodes != nil {
		in, out := &in.Nodes, &out.Nodes
		*out = make([]core.RedisNode, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.Monitor.DeepCopyInto(&out.Monitor)
	if in.DetailedStatusRef != nil {
		in, out := &in.DetailedStatusRef, &out.DetailedStatusRef
		*out = new(corev1.ObjectReference)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisFailoverStatus.
func (in *RedisFailoverStatus) DeepCopy() *RedisFailoverStatus {
	if in == nil {
		return nil
	}
	out := new(RedisFailoverStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisSentinel) DeepCopyInto(out *RedisSentinel) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisSentinel.
func (in *RedisSentinel) DeepCopy() *RedisSentinel {
	if in == nil {
		return nil
	}
	out := new(RedisSentinel)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RedisSentinel) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisSentinelList) DeepCopyInto(out *RedisSentinelList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]RedisSentinel, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisSentinelList.
func (in *RedisSentinelList) DeepCopy() *RedisSentinelList {
	if in == nil {
		return nil
	}
	out := new(RedisSentinelList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *RedisSentinelList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisSentinelSpec) DeepCopyInto(out *RedisSentinelSpec) {
	*out = *in
	if in.ImagePullSecrets != nil {
		in, out := &in.ImagePullSecrets, &out.ImagePullSecrets
		*out = make([]corev1.LocalObjectReference, len(*in))
		copy(*out, *in)
	}
	in.Resources.DeepCopyInto(&out.Resources)
	if in.CustomConfig != nil {
		in, out := &in.CustomConfig, &out.CustomConfig
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Exporter != nil {
		in, out := &in.Exporter, &out.Exporter
		*out = new(SentinelExporter)
		**out = **in
	}
	in.Expose.DeepCopyInto(&out.Expose)
	if in.Affinity != nil {
		in, out := &in.Affinity, &out.Affinity
		*out = new(corev1.Affinity)
		(*in).DeepCopyInto(*out)
	}
	if in.SecurityContext != nil {
		in, out := &in.SecurityContext, &out.SecurityContext
		*out = new(corev1.PodSecurityContext)
		(*in).DeepCopyInto(*out)
	}
	if in.Tolerations != nil {
		in, out := &in.Tolerations, &out.Tolerations
		*out = make([]corev1.Toleration, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.NodeSelector != nil {
		in, out := &in.NodeSelector, &out.NodeSelector
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.PodAnnotations != nil {
		in, out := &in.PodAnnotations, &out.PodAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.ServiceAnnotations != nil {
		in, out := &in.ServiceAnnotations, &out.ServiceAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisSentinelSpec.
func (in *RedisSentinelSpec) DeepCopy() *RedisSentinelSpec {
	if in == nil {
		return nil
	}
	out := new(RedisSentinelSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisSentinelStatus) DeepCopyInto(out *RedisSentinelStatus) {
	*out = *in
	if in.Nodes != nil {
		in, out := &in.Nodes, &out.Nodes
		*out = make([]core.RedisNode, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisSentinelStatus.
func (in *RedisSentinelStatus) DeepCopy() *RedisSentinelStatus {
	if in == nil {
		return nil
	}
	out := new(RedisSentinelStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisSettings) DeepCopyInto(out *RedisSettings) {
	*out = *in
	if in.ImagePullSecrets != nil {
		in, out := &in.ImagePullSecrets, &out.ImagePullSecrets
		*out = make([]corev1.LocalObjectReference, len(*in))
		copy(*out, *in)
	}
	in.Resources.DeepCopyInto(&out.Resources)
	if in.CustomConfig != nil {
		in, out := &in.CustomConfig, &out.CustomConfig
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	in.Storage.DeepCopyInto(&out.Storage)
	in.Exporter.DeepCopyInto(&out.Exporter)
	in.Expose.DeepCopyInto(&out.Expose)
	if in.Affinity != nil {
		in, out := &in.Affinity, &out.Affinity
		*out = new(corev1.Affinity)
		(*in).DeepCopyInto(*out)
	}
	if in.SecurityContext != nil {
		in, out := &in.SecurityContext, &out.SecurityContext
		*out = new(corev1.PodSecurityContext)
		(*in).DeepCopyInto(*out)
	}
	if in.Tolerations != nil {
		in, out := &in.Tolerations, &out.Tolerations
		*out = make([]corev1.Toleration, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.NodeSelector != nil {
		in, out := &in.NodeSelector, &out.NodeSelector
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.PodAnnotations != nil {
		in, out := &in.PodAnnotations, &out.PodAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.ServiceAnnotations != nil {
		in, out := &in.ServiceAnnotations, &out.ServiceAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	in.Backup.DeepCopyInto(&out.Backup)
	out.Restore = in.Restore
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisSettings.
func (in *RedisSettings) DeepCopy() *RedisSettings {
	if in == nil {
		return nil
	}
	out := new(RedisSettings)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisStatusInstance) DeepCopyInto(out *RedisStatusInstance) {
	*out = *in
	out.Redis = in.Redis
	if in.Sentinel != nil {
		in, out := &in.Sentinel, &out.Sentinel
		*out = new(RedisStatusInstanceSentinel)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisStatusInstance.
func (in *RedisStatusInstance) DeepCopy() *RedisStatusInstance {
	if in == nil {
		return nil
	}
	out := new(RedisStatusInstance)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisStatusInstanceRedis) DeepCopyInto(out *RedisStatusInstanceRedis) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisStatusInstanceRedis.
func (in *RedisStatusInstanceRedis) DeepCopy() *RedisStatusInstanceRedis {
	if in == nil {
		return nil
	}
	out := new(RedisStatusInstanceRedis)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisStatusInstanceSentinel) DeepCopyInto(out *RedisStatusInstanceSentinel) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisStatusInstanceSentinel.
func (in *RedisStatusInstanceSentinel) DeepCopy() *RedisStatusInstanceSentinel {
	if in == nil {
		return nil
	}
	out := new(RedisStatusInstanceSentinel)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisStatusMaster) DeepCopyInto(out *RedisStatusMaster) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisStatusMaster.
func (in *RedisStatusMaster) DeepCopy() *RedisStatusMaster {
	if in == nil {
		return nil
	}
	out := new(RedisStatusMaster)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisStorage) DeepCopyInto(out *RedisStorage) {
	*out = *in
	if in.EmptyDir != nil {
		in, out := &in.EmptyDir, &out.EmptyDir
		*out = new(corev1.EmptyDirVolumeSource)
		(*in).DeepCopyInto(*out)
	}
	if in.PersistentVolumeClaim != nil {
		in, out := &in.PersistentVolumeClaim, &out.PersistentVolumeClaim
		*out = new(corev1.PersistentVolumeClaim)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisStorage.
func (in *RedisStorage) DeepCopy() *RedisStorage {
	if in == nil {
		return nil
	}
	out := new(RedisStorage)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SentinelExporter) DeepCopyInto(out *SentinelExporter) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SentinelExporter.
func (in *SentinelExporter) DeepCopy() *SentinelExporter {
	if in == nil {
		return nil
	}
	out := new(SentinelExporter)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SentinelMonitorNode) DeepCopyInto(out *SentinelMonitorNode) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SentinelMonitorNode.
func (in *SentinelMonitorNode) DeepCopy() *SentinelMonitorNode {
	if in == nil {
		return nil
	}
	out := new(SentinelMonitorNode)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SentinelReference) DeepCopyInto(out *SentinelReference) {
	*out = *in
	if in.Nodes != nil {
		in, out := &in.Nodes, &out.Nodes
		*out = make([]SentinelMonitorNode, len(*in))
		copy(*out, *in)
	}
	out.Auth = in.Auth
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SentinelReference.
func (in *SentinelReference) DeepCopy() *SentinelReference {
	if in == nil {
		return nil
	}
	out := new(SentinelReference)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SentinelSettings) DeepCopyInto(out *SentinelSettings) {
	*out = *in
	in.RedisSentinelSpec.DeepCopyInto(&out.RedisSentinelSpec)
	if in.SentinelReference != nil {
		in, out := &in.SentinelReference, &out.SentinelReference
		*out = new(SentinelReference)
		(*in).DeepCopyInto(*out)
	}
	if in.MonitorConfig != nil {
		in, out := &in.MonitorConfig, &out.MonitorConfig
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Quorum != nil {
		in, out := &in.Quorum, &out.Quorum
		*out = new(int32)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SentinelSettings.
func (in *SentinelSettings) DeepCopy() *SentinelSettings {
	if in == nil {
		return nil
	}
	out := new(SentinelSettings)
	in.DeepCopyInto(out)
	return out
}