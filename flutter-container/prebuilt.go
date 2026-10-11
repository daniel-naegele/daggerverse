package main

import (
	"fmt"

	"dagger/flutter-container/internal/dagger"
)

const (
	variantFlutter  = "flutter"
	variantAndroid  = "android"
	variantEmulator = "emulator"
)

// imageRef is the registry reference Publish uses for a variant, and PrebuiltFlutter /
// PrebuiltAndroid / PrebuiltEmulator pull. The android and emulator images bake in the
// Android API level, so a non-default level gets its own tags.
func (m *FlutterContainer) imageRef(registry, variant string) string {
	if registry == "" {
		registry = defaultRegistry
	}
	tag := m.FlutterVersion
	if variant != variantFlutter {
		tag += "-" + variant
		if m.AndroidVersion != defaultAndroidVersion {
			tag += "-api" + m.AndroidVersion
		}
	}
	return fmt.Sprintf("%s/flutter:%s", registry, tag)
}

// PrebuiltFlutter pulls the published Flutter SDK image for the configured version
// instead of building it.
func (m *FlutterContainer) PrebuiltFlutter(
	// Registry the images were published to.
	// +optional
	// +default="ghcr.io/daniel-naegele"
	registry string,
) *dagger.Container {
	return dag.Container().From(m.imageRef(registry, variantFlutter))
}

// PrebuiltAndroid pulls the published Flutter + Android SDK image for the configured
// Flutter version and Android API level instead of building it.
func (m *FlutterContainer) PrebuiltAndroid(
	// Registry the images were published to.
	// +optional
	// +default="ghcr.io/daniel-naegele"
	registry string,
) *dagger.Container {
	return dag.Container().From(m.imageRef(registry, variantAndroid))
}

// PrebuiltEmulator pulls the published emulator image (linux/amd64 only) for the
// configured Flutter version and Android API level instead of building it.
func (m *FlutterContainer) PrebuiltEmulator(
	// Registry the images were published to.
	// +optional
	// +default="ghcr.io/daniel-naegele"
	registry string,
) *dagger.Container {
	return dag.Container().From(m.imageRef(registry, variantEmulator))
}
