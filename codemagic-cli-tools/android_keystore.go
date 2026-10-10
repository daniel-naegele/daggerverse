package main

import (
	"context"
	"path"
	"strconv"
	"strings"

	"dagger/codemagic-cli-tools/internal/dagger"
)

// CreateKeystore creates a new Android keystore with a single key (`android-keystore create`) and returns it.
//
// At least one issuer attribute (commonName, organization, ..., or distinguishedName) is required.
// Passwords must be at least 6 characters long.
func (m *CodemagicCliTools) CreateKeystore(
	// Keystore password.
	keystorePassword *dagger.Secret,
	// Alias of the key to create.
	keyAlias string,
	// Key password. Defaults to the keystore password.
	// +optional
	keyPassword *dagger.Secret,
	// Issuer common name (first and last name or company name).
	// +optional
	commonName string,
	// Issuer organization.
	// +optional
	organization string,
	// Issuer organizational unit.
	// +optional
	organizationUnit string,
	// Issuer locality (city, county, ...).
	// +optional
	locality string,
	// Issuer state or province.
	// +optional
	state string,
	// Issuer two-letter country code.
	// +optional
	country string,
	// Full issuer distinguished name, e.g. "CN=corp.example.com,L=Mountain View,C=US". Takes precedence
	// over the individual attributes.
	// +optional
	distinguishedName string,
	// Validity of the key in days.
	// +optional
	// +default=10000
	validity int,
) *dagger.File {
	if keyPassword == nil {
		keyPassword = keystorePassword
	}
	if validity <= 0 {
		validity = 10000
	}
	out := path.Join(workspaceDir, "keystore.jks")
	args := []string{
		"android-keystore", "create",
		"--keystore", out,
		"--keystore-pass", "@env:" + envKeystorePassword,
		"--alias", keyAlias,
		"--key-pass", "@env:" + envKeyPassword,
		"--validity", strconv.Itoa(validity),
	}
	for _, a := range []struct{ flag, value string }{
		{"--common-name", commonName},
		{"--organization", organization},
		{"--organization-unit", organizationUnit},
		{"--locality", locality},
		{"--state", state},
		{"--country", country},
		{"--distinguished-name", distinguishedName},
	} {
		if a.value != "" {
			args = append(args, a.flag, a.value)
		}
	}
	return m.base().
		WithSecretVariable(envKeystorePassword, keystorePassword).
		WithSecretVariable(envKeyPassword, keyPassword).
		WithExec(args).
		File(out)
}

// AndroidKeystore wraps the `android-keystore` tool for an existing keystore.
type AndroidKeystore struct {
	// +private
	Ctr *dagger.Container
	// +private
	KeyAlias string
}

// AndroidKeystore returns an `android-keystore` wrapper for the given keystore.
func (m *CodemagicCliTools) AndroidKeystore(
	// Keystore file (.jks / .keystore).
	keystore *dagger.File,
	// Keystore password.
	keystorePassword *dagger.Secret,
	// Alias of the key to inspect.
	keyAlias string,
) *AndroidKeystore {
	return &AndroidKeystore{
		Ctr: m.base().
			WithMountedFile(keystorePath, keystore, dagger.ContainerWithMountedFileOpts{Owner: user}).
			WithSecretVariable(envKeystorePassword, keystorePassword),
		KeyAlias: keyAlias,
	}
}

func (k *AndroidKeystore) args(action string, extra ...string) []string {
	return append([]string{
		"android-keystore", action,
		"--keystore", keystorePath,
		"--keystore-pass", "@env:" + envKeystorePassword,
		"--alias", k.KeyAlias,
	}, extra...)
}

// Verify checks that the keystore can be unlocked with the password and contains the alias
// (`android-keystore verify`). Fails otherwise.
func (k *AndroidKeystore) Verify(ctx context.Context) (string, error) {
	out, err := k.Ctr.WithExec(k.args("verify")).CombinedOutput(ctx)
	if err != nil {
		return "", wrapExecError(err)
	}
	return strings.TrimSpace(out), nil
}

// Certificate returns the certificate of the key as JSON (`android-keystore certificate --json`).
func (k *AndroidKeystore) Certificate(ctx context.Context) (string, error) {
	return output(ctx, k.Ctr, k.args("certificate", "--json"))
}
