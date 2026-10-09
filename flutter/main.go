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
)

type Flutter struct {
	// Flutter SDK version tag (e.g. "3.47.6").
	FlutterVersion string
	// Android platform API level for the emulator system image (e.g. "36").
	AndroidVersion string
}

func New() *Flutter {
	return &Flutter{
		FlutterVersion: "3.47.6",
		AndroidVersion: "36",
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

// images returns the flutter-container module configured with this module's versions.
func (m *Flutter) images() *dagger.FlutterContainer {
	return dag.FlutterContainer().
		WithFlutterVersion(m.FlutterVersion).
		WithAndroidVersion(m.AndroidVersion)
}

// FlutterImage returns the Flutter SDK image used by Test, Analyze and Format.
func (m *Flutter) FlutterImage() *dagger.Container {
	return m.images().Flutter()
}

// AndroidImage returns the Flutter + Android SDK image used by BuildApk and BuildAppBundle.
func (m *Flutter) AndroidImage() *dagger.Container {
	return m.images().Android()
}

// EmulatorImage returns the emulator image (AVD for AndroidVersion) used by IntegrationTest.
func (m *Flutter) EmulatorImage() *dagger.Container {
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

// withJunitReport installs the junitreport package used to convert Dart JSON
// test reports into JUnit XML.
func withJunitReport(ctr *dagger.Container) *dagger.Container {
	return ctr.WithExec([]string{"dart", "pub", "global", "activate", "junitreport"})
}

// dartDefineArgs turns KEY=VALUE pairs into --dart-define flags.
func dartDefineArgs(defines []string) []string {
	args := make([]string, 0, len(defines))
	for _, d := range defines {
		args = append(args, "--dart-define="+d)
	}
	return args
}
