package main

import (
	"context"
	"path"
	"strings"

	"dagger/codemagic-cli-tools/internal/dagger"
)

// UniversalApk generates a universal APK from an App Bundle using the `universal-apk generate` tool.
//
// Deprecated upstream in favour of `android-app-bundle build-universal-apk`; prefer
// AndroidAppBundle.BuildUniversalApk. Kept for parity with the CLI. Requires linux/amd64.
func (m *CodemagicCliTools) UniversalApk(
	ctx context.Context,
	// Android App Bundle (.aab).
	bundle *dagger.File,
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
	name, err := bundle.Name(ctx)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSuffix(name, ".aab") + ".aab"
	p := path.Join(workspaceDir, name)
	ctr := m.base().WithFile(p, bundle, dagger.ContainerWithFileOpts{Owner: user})
	// universal-apk accepts the same --ks/--ks-pass/--ks-key-alias/--key-pass flags; the passwords are
	// resolved through android-app-bundle's password types, which understand "@env:" references.
	ctr, signArgs, err := withOptionalSigning(ctr, keystore, keystorePassword, keyAlias, keyPassword)
	if err != nil {
		return nil, err
	}
	args := append([]string{"universal-apk", "generate", "--pattern", p}, signArgs...)
	return ctr.WithExec(args).File(strings.TrimSuffix(p, ".aab") + "-universal.apk"), nil
}
