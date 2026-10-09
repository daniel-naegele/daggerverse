// Minimal sops + age toolbox image and helpers for secret handling in pipelines.
//
// Builds a small alpine-based image containing statically linked sops, age,
// age-keygen and ssh-to-age binaries downloaded from their GitHub releases
// (checksum-verified), and exposes functions to encrypt and decrypt sops files
// with age keys, SSH ed25519 keys (converted via ssh-to-age) or a remote sops
// keyservice. Versions are pinned and can be overridden via WithSopsVersion,
// WithAgeVersion and WithSshToAgeVersion.

package main

const (
	// Base image for the published container.
	alpineImage = "alpine:3.24.2"
	// Directory where input files are mounted.
	workDir = "/work"
	// Directory where output files are written.
	outDir = "/out"
	// Mount paths for key material.
	ageKeyPath = "/run/secrets/sops/age.key"
	sshKeyPath = "/run/secrets/sops/ssh.key"
	// Wrapper script that assembles SOPS_AGE_KEY from mounted keys and runs sops.
	wrapperPath = "/usr/local/bin/sops-with-keys"
	// Default keyservice port.
	defaultKeyservicePort = 5000
)

type Sops struct {
	// sops version (getsops/sops release, without "v" prefix).
	SopsVersion string
	// age version (FiloSottile/age release, without "v" prefix).
	AgeVersion string
	// ssh-to-age version (Mic92/ssh-to-age release, without "v" prefix).
	SshToAgeVersion string
}

func New() *Sops {
	return &Sops{
		SopsVersion:     "3.13.3",
		AgeVersion:      "1.3.2",
		SshToAgeVersion: "1.3.0",
	}
}

// WithSopsVersion returns this module configured to use the given sops version.
func (m *Sops) WithSopsVersion(version string) *Sops {
	m.SopsVersion = trimV(version)
	return m
}

// WithAgeVersion returns this module configured to use the given age version.
func (m *Sops) WithAgeVersion(version string) *Sops {
	m.AgeVersion = trimV(version)
	return m
}

// WithSshToAgeVersion returns this module configured to use the given ssh-to-age version.
func (m *Sops) WithSshToAgeVersion(version string) *Sops {
	m.SshToAgeVersion = trimV(version)
	return m
}
