package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"

	"dagger/sops/internal/dagger"
)

const (
	PlatformAMD64 = dagger.Platform("linux/amd64")
	PlatformARM64 = dagger.Platform("linux/arm64")
)

// knownAgeDigests holds sha256 digests of age release archives. age does not
// publish a checksums file, so for versions not listed here the digest is
// looked up from the GitHub release asset metadata.
var knownAgeDigests = map[string]map[string]string{
	"1.3.2": {
		"amd64": "sha256:cbe24006683f8eb669266162894b9a522a1af52f2665fbc63a4bb032ed26ac10",
		"arm64": "sha256:6b8dc4333c53a5a57c9e5834e3a48f92605d7154014cd07269ff3327db5d37f4",
	},
}

// Container returns a minimal alpine-based image with sops, age, age-keygen and
// ssh-to-age installed in /usr/local/bin. The entrypoint is sops and the
// working directory is /work.
func (m *Sops) Container(
	ctx context.Context,
	// Target platform (linux/amd64 or linux/arm64). Defaults to the engine's platform.
	// +optional
	platform dagger.Platform,
) (*dagger.Container, error) {
	if platform == "" {
		p, err := dag.DefaultPlatform(ctx)
		if err != nil {
			return nil, fmt.Errorf("resolve default platform: %w", err)
		}
		platform = p
	}
	arch, err := platformArch(platform)
	if err != nil {
		return nil, err
	}

	sops, err := m.sopsBinary(ctx, arch)
	if err != nil {
		return nil, err
	}
	sshToAge, err := m.sshToAgeBinary(ctx, arch)
	if err != nil {
		return nil, err
	}
	ageDir, err := m.ageBinaries(ctx, arch)
	if err != nil {
		return nil, err
	}

	binOpts := dagger.ContainerWithFileOpts{Permissions: 0o755, Owner: "0:0"}
	return dag.Container(dagger.ContainerOpts{Platform: platform}).
		From(alpineImage).
		WithFile("/usr/local/bin/sops", sops, binOpts).
		WithFile("/usr/local/bin/ssh-to-age", sshToAge, binOpts).
		WithFile("/usr/local/bin/age", ageDir.File("age"), binOpts).
		WithFile("/usr/local/bin/age-keygen", ageDir.File("age-keygen"), binOpts).
		WithLabel("org.opencontainers.image.title", "sops").
		WithLabel("org.opencontainers.image.description", "sops "+m.SopsVersion+", age "+m.AgeVersion+", ssh-to-age "+m.SshToAgeVersion).
		WithLabel("org.opencontainers.image.source", "https://github.com/daniel-naegele/daggerverse").
		WithLabel("org.opencontainers.image.version", m.SopsVersion).
		WithWorkdir(workDir).
		WithEntrypoint([]string{"sops"}), nil
}

// platformArch maps a dagger platform to the release asset architecture name.
func platformArch(p dagger.Platform) (string, error) {
	parts := strings.Split(string(p), "/")
	if len(parts) < 2 || parts[0] != "linux" {
		return "", fmt.Errorf("unsupported platform %q", p)
	}
	switch parts[1] {
	case "amd64", "arm64":
		return parts[1], nil
	}
	return "", fmt.Errorf("unsupported platform %q (supported: linux/amd64, linux/arm64)", p)
}

// sopsBinary downloads the static sops binary and verifies it against the release checksums.
func (m *Sops) sopsBinary(ctx context.Context, arch string) (*dagger.File, error) {
	base := fmt.Sprintf("https://github.com/getsops/sops/releases/download/v%s/", m.SopsVersion)
	name := fmt.Sprintf("sops-v%s.linux.%s", m.SopsVersion, arch)
	sum, err := checksumFromFile(ctx, base+fmt.Sprintf("sops-v%s.checksums.txt", m.SopsVersion), name)
	if err != nil {
		return nil, err
	}
	return dag.HTTP(base+name, dagger.HTTPOpts{Name: "sops", Checksum: sum, Permissions: 0o755}), nil
}

// sshToAgeBinary downloads the static ssh-to-age binary and verifies it against the release checksums.
func (m *Sops) sshToAgeBinary(ctx context.Context, arch string) (*dagger.File, error) {
	name := "ssh-to-age.linux-" + arch
	var errs []error
	// Release tags are inconsistently prefixed ("v1.3.0" but "1.2.0").
	for _, tag := range []string{"v" + m.SshToAgeVersion, m.SshToAgeVersion} {
		base := fmt.Sprintf("https://github.com/Mic92/ssh-to-age/releases/download/%s/", tag)
		sum, err := checksumFromFile(ctx, base+"sha256sums.txt", name)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		return dag.HTTP(base+name, dagger.HTTPOpts{Name: "ssh-to-age", Checksum: sum, Permissions: 0o755}), nil
	}
	return nil, fmt.Errorf("ssh-to-age %s: %w", m.SshToAgeVersion, errors.Join(errs...))
}

// ageBinaries downloads and extracts the age release archive, returning a
// directory containing the age and age-keygen binaries.
func (m *Sops) ageBinaries(ctx context.Context, arch string) (*dagger.Directory, error) {
	name := fmt.Sprintf("age-v%s-linux-%s.tar.gz", m.AgeVersion, arch)
	url := fmt.Sprintf("https://github.com/FiloSottile/age/releases/download/v%s/%s", m.AgeVersion, name)
	sum, err := ageDigest(ctx, m.AgeVersion, arch, name)
	if err != nil {
		return nil, err
	}
	archive := dag.HTTP(url, dagger.HTTPOpts{Name: "age.tar.gz", Checksum: sum})
	// Extract on the engine's native platform; the binaries are static so the
	// result can be copied into an image of any platform.
	return dag.Container().
		From(alpineImage).
		WithMountedFile("/tmp/age.tar.gz", archive).
		WithExec([]string{"tar", "-xzf", "/tmp/age.tar.gz", "-C", "/tmp"}).
		Directory("/tmp/age"), nil
}

// checksumFromFile fetches a sha256sum-style checksums file and returns the
// digest ("sha256:<hex>") for the entry whose basename equals name.
func checksumFromFile(ctx context.Context, url, name string) (string, error) {
	contents, err := dag.HTTP(url).Contents(ctx)
	if err != nil {
		return "", fmt.Errorf("fetch checksums %s: %w", url, err)
	}
	for _, line := range strings.Split(contents, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if path.Base(strings.TrimPrefix(fields[1], "*")) == name {
			return "sha256:" + fields[0], nil
		}
	}
	return "", fmt.Errorf("no checksum for %s in %s", name, url)
}

// ageDigest returns the sha256 digest of an age release archive, from the
// pinned table or from the GitHub release asset metadata.
func ageDigest(ctx context.Context, version, arch, asset string) (string, error) {
	if d, ok := knownAgeDigests[version][arch]; ok {
		return d, nil
	}
	var rel struct {
		Assets []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
		} `json:"assets"`
	}
	if err := githubAPI(ctx, "https://api.github.com/repos/FiloSottile/age/releases/tags/v"+version, nil, &rel); err != nil {
		return "", fmt.Errorf("look up age %s digest: %w", version, err)
	}
	for _, a := range rel.Assets {
		if a.Name == asset {
			if !strings.HasPrefix(a.Digest, "sha256:") {
				return "", fmt.Errorf("age release asset %s has no published sha256 digest (GitHub only records digests for assets uploaded since mid-2025); add it to knownAgeDigests to use this version", asset)
			}
			return a.Digest, nil
		}
	}
	return "", fmt.Errorf("age release v%s has no asset %s", version, asset)
}

// githubAPI performs a GET request against the GitHub API and decodes the JSON response.
func githubAPI(ctx context.Context, url string, token *dagger.Secret, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "daggerverse-sops")
	if token != nil {
		t, err := token.Plaintext(ctx)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// trimV strips a leading "v" from a version string.
func trimV(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}
