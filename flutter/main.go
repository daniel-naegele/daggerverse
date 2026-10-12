// Flutter CI tasks: tests, static analysis, formatting, Android builds and
// integration tests on an Android emulator.
//
// Images are built by the flutter-container module (Flutter SDK, Android SDK and
// emulator stages). Every function accepts an optional pre-built image to skip the
// local image build, e.g. one pulled from ghcr.io/daniel-naegele/flutter.

package main

import (
	"dagger/flutter/internal/dagger"
)

const (
	flutterHome  = "/opt/flutter"
	androidHome  = "/opt/android-sdk-linux"
	workspaceDir = "/workspace"
	reportsDir   = "/reports"
	pubCacheDir  = "/cache/pub"
	gradleHome   = "/cache/gradle"
	// pubGlobalDir holds globally activated Dart tools baked into the images.
	pubGlobalDir = "/opt/pub-global"
	// defaultAndroidVersion is the API level of the published images.
	defaultAndroidVersion = "36"
)

type Flutter struct {
	// Flutter SDK version tag (e.g. "3.47.6").
	FlutterVersion string
	// Android platform API level for the emulator system image (e.g. "36").
	AndroidVersion string
	// Registry to pull prebuilt images from (e.g. "ghcr.io/daniel-naegele"). When
	// empty the images are built from scratch. Prebuilt images only exist for the
	// default Android API level, so a different AndroidVersion builds locally.
	Registry string
}

func New() *Flutter {
	return &Flutter{
		FlutterVersion: "3.47.7",
		AndroidVersion: defaultAndroidVersion,
		Registry:       "ghcr.io/daniel-naegele",
	}
}

// WithFlutterVersion returns this module configured to use the given Flutter version.
func (m *Flutter) WithFlutterVersion(version string) *Flutter {
	m.FlutterVersion = version
	return m
}

// WithAndroidVersion returns this module configured to use the given Android API level.
func (m *Flutter) WithAndroidVersion(version string) *Flutter {
	m.AndroidVersion = version
	return m
}

// WithPrebuiltImages returns this module configured to pull the published
// flutter-container images from a registry instead of building them. The images for
// the configured Flutter version and Android API level must have been published
// (flutter-container's publish function).
func (m *Flutter) WithPrebuiltImages(
	// Registry the images were published to.
	// +optional
	// +default="ghcr.io/daniel-naegele"
	registry string,
) *Flutter {
	if registry == "" {
		registry = "ghcr.io/daniel-naegele"
	}
	m.Registry = registry
	return m
}

// WithoutPrebuiltImages returns this module configured to build the images from
// scratch instead of pulling published ones.
func (m *Flutter) WithoutPrebuiltImages() *Flutter {
	m.Registry = ""
	return m
}

// prebuilt reports whether published images are used: a registry is set and the
// Android API level is the one they were built with.
func (m *Flutter) prebuilt() bool {
	return m.Registry != "" && m.AndroidVersion == defaultAndroidVersion
}

// images returns the flutter-container module configured with this module's versions.
func (m *Flutter) images() *dagger.FlutterContainer {
	return dag.FlutterContainer().
		WithFlutterVersion(m.FlutterVersion).
		WithAndroidVersion(m.AndroidVersion)
}

// FlutterImage returns the Flutter SDK image used by Test, Analyze and Format.
func (m *Flutter) FlutterImage() *dagger.Container {
	if m.prebuilt() {
		return m.images().PrebuiltFlutter(dagger.FlutterContainerPrebuiltFlutterOpts{Registry: m.Registry})
	}
	return m.images().Flutter()
}

// AndroidImage returns the Flutter + Android SDK image used by BuildApk and BuildAppBundle.
func (m *Flutter) AndroidImage() *dagger.Container {
	if m.prebuilt() {
		return m.images().PrebuiltAndroid(dagger.FlutterContainerPrebuiltAndroidOpts{Registry: m.Registry})
	}
	return m.images().Android()
}

// EmulatorImage returns the emulator image (AVD for AndroidVersion) used by IntegrationTest.
func (m *Flutter) EmulatorImage() *dagger.Container {
	if m.prebuilt() {
		return m.images().PrebuiltEmulator(dagger.FlutterContainerPrebuiltEmulatorOpts{Registry: m.Registry})
	}
	return m.images().Emulator()
}

// flutterCtr returns the Flutter SDK image (or the override) prepared for project.
func (m *Flutter) flutterCtr(project *dagger.Directory, image *dagger.Container) *dagger.Container {
	if image == nil {
		image = m.FlutterImage()
	}
	return withProject(image, project)
}

// androidCtr returns the Android SDK image (or the override) prepared for project.
func (m *Flutter) androidCtr(project *dagger.Directory, image *dagger.Container) *dagger.Container {
	if image == nil {
		image = m.AndroidImage()
	}
	return withGradleCache(withProject(image, project))
}

// emulatorCtr returns the emulator image (or the override) prepared for project.
func (m *Flutter) emulatorCtr(project *dagger.Directory, image *dagger.Container) *dagger.Container {
	if image == nil {
		image = m.EmulatorImage()
	}
	return withGradleCache(withProject(image, project))
}

// withProject mounts the pub cache, copies the project into the workspace and
// resolves its dependencies.
func withProject(ctr *dagger.Container, project *dagger.Directory) *dagger.Container {
	return ctr.
		WithEnvVariable("PUB_CACHE", pubCacheDir).
		WithEnvVariable("CI", "true").
		WithMountedCache(pubCacheDir, dag.CacheVolume("flutter-pub-cache")).
		WithExec([]string{"flutter", "config", "--no-analytics", "--no-cli-animations"}).
		WithDirectory(workspaceDir, project).
		WithWorkdir(workspaceDir).
		WithExec([]string{"flutter", "pub", "get"})
}

// withGradleCache mounts a Gradle user home cache and disables the Gradle daemon.
// The cache is locked so concurrent builds do not fight over Gradle's lock files
// across container boundaries.
func withGradleCache(ctr *dagger.Container) *dagger.Container {
	return ctr.
		WithEnvVariable("GRADLE_USER_HOME", gradleHome).
		WithEnvVariable("GRADLE_OPTS", "-Dorg.gradle.daemon=false -Dorg.gradle.vfs.watch=false").
		WithMountedCache(gradleHome, dag.CacheVolume("flutter-gradle-cache"), dagger.ContainerWithMountedCacheOpts{
			Sharing: dagger.CacheSharingModeLocked,
		})
}

// junitCmd converts a Dart JSON test report into JUnit XML. junitreport is baked
// into the flutter-container images (PUB_CACHE=pubGlobalDir, separate from the
// project pub cache); withJunitReport installs it for images that lack it.
const junitCmd = "PUB_CACHE=" + pubGlobalDir + " dart pub global run junitreport:tojunit"

// withJunitReport makes sure junitreport is available, downloading it only into
// images that do not ship it.
func withJunitReport(ctr *dagger.Container) *dagger.Container {
	return ctr.WithExec([]string{"sh", "-c",
		"[ -d " + pubGlobalDir + "/global_packages/junitreport ] || PUB_CACHE=" + pubGlobalDir + " dart pub global activate junitreport"})
}

// dartDefineArgs turns KEY=VALUE pairs into --dart-define flags.
func dartDefineArgs(defines []string) []string {
	args := make([]string, 0, len(defines))
	for _, d := range defines {
		args = append(args, "--dart-define="+d)
	}
	return args
}
