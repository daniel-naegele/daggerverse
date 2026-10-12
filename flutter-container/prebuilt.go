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
// PrebuiltAndroid / PrebuiltEmulator pull. Published images always use the default
// Android API level (the one Flutter compiles against), so it is not part of the tag.
func (m *FlutterContainer) imageRef(registry, variant string) string {
	if registry == "" {
		registry = defaultRegistry
	}
	tag := m.FlutterVersion
	if variant != variantFlutter {
		tag += "-" + variant
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
