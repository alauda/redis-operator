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

package config

import (
	"errors"
	"os"
	"strings"

	"github.com/Masterminds/semver/v3"
)

var (
	ErrImageNotFound = errors.New("image not found")
	ErrInvalidImage  = errors.New("invalid source image")
)

var (
	minVerionOfRedisTLSSupported, _ = semver.NewVersion("6.0-AAA")
)

var redisVersionEnvs = []string{
	"REDIS_VERSION_4_IMAGE",
	"REDIS_VERSION_5_IMAGE",
	"REDIS_VERSION_6_IMAGE",
	"REDIS_VERSION_7_2_IMAGE",
}

func GetRedisVersion(image string) string {
	if image == "" {
		image = GetDefaultRedisImage()
	}
	if idx := strings.Index(image, ":"); idx != -1 {
		if dashIdx := strings.Index(image[idx+1:], "-"); dashIdx != -1 {
			return image[idx+1 : idx+1+dashIdx]
		}
		return image[idx+1:]
	}
	return ""
}

func DebugEnabled() bool {
	return os.Getenv("DEBUG") != ""
}

func GetWatchNamespace() string {
	ns := os.Getenv("WATCH_NAMESPACE")
	if ns == "" {
		ns = "operators"
	}
	return ns
}

func GetPodName() string {
	return os.Getenv("POD_NAME")
}

func GetPodUid() string {
	return os.Getenv("POD_UID")
}

func Getenv(name string, defaults ...string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}

	for _, v := range defaults {
		if v != "" {
			return defaults[0]
		}
	}
	return ""
}

func GetDefaultRedisImage() string {
	return Getenv("DEFAULT_REDIS_IMAGE")
}

func GetRedisImage(src string) (string, error) {
	if src == "" {
		if val := GetDefaultRedisImage(); val == "" {
			return "", ErrImageNotFound
		} else {
			return val, nil
		}
	}

	fields := strings.SplitN(src, ":", 2)
	if len(fields) != 2 {
		return "", ErrInvalidImage
	}
	return GetRedisImageByVersion(fields[1])
}

func GetRedisImageByVersion(version string) (string, error) {
	wantedVersion, err := semver.NewVersion(version)
	if err != nil {
		return "", ErrInvalidImage
	}

	var lastErr error
	for _, name := range redisVersionEnvs {
		val := os.Getenv(name)
		if val == "" {
			continue
		}
		fields := strings.SplitN(val, ":", 2)
		if len(fields) != 2 {
			continue
		}

		ver, err := semver.NewVersion(fields[1])
		if err != nil {
			lastErr = err
			continue
		}
		if ver.Major() == wantedVersion.Major() && ver.Minor() == wantedVersion.Minor() {
			return val, nil
		}
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", ErrImageNotFound
}

func GetDefaultBackupImage() string {
	return Getenv("REDIS_TOOLS_IMAGE")
}

func GetRedisToolsImage() string {
	return Getenv("REDIS_TOOLS_IMAGE")
}

func GetDefaultExporterImage() string {
	return Getenv("REDIS_EXPORTER_IMAGE", Getenv("DEFAULT_EXPORTER_IMAGE", "build-harbor.alauda.cn/middleware/oliver006/redis_exporter:v1.3.5-alpine"))
}
