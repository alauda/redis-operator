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

package util

// MergeLabels merges all the label maps received as argument into a single new label map.
func MergeMap(maps ...map[string]string) map[string]string {
	ret := map[string]string{}

	for _, item := range maps {
		for k, v := range item {
			ret[k] = v
		}
	}
	return ret
}

func MapKeys(m map[string]string) (ret []string) {
	for k := range m {
		ret = append(ret, k)
	}
	return
}
