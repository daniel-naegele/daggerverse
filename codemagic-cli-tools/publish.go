package main

import (
	"context"
	"fmt"

	"dagger/codemagic-cli-tools/internal/dagger"
)

// Publish builds the multi-arch codemagic-cli-tools image and pushes it to a container registry as
// <registry>/codemagic-cli-tools:<CodemagicVersion>.
//
// Returns the published image reference (with digest).
func (m *CodemagicCliTools) Publish(
	ctx context.Context,
	// Registry/namespace to push to, e.g. "ghcr.io/daniel-naegele".
	registry string,
	// Registry username.
	username string,
	// Registry password or token.
	password *dagger.Secret,
	// Comma-separated list of platforms to build.
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
		variants[i] = m.Container(p)
	}

	tag := fmt.Sprintf("%s/codemagic-cli-tools:%s", registry, m.CodemagicVersion)
	ref, err := dag.Container().
		WithRegistryAuth(registry, username, password).
		Publish(ctx, tag, dagger.ContainerPublishOpts{PlatformVariants: variants})
	if err != nil {
		return "", fmt.Errorf("publish %s: %w", tag, err)
	}
	return ref, nil
}
