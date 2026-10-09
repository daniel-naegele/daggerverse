package main

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"

	"dagger/sops/internal/dagger"
)

// wrapperScript assembles SOPS_AGE_KEY from the mounted age key and/or SSH key
// (converted with ssh-to-age) and then runs sops with the given arguments.
// Keys are only ever read from secret mounts, never written to image layers.
const wrapperScript = `#!/bin/sh
set -eu
keys=""
if [ -f "` + ageKeyPath + `" ]; then
  keys="$(cat "` + ageKeyPath + `")"
fi
if [ -f "` + sshKeyPath + `" ]; then
  keys="$keys
$(ssh-to-age -private-key -i "` + sshKeyPath + `")"
fi
if [ -n "$keys" ]; then
  export SOPS_AGE_KEY="$keys"
fi
exec sops "$@"
`

// withKeys returns the toolbox container with the wrapper script installed and
// the given key secrets mounted.
func (m *Sops) withKeys(ctx context.Context, ageKey, sshKey *dagger.Secret) (*dagger.Container, error) {
	ctr, err := m.Container(ctx, "")
	if err != nil {
		return nil, err
	}
	ctr = ctr.WithNewFile(wrapperPath, wrapperScript, dagger.ContainerWithNewFileOpts{Permissions: 0o755})
	if ageKey != nil {
		ctr = ctr.WithMountedSecret(ageKeyPath, ageKey)
	}
	if sshKey != nil {
		ctr = ctr.WithMountedSecret(sshKeyPath, sshKey)
	}
	return ctr, nil
}

// Decrypt decrypts a sops-encrypted file and returns the plaintext file (same name).
//
// Decryption keys can come from an age identity (ageKey), an SSH ed25519
// private key (sshKey, converted to an age identity via ssh-to-age) and/or one
// or more sops keyservices (keyservice / keyserviceAddress) that hold the key,
// so the decrypting container never sees it.
func (m *Sops) Decrypt(
	ctx context.Context,
	// sops-encrypted file. The format is detected from the file extension unless inputType is set.
	file *dagger.File,
	// age identity file contents ("AGE-SECRET-KEY-1..."; multiple lines allowed).
	// +optional
	ageKey *dagger.Secret,
	// OpenSSH ed25519 private key; converted to an age identity with ssh-to-age.
	// The file must have been encrypted to the matching ssh-to-age recipient.
	// +optional
	sshKey *dagger.Secret,
	// sops keyservice to bind (e.g. from Keyservice()); reached at tcp://sops-keyservice:<keyservicePort>.
	// +optional
	keyservice *dagger.Service,
	// Port of the bound keyservice.
	// +optional
	// +default=5000
	keyservicePort int,
	// Additional keyservice addresses (e.g. "tcp://10.0.0.5:5000").
	// +optional
	keyserviceAddress []string,
	// Input format (yaml, json, dotenv, ini, binary). Defaults to the file extension.
	// +optional
	inputType string,
	// Output format (yaml, json, dotenv, ini, binary). Defaults to the input format.
	// +optional
	outputType string,
	// Extract a single value, e.g. '["database"]["password"]'.
	// +optional
	extract string,
) (*dagger.File, error) {
	name, err := file.Name(ctx)
	if err != nil {
		return nil, err
	}
	ctr, err := m.withKeys(ctx, ageKey, sshKey)
	if err != nil {
		return nil, err
	}

	args := []string{wrapperPath, "decrypt"}
	if keyservice != nil {
		if keyservicePort == 0 {
			keyservicePort = defaultKeyservicePort
		}
		ctr = ctr.WithServiceBinding("sops-keyservice", keyservice)
		args = append(args, "--keyservice", "tcp://sops-keyservice:"+strconv.Itoa(keyservicePort))
	}
	for _, addr := range keyserviceAddress {
		args = append(args, "--keyservice", addr)
	}
	args = appendFlag(args, "--input-type", inputType)
	args = appendFlag(args, "--output-type", outputType)
	args = appendFlag(args, "--extract", extract)
	args = append(args, "--output", path.Join(outDir, name), path.Join(workDir, name))

	return ctr.
		WithMountedFile(path.Join(workDir, name), file).
		WithDirectory(outDir, dag.Directory()).
		WithExec(args).
		File(path.Join(outDir, name)), nil
}

// Encrypt encrypts a plaintext file with sops and returns the encrypted file (same name).
//
// Only public material is needed: age recipients, SSH public keys (converted
// to age recipients via ssh-to-age) and/or a .sops.yaml config with creation
// rules. No private or decryption key is required.
func (m *Sops) Encrypt(
	ctx context.Context,
	// Plaintext file. The format is detected from the file extension unless inputType is set.
	file *dagger.File,
	// age recipients ("age1...").
	// +optional
	ageRecipients []string,
	// SSH ed25519 public keys ("ssh-ed25519 AAAA..."); converted to age recipients with ssh-to-age.
	// +optional
	sshPublicKeys []string,
	// sops config (.sops.yaml) whose creation_rules select recipients and options.
	// +optional
	config *dagger.File,
	// Path of the file relative to the config, used to match creation_rules path_regex. Defaults to the file name.
	// +optional
	filePath string,
	// Only encrypt values whose key matches this regex.
	// +optional
	encryptedRegex string,
	// Do not encrypt values whose key matches this regex.
	// +optional
	unencryptedRegex string,
	// Only encrypt values whose key ends with this suffix.
	// +optional
	encryptedSuffix string,
	// Do not encrypt values whose key ends with this suffix.
	// +optional
	unencryptedSuffix string,
	// Input format (yaml, json, dotenv, ini, binary). Defaults to the file extension.
	// +optional
	inputType string,
	// Output format (yaml, json, dotenv, ini, binary). Defaults to the input format.
	// +optional
	outputType string,
) (*dagger.File, error) {
	if len(ageRecipients) == 0 && len(sshPublicKeys) == 0 && config == nil {
		return nil, fmt.Errorf("encrypt: one of ageRecipients, sshPublicKeys or config is required")
	}
	name, err := file.Name(ctx)
	if err != nil {
		return nil, err
	}
	if filePath == "" {
		filePath = name
	}
	filePath = strings.TrimPrefix(path.Clean("/"+filePath), "/")

	recipients := append([]string{}, ageRecipients...)
	for _, k := range sshPublicKeys {
		r, err := m.SshToAge(ctx, k)
		if err != nil {
			return nil, err
		}
		recipients = append(recipients, r)
	}

	ctr, err := m.Container(ctx, "")
	if err != nil {
		return nil, err
	}

	out := path.Join(outDir, path.Base(filePath))
	args := []string{"sops"}
	if config != nil {
		// --config is a global flag and must precede the subcommand.
		ctr = ctr.WithMountedFile(path.Join(workDir, ".sops.yaml"), config)
		args = append(args, "--config", path.Join(workDir, ".sops.yaml"))
	}
	args = append(args, "encrypt")
	if len(recipients) > 0 {
		args = append(args, "--age", strings.Join(recipients, ","))
	}
	args = appendFlag(args, "--encrypted-regex", encryptedRegex)
	args = appendFlag(args, "--unencrypted-regex", unencryptedRegex)
	args = appendFlag(args, "--encrypted-suffix", encryptedSuffix)
	args = appendFlag(args, "--unencrypted-suffix", unencryptedSuffix)
	args = appendFlag(args, "--input-type", inputType)
	args = appendFlag(args, "--output-type", outputType)
	args = append(args, "--output", out, filePath)

	return ctr.
		WithMountedFile(path.Join(workDir, filePath), file).
		WithDirectory(outDir, dag.Directory()).
		WithExec(args).
		File(out), nil
}

// Keyservice returns a sops keyservice (gRPC over TCP) holding the given keys.
//
// Bind it to another container and pass "--keyservice tcp://<alias>:<port>" to
// sops there; that container can then decrypt without ever holding the key.
// Decrypt does this automatically via its keyservice argument.
func (m *Sops) Keyservice(
	ctx context.Context,
	// age identity file contents ("AGE-SECRET-KEY-1...").
	// +optional
	ageKey *dagger.Secret,
	// OpenSSH ed25519 private key; converted to an age identity with ssh-to-age.
	// +optional
	sshKey *dagger.Secret,
	// TCP port to listen on.
	// +optional
	// +default=5000
	port int,
) (*dagger.Service, error) {
	if ageKey == nil && sshKey == nil {
		return nil, fmt.Errorf("keyservice: ageKey or sshKey is required")
	}
	if port == 0 {
		port = defaultKeyservicePort
	}
	ctr, err := m.withKeys(ctx, ageKey, sshKey)
	if err != nil {
		return nil, err
	}
	return ctr.
		WithExposedPort(port).
		AsService(dagger.ContainerAsServiceOpts{
			Args: []string{wrapperPath, "keyservice", "--network", "tcp", "--address", fmt.Sprintf("0.0.0.0:%d", port)},
		}), nil
}

// appendFlag appends "flag value" to args if value is non-empty.
func appendFlag(args []string, flag, value string) []string {
	if value == "" {
		return args
	}
	return append(args, flag, value)
}
