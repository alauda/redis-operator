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

package vc

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/alauda/redis-operator/internal/config"
	v1 "github.com/alauda/redis-operator/internal/vc/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrBundleVersionNotFound = errors.New("bundle version not found")
	ErrInvalidImage          = errors.New("invalid source image")
)

type BundleVersion v1.ImageVersion

func (bv *BundleVersion) IsLatest() bool {
	return bv.Labels[config.LatestKey] == "true"
}

func (bv *BundleVersion) GetComponent(compName string, displayVersion string) *v1.ComponentVersion {
	if bv == nil || compName == "" {
		return nil
	}

	for name, comp := range bv.Spec.Components {
		if name == compName {
			for _, version := range comp.ComponentVersions {
				if version.DisplayVersion == displayVersion || displayVersion == "" {
					return version.DeepCopy()
				}
			}
		}
	}
	return nil
}

func (bv *BundleVersion) GetImage(compName string, displayVersion string) (string, error) {
	if bv == nil || compName == "" {
		return "", errors.New("image version is nil")
	}
	for name, comp := range bv.Spec.Components {
		if name == compName {
			for _, version := range comp.ComponentVersions {
				if version.DisplayVersion == displayVersion || displayVersion == "" {
					return fmt.Sprintf("%s:%s", version.Image, version.Tag), nil
				}
			}
		}
	}
	return "", ErrBundleVersionNotFound
}

func (bv *BundleVersion) GetRedisImage(displayVersion string) (string, error) {
	if bv == nil {
		return "", ErrBundleVersionNotFound
	}
	return bv.GetImage("redis", displayVersion)
}

func (bv *BundleVersion) GetDefaultRedisImage() (string, error) {
	if bv == nil {
		return "", ErrBundleVersionNotFound
	}
	defaultVersion := config.DefaultRedisVersion

	return bv.GetRedisImage(defaultVersion)
}

func (bv *BundleVersion) GetActiveRedisImage(version string) (string, error) {
	if bv == nil {
		return "", ErrBundleVersionNotFound
	}
	return bv.GetImage("activeredis", version)
}

func (bv *BundleVersion) GetRedisToolsImage() (string, error) {
	if bv == nil {
		return "", ErrBundleVersionNotFound
	}
	return bv.GetImage("redis-tools", "")
}

func (bv *BundleVersion) GetExposePodImage() (string, error) {
	if bv == nil {
		return "", ErrBundleVersionNotFound
	}
	return bv.GetImage("expose-pod", "")
}

func (bv *BundleVersion) GetRedisExporterImage() (string, error) {
	if bv == nil {
		return "", ErrBundleVersionNotFound
	}
	return bv.GetImage("redis-exporter", "")
}

func (bv *BundleVersion) GetRedisProxyImage() (string, error) {
	if bv == nil {
		return "", ErrBundleVersionNotFound
	}
	return bv.GetImage("redis-proxy", "")
}

func (bv *BundleVersion) GetRedisShakeImage() (string, error) {
	if bv == nil {
		return "", ErrBundleVersionNotFound
	}
	return bv.GetImage("redis-shake", "")
}

func GetBundleVersion(ctx context.Context, cli client.Client, version string) (*BundleVersion, error) {
	if version == "" {
		return nil, nil
	}

	if ver, _ := semver.NewVersion(version); ver != nil {
		version = ver.String()
	}
	labels := map[string]string{
		config.InstanceTypeKey: config.CoreComponentName,
		config.CRVersionKey:    version,
	}

	ret := v1.ImageVersionList{}
	if err := cli.List(ctx, &ret, client.MatchingLabels(labels)); err != nil {
		return nil, err
	}
	if len(ret.Items) > 1 {
		return nil, fmt.Errorf("more than one image version for crVersion %s", version)
	}
	if len(ret.Items) == 0 {
		return nil, ErrBundleVersionNotFound
	}
	return (*BundleVersion)(&ret.Items[0]), nil
}

func GetLatestBundleVersion(ctx context.Context, cli client.Client) (*BundleVersion, error) {
	labels := map[string]string{
		config.InstanceTypeKey: config.CoreComponentName,
		config.LatestKey:       "true",
	}

	ret := v1.ImageVersionList{}
	if err := cli.List(ctx, &ret, client.MatchingLabels(labels)); err != nil {
		return nil, err
	}
	if len(ret.Items) == 0 {
		return nil, ErrBundleVersionNotFound
	}
	return (*BundleVersion)(&ret.Items[0]), nil
}

func IsBundleVersionUpgradeable(oldBV, newBV *BundleVersion, displayVersion string) bool {
	if oldBV == nil {
		return true
	}
	if newBV == nil {
		return false
	}

	compName := config.CoreComponentName

	oldVersion, err := semver.NewVersion(oldBV.Spec.CrVersion)
	if err != nil {
		return true
	}
	newVersion, err := semver.NewVersion(newBV.Spec.CrVersion)
	if err != nil {
		return false
	}
	if oldVersion.GreaterThan(newVersion) {
		return false
	}

	oldComp := oldBV.GetComponent(compName, displayVersion)
	newComp := newBV.GetComponent(compName, displayVersion)
	if oldComp == nil {
		return true
	}
	if newComp == nil {
		return false
	}
	if oldComp.Version == "" || newComp.Version == "" {
		return true
	}
	oldCompVer, err := semver.NewVersion(oldComp.Version)
	if err != nil {
		return true
	}
	newCompVer, err := semver.NewVersion(newComp.Version)
	if err != nil {
		return false
	}
	return !oldCompVer.GreaterThan(newCompVer)
}
