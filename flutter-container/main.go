// Flutter container image builder.
//
// Provides functions for building Flutter Docker images (flutter, android, emulator stages).
// Images are pre-configured with sensible defaults; versions can be overridden via
// WithFlutterVersion and WithAndroidVersion.

package main

import (
	"context"
	"strings"

	"dagger/flutter-container/internal/dagger"
)

const (
	flutterHome  = "/opt/flutter"
	androidHome  = "/opt/android-sdk-linux"
	workspaceDir = "/workspace"
)

type FlutterContainer struct {
	// Flutter SDK version tag (e.g. "3.41.9").
	FlutterVersion string
	// Android platform API level for the emulator system image (e.g. "36").
	AndroidVersion string
	// Build number of the Android SDK command-line tools installed on linux/amd64
	// (commandlinetools-linux-<build>_latest.zip). Must ship the Android CLI
	// (`android`), i.e. cmdline-tools 22.0 (15859902) or newer.
	CmdlineToolsVersion string
}

func New() *FlutterContainer {
	return &FlutterContainer{
		FlutterVersion:      "3.47.7",
		AndroidVersion:      "36",
		CmdlineToolsVersion: "16111833", // cmdline-tools 23.0
	}
}

// WithFlutterVersion returns this module configured to use the given Flutter version.
func (m *FlutterContainer) WithFlutterVersion(version string) *FlutterContainer {
	m.FlutterVersion = version
	return m
}

// WithAndroidVersion returns this module configured to use the given Android API level.
func (m *FlutterContainer) WithAndroidVersion(version string) *FlutterContainer {
	m.AndroidVersion = version
	return m
}

// WithCmdlineToolsVersion returns this module configured to use the given Android
// SDK command-line tools build number (e.g. "16111833" for cmdline-tools 23.0) on
// linux/amd64. linux/arm64 always uses cmdline-tools 22.0, see androidSdkSetup.
func (m *FlutterContainer) WithCmdlineToolsVersion(version string) *FlutterContainer {
	m.CmdlineToolsVersion = version
	return m
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

// imageDigest returns the current digest of an image from a registry, or "" if not found.
func imageDigest(ctx context.Context, tag string) string {
	ref, err := dag.Container().From(tag).ImageRef(ctx)
	if err != nil {
		return ""
	}
	if idx := strings.Index(ref, "@"); idx >= 0 {
		return ref[idx+1:]
	}
	return ref
}
