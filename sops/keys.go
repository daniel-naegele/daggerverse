package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"dagger/sops/internal/dagger"
)

// AgeKeypair is a freshly generated age identity and its recipient.
type AgeKeypair struct {
	// age recipient ("age1..."), safe to share.
	PublicKey string
	// age identity file contents ("AGE-SECRET-KEY-1...").
	PrivateKey *dagger.Secret
}

// AgeKeygen generates a new age keypair with age-keygen.
//
// Every call generates a fresh key, so read PublicKey and PrivateKey from the
// same returned object (in Go: pin it via ID / LoadSopsAgeKeypairFromID).
// +cache="never"
func (m *Sops) AgeKeygen(ctx context.Context) (*AgeKeypair, error) {
	ctr, err := m.Container(ctx, "")
	if err != nil {
		return nil, err
	}
	ctr = ctr.
		// Never reuse a cached key.
		WithEnvVariable("SOPS_KEYGEN_NONCE", strconv.FormatInt(time.Now().UnixNano(), 10)).
		WithExec([]string{"age-keygen", "-o", "/tmp/key.txt"})
	pub, err := ctr.WithExec([]string{"age-keygen", "-y", "/tmp/key.txt"}).Stdout(ctx)
	if err != nil {
		return nil, err
	}
	pub = strings.TrimSpace(pub)
	priv, err := ctr.File("/tmp/key.txt").Contents(ctx)
	if err != nil {
		return nil, err
	}
	return &AgeKeypair{
		PublicKey:  pub,
		PrivateKey: dag.SetSecret("age-key-"+pub, priv),
	}, nil
}

// SshToAge converts an SSH ed25519 public key ("ssh-ed25519 AAAA...") to an age recipient ("age1...").
func (m *Sops) SshToAge(ctx context.Context, publicKey string) (string, error) {
	ctr, err := m.Container(ctx, "")
	if err != nil {
		return "", err
	}
	out, err := ctr.
		WithNewFile("/tmp/key.pub", strings.TrimSpace(publicKey)+"\n").
		WithExec([]string{"ssh-to-age", "-i", "/tmp/key.pub"}).
		Stdout(ctx)
	if err != nil {
		return "", fmt.Errorf("ssh-to-age: %w", err)
	}
	return strings.TrimSpace(out), nil
}
