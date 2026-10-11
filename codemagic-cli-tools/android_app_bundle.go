package main

import (
	"context"
	"fmt"
	"path"
	"strings"

	"dagger/codemagic-cli-tools/internal/dagger"
)

const (
	envKeystorePassword = "CM_KEYSTORE_PASSWORD"
	envKeyPassword      = "CM_KEY_PASSWORD"
	keystorePath        = secretsDir + "/keystore.jks"
)

// AndroidAppBundle wraps the `android-app-bundle` tool for a single Android App Bundle (.aab).
type AndroidAppBundle struct {
	// +private
	Ctr *dagger.Container
	// +private
	BundlePath string
}

// AndroidAppBundle returns an `android-app-bundle` wrapper operating on the given .aab file.
func (m *CodemagicCliTools) AndroidAppBundle(
	ctx context.Context,
	// Android App Bundle (.aab).
	bundle *dagger.File,
) (*AndroidAppBundle, error) {
	name, err := bundle.Name(ctx)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(name, ".aab") {
		name += ".aab"
	}
	p := path.Join(workspaceDir, name)
	return &AndroidAppBundle{
		Ctr:        m.base().WithFile(p, bundle, dagger.ContainerWithFileOpts{Owner: user}),
		BundlePath: p,
	}, nil
}

// Dump prints a target of the bundle (`android-app-bundle dump`), e.g. the manifest.
//
// Use xpath to extract a single attribute, e.g. "/manifest/@android:versionCode" or "/manifest/@package".
func (b *AndroidAppBundle) Dump(
	ctx context.Context,
	// What to dump: manifest, resources, config or runtime-enabled-sdk-config.
	// +optional
	// +default="manifest"
	target string,
	// XML path to a specific attribute in the target (manifest only).
	// +optional
	xpath string,
	// Module to dump the manifest of (defaults to "base").
	// +optional
	module string,
	// Resource name or ID to look up (resources only).
	// +optional
	resource string,
	// Also print resource values (resources only).
	// +optional
	values bool,
) (string, error) {
	if target == "" {
		target = "manifest"
	}
	args := []string{"android-app-bundle", "dump", target, "--bundle", b.BundlePath}
	if xpath != "" {
		args = append(args, "--xpath", xpath)
	}
	if module != "" {
		args = append(args, "--module", module)
	}
	if resource != "" {
		args = append(args, "--resource", resource)
	}
	if values {
		args = append(args, "--values")
	}
	return output(ctx, b.Ctr, args)
}

// Validate verifies the bundle is valid and returns information about it (`android-app-bundle validate`).
func (b *AndroidAppBundle) Validate(ctx context.Context) (string, error) {
	return output(ctx, b.Ctr, []string{"android-app-bundle", "validate", "--bundle", b.BundlePath})
}

// IsSigned reports whether the bundle is signed (`android-app-bundle is-signed`).
func (b *AndroidAppBundle) IsSigned(ctx context.Context) (bool, error) {
	ctr := b.Ctr.WithExec(
		[]string{"android-app-bundle", "is-signed", "--bundle", b.BundlePath},
		dagger.ContainerWithExecOpts{Expect: dagger.ReturnTypeAny},
	)
	code, err := ctr.ExitCode(ctx)
	if err != nil {
		return false, err
	}
	if code == 0 {
		return true, nil
	}
	out, err := ctr.CombinedOutput(ctx)
	if err != nil {
		return false, err
	}
	if strings.Contains(out, "is not signed") {
		return false, nil
	}
	return false, fmt.Errorf("android-app-bundle is-signed failed with exit code %d:\n%s", code, out)
}

// Sign signs the bundle with the given keystore using jarsigner (`android-app-bundle sign`) and returns
// the signed bundle.
func (b *AndroidAppBundle) Sign(
	// Keystore file (.jks / .keystore).
	keystore *dagger.File,
	// Keystore password.
	keystorePassword *dagger.Secret,
	// Alias of the signing key.
	keyAlias string,
	// Key password. Defaults to the keystore password.
	// +optional
	keyPassword *dagger.Secret,
) (*dagger.File, error) {
	ctr, signArgs, err := withSigning(b.Ctr, keystore, keystorePassword, keyAlias, keyPassword)
	if err != nil {
		return nil, err
	}
	args := append([]string{"android-app-bundle", "sign", "--bundle", b.BundlePath}, signArgs...)
	return ctr.WithExec(args).File(b.BundlePath), nil
}

// BuildApks generates an APK set archive (.apks) from the bundle (`android-app-bundle build-apks`).
//
// Signing is optional; without a keystore bundletool signs with a debug key. Requires linux/amd64
// (bundletool's bundled aapt2 is x86_64 only).
func (b *AndroidAppBundle) BuildApks(
	// Keystore used to sign the generated APKs.
	// +optional
	keystore *dagger.File,
	// Keystore password (required with keystore).
	// +optional
	keystorePassword *dagger.Secret,
	// Alias of the signing key (required with keystore).
	// +optional
	keyAlias string,
	// Key password. Defaults to the keystore password.
	// +optional
	keyPassword *dagger.Secret,
	// Build a single universal APK inside the set (--mode universal).
	// +optional
	universal bool,
) (*dagger.File, error) {
	ctr, signArgs, err := withOptionalSigning(b.Ctr, keystore, keystorePassword, keyAlias, keyPassword)
	if err != nil {
		return nil, err
	}
	args := append([]string{"android-app-bundle", "build-apks", "--bundle", b.BundlePath}, signArgs...)
	if universal {
		args = append(args, "--mode", "universal")
	}
	return ctr.WithExec(args).File(strings.TrimSuffix(b.BundlePath, ".aab") + ".apks"), nil
}

// BuildUniversalApk generates a single universal APK from the bundle
// (`android-app-bundle build-universal-apk`).
//
// Signing is optional; without a keystore bundletool signs with a debug key. Requires linux/amd64
// (bundletool's bundled aapt2 is x86_64 only).
func (b *AndroidAppBundle) BuildUniversalApk(
	// Keystore used to sign the generated APK.
	// +optional
	keystore *dagger.File,
	// Keystore password (required with keystore).
	// +optional
	keystorePassword *dagger.Secret,
	// Alias of the signing key (required with keystore).
	// +optional
	keyAlias string,
	// Key password. Defaults to the keystore password.
	// +optional
	keyPassword *dagger.Secret,
) (*dagger.File, error) {
	ctr, signArgs, err := withOptionalSigning(b.Ctr, keystore, keystorePassword, keyAlias, keyPassword)
	if err != nil {
		return nil, err
	}
	args := append([]string{"android-app-bundle", "build-universal-apk", "--bundle", b.BundlePath}, signArgs...)
	return ctr.WithExec(args).File(strings.TrimSuffix(b.BundlePath, ".aab") + "-universal.apk"), nil
}

// BundletoolInfo returns the bundletool and Java paths/version used by codemagic as JSON
// (`android-app-bundle bundletool info --json`).
func (m *CodemagicCliTools) BundletoolInfo(ctx context.Context) (string, error) {
	return output(ctx, m.base(), []string{"android-app-bundle", "bundletool", "info", "--json"})
}

// withOptionalSigning is withSigning, but returns the container unchanged when no keystore is given.
func withOptionalSigning(
	ctr *dagger.Container,
	keystore *dagger.File,
	keystorePassword *dagger.Secret,
	keyAlias string,
	keyPassword *dagger.Secret,
) (*dagger.Container, []string, error) {
	if keystore == nil {
		if keystorePassword != nil || keyAlias != "" || keyPassword != nil {
			return nil, nil, fmt.Errorf("keystorePassword, keyAlias and keyPassword require keystore")
		}
		return ctr, nil, nil
	}
	return withSigning(ctr, keystore, keystorePassword, keyAlias, keyPassword)
}

// withSigning mounts the keystore and exposes the passwords as secret environment variables, returning the
// container and the android-app-bundle signing arguments referencing them via "@env:".
func withSigning(
	ctr *dagger.Container,
	keystore *dagger.File,
	keystorePassword *dagger.Secret,
	keyAlias string,
	keyPassword *dagger.Secret,
) (*dagger.Container, []string, error) {
	if keystore == nil || keystorePassword == nil || keyAlias == "" {
		return nil, nil, fmt.Errorf("signing requires keystore, keystorePassword and keyAlias")
	}
	if keyPassword == nil {
		keyPassword = keystorePassword
	}
	ctr = ctr.
		WithMountedFile(keystorePath, keystore, dagger.ContainerWithMountedFileOpts{Owner: user}).
		WithSecretVariable(envKeystorePassword, keystorePassword).
		WithSecretVariable(envKeyPassword, keyPassword)
	return ctr, []string{
		"--ks", keystorePath,
		"--ks-pass", "@env:" + envKeystorePassword,
		"--ks-key-alias", keyAlias,
		"--key-pass", "@env:" + envKeyPassword,
	}, nil
}
