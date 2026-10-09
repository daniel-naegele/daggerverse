package main

import (
	"context"
	"fmt"
	"strings"

	"dagger/sops/internal/dagger"
)

// Publish builds the multi-arch sops image and pushes it to a container registry.
//
// Publishes two tags:
//   - <registry>/sops:<sopsVersion>
//   - <registry>/sops:latest
//
// Returns the image reference (with digest) of the version tag.
func (m *Sops) Publish(
	ctx context.Context,
	registry string,
	username string,
	password *dagger.Secret,
	// +optional
	// +default="linux/amd64,linux/arm64"
	platforms string,
) (string, error) {
	if platforms == "" {
		platforms = "linux/amd64,linux/arm64"
	}
	platformList := parsePlatforms(platforms)

	variants := make([]*dagger.Container, len(platformList))
	for i, p := range platformList {
		ctr, err := m.Container(ctx, p)
		if err != nil {
			return "", fmt.Errorf("build image for %s: %w", p, err)
		}
		variants[i] = ctr
	}

	var ref string
	for _, tag := range []string{m.SopsVersion, "latest"} {
		r, err := dag.Container().
			WithRegistryAuth(registry, username, password).
			Publish(ctx, fmt.Sprintf("%s/sops:%s", registry, tag), dagger.ContainerPublishOpts{
				PlatformVariants: variants,
			})
		if err != nil {
			return "", fmt.Errorf("publish sops:%s: %w", tag, err)
		}
		if ref == "" {
			ref = r
		}
	}
	return ref, nil
}

// parsePlatforms splits a comma-separated platform string into a slice.
func parsePlatforms(s string) []dagger.Platform {
	parts := strings.Split(s, ",")
	out := make([]dagger.Platform, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, dagger.Platform(p))
		}
	}
	return out
}
