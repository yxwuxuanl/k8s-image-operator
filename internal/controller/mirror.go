package controller

import (
	"flag"
	"fmt"
	apiv1 "github.com/yxwuxuanl/k8s-image-operator/api/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"slices"
	"strings"
)

var (
	craneImage = flag.String("crane-image", "", "")
)

func buildMirrorPodTemplate(mirror *apiv1.Mirror) corev1.PodTemplateSpec {
	volumes := []corev1.Volume{
		{
			Name: "data",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					SizeLimit: mirror.Spec.SizeLimit,
				},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "data",
			MountPath: "/data",
		},
	}

	if mirror.Spec.DockerConfig != nil {
		volumes = append(volumes, corev1.Volume{
			Name: "dockerconfig",
			VolumeSource: corev1.VolumeSource{
				Secret: mirror.Spec.DockerConfig,
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "dockerconfig",
			MountPath: "/.docker/config.json",
			SubPath:   corev1.DockerConfigJsonKey,
		})
	}

	var verbose string
	if mirror.Spec.Verbose {
		verbose = "-v"
	}

	pullCmd := []string{
		"crane",
		"pull",
		"\\$SOURCE_${JOB_COMPLETION_INDEX}",
		"/data/image.tar.gz",
		"\\$PLATFORM_ARG_${JOB_COMPLETION_INDEX}",
		verbose,
	}

	pullContainer := corev1.Container{
		Name:            "pull",
		Image:           *craneImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		VolumeMounts:    volumeMounts,
		Env: []corev1.EnvVar{
			{
				Name:  "DOCKER_CONFIG",
				Value: "/.docker",
			},
		},
		Command: []string{"sh"},
		Args:    []string{"-c", ptr.To(buildMirrorScript(mirror.Spec.Images, pullCmd)).String()},
	}

	var pushCmd [][]string
	pushCmd = append(pushCmd, []string{
		"crane",
		"push",
		"/data/image.tar.gz",
		"\\$TARGET_${JOB_COMPLETION_INDEX}",
		"\\$PLATFORM_ARG_${JOB_COMPLETION_INDEX}",
		verbose,
	})

	if mirror.Spec.SetSourceAnnotation {
		pushCmd = append(pushCmd, []string{
			"crane",
			"mutate",
			"-a",
			"mirror-source=\\$SOURCE_${JOB_COMPLETION_INDEX}",
			"\\$TARGET_${JOB_COMPLETION_INDEX}",
			verbose,
		})
	}

	pushContainer := corev1.Container{
		Name:            "push",
		Image:           *craneImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		VolumeMounts:    volumeMounts,
		Command:         []string{"sh"},
		Env: []corev1.EnvVar{
			{
				Name:  "DOCKER_CONFIG",
				Value: "/.docker",
			},
		},
		Args: []string{
			"-c",
			ptr.To(buildMirrorScript(mirror.Spec.Images, pushCmd...)).String(),
		},
	}

	if proxy := mirror.Spec.HttpProxy; proxy != "" {
		httpProxyEnvs := []corev1.EnvVar{
			{
				Name:  "HTTP_PROXY",
				Value: proxy,
			},
			{
				Name:  "HTTPS_PROXY",
				Value: proxy,
			},
		}

		pullContainer.Env = append(pullContainer.Env, httpProxyEnvs...)
		if mirror.Spec.PushUseProxy {
			pushContainer.Env = append(pushContainer.Env, httpProxyEnvs...)
		}
	}

	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"image.lin2ur.cn/mirror": mirror.GetName(),
			},
		},
		Spec: corev1.PodSpec{
			Volumes:               volumes,
			InitContainers:        []corev1.Container{pullContainer},
			Containers:            []corev1.Container{pushContainer},
			NodeSelector:          mirror.Spec.NodeSelector,
			RestartPolicy:         corev1.RestartPolicyNever,
			ActiveDeadlineSeconds: mirror.Spec.ActiveDeadlineSeconds,
		},
	}
}

func buildMirrorJob(mirror *apiv1.Mirror) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mirror.GetName(),
			Namespace: mirror.GetNamespace(),
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:   ptr.To(int32(0)),
			Parallelism:    ptr.To(mirror.Spec.Parallelism),
			Completions:    ptr.To(int32(len(toMirrorImage(mirror.Spec.Images)))),
			CompletionMode: ptr.To(batchv1.IndexedCompletion),
			Template:       buildMirrorPodTemplate(mirror),
		},
	}
}

type mirrorImage struct {
	source, target string
	platforms      []string
}

func toMirrorImage(images []apiv1.MirrorImage) []mirrorImage {
	var result []mirrorImage
	for _, image := range images {
		if len(image.Tags) == 0 {
			result = append(result, mirrorImage{
				source:    image.Source,
				target:    image.Target,
				platforms: image.Platforms,
			})
			continue
		}

		for _, tag := range image.Tags {
			result = append(result, mirrorImage{
				source:    fmt.Sprintf("%s:%s", image.Source, tag),
				target:    fmt.Sprintf("%s:%s", image.Target, tag),
				platforms: image.Platforms,
			})
		}
	}

	return result
}

func buildMirrorScript(images []apiv1.MirrorImage, cmds ...[]string) strings.Builder {
	var bld strings.Builder

	for i, image := range toMirrorImage(images) {
		var platforms []string
		for _, platform := range image.platforms {
			platforms = append(platforms, "--platform", platform)
		}

		bld.WriteString(fmt.Sprintf(`PLATFORM_ARG_%d="%s"`+"\n", i, strings.Join(platforms, " ")))
		bld.WriteString(fmt.Sprintf("SOURCE_%d=%s\n", i, image.source))
		bld.WriteString(fmt.Sprintf("TARGET_%d=%s\n", i, image.target))
	}

	bld.WriteString("set -ex\n")

	for _, cmd := range cmds {
		cmd = slices.DeleteFunc(cmd, func(s string) bool {
			return len(s) == 0
		})

		bld.WriteString(`eval "` + strings.Join(cmd, " ") + `"` + "\n")
	}

	return bld
}
