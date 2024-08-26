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

// Code generated by mockery v2.45.0. DO NOT EDIT.

package mocks

import (
	context "context"

	mock "github.com/stretchr/testify/mock"
	v1 "k8s.io/api/core/v1"
)

// ConfigMap is an autogenerated mock type for the ConfigMap type
type ConfigMap struct {
	mock.Mock
}

// CreateConfigMap provides a mock function with given fields: ctx, namespace, configMap
func (_m *ConfigMap) CreateConfigMap(ctx context.Context, namespace string, configMap *v1.ConfigMap) error {
	ret := _m.Called(ctx, namespace, configMap)

	if len(ret) == 0 {
		panic("no return value specified for CreateConfigMap")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, *v1.ConfigMap) error); ok {
		r0 = rf(ctx, namespace, configMap)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// CreateIfNotExistsConfigMap provides a mock function with given fields: ctx, namespace, configMap
func (_m *ConfigMap) CreateIfNotExistsConfigMap(ctx context.Context, namespace string, configMap *v1.ConfigMap) error {
	ret := _m.Called(ctx, namespace, configMap)

	if len(ret) == 0 {
		panic("no return value specified for CreateIfNotExistsConfigMap")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, *v1.ConfigMap) error); ok {
		r0 = rf(ctx, namespace, configMap)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// CreateOrUpdateConfigMap provides a mock function with given fields: ctx, namespace, np
func (_m *ConfigMap) CreateOrUpdateConfigMap(ctx context.Context, namespace string, np *v1.ConfigMap) error {
	ret := _m.Called(ctx, namespace, np)

	if len(ret) == 0 {
		panic("no return value specified for CreateOrUpdateConfigMap")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, *v1.ConfigMap) error); ok {
		r0 = rf(ctx, namespace, np)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// DeleteConfigMap provides a mock function with given fields: ctx, namespace, name
func (_m *ConfigMap) DeleteConfigMap(ctx context.Context, namespace string, name string) error {
	ret := _m.Called(ctx, namespace, name)

	if len(ret) == 0 {
		panic("no return value specified for DeleteConfigMap")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string) error); ok {
		r0 = rf(ctx, namespace, name)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// GetConfigMap provides a mock function with given fields: ctx, namespace, name
func (_m *ConfigMap) GetConfigMap(ctx context.Context, namespace string, name string) (*v1.ConfigMap, error) {
	ret := _m.Called(ctx, namespace, name)

	if len(ret) == 0 {
		panic("no return value specified for GetConfigMap")
	}

	var r0 *v1.ConfigMap
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string) (*v1.ConfigMap, error)); ok {
		return rf(ctx, namespace, name)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, string) *v1.ConfigMap); ok {
		r0 = rf(ctx, namespace, name)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.ConfigMap)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, string) error); ok {
		r1 = rf(ctx, namespace, name)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ListConfigMaps provides a mock function with given fields: ctx, namespace
func (_m *ConfigMap) ListConfigMaps(ctx context.Context, namespace string) (*v1.ConfigMapList, error) {
	ret := _m.Called(ctx, namespace)

	if len(ret) == 0 {
		panic("no return value specified for ListConfigMaps")
	}

	var r0 *v1.ConfigMapList
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (*v1.ConfigMapList, error)); ok {
		return rf(ctx, namespace)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) *v1.ConfigMapList); ok {
		r0 = rf(ctx, namespace)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*v1.ConfigMapList)
		}
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, namespace)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateConfigMap provides a mock function with given fields: ctx, namespace, configMap
func (_m *ConfigMap) UpdateConfigMap(ctx context.Context, namespace string, configMap *v1.ConfigMap) error {
	ret := _m.Called(ctx, namespace, configMap)

	if len(ret) == 0 {
		panic("no return value specified for UpdateConfigMap")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, *v1.ConfigMap) error); ok {
		r0 = rf(ctx, namespace, configMap)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// UpdateIfConfigMapChanged provides a mock function with given fields: ctx, newConfigmap
func (_m *ConfigMap) UpdateIfConfigMapChanged(ctx context.Context, newConfigmap *v1.ConfigMap) error {
	ret := _m.Called(ctx, newConfigmap)

	if len(ret) == 0 {
		panic("no return value specified for UpdateIfConfigMapChanged")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, *v1.ConfigMap) error); ok {
		r0 = rf(ctx, newConfigmap)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// NewConfigMap creates a new instance of ConfigMap. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewConfigMap(t interface {
	mock.TestingT
	Cleanup(func())
}) *ConfigMap {
	mock := &ConfigMap{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}