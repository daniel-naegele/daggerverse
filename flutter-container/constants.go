package main

import "dagger/flutter-container/internal/dagger"

const (
	PlatformAMD64 dagger.Platform = "linux/amd64"
	PlatformARM64 dagger.Platform = "linux/arm64"
)

// legacyCmdlineToolsVersion is the build number of cmdline-tools 22.0, the last
// release with the Java-based sdkmanager. Used on linux/arm64, where the Android
// CLI (and thus the sdkmanager shim of cmdline-tools 23.0+) cannot run.
const legacyCmdlineToolsVersion = "15859902"

const (
	abiAMD64 = "x86_64"
	abiARM64 = "arm64-v8a"
)

// emulatorABI maps a container platform to the Android emulator ABI name.
func emulatorABI(platform dagger.Platform) string {
	if platform == PlatformARM64 {
		return abiARM64
	}
	return abiAMD64
}
