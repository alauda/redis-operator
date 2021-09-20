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
	PhaseFail            Phase = "Fail"
	PhaseCreating        Phase = "Creating"
	PhasePending         Phase = "Pending"
	PhaseReady           Phase = "Ready"
	PhaseWaitingPodReady Phase = "WaitingPodReady"
	PhasePaused          Phase = "Paused"

	DefaultRedisPort string = "6379"
)

func (r *RedisFailoverStatus) SetPausedPhase(message string) {
	r.Phase = PhasePaused
	r.Message = message
}

func (r *RedisFailoverStatus) SetFailedPhase(message string) {
	r.Phase = PhaseFail
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
	r.Phase = PhaseWaitingPodReady
	r.Message = message
}

func (r *RedisFailoverStatus) IsWaitingPodReady() bool {
	return r.Phase == PhaseWaitingPodReady
}

func (r *RedisFailoverStatus) IsPaused() bool {
	return r.Phase == PhasePaused
}

func (r *RedisFailoverStatus) SetReady(message string) {
	r.Phase = PhaseReady
	r.Message = message
}
