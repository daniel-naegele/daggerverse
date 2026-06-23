package main

import (
	"context"
	"fmt"

	"dagger/flutter-container/internal/dagger"
)

// Publish builds multi-arch Flutter images and pushes them to a container registry.
//
// Publishes three tags:
//   - flutter:<version>          — flutter base (ubuntu + Flutter SDK)
//   - flutter:<version>-android  — android base (flutter + Android SDK + NDK)
//   - flutter:<version>-emulator — emulator (android + emulator + AVD, per-platform ABI)
//
// Returns the digest of the published flutter base image.
func (m *FlutterContainer) Publish(
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

	flutterVariants := make([]*dagger.Container, len(platformList))
	androidVariants := make([]*dagger.Container, len(platformList))
	// ONLY build linux/amd64
	emulatorVariants := make([]*dagger.Container, 1)

	for i, p := range platformList {
		flutterVariants[i] = flutterBase(p, m.FlutterVersion).
			WithRegistryAuth(registry, username, password)
		androidVariants[i] = androidBase(p, m.FlutterVersion).
			WithRegistryAuth(registry, username, password)
	}
	emu, err := m.Emulator(ctx, PlatformAMD64)
	if err != nil {
		return "", fmt.Errorf("build emulator for %s: %w", PlatformAMD64, err)
	}
	emulatorVariants[0] = emu.WithRegistryAuth(registry, username, password)

	flutterTag := fmt.Sprintf("%s/flutter:%s", registry, m.FlutterVersion)
	androidTag := fmt.Sprintf("%s/flutter:%s-android", registry, m.FlutterVersion)
	emulatorTag := fmt.Sprintf("%s/flutter:%s-emulator", registry, m.FlutterVersion)

	flutterDigest, err := dag.Container().
		WithRegistryAuth(registry, username, password).
		Publish(ctx, flutterTag, dagger.ContainerPublishOpts{
			PlatformVariants: flutterVariants,
		})
	if err != nil {
		return "", fmt.Errorf("publish flutter image: %w", err)
	}

	if _, err = dag.Container().
		WithRegistryAuth(registry, username, password).
		Publish(ctx, androidTag, dagger.ContainerPublishOpts{
			PlatformVariants: androidVariants,
		}); err != nil {
		return "", fmt.Errorf("publish android image: %w", err)
	}

	if _, err = dag.Container().
		WithRegistryAuth(registry, username, password).
		Publish(ctx, emulatorTag, dagger.ContainerPublishOpts{
			PlatformVariants: emulatorVariants,
		}); err != nil {
		return "", fmt.Errorf("publish emulator image: %w", err)
	}

	return flutterDigest, nil
}
