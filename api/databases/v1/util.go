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

const (
	DefaultRedisPort string = "6379"
)

func (r *RedisFailoverStatus) SetPausedPhase(message string) {
	r.Phase = Paused
	r.Message = message
}

func (r *RedisFailoverStatus) SetFailedPhase(message string) {
	r.Phase = Fail
	r.Message = message
}

func (r *RedisFailoverStatus) SetMasterOK(ip string, port string) {
	r.Master.Address = ip + ":" + port
	r.Master.Status = "ok"
}

func (r *RedisFailoverStatus) SetMasterDown(ip string, port string) {
	r.Master.Address = ip + ":" + port
	r.Master.Status = "down"
}

func (r *RedisFailoverStatus) SetWaitingPodReady(message string) {
	r.Phase = WaitingPodReady
	r.Message = message
}

func (r *RedisFailoverStatus) IsWaitingPodReady() bool {
	return r.Phase == WaitingPodReady
}

func (r *RedisFailoverStatus) IsPaused() bool {
	return r.Phase == Paused
}

func (r *RedisFailoverStatus) SetReady(message string) {
	r.Phase = Ready
	r.Message = message
}
