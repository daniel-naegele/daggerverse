// Tests for the codemagic-cli-tools module.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"dagger/tests/internal/dagger"

	"github.com/sourcegraph/conc/pool"
)

const (
	// fixtureURL is a small, real Android App Bundle from bundletool's own test resources
	// (Apache-2.0, google/bundletool), pinned to a commit and verified by checksum.
	fixtureURL = "https://raw.githubusercontent.com/google/bundletool/" +
		"586a43a450712a1067f3d92cf7574dee68226302/" +
		"src/test/resources/com/android/tools/build/bundletool/testdata/bundle/install-time-permanent-modules.aab"
	fixtureChecksum = "sha256:83d7d10b6036da2f94ad34483a5c3d5a32891b08e4fd1a658165c55258bdaff2"

	keyAlias = "upload"
)

type Tests struct{}

// All executes all tests (native platform).
func (m *Tests) All(ctx context.Context) error {
	p := pool.New().WithErrors().WithContext(ctx)

	p.Go(m.Tools)
	p.Go(m.Keystore)
	p.Go(m.AppBundle)
	p.Go(m.UniversalApk)
	p.Go(m.GitChangelog)
	p.Go(m.GooglePlay)
	p.Go(m.Exec)

	return p.Wait()
}

func cm() *dagger.CodemagicCliTools {
	return dag.CodemagicCliTools()
}

func password() *dagger.Secret {
	return dag.SetSecret("keystore-password", "s3cr3t-pass")
}

func keystore() *dagger.File {
	return cm().CreateKeystore(password(), keyAlias, dagger.CodemagicCliToolsCreateKeystoreOpts{
		CommonName:   "Dagger Test",
		Organization: "daggerverse",
		Country:      "DE",
	})
}

// signedFixture is the bundletool test bundle as published (already signed).
func signedFixture() *dagger.File {
	return dag.HTTP(fixtureURL, dagger.HTTPOpts{Name: "app.aab", Checksum: fixtureChecksum})
}

// unsignedFixture is the fixture with its JAR signature (META-INF/*.SF, *.RSA, MANIFEST.MF) stripped.
func unsignedFixture() *dagger.File {
	script := `
import re, sys, zipfile
src, dst = sys.argv[1], sys.argv[2]
sig = re.compile(r"^META-INF/(MANIFEST\.MF|[^/]+\.(SF|RSA|DSA|EC))$")
with zipfile.ZipFile(src) as zin, zipfile.ZipFile(dst, "w", zipfile.ZIP_DEFLATED) as zout:
    for item in zin.infolist():
        if not sig.match(item.filename):
            zout.writestr(item, zin.read(item.filename))
`
	return cm().Container().
		WithFile("/tmp/in.aab", signedFixture(), dagger.ContainerWithFileOpts{Owner: "codemagic"}).
		WithExec([]string{"python", "-c", script, "/tmp/in.aab", "/tmp/unsigned.aab"}).
		File("/tmp/unsigned.aab")
}

// Tools checks that all binaries are wired up and the pinned bundletool is used.
func (m *Tests) Tools(ctx context.Context) error {
	info, err := cm().BundletoolInfo(ctx)
	if err != nil {
		return err
	}
	var parsed struct{ Bundletool, Java, Version string }
	if err := json.Unmarshal([]byte(info), &parsed); err != nil {
		return fmt.Errorf("bundletool info is not JSON: %w\n%s", err, info)
	}
	want, err := cm().BundletoolVersion(ctx)
	if err != nil {
		return err
	}
	if parsed.Version != want || parsed.Bundletool != "/opt/bundletool/bundletool.jar" {
		return fmt.Errorf("unexpected bundletool info: %s (want version %s)", info, want)
	}

	out, err := cm().Container().
		WithExec([]string{"sh", "-c", strings.Join([]string{
			"set -e",
			"id -u",
			"java -version",
			"keytool -help >/dev/null 2>&1",
			"jarsigner -help >/dev/null 2>&1",
			"bundletool version",
			"git --version",
			"for t in android-app-bundle android-keystore google-play git-changelog universal-apk codemagic-cli-tools; do $t --help >/dev/null; done",
		}, "\n")}).
		CombinedOutput(ctx)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(out, "1000\n") {
		return fmt.Errorf("expected container to run as uid 1000, got:\n%s", out)
	}
	return nil
}

// Keystore creates a keystore, verifies it and reads its certificate.
func (m *Tests) Keystore(ctx context.Context) error {
	ks := cm().AndroidKeystore(keystore(), password(), keyAlias)
	if _, err := ks.Verify(ctx); err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	cert, err := ks.Certificate(ctx)
	if err != nil {
		return fmt.Errorf("certificate: %w", err)
	}
	if !strings.Contains(cert, "Dagger Test") {
		return fmt.Errorf("certificate does not mention issuer:\n%s", cert)
	}

	// Wrong password and wrong alias must fail.
	wrong := dag.SetSecret("wrong-password", "not-the-password")
	if _, err := cm().AndroidKeystore(keystore(), wrong, keyAlias).Verify(ctx); err == nil {
		return fmt.Errorf("verify with wrong password unexpectedly succeeded")
	}
	if _, err := cm().AndroidKeystore(keystore(), password(), "nope").Verify(ctx); err == nil {
		return fmt.Errorf("verify with wrong alias unexpectedly succeeded")
	}
	return nil
}

// AppBundle exercises android-app-bundle: dump, validate, is-signed, sign, build-apks, build-universal-apk.
func (m *Tests) AppBundle(ctx context.Context) error {
	unsigned := cm().AndroidAppBundle(unsignedFixture())

	pkg, err := unsigned.Dump(ctx, dagger.CodemagicCliToolsAndroidAppBundleDumpOpts{Xpath: "/manifest/@package"})
	if err != nil {
		return fmt.Errorf("dump: %w", err)
	}
	if pkg == "" || strings.Contains(pkg, "\n") {
		return fmt.Errorf("unexpected package name %q", pkg)
	}
	manifest, err := unsigned.Dump(ctx)
	if err != nil {
		return fmt.Errorf("dump manifest: %w", err)
	}
	if !strings.Contains(manifest, "<manifest") {
		return fmt.Errorf("unexpected manifest dump:\n%s", manifest)
	}

	if _, err := unsigned.Validate(ctx); err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	if signed, err := unsigned.IsSigned(ctx); err != nil {
		return fmt.Errorf("is-signed (unsigned): %w", err)
	} else if signed {
		return fmt.Errorf("stripped bundle reported as signed")
	}

	signedFile := unsigned.Sign(keystore(), password(), keyAlias)
	signedBundle := cm().AndroidAppBundle(signedFile)
	if signed, err := signedBundle.IsSigned(ctx); err != nil {
		return fmt.Errorf("is-signed (signed): %w", err)
	} else if !signed {
		return fmt.Errorf("bundle not signed after sign")
	}
	if name, err := signedFile.Name(ctx); err != nil {
		return err
	} else if name != "unsigned.aab" {
		return fmt.Errorf("signed bundle has unexpected name %q", name)
	}

	// Unsigned (debug key) universal APK.
	if err := assertZipEntry(ctx, unsigned.BuildUniversalApk(), "AndroidManifest.xml"); err != nil {
		return fmt.Errorf("build-universal-apk: %w", err)
	}
	// Universal APK signed with our keystore.
	apk := unsigned.BuildUniversalApk(dagger.CodemagicCliToolsAndroidAppBundleBuildUniversalApkOpts{
		Keystore:         keystore(),
		KeystorePassword: password(),
		KeyAlias:         keyAlias,
	})
	if err := assertZipEntry(ctx, apk, "AndroidManifest.xml"); err != nil {
		return fmt.Errorf("build-universal-apk (signed): %w", err)
	}
	// APK set.
	if err := assertZipEntry(ctx, unsigned.BuildApks(dagger.CodemagicCliToolsAndroidAppBundleBuildApksOpts{Universal: true}), "universal.apk"); err != nil {
		return fmt.Errorf("build-apks: %w", err)
	}

	// Signing with partial signing options must be rejected.
	if _, err := unsigned.BuildUniversalApk(dagger.CodemagicCliToolsAndroidAppBundleBuildUniversalApkOpts{KeyAlias: keyAlias}).Sync(ctx); err == nil {
		return fmt.Errorf("build-universal-apk with keyAlias but no keystore unexpectedly succeeded")
	}
	return nil
}

// UniversalApk exercises the deprecated universal-apk tool, with and without signing.
func (m *Tests) UniversalApk(ctx context.Context) error {
	if err := assertZipEntry(ctx, cm().UniversalApk(unsignedFixture()), "AndroidManifest.xml"); err != nil {
		return fmt.Errorf("universal-apk: %w", err)
	}
	apk := cm().UniversalApk(unsignedFixture(), dagger.CodemagicCliToolsUniversalApkOpts{
		Keystore:         keystore(),
		KeystorePassword: password(),
		KeyAlias:         keyAlias,
	})
	if err := assertZipEntry(ctx, apk, "AndroidManifest.xml"); err != nil {
		return fmt.Errorf("universal-apk (signed): %w", err)
	}
	return nil
}

// GitChangelog builds a git repository in the pipeline and generates a changelog from it.
func (m *Tests) GitChangelog(ctx context.Context) error {
	repo := cm().Container().
		WithEnvVariable("GIT_AUTHOR_NAME", "Test").
		WithEnvVariable("GIT_AUTHOR_EMAIL", "test@example.com").
		WithEnvVariable("GIT_COMMITTER_NAME", "Test").
		WithEnvVariable("GIT_COMMITTER_EMAIL", "test@example.com").
		WithWorkdir("/tmp/repo").
		WithExec([]string{"sh", "-c", strings.Join([]string{
			"set -e",
			"git init -q -b main",
			"git commit -q --allow-empty -m 'Initial commit'",
			"git rev-parse HEAD > /tmp/first",
			"git commit -q --allow-empty -m 'Add login screen'",
			"git commit -q --allow-empty -m 'Merge branch feature into main'",
			"git commit -q --allow-empty -m 'Fix crash on startup'",
		}, "\n")})
	src := repo.Directory("/tmp/repo")
	first, err := repo.File("/tmp/first").Contents(ctx)
	if err != nil {
		return err
	}

	all, err := cm().GitChangelog(ctx, src)
	if err != nil {
		return fmt.Errorf("git-changelog: %w", err)
	}
	for _, want := range []string{"* Fix crash on startup", "* Add login screen", "* Initial commit"} {
		if !strings.Contains(all, want) {
			return fmt.Errorf("changelog missing %q:\n%s", want, all)
		}
	}
	if strings.Contains(all, "Merge branch") {
		return fmt.Errorf("changelog should skip merge commits:\n%s", all)
	}

	since, err := cm().GitChangelog(ctx, src, dagger.CodemagicCliToolsGitChangelogOpts{
		PreviousCommit: strings.TrimSpace(first),
	})
	if err != nil {
		return fmt.Errorf("git-changelog --previous-commit: %w", err)
	}
	if strings.Contains(since, "Initial commit") || !strings.Contains(since, "Add login screen") {
		return fmt.Errorf("unexpected changelog since previous commit:\n%s", since)
	}

	limited, err := cm().GitChangelog(ctx, src, dagger.CodemagicCliToolsGitChangelogOpts{CommitLimit: 1})
	if err != nil {
		return fmt.Errorf("git-changelog --commit-limit: %w", err)
	}
	if strings.TrimSpace(limited) != "* Fix crash on startup" {
		return fmt.Errorf("unexpected limited changelog:\n%s", limited)
	}
	return nil
}

// GooglePlay checks argument wiring without real credentials: help output works and invalid credentials
// fail cleanly before any API call.
func (m *Tests) GooglePlay(ctx context.Context) error {
	help, err := cm().Exec([]string{"google-play", "bundles", "publish", "--help"}).Stdout(ctx)
	if err != nil {
		return err
	}
	for _, flag := range []string{"--bundle", "--track", "--release-notes", "--rollout-fraction"} {
		if !strings.Contains(help, flag) {
			return fmt.Errorf("google-play bundles publish --help missing %s", flag)
		}
	}

	notServiceAccount := cm().GooglePlay(dag.SetSecret("gp-invalid", `{"type": "authorized_user"}`))
	_, err = notServiceAccount.LatestBuildNumber(ctx, "com.example.app")
	if err == nil {
		return fmt.Errorf("get-latest-build-number with invalid credentials unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "service account") {
		return fmt.Errorf("unexpected error for invalid credentials: %w", err)
	}

	_, err = cm().GooglePlay(dag.SetSecret("gp-not-json", "not json")).ListTracks(ctx, "com.example.app")
	if err == nil || !strings.Contains(err.Error(), "not a valid JSON") {
		return fmt.Errorf("unexpected result for non-JSON credentials: %v", err)
	}
	return nil
}

// Exec checks the escape hatch, including secret environment variables and source directories.
func (m *Tests) Exec(ctx context.Context) error {
	src := dag.Directory().WithNewFile("hello.txt", "hello")
	out, err := cm().Exec(
		[]string{"sh", "-c", `cat hello.txt && test "$TOKEN" = "t0ken" && touch written`},
		dagger.CodemagicCliToolsExecOpts{
			Source:    src,
			SecretEnv: []string{"TOKEN"},
			Secrets:   []*dagger.Secret{dag.SetSecret("token", "t0ken")},
		},
	).Stdout(ctx)
	if err != nil {
		return err
	}
	if out != "hello" {
		return fmt.Errorf("unexpected exec output %q", out)
	}
	return nil
}

// Arm64 checks that the linux/arm64 image builds and its tools run (requires emulation on amd64 hosts).
// APK generation is not tested: bundletool's bundled aapt2 is x86_64 only.
func (m *Tests) Arm64(ctx context.Context) error {
	out, err := cm().Container(dagger.CodemagicCliToolsContainerOpts{Platform: "linux/arm64"}).
		WithExec([]string{"sh", "-c", "uname -m && java -version && android-app-bundle bundletool info && git-changelog --help >/dev/null"}).
		CombinedOutput(ctx)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(out, "aarch64") {
		return fmt.Errorf("expected aarch64, got:\n%s", out)
	}
	return nil
}

// assertZipEntry checks that file is a ZIP archive containing entry.
func assertZipEntry(ctx context.Context, file *dagger.File, entry string) error {
	_, err := cm().Container().
		WithFile("/tmp/archive.zip", file, dagger.ContainerWithFileOpts{Owner: "codemagic"}).
		WithExec([]string{"python", "-c",
			"import sys, zipfile; sys.exit(0 if sys.argv[2] in zipfile.ZipFile(sys.argv[1]).namelist() else 1)",
			"/tmp/archive.zip", entry}).
		Sync(ctx)
	return err
}
