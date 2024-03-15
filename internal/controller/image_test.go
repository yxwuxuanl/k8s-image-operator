package controller

import (
	v1 "github.com/yxwuxuanl/k8s-image-operator/api/v1"
	"log"
	"net"
	"testing"
)

func TestNormalizeImage(t *testing.T) {
	tests := map[string]string{
		"busybox":            "docker.io/library/busybox:latest",
		"docker.io/busybox":  "docker.io/library/busybox:latest",
		"gcr.io/busybox":     "gcr.io/busybox:latest",
		"foo/busybox":        "docker.io/foo/busybox:latest",
		"gcr.io/busybox:1.0": "gcr.io/busybox:1.0",
	}

	for s, s2 := range tests {
		if normalizeImage(s) != s2 {
			t.Fatalf("test `%s` failed", s)
		}
	}
}

func TestRewriteImage(t *testing.T) {
	tests := []struct {
		raw, rewrite string
		rule         v1.RewriteRule
	}{
		{
			raw:     "docker.io/foo/busybox",
			rewrite: "mirror.docker.io/foo/busybox:latest",
			rule: v1.RewriteRule{
				Registry:    "docker.io",
				Replacement: "mirror.docker.io",
			},
		},
		{
			raw:     "docker.io/foo/busybox",
			rewrite: "docker.io/bar/busybox:latest",
			rule: v1.RewriteRule{
				Regex:       "docker.io/foo/(.+)$",
				Replacement: "docker.io/bar/$1",
			},
		},
	}

	for _, test := range tests {
		image, _ := rewriteImage(test.raw, []v1.RewriteRule{test.rule})
		if image != test.rewrite {
			t.Fatalf("test `%s` failed", test.raw)
		}
	}
}

func TestPort(t *testing.T) {
	host, p, err := net.SplitHostPort(":x")
	log.Printf("%s %s %v", host, p, err)
}
