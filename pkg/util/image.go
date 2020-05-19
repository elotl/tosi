package util

import (
	"fmt"
	"strings"

	"github.com/opencontainers/go-digest"
)

// This parses a full image name with the registry name as the first part.
func ParseFullImage(fullImage string) (string, string) {
	registry := ""
	repo := ""
	parts := strings.Split(fullImage, "/")
	if len(parts) == 1 {
		// Old docker-style image name, e.g. "alpine".
		registry = "https://registry-1.docker.io/"
		repo = "library/" + parts[0]
		return registry, repo
	}
	if parts[0] == "k8s.gcr.io" {
		// k8s.gcr.io is an alias used by GCR.
		registry = "https://gcr.io/"
		repo = "google_containers/" + strings.Join(parts[1:], "/")
		return registry, repo
	}
	if strings.Contains(parts[0], ".") {
		registry = strings.ToLower(parts[0])
		if !strings.HasPrefix(registry, "http://") &&
			!strings.HasPrefix(registry, "https://") {
			registry = "https://" + registry + "/"
		}
		repo = strings.Join(parts[1:], "/")
		return registry, repo
	}
	registry = "https://registry-1.docker.io/"
	repo = strings.Join(parts, "/")
	return registry, repo
}

// This parses an image name with an optional tag, without the registry name.
func ParseImageSpec(image string) (string, string, error) {
	repo := image
	dgest := ""
	reference := "latest"
	if strings.Contains(repo, "@") { // Exact hash for the image.
		parts := strings.Split(repo, "@")
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid image spec %q", image)
		}
		repo = parts[0]
		dgest = parts[1]
		d, err := digest.Parse(dgest)
		if err != nil {
			return "", "", fmt.Errorf("invalid image hash in %q", image)
		}
		reference = d.String()
	}
	if strings.Contains(repo, ":") {
		parts := strings.Split(repo, ":")
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid image spec %q", image)
		}
		repo = parts[0]
		if dgest == "" {
			// Only use the tag if no digest is specified.
			reference = parts[1]
		}
	}
	return repo, reference, nil
}
