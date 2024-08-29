package failoverbuilder

import (
	"os"
	"testing"

	v1 "github.com/alauda/redis-operator/api/databases/v1"
	"github.com/alauda/redis-operator/pkg/types/user"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

func TestCreateRedisExporterContainer(t *testing.T) {
	tests := []struct {
		name     string
		rf       *v1.RedisFailover
		opUser   *user.User
		expected corev1.Container
	}{
		{
			name: "Basic test",
			rf: &v1.RedisFailover{
				Spec: v1.RedisFailoverSpec{
					Redis: v1.RedisSettings{
						Exporter: v1.RedisExporter{
							Enabled: true,
							Image:   "redis-exporter:latest",
						},
					},
				},
			},
			opUser: &user.User{
				Name: "default",
				Password: &user.Password{
					SecretName: "secret-name",
				},
			},
			expected: corev1.Container{
				Name:            exporterContainerName,
				Command:         []string{"/bin/sh", "-c", "if [ -f /account/password ]; then echo \"{\\\"${REDIS_ADDR}\\\": \\\"$(cat /account/password)\\\"}\" > /tmp/passwords.json; fi /redis_exporter"},
				Image:           "redis-exporter:latest",
				ImagePullPolicy: corev1.PullAlways,
				Env: []corev1.EnvVar{
					{Name: "REDIS_ALIAS", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}}},
					{Name: "REDIS_USER", Value: ""},
					{Name: "REDIS_PASSWORD_FILE", Value: "/tmp/passwords.json"},
					{Name: "REDIS_ADDR", Value: "redis://local.inject:6379"},
				},
				Ports: []corev1.ContainerPort{
					{Name: "metrics", ContainerPort: 9121, Protocol: corev1.ProtocolTCP},
				},
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("200Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
				},
				SecurityContext: &corev1.SecurityContext{
					ReadOnlyRootFilesystem: pointer.Bool(true),
				},
				VolumeMounts: []corev1.VolumeMount{
					{Name: redisAuthName, MountPath: "/account"},
					{Name: RedisExporterTempVolumeName, MountPath: "/tmp"},
				},
			},
		},
		{
			name: "With TLS",
			rf: &v1.RedisFailover{
				Spec: v1.RedisFailoverSpec{
					Redis: v1.RedisSettings{
						EnableTLS: true,
						Exporter: v1.RedisExporter{
							Enabled: true,
							Image:   "redis-exporter:latest",
						},
					},
				},
			},
			opUser: &user.User{
				Name: "default",
				Password: &user.Password{
					SecretName: "secret-name",
				},
			},
			expected: corev1.Container{
				Name:            exporterContainerName,
				Command:         []string{"/bin/sh", "-c", "if [ -f /account/password ]; then echo \"{\\\"${REDIS_ADDR}\\\": \\\"$(cat /account/password)\\\"}\" > /tmp/passwords.json; fi /redis_exporter"},
				Image:           "redis-exporter:latest",
				ImagePullPolicy: corev1.PullAlways,
				Env: []corev1.EnvVar{
					{Name: "REDIS_ALIAS", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}}},
					{Name: "REDIS_USER", Value: ""},
					{Name: "REDIS_PASSWORD_FILE", Value: "/tmp/passwords.json"},
					{Name: "REDIS_EXPORTER_TLS_CLIENT_KEY_FILE", Value: "/tls/tls.key"},
					{Name: "REDIS_EXPORTER_TLS_CLIENT_CERT_FILE", Value: "/tls/tls.crt"},
					{Name: "REDIS_EXPORTER_TLS_CA_CERT_FILE", Value: "/tls/ca.crt"},
					{Name: "REDIS_EXPORTER_SKIP_TLS_VERIFICATION", Value: "true"},
					{Name: "REDIS_ADDR", Value: "rediss://local.inject:6379"},
				},
				Ports: []corev1.ContainerPort{
					{Name: "metrics", ContainerPort: 9121, Protocol: corev1.ProtocolTCP},
				},
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("200Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
				},
				SecurityContext: &corev1.SecurityContext{
					ReadOnlyRootFilesystem: pointer.Bool(true),
				},
				VolumeMounts: []corev1.VolumeMount{
					{Name: redisAuthName, MountPath: "/account"},
					{Name: RedisExporterTempVolumeName, MountPath: "/tmp"},
					{Name: RedisTLSVolumeName, MountPath: "/tls"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := createRedisExporterContainer(tt.rf, tt.opUser)
			assert.Equal(t, tt.expected.Name, container.Name)
			assert.Equal(t, tt.expected.Image, container.Image)
			assert.Equal(t, tt.expected.ImagePullPolicy, container.ImagePullPolicy)
			assert.Equal(t, tt.expected.Ports, container.Ports)
			assert.ElementsMatch(t, tt.expected.Env, container.Env)
			assert.Equal(t, tt.expected.Resources, container.Resources)
			assert.Equal(t, tt.expected.SecurityContext, container.SecurityContext)
			assert.ElementsMatch(t, tt.expected.VolumeMounts, container.VolumeMounts)
		})
	}
}

func TestCreateStandaloneInitContainer(t *testing.T) {
	tests := []struct {
		name     string
		rf       *v1.RedisFailover
		expected corev1.Container
	}{
		{
			name: "Default settings",
			rf: &v1.RedisFailover{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationStandaloneLoadFilePath: "dump.rdb",
					},
				},
			},
			expected: corev1.Container{
				Name:            "standalone-pod",
				Image:           "redis-tools:latest",
				ImagePullPolicy: corev1.PullAlways,
				VolumeMounts: []corev1.VolumeMount{
					{Name: "redis-data", MountPath: "/data"},
					{Name: "redis-standalone", MountPath: "/tmp-data"},
				},
				Command: []string{"sh", "-c", "if [ -e '/data/dump.rdb' ]; then\necho 'redis storage file exist,skip' \nelse \necho 'copy redis storage file' && cp /tmp-data/dump.rdb /data/dump.rdb && chown 999:1000 /data/dump.rdb && chmod 644 /data/dump.rdb \nfi"},
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
				},
				SecurityContext: &corev1.SecurityContext{
					Privileged: pointer.Bool(false),
				},
			},
		},
		{
			name: "Hostpath storage",
			rf: &v1.RedisFailover{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationStandaloneLoadFilePath: "appendonly.aof",
						AnnotationStandaloneInitStorage:  "hostpath",
					},
				},
			},
			expected: corev1.Container{
				Name:            "standalone-pod",
				Image:           "redis-tools:latest",
				ImagePullPolicy: corev1.PullAlways,
				VolumeMounts: []corev1.VolumeMount{
					{Name: "redis-data", MountPath: "/data"},
					{Name: "redis-standalone", MountPath: "/tmp-data"},
				},
				Command: []string{"sh", "-c", "if [ -e '/data/appendonly.aof' ]; then\necho 'redis storage file exist,skip' \nelse \necho 'copy redis storage file' && cp /tmp-data/appendonly.aof /data/appendonly.aof && chown 999:1000 /data/appendonly.aof && chmod 644 /data/appendonly.aof \nfi"},
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
				},
				SecurityContext: &corev1.SecurityContext{
					Privileged:   pointer.Bool(false),
					RunAsGroup:   pointer.Int64(0),
					RunAsUser:    pointer.Int64(0),
					RunAsNonRoot: pointer.Bool(false),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("REDIS_TOOLS_IMAGE", "redis-tools:latest")

			container := createStandaloneInitContainer(tt.rf)
			assert.Equal(t, tt.expected.Name, container.Name)
			assert.Equal(t, tt.expected.Image, container.Image)
			assert.Equal(t, tt.expected.ImagePullPolicy, container.ImagePullPolicy)
			assert.ElementsMatch(t, tt.expected.VolumeMounts, container.VolumeMounts)
			assert.Equal(t, tt.expected.Command, container.Command)
			assert.Equal(t, tt.expected.Resources, container.Resources)
			assert.Equal(t, tt.expected.SecurityContext, container.SecurityContext)
		})
	}
}
