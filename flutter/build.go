package main

import (
	"context"
	"fmt"

	"dagger/flutter/internal/dagger"
)

const (
	signingSecretsDir = "/run/flutter-signing"
	signingTmpDir     = "/run/flutter-key-properties"
	buildOutDir       = "/out"
)

// buildScript optionally writes android/key.properties from mounted secrets,
// runs flutter build and copies the single resulting artifact to buildOutDir.
//
// key.properties is written to a tmpfs and symlinked into android/, so the
// passwords never end up in a container layer.
const buildScript = `set -eu
kind="$1"; shift

if [ -n "${FLUTTER_SIGNING:-}" ]; then
  esc() { sed 's/\\/\\\\/g' "$1"; }
  {
    printf 'storePassword=%s\n' "$(esc ` + signingSecretsDir + `/store-password)"
    printf 'keyPassword=%s\n' "$(esc ` + signingSecretsDir + `/key-password)"
    printf 'keyAlias=%s\n' "$FLUTTER_KEY_ALIAS"
    printf 'storeFile=%s\n' "` + signingSecretsDir + `/keystore"
  } > ` + signingTmpDir + `/key.properties
  ln -sf ` + signingTmpDir + `/key.properties android/key.properties
fi

flutter build "$kind" "$@"

case "$kind" in
  apk) dir=build/app/outputs/flutter-apk; pattern='*.apk' ;;
  appbundle) dir=build/app/outputs/bundle; pattern='*.aab' ;;
  *) echo "unsupported build kind: $kind" >&2; exit 1 ;;
esac

artifacts="$(find "$dir" -type f -name "$pattern")"
count="$(printf '%s\n' "$artifacts" | grep -c . || true)"
if [ "$count" -ne 1 ]; then
  echo "expected exactly one $pattern in $dir, found $count:" >&2
  printf '%s\n' "$artifacts" >&2
  exit 1
fi
mkdir -p ` + buildOutDir + `
cp "$artifacts" ` + buildOutDir + `/
`

// androidBuildOpts are the options shared by BuildApk and BuildAppBundle.
type androidBuildOpts struct {
	mode          string
	flavor        string
	target        string
	buildName     string
	buildNumber   string
	dartDefines   []string
	keystore      *dagger.File
	storePassword *dagger.Secret
	keyPassword   *dagger.Secret
	keyAlias      string
}

func (m *Flutter) buildAndroid(
	ctx context.Context,
	kind string,
	project *dagger.Directory,
	o androidBuildOpts,
	androidImage *dagger.Container,
) (*dagger.File, error) {
	switch o.mode {
	case "":
		o.mode = "release"
	case "debug", "profile", "release":
	default:
		return nil, fmt.Errorf("invalid mode %q: must be debug, profile or release", o.mode)
	}

	args := []string{kind, "--" + o.mode}
	if o.flavor != "" {
		args = append(args, "--flavor", o.flavor)
	}
	if o.target != "" {
		args = append(args, "--target", o.target)
	}
	if o.buildName != "" {
		args = append(args, "--build-name", o.buildName)
	}
	if o.buildNumber != "" {
		args = append(args, "--build-number", o.buildNumber)
	}
	args = append(args, dartDefineArgs(o.dartDefines)...)

	ctr := m.androidCtr(project, androidImage)

	if o.keystore != nil {
		if o.storePassword == nil || o.keyAlias == "" {
			return nil, fmt.Errorf("signing with a keystore requires storePassword and keyAlias")
		}
		keyPassword := o.keyPassword
		if keyPassword == nil {
			keyPassword = o.storePassword
		}
		ctr = ctr.
			WithEnvVariable("FLUTTER_SIGNING", "true").
			WithEnvVariable("FLUTTER_KEY_ALIAS", o.keyAlias).
			WithMountedFile(signingSecretsDir+"/keystore", o.keystore).
			WithMountedSecret(signingSecretsDir+"/store-password", o.storePassword).
			WithMountedSecret(signingSecretsDir+"/key-password", keyPassword).
			WithMountedTemp(signingTmpDir)
	}

	out := ctr.
		WithExec(append([]string{"sh", "-c", buildScript, "sh"}, args...)).
		Directory(buildOutDir)

	entries, err := out.Entries(ctx)
	if err != nil {
		return nil, err
	}
	if len(entries) != 1 {
		return nil, fmt.Errorf("expected exactly one build artifact, got %v", entries)
	}
	return out.File(entries[0]), nil
}

// BuildApk builds an Android APK (flutter build apk) and returns it.
//
// Release signing: pass keystore, storePassword, keyAlias (and keyPassword if it
// differs from storePassword). The module then writes android/key.properties
// with the keys storePassword, keyPassword, keyAlias and storeFile, following the
// Flutter deployment docs (https://docs.flutter.dev/deployment/android#configure-signing-in-gradle).
// The project's android/app/build.gradle(.kts) must read that file and use it
// for its release signingConfig. Without a keystore, the project's own signing
// configuration applies (typically debug signing).
func (m *Flutter) BuildApk(
	ctx context.Context,
	// Flutter project directory.
	// +ignore=["build", ".dart_tool", "android/.gradle", "ios/Pods"]
	project *dagger.Directory,
	// Build mode: debug, profile or release.
	// +optional
	// +default="release"
	mode string,
	// Product flavor to build.
	// +optional
	flavor string,
	// Main entry point (e.g. lib/main_prod.dart).
	// +optional
	target string,
	// Version name (e.g. 1.2.3), overrides pubspec.yaml.
	// +optional
	buildName string,
	// Version code (e.g. 42), overrides pubspec.yaml.
	// +optional
	buildNumber string,
	// Compile-time constants as KEY=VALUE, passed as --dart-define.
	// +optional
	dartDefines []string,
	// Upload/release keystore (JKS or PKCS12).
	// +optional
	keystore *dagger.File,
	// Keystore password.
	// +optional
	storePassword *dagger.Secret,
	// Key password (defaults to storePassword).
	// +optional
	keyPassword *dagger.Secret,
	// Key alias inside the keystore.
	// +optional
	keyAlias string,
	// Pre-built Android image to use instead of building one (e.g. ghcr.io/daniel-naegele/flutter:<version>-android).
	// +optional
	androidImage *dagger.Container,
) (*dagger.File, error) {
	return m.buildAndroid(ctx, "apk", project, androidBuildOpts{
		mode: mode, flavor: flavor, target: target,
		buildName: buildName, buildNumber: buildNumber, dartDefines: dartDefines,
		keystore: keystore, storePassword: storePassword, keyPassword: keyPassword, keyAlias: keyAlias,
	}, androidImage)
}

// BuildAppBundle builds an Android App Bundle (flutter build appbundle) and returns the .aab.
//
// Signing works exactly as for BuildApk (android/key.properties).
func (m *Flutter) BuildAppBundle(
	ctx context.Context,
	// Flutter project directory.
	// +ignore=["build", ".dart_tool", "android/.gradle", "ios/Pods"]
	project *dagger.Directory,
	// Build mode: debug, profile or release.
	// +optional
	// +default="release"
	mode string,
	// Product flavor to build.
	// +optional
	flavor string,
	// Main entry point (e.g. lib/main_prod.dart).
	// +optional
	target string,
	// Version name (e.g. 1.2.3), overrides pubspec.yaml.
	// +optional
	buildName string,
	// Version code (e.g. 42), overrides pubspec.yaml.
	// +optional
	buildNumber string,
	// Compile-time constants as KEY=VALUE, passed as --dart-define.
	// +optional
	dartDefines []string,
	// Upload/release keystore (JKS or PKCS12).
	// +optional
	keystore *dagger.File,
	// Keystore password.
	// +optional
	storePassword *dagger.Secret,
	// Key password (defaults to storePassword).
	// +optional
	keyPassword *dagger.Secret,
	// Key alias inside the keystore.
	// +optional
	keyAlias string,
	// Pre-built Android image to use instead of building one.
	// +optional
	androidImage *dagger.Container,
) (*dagger.File, error) {
	return m.buildAndroid(ctx, "appbundle", project, androidBuildOpts{
		mode: mode, flavor: flavor, target: target,
		buildName: buildName, buildNumber: buildNumber, dartDefines: dartDefines,
		keystore: keystore, storePassword: storePassword, keyPassword: keyPassword, keyAlias: keyAlias,
	}, androidImage)
}
