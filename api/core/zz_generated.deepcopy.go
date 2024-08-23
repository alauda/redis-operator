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

package core

import ()

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HostPort) DeepCopyInto(out *HostPort) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HostPort.
func (in *HostPort) DeepCopy() *HostPort {
	if in == nil {
		return nil
	}
	out := new(HostPort)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InstanceAccess) DeepCopyInto(out *InstanceAccess) {
	*out = *in
	in.InstanceAccessBase.DeepCopyInto(&out.InstanceAccessBase)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InstanceAccess.
func (in *InstanceAccess) DeepCopy() *InstanceAccess {
	if in == nil {
		return nil
	}
	out := new(InstanceAccess)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *InstanceAccessBase) DeepCopyInto(out *InstanceAccessBase) {
	*out = *in
	if in.Annotations != nil {
		in, out := &in.Annotations, &out.Annotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.NodePortMap != nil {
		in, out := &in.NodePortMap, &out.NodePortMap
		*out = make(map[string]int32, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new InstanceAccessBase.
func (in *InstanceAccessBase) DeepCopy() *InstanceAccessBase {
	if in == nil {
		return nil
	}
	out := new(InstanceAccessBase)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisBackup) DeepCopyInto(out *RedisBackup) {
	*out = *in
	if in.Schedule != nil {
		in, out := &in.Schedule, &out.Schedule
		*out = make([]Schedule, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisBackup.
func (in *RedisBackup) DeepCopy() *RedisBackup {
	if in == nil {
		return nil
	}
	out := new(RedisBackup)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisBackupStorage) DeepCopyInto(out *RedisBackupStorage) {
	*out = *in
	out.Size = in.Size.DeepCopy()
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisBackupStorage.
func (in *RedisBackupStorage) DeepCopy() *RedisBackupStorage {
	if in == nil {
		return nil
	}
	out := new(RedisBackupStorage)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisBackupTarget) DeepCopyInto(out *RedisBackupTarget) {
	*out = *in
	out.S3Option = in.S3Option
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisBackupTarget.
func (in *RedisBackupTarget) DeepCopy() *RedisBackupTarget {
	if in == nil {
		return nil
	}
	out := new(RedisBackupTarget)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisDetailedNode) DeepCopyInto(out *RedisDetailedNode) {
	*out = *in
	in.RedisNode.DeepCopyInto(&out.RedisNode)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisDetailedNode.
func (in *RedisDetailedNode) DeepCopy() *RedisDetailedNode {
	if in == nil {
		return nil
	}
	out := new(RedisDetailedNode)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisNode) DeepCopyInto(out *RedisNode) {
	*out = *in
	if in.Slots != nil {
		in, out := &in.Slots, &out.Slots
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisNode.
func (in *RedisNode) DeepCopy() *RedisNode {
	if in == nil {
		return nil
	}
	out := new(RedisNode)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisRestore) DeepCopyInto(out *RedisRestore) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisRestore.
func (in *RedisRestore) DeepCopy() *RedisRestore {
	if in == nil {
		return nil
	}
	out := new(RedisRestore)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *S3Option) DeepCopyInto(out *S3Option) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new S3Option.
func (in *S3Option) DeepCopy() *S3Option {
	if in == nil {
		return nil
	}
	out := new(S3Option)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Schedule) DeepCopyInto(out *Schedule) {
	*out = *in
	in.Storage.DeepCopyInto(&out.Storage)
	out.Target = in.Target
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Schedule.
func (in *Schedule) DeepCopy() *Schedule {
	if in == nil {
		return nil
	}
	out := new(Schedule)
	in.DeepCopyInto(out)
	return out
}
