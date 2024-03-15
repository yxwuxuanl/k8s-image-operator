package controller

import (
	v1 "github.com/yxwuxuanl/k8s-image-operator/api/v1"
	"k8s.io/klog/v2"
	"regexp"
	"strings"
)

func rewriteImage(image string, rules []v1.RewriteRule) (string, bool) {
	image = normalizeImage(image)
	// build
	for _, rule := range rules {
		if rule.Registry != "" {
			if strings.HasPrefix(image, rule.Registry+"/") {
				return strings.Replace(image, rule.Registry, rule.Replacement, 1), true
			}
		}

		if rule.Regex != "" {
			re, err := regexp.Compile(rule.Regex)
			if err != nil {
				klog.ErrorS(err, "regexp compile failed", "regex", rule.Regex)
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
