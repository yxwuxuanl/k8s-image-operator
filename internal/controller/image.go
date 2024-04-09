package controller

import (
	"context"
	"fmt"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	containerregistryv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	v1 "github.com/yxwuxuanl/k8s-image-operator/api/v1"
	"k8s.io/utils/lru"
	"regexp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strings"
	"time"
)

func rewriteImage(image string, rules []v1.RewriteRule) (string, bool) {
	image = normalizeImage(image)

	for _, rule := range rules {
		if rule.Registry != "" {
			if strings.HasPrefix(image, rule.Registry+"/") {
				return strings.Replace(image, rule.Registry, rule.Replacement, 1), true
			}
		}

		if rule.Regex != "" {
			re, err := regexp.Compile(rule.Regex)
			if err != nil {
				ctrl.Log.Error(err, "failed to compile regex", "regex", rule.Regex)
				continue
			}

			if re.MatchString(image) {
				return re.ReplaceAllString(image, rule.Replacement), true
			}
		}
	}

	return "", false
}

func normalizeImage(image string) string {
	if !strings.Contains(image, ":") {
		image += ":latest"
	}

	if count := strings.Count(image, "/"); count < 2 {
		if count == 0 {
			return "docker.io/library/" + image
		}

		items := strings.SplitN(image, "/", 2)
		if items[0] == "docker.io" {
			return "docker.io/library/" + items[1]
		}

		if strings.Contains(items[0], ".") {
			return image
		}

		return "docker.io/" + image
	}

	return image
}

func getImageTag(image string) string {
	if i := strings.Index(image, ":"); i > -1 {
		return image[i:]
	}

	return "latest"
}

var imagePlatformCache = lru.New(100)

func getImagePlatform(ctx context.Context, image string) ([]*containerregistryv1.Platform, error) {
	if v, ok := imagePlatformCache.Get(image); ok {
		return v.([]*containerregistryv1.Platform), nil
	}

	opts := crane.GetOptions(crane.WithContext(ctx))

	ref, err := name.ParseReference(image, opts.Name...)
	if err != nil {
		return nil, err
	}

	start := time.Now()

	descriptor, err := remote.Get(ref, opts.Remote...)
	if err != nil {
		return nil, fmt.Errorf("failed to get remote descriptor: %w", err)
	}

	var platforms []*containerregistryv1.Platform

	if imageIndex, err := descriptor.ImageIndex(); err == nil {
		indexManifest, err := imageIndex.IndexManifest()
		if err != nil {
			return nil, fmt.Errorf("failed to get index manifest: %w", err)
		}

		for _, manifest := range indexManifest.Manifests {
			if manifest.Platform.OS != "" && manifest.Platform.OS != "unknown" {
				platforms = append(platforms, manifest.Platform)
			}
		}
	} else {
		image, err := descriptor.Image()
		if err != nil {
			return nil, fmt.Errorf("failed to get image: %w", err)
		}

		configFile, err := image.ConfigFile()
		if err != nil {
			return nil, fmt.Errorf("failed to get config file: %w", err)
		}

		platforms = append(platforms, configFile.Platform())
	}

	log.FromContext(ctx).Info(
		"get image platform",
		"image", image,
		"platforms", platforms,
		"duration", time.Since(start),
	)

	imagePlatformCache.Add(image, platforms)
	return platforms, nil
}
