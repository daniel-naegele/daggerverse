package main

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"dagger/tests/internal/dagger"

	"github.com/sourcegraph/conc/pool"
)

type Tests struct{}

// fixture is a plaintext test file and the secret values it contains.
type fixture struct {
	name    string
	secrets []string
}

var fixtures = []fixture{
	{"secrets.yaml", []string{"hunter2-yaml", "tok-yaml-123"}},
	{"secrets.json", []string{"hunter2-json", "tok-json-123"}},
	{"secrets.env", []string{"hunter2-dotenv", "tok-dotenv-123"}},
}

// All executes all tests.
func (m *Tests) All(ctx context.Context) error {
	p := pool.New().WithErrors().WithContext(ctx)

	p.Go(m.ContainerAmd64)
	p.Go(m.ContainerArm64)
	p.Go(m.AgeRoundTrip)
	p.Go(m.SSHRoundTrip)
	p.Go(m.KeyserviceRoundTrip)
	p.Go(m.KeyserviceRawClient)
	p.Go(m.ConfigEncrypt)
	p.Go(m.Extract)
	p.Go(m.DecryptWithoutKeyFails)

	return p.Wait()
}

func testdata(name string) *dagger.File {
	return dag.CurrentModule().Source().File("testdata/" + name)
}

// assertEncrypted checks that the file is sops-encrypted and leaks no plaintext secret.
func assertEncrypted(ctx context.Context, f *dagger.File, fx fixture) error {
	enc, err := f.Contents(ctx)
	if err != nil {
		return fmt.Errorf("%s: encrypt: %w", fx.name, err)
	}
	if !strings.Contains(enc, "ENC[AES256_GCM") {
		return fmt.Errorf("%s: output is not sops-encrypted:\n%s", fx.name, enc)
	}
	for _, s := range fx.secrets {
		if strings.Contains(enc, s) {
			return fmt.Errorf("%s: encrypted output leaks %q", fx.name, s)
		}
	}
	return nil
}

// assertDecrypted checks that the decrypted file contains every secret value.
func assertDecrypted(ctx context.Context, f *dagger.File, fx fixture) error {
	dec, err := f.Contents(ctx)
	if err != nil {
		return fmt.Errorf("%s: decrypt: %w", fx.name, err)
	}
	for _, s := range fx.secrets {
		if !strings.Contains(dec, s) {
			return fmt.Errorf("%s: decrypted output misses %q:\n%s", fx.name, s, dec)
		}
	}
	return nil
}

// ContainerAmd64 checks that all tools run on linux/amd64.
func (m *Tests) ContainerAmd64(ctx context.Context) error {
	out, err := dag.Sops().Container(dagger.SopsContainerOpts{Platform: "linux/amd64"}).
		WithExec([]string{"sh", "-c", "sops --version; age --version; age-keygen --version; ssh-to-age -version"}).
		Stdout(ctx)
	if err != nil {
		return err
	}
	for _, want := range []string{"sops 3.", "v1."} {
		if !strings.Contains(out, want) {
			return fmt.Errorf("amd64 version output misses %q:\n%s", want, out)
		}
	}
	return nil
}

// ContainerArm64 checks that the linux/arm64 image contains all binaries (no emulation needed).
func (m *Tests) ContainerArm64(ctx context.Context) error {
	ctr := dag.Sops().Container(dagger.SopsContainerOpts{Platform: "linux/arm64"})
	platform, err := ctr.Platform(ctx)
	if err != nil {
		return err
	}
	if platform != "linux/arm64" {
		return fmt.Errorf("arm64 container has platform %q", platform)
	}
	entries, err := ctr.Rootfs().Directory("usr/local/bin").Entries(ctx)
	if err != nil {
		return err
	}
	for _, bin := range []string{"sops", "age", "age-keygen", "ssh-to-age"} {
		if !slices.Contains(entries, bin) {
			return fmt.Errorf("arm64 image misses %s (have %v)", bin, entries)
		}
	}
	return nil
}

// AgeRoundTrip encrypts each fixture to a fresh age recipient and decrypts it with the age key.
func (m *Tests) AgeRoundTrip(ctx context.Context) error {
	kp, pub, err := ageKeypair(ctx)
	if err != nil {
		return err
	}
	for _, fx := range fixtures {
		enc := dag.Sops().Encrypt(testdata(fx.name), dagger.SopsEncryptOpts{AgeRecipients: []string{pub}})
		if err := assertEncrypted(ctx, enc, fx); err != nil {
			return err
		}
		dec := dag.Sops().Decrypt(enc, dagger.SopsDecryptOpts{AgeKey: kp.PrivateKey()})
		if err := assertDecrypted(ctx, dec, fx); err != nil {
			return err
		}
	}
	return nil
}

// ageKeypair generates a fresh age keypair. The object is pinned by ID so that
// the public and private key come from the same AgeKeygen call.
func ageKeypair(ctx context.Context) (*dagger.SopsAgeKeypair, string, error) {
	id, err := dag.Sops().AgeKeygen().ID(ctx)
	if err != nil {
		return nil, "", err
	}
	kp := dag.LoadSopsAgeKeypairFromID(dagger.SopsAgeKeypairID(id))
	pub, err := kp.PublicKey(ctx)
	if err != nil {
		return nil, "", err
	}
	return kp, pub, nil
}

// sshKeypair generates a throwaway ed25519 SSH keypair with ssh-keygen.
func sshKeypair(ctx context.Context) (string, *dagger.Secret, error) {
	ctr := dag.Container().From("alpine:3.24.2").
		WithExec([]string{"apk", "add", "--no-cache", "openssh-keygen"}).
		WithEnvVariable("NONCE", strconv.FormatInt(time.Now().UnixNano(), 10)).
		WithExec([]string{"ssh-keygen", "-t", "ed25519", "-N", "", "-C", "sops-test", "-f", "/tmp/id_ed25519"})
	pub, err := ctr.File("/tmp/id_ed25519.pub").Contents(ctx)
	if err != nil {
		return "", nil, err
	}
	priv, err := ctr.File("/tmp/id_ed25519").Contents(ctx)
	if err != nil {
		return "", nil, err
	}
	pub = strings.TrimSpace(pub)
	return pub, dag.SetSecret("ssh-test-key-"+pub, priv), nil
}

// SSHRoundTrip encrypts to an SSH public key and decrypts with the SSH private key (via ssh-to-age).
func (m *Tests) SSHRoundTrip(ctx context.Context) error {
	pub, priv, err := sshKeypair(ctx)
	if err != nil {
		return err
	}
	recipient, err := dag.Sops().SSHToAge(ctx, pub)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(recipient, "age1") {
		return fmt.Errorf("ssh-to-age returned %q", recipient)
	}
	for _, fx := range fixtures {
		enc := dag.Sops().Encrypt(testdata(fx.name), dagger.SopsEncryptOpts{SSHPublicKeys: []string{pub}})
		if err := assertEncrypted(ctx, enc, fx); err != nil {
			return err
		}
		encText, err := enc.Contents(ctx)
		if err != nil {
			return err
		}
		if !strings.Contains(encText, recipient) {
			return fmt.Errorf("%s: not encrypted to ssh-derived recipient %s", fx.name, recipient)
		}
		dec := dag.Sops().Decrypt(enc, dagger.SopsDecryptOpts{SSHKey: priv})
		if err := assertDecrypted(ctx, dec, fx); err != nil {
			return err
		}
	}
	return nil
}

// KeyserviceRoundTrip decrypts via a keyservice; the decrypting container holds no key.
func (m *Tests) KeyserviceRoundTrip(ctx context.Context) error {
	kp, pub, err := ageKeypair(ctx)
	if err != nil {
		return err
	}
	svc := dag.Sops().Keyservice(dagger.SopsKeyserviceOpts{AgeKey: kp.PrivateKey()})
	for _, fx := range fixtures {
		enc := dag.Sops().Encrypt(testdata(fx.name), dagger.SopsEncryptOpts{AgeRecipients: []string{pub}})
		dec := dag.Sops().Decrypt(enc, dagger.SopsDecryptOpts{Keyservice: svc})
		if err := assertDecrypted(ctx, dec, fx); err != nil {
			return err
		}
	}
	return nil
}

// KeyserviceRawClient binds an SSH-key keyservice into a plain client container and runs sops there.
func (m *Tests) KeyserviceRawClient(ctx context.Context) error {
	pub, priv, err := sshKeypair(ctx)
	if err != nil {
		return err
	}
	fx := fixtures[0]
	enc := dag.Sops().Encrypt(testdata(fx.name), dagger.SopsEncryptOpts{SSHPublicKeys: []string{pub}})
	svc := dag.Sops().Keyservice(dagger.SopsKeyserviceOpts{SSHKey: priv, Port: 6000})
	out, err := dag.Sops().Container().
		WithServiceBinding("keys", svc).
		WithMountedFile("/work/"+fx.name, enc).
		WithExec([]string{"sops", "decrypt", "--enable-local-keyservice=false", "--keyservice", "tcp://keys:6000", fx.name}).
		Stdout(ctx)
	if err != nil {
		return err
	}
	for _, s := range fx.secrets {
		if !strings.Contains(out, s) {
			return fmt.Errorf("raw keyservice client output misses %q:\n%s", s, out)
		}
	}
	return nil
}

// ConfigEncrypt encrypts using only a .sops.yaml (creation rule with encrypted_regex).
func (m *Tests) ConfigEncrypt(ctx context.Context) error {
	kp, pub, err := ageKeypair(ctx)
	if err != nil {
		return err
	}
	config := dag.Directory().WithNewFile(".sops.yaml", fmt.Sprintf(`creation_rules:
  - path_regex: secrets/.*\.yaml$
    encrypted_regex: ^password$
    age: %s
`, pub)).File(".sops.yaml")
	fx := fixtures[0]
	enc := dag.Sops().Encrypt(testdata(fx.name), dagger.SopsEncryptOpts{Config: config, FilePath: "secrets/" + fx.name})
	encText, err := enc.Contents(ctx)
	if err != nil {
		return err
	}
	if strings.Contains(encText, "hunter2-yaml") || !strings.Contains(encText, "tok-yaml-123") {
		return fmt.Errorf("encrypted_regex not applied:\n%s", encText)
	}
	return assertDecrypted(ctx, dag.Sops().Decrypt(enc, dagger.SopsDecryptOpts{AgeKey: kp.PrivateKey()}), fx)
}

// Extract decrypts a single value.
func (m *Tests) Extract(ctx context.Context) error {
	kp, pub, err := ageKeypair(ctx)
	if err != nil {
		return err
	}
	enc := dag.Sops().Encrypt(testdata("secrets.json"), dagger.SopsEncryptOpts{AgeRecipients: []string{pub}})
	out, err := dag.Sops().Decrypt(enc, dagger.SopsDecryptOpts{AgeKey: kp.PrivateKey(), Extract: `["database"]["password"]`}).Contents(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) != "hunter2-json" {
		return fmt.Errorf("extract returned %q", out)
	}
	return nil
}

// DecryptWithoutKeyFails checks that decryption fails without a matching key.
func (m *Tests) DecryptWithoutKeyFails(ctx context.Context) error {
	_, pub, err := ageKeypair(ctx)
	if err != nil {
		return err
	}
	other, _, err := ageKeypair(ctx)
	if err != nil {
		return err
	}
	enc := dag.Sops().Encrypt(testdata("secrets.yaml"), dagger.SopsEncryptOpts{AgeRecipients: []string{pub}})
	if _, err := dag.Sops().Decrypt(enc, dagger.SopsDecryptOpts{AgeKey: other.PrivateKey()}).Contents(ctx); err == nil {
		return fmt.Errorf("decrypt with wrong key unexpectedly succeeded")
	}
	return nil
}
