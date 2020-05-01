package util

import (
	"fmt"
	"strings"

	"github.com/opencontainers/go-digest"
)

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
