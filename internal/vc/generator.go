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
	"crypto/md5" // nolint:gosec
	stderrs "errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"

	clusterv1alpha1 "github.com/alauda/redis-operator/api/cluster/v1alpha1"
	"github.com/alauda/redis-operator/api/core"
	databasesv1 "github.com/alauda/redis-operator/api/databases/v1"
	middlewarev1 "github.com/alauda/redis-operator/api/middleware/v1"
	"github.com/alauda/redis-operator/internal/config"
	vc "github.com/alauda/redis-operator/internal/vc/v1"

	"github.com/Masterminds/semver/v3"
	"github.com/go-logr/logr"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GenerateRedisBundleImageVersion(version string) string {
	return fmt.Sprintf("redis-artifact-%s", version)
}

func ParseImageAndTag(image string) (string, string, error) {
	fields := strings.Split(image, ":")
	if len(fields) != 2 {
		return "", "", fmt.Errorf("invalid image %s", image)
	}
	return fields[0], fields[1], nil
}

func GetImageNameAndTagFromEnv(name string) (string, string, error) {
	val := os.Getenv(name)
	if val == "" {
		return "", "", fmt.Errorf("env %s is empty", name)
	}
	return ParseImageAndTag(val)
}

func appendComponentVersion(comp *vc.Component, imagetag, version, displayVersion string) error {
	if imagetag == "" {
		return nil
	}
	if image, tag, err := ParseImageAndTag(imagetag); err != nil {
		return err
	} else {
		if version != "" {
			version = fmt.Sprintf("%s.0", version)
		}
		if !slices.ContainsFunc(comp.ComponentVersions, func(vc vc.ComponentVersion) bool {
			return vc.Tag == tag
		}) {
			comp.ComponentVersions = append(comp.ComponentVersions, vc.ComponentVersion{
				Image:          image,
				Tag:            tag,
				Version:        version,
				DisplayVersion: displayVersion,
			})
		}
	}
	return nil
}

// GenerateOldImageVersion generate the old version of ImageVersion
// this function only fetch three images: redis,redis-exporter,redis-tools
// images of redisbackup,redisproxy,redisshake will be ignored
func GenerateOldImageVersion(ctx context.Context, cli client.Client, namespace string, logger logr.Logger) (*vc.ImageVersion, error) {
	var (
		operatorVersion string
		listOptions     = client.ListOptions{Namespace: namespace, Limit: 16}
		comps           = map[string]*vc.Component{
			"redis":          {CoreComponent: true},
			"activeredis":    {},
			"redis-exporter": {},
			"redis-tools":    {},
			"redis-shake":    {},
			"redis-proxy":    {},
			"expose-pod":     {},
		}
	)

	for {
		var ret middlewarev1.RedisList
		if err := cli.List(ctx, &ret, &listOptions); err != nil {
			logger.Error(err, "failed to list redis")
			return nil, fmt.Errorf("failed to list redis: %w", err)
		}

		for _, obj := range ret.Items {
			item := obj.DeepCopy()
			if ov := item.Annotations[config.OperatorVersionAnnotation]; strings.HasPrefix(ov, "v3.14.") ||
				strings.HasPrefix(ov, "v3.15.") ||
				strings.HasPrefix(ov, "v3.16.") {
				if operatorVersion == "" {
					operatorVersion = ov
				}
			} else if ov := item.Status.UpgradeStatus.CRVersion; strings.HasPrefix(ov, "3.14.") ||
				strings.HasPrefix(ov, "3.15.") ||
				strings.HasPrefix(ov, "3.16.") {
				if operatorVersion == "" {
					operatorVersion = ov
				}
			} else {
				continue
			}

			version := item.Spec.Version
			stsName := fmt.Sprintf("rfr-%s", item.Name)
			switch item.Spec.Arch {
			case core.RedisSentinel:
				var inst databasesv1.RedisFailover
				if err := cli.Get(ctx, client.ObjectKeyFromObject(item), &inst); err != nil {
					logger.Error(err, "failed to get redisfailover", "redisfailover", client.ObjectKeyFromObject(item))
					if !errors.IsNotFound(err) {
						return nil, fmt.Errorf("failed to get redisfailover: %w", err)
					}
					continue
				}

				if inst.Spec.EnableActiveRedis {
					if err := appendComponentVersion(comps["activeredis"], inst.Spec.Redis.Image, version, version); err != nil {
						logger.Error(err, "failed to load activeredis image", "container", inst.Spec.Redis.Image)
					}
				} else {
					if err := appendComponentVersion(comps["redis"], inst.Spec.Redis.Image, version, version); err != nil {
						logger.Error(err, "failed to load redis image", "container", inst.Spec.Redis.Image)
					}
				}
				if inst.Spec.Redis.Exporter.Image != "" {
					if err := appendComponentVersion(comps["redis-exporter"], inst.Spec.Redis.Exporter.Image, "", ""); err != nil {
						logger.Error(err, "failed to load redis-exporter image", "container", inst.Spec.Redis.Exporter.Image)
					}
				}
				stsName = fmt.Sprintf("rfr-%s", item.Name)
			case core.RedisCluster:
				var inst clusterv1alpha1.DistributedRedisCluster
				if err := cli.Get(ctx, client.ObjectKeyFromObject(item), &inst); err != nil {
					logger.Error(err, "failed to get rediscluster", "rediscluster", client.ObjectKeyFromObject(item))
					if !errors.IsNotFound(err) {
						return nil, fmt.Errorf("failed to get rediscluster: %w", err)
					}
					continue
				}

				if inst.Spec.EnableActiveRedis {
					if err := appendComponentVersion(comps["activeredis"], inst.Spec.Image, version, version); err != nil {
						logger.Error(err, "failed to load activeredis image", "container", inst.Spec.Image)
					}
				} else {
					if err := appendComponentVersion(comps["redis"], inst.Spec.Image, version, version); err != nil {
						logger.Error(err, "failed to load redis image", "container", inst.Spec.Image)
					}
				}
				if inst.Spec.Monitor != nil && inst.Spec.Monitor.Image != "" {
					if err := appendComponentVersion(comps["redis-exporter"], inst.Spec.Monitor.Image, "", ""); err != nil {
						logger.Error(err, "failed to load redis-exporter image", "container", inst.Spec.Monitor.Image)
					}
				}
				stsName = fmt.Sprintf("drc-%s-0", item.Name)
			}

			if stsName != "" {
				// get statefulset of redisfailover
				var sts appv1.StatefulSet
				if err := cli.Get(ctx, client.ObjectKey{Namespace: item.Namespace, Name: stsName}, &sts); err != nil {
					logger.Error(err, "failed to list statefulset", "instance", client.ObjectKeyFromObject(item))
					if !errors.IsNotFound(err) {
						return nil, fmt.Errorf("failed to list statefulset: %w", err)
					}
					continue
				}
				for _, container := range append(append([]corev1.Container{}, sts.Spec.Template.Spec.InitContainers...), sts.Spec.Template.Spec.Containers...) {
					if container.Image == "" {
						continue
					}
					if strings.Contains(container.Image, "redis_exporter") {
						if err := appendComponentVersion(comps["redis-exporter"], container.Image, "", ""); err != nil {
							logger.Error(err, "failed to load redis-exporter image", "container", container.Image)
						}
					} else if strings.Contains(container.Image, "redis-tools") {
						if err := appendComponentVersion(comps["redis-tools"], container.Image, "", ""); err != nil {
							logger.Error(err, "failed to load redis-tools image", "container", container.Image)
						}
					} else if strings.Contains(container.Image, "expose_pod") {
						// sentinel 3.14 used only
						if err := appendComponentVersion(comps["expose-pod"], container.Image, "", ""); err != nil {
							logger.Error(err, "failed to load expose-pod image", "container", container.Image)
						}
					}
				}
			}
		}

		if ret.Continue == "" {
			break
		}
		listOptions.Continue = ret.Continue
	}

	if operatorVersion == "" {
		return nil, ErrBundleVersionNotFound
	}

	ver, err := semver.NewVersion(operatorVersion)
	if err != nil {
		logger.Error(err, "failed to parse operator version", "version", operatorVersion)
		return nil, fmt.Errorf("failed to parse operator version: %w", err)
	}

	iv := vc.ImageVersion{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				config.InstanceTypeKey: config.CoreComponentName,
				config.CRVersionKey:    ver.String(),
			},
			Name: GenerateRedisBundleImageVersion(ver.String()),
		},
		Spec: vc.ImageVersionSpec{
			CrVersion:  ver.String(),
			Components: map[string]vc.Component{},
		},
	}
	for name, comp := range comps {
		logger.Info("component", "name", name, "comp", comp, "version", operatorVersion)
		if len(comp.ComponentVersions) == 0 {
			continue
		}
		iv.Spec.Components[name] = *comp
	}
	return &iv, nil
}

func GenerateCurrentImageVersion(ctx context.Context, cli client.Client, logger logr.Logger) (*vc.ImageVersion, error) {
	operatorVersion := config.GetOperatorVersion()
	if operatorVersion == "" {
		logger.Error(fmt.Errorf("operator version is empty"), "operator version is empty")
		return nil, fmt.Errorf("operator version is empty")
	}
	ver, err := semver.NewVersion(operatorVersion)
	if err != nil {
		logger.Error(err, "failed to parse operator version", "operator version", operatorVersion)
		return nil, fmt.Errorf("failed to parse operator version: %w", err)
	}

	redisImage5, redisImage5Tag, err := GetImageNameAndTagFromEnv("REDIS_VERSION_5_IMAGE")
	if err != nil {
		logger.Error(err, "failed to get redis image 5")
		return nil, fmt.Errorf("failed to get redis image 5: %w", err)
	}
	redisImage5Version := os.Getenv("REDIS_VERSION_5_VERSION")

	redisImage6, redisImage6Tag, err := GetImageNameAndTagFromEnv("REDIS_VERSION_6_IMAGE")
	if err != nil {
		logger.Error(err, "failed to get redis image 6")
		return nil, fmt.Errorf("failed to get redis image 6: %w", err)
	}
	redisImage6Version := os.Getenv("REDIS_VERSION_6_VERSION")

	redisImage7, redisImage7Tag, err := GetImageNameAndTagFromEnv("REDIS_VERSION_7_2_IMAGE")
	if err != nil {
		logger.Error(err, "failed to get redis image 7.2")
		return nil, fmt.Errorf("failed to get redis image 7.2: %w", err)
	}
	redisImage7Version := os.Getenv("REDIS_VERSION_7_2_VERSION")

	toolsImage, toolsImageTag, err := GetImageNameAndTagFromEnv("REDIS_TOOLS_IMAGE")
	if err != nil {
		logger.Error(err, "failed to get redis tools image")
		return nil, fmt.Errorf("failed to get redis tools image: %w", err)
	}
	exporterImage, exporterImageTag, err := GetImageNameAndTagFromEnv("DEFAULT_EXPORTER_IMAGE")
	if err != nil {
		logger.Error(err, "failed to get redis exporter image")
		return nil, fmt.Errorf("failed to get redis exporter image: %w", err)
	}

	iv := vc.ImageVersion{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				config.InstanceTypeKey: config.CoreComponentName,
				config.CRVersionKey:    ver.String(),
			},
			Name: GenerateRedisBundleImageVersion(ver.String()),
		},
		Spec: vc.ImageVersionSpec{
			CrVersion: ver.String(),
			Components: map[string]vc.Component{
				"redis": {
					CoreComponent: true,
					ComponentVersions: []vc.ComponentVersion{
						{Image: redisImage5, Tag: redisImage5Tag, Version: redisImage5Version, DisplayVersion: "5.0"},
						{Image: redisImage6, Tag: redisImage6Tag, Version: redisImage6Version, DisplayVersion: "6.0"},
						{Image: redisImage7, Tag: redisImage7Tag, Version: redisImage7Version, DisplayVersion: "7.2"},
					},
				},
				"redis-exporter": {
					ComponentVersions: []vc.ComponentVersion{
						{Image: exporterImage, Tag: exporterImageTag},
					},
				},
				"redis-tools": {
					ComponentVersions: []vc.ComponentVersion{
						{Image: toolsImage, Tag: toolsImageTag},
					},
				},
			},
		},
	}
	return &iv, nil
}

func CreateOrUpdateImageVersion(ctx context.Context, cli client.Client, iv *vc.ImageVersion) error {
	var rawdata []string
	for _, comp := range iv.Spec.Components {
		for _, version := range comp.ComponentVersions {
			rawdata = append(rawdata, fmt.Sprintf("%s:%s", version.Image, version.Tag))
		}
	}
	sort.Strings(rawdata)

	hashsum := fmt.Sprintf("%x", md5.Sum([]byte(strings.Join(rawdata, ",")))) // nolint:gosec
	if iv.Labels == nil {
		iv.Labels = map[string]string{}
	}
	iv.Labels[config.CRVersionSHAKey] = hashsum

	var oldIV vc.ImageVersion
	if err := cli.Get(ctx, client.ObjectKeyFromObject(iv), &oldIV); errors.IsNotFound(err) {
		if err := cli.Create(ctx, iv); errors.IsAlreadyExists(err) {
			return nil
		} else if err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if !reflect.DeepEqual(oldIV.Spec, iv.Spec) || !reflect.DeepEqual(oldIV.Labels, iv.Labels) {
		return retry.RetryOnConflict(retry.DefaultRetry, func() error {
			var oldIV vc.ImageVersion
			if err := cli.Get(ctx, client.ObjectKeyFromObject(iv), &oldIV); err != nil {
				return err
			}
			iv.ResourceVersion = oldIV.ResourceVersion
			return cli.Update(ctx, iv)
		})
	}
	return nil
}

func CleanImageVersions(ctx context.Context, cli client.Client, logger logr.Logger) error {
	var ret vc.ImageVersionList
	if err := cli.List(ctx, &ret, client.MatchingLabels{config.InstanceTypeKey: config.CoreComponentName}); err != nil {
		logger.Error(err, "failed to list image version")
		return fmt.Errorf("failed to list image version: %w", err)
	}
	var (
		minorVersions []string
		ivGroup       = map[string][]*vc.ImageVersion{}
		latest        *vc.ImageVersion
		latestVersion *semver.Version
	)
	for _, item := range ret.Items {
		ver, err := semver.NewVersion(item.Spec.CrVersion)
		if err != nil {
			logger.Error(err, "failed to parse version, deleted", "version", item.Spec.CrVersion)
			if err := cli.Delete(ctx, item.DeepCopy()); err != nil {
				logger.Error(err, "failed to delete image version", "image version", item)
				return fmt.Errorf("failed to delete image version: %w", err)
			}
			continue
		}
		key := fmt.Sprintf("%d.%d.0", ver.Major(), ver.Minor())
		ivGroup[key] = append(ivGroup[key], item.DeepCopy())
		if !slices.Contains(minorVersions, key) {
			minorVersions = append(minorVersions, key)
		}
		if latestVersion == nil || ver.GreaterThan(latestVersion) {
			latestVersion = ver
			latest = item.DeepCopy()
		}
	}

	sort.SliceStable(minorVersions, func(i, j int) bool {
		left := semver.MustParse(minorVersions[i])
		right := semver.MustParse(minorVersions[j])
		return left.GreaterThan(right)
	})

	for key, items := range ivGroup {
		sort.SliceStable(items, func(i, j int) bool {
			iv := semver.MustParse(ret.Items[i].Spec.CrVersion)
			jv := semver.MustParse(ret.Items[j].Spec.CrVersion)
			return iv.GreaterThan(jv)
		})
		ivGroup[key] = items
	}

	for i, key := range minorVersions {
		if i >= 3 {
			for _, item := range ivGroup[key] {
				if err := cli.Delete(ctx, item); err != nil {
					logger.Error(err, "failed to delete image version", "image version", item)
					return fmt.Errorf("failed to delete image version: %w", err)
				}
			}
			continue
		}
		for j, obj := range ivGroup[key] {
			item := obj.DeepCopy()
			// only keep 3 newest versions for each minor version
			if j >= 3 {
				if err := cli.Delete(ctx, item); err != nil {
					logger.Error(err, "failed to delete image version", "image version", item)
					return fmt.Errorf("failed to delete image version: %w", err)
				}
				continue
			}
			if _, ok := item.Labels[config.LatestKey]; ok && item.Name != latest.Name {
				delete(item.Labels, config.LatestKey)
				if err := CreateOrUpdateImageVersion(ctx, cli, item); err != nil {
					logger.Error(err, "failed to patch image version", "image version", item)
					return fmt.Errorf("failed to patch image version: %w", err)
				}
			}
		}
	}
	if latest.Labels[config.LatestKey] != "true" {
		if latest.Labels == nil {
			latest.Labels = map[string]string{}
		}
		latest.Labels[config.LatestKey] = "true"
		return CreateOrUpdateImageVersion(ctx, cli, latest)
	}
	return nil
}

func mergeImageVersion(oldIV, newIV *vc.ImageVersion) *vc.ImageVersion {
	for name, oldComp := range oldIV.Spec.Components {
		if comp, ok := newIV.Spec.Components[name]; ok {
			for _, version := range oldComp.ComponentVersions {
				if !slices.ContainsFunc(comp.ComponentVersions, func(vc vc.ComponentVersion) bool {
					return vc.Tag == version.Tag
				}) {
					comp.ComponentVersions = append(comp.ComponentVersions, version)
				}
			}
			newIV.Spec.Components[name] = comp
		} else {
			newIV.Spec.Components[name] = oldComp
		}
	}
	return newIV
}

func InstallOldImageVersion(ctx context.Context, cli client.Client, namespace string, logger logr.Logger) error {
	iv, err := GenerateOldImageVersion(ctx, cli, namespace, logger)
	if err != nil {
		if stderrs.Is(err, ErrBundleVersionNotFound) {
			return nil
		}
		return err
	}
	if iv == nil {
		return nil
	}

	logger.Info("install old image version", "version", iv.Name)
	var oldIV vc.ImageVersion
	if err := cli.Get(ctx, client.ObjectKeyFromObject(iv), &oldIV); errors.IsNotFound(err) {
		return CreateOrUpdateImageVersion(ctx, cli, iv)
	} else if err != nil {
		return err
	}
	iv = mergeImageVersion(&oldIV, iv)
	return CreateOrUpdateImageVersion(ctx, cli, iv)
}

func InstallCurrentImageVersion(ctx context.Context, cli client.Client, logger logr.Logger) error {
	iv, err := GenerateCurrentImageVersion(ctx, cli, logger)
	if err != nil {
		return err
	}
	return CreateOrUpdateImageVersion(ctx, cli, iv)
}
