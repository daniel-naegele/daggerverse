// Tests for the flutter module, run against the fixture app in ../testdata/app.
package main

import (
	"context"
	"fmt"
	"strings"

	"dagger/tests/internal/dagger"

	"github.com/sourcegraph/conc/pool"
)

const (
	keyAlias  = "upload"
	signerDN  = "CN=Dagger Flutter Test, O=daggerverse, C=DE"
	signerCN  = "CN=Dagger Flutter Test"
	apkSigner = `"$(ls -d "$ANDROID_HOME"/build-tools/* | sort -V | tail -n 1)/apksigner"`
)

type Tests struct {
	// +private
	App *dagger.Directory
}

func New(
	// Fixture Flutter app.
	// +defaultPath="../testdata/app"
	// +ignore=["build", ".dart_tool", "android/.gradle"]
	app *dagger.Directory,
) *Tests {
	return &Tests{App: app}
}

// All executes all tests.
func (m *Tests) All(ctx context.Context) error {
	p := pool.New().WithErrors().WithContext(ctx)

	p.Go(m.Test)
	p.Go(m.TestFailureReported)
	p.Go(m.Analyze)
	p.Go(m.Format)
	p.Go(m.FormatDetectsUnformatted)
	p.Go(m.ApkDebug)
	p.Go(m.ApkRelease)
	p.Go(m.AppBundleRelease)
	p.Go(m.IntegrationTest)
	p.Go(m.VersionOverride)

	return p.Wait()
}

// Test runs unit/widget tests with coverage and checks the reports.
func (m *Tests) Test(ctx context.Context) error {
	reports := dag.Flutter().Test(m.App, dagger.FlutterTestOpts{Coverage: true})

	junit, err := reports.File("junit.xml").Contents(ctx)
	if err != nil {
		return fmt.Errorf("junit.xml: %w", err)
	}
	for _, name := range []string{"increment adds one", "Counter increments smoke test"} {
		if !strings.Contains(junit, name) {
			return fmt.Errorf("junit.xml does not mention %q:\n%s", name, junit)
		}
	}
	if strings.Contains(junit, "<failure") || strings.Contains(junit, "<error") {
		return fmt.Errorf("junit.xml contains failures:\n%s", junit)
	}
	if strings.Contains(junit, "workspace") {
		return fmt.Errorf("junit.xml leaks the container workspace path:\n%s", junit)
	}

	lcov, err := reports.File("lcov.info").Contents(ctx)
	if err != nil {
		return fmt.Errorf("lcov.info: %w", err)
	}
	if !strings.Contains(lcov, "SF:lib/counter.dart") {
		return fmt.Errorf("lcov.info has no coverage for lib/counter.dart")
	}
	if _, err := reports.File("coverage-html/index.html").Sync(ctx); err != nil {
		return fmt.Errorf("coverage html: %w", err)
	}
	return nil
}

// TestFailureReported checks that a failing test fails Test, and that
// ignoreFailures still returns a JUnit report containing the failure.
func (m *Tests) TestFailureReported(ctx context.Context) error {
	app := m.App.WithNewFile("test/failing_test.dart", `import 'package:flutter_test/flutter_test.dart';

void main() {
  test('deliberately failing', () {
    expect(1, 2);
  });
}
`)
	_, err := dag.Flutter().Test(app).Sync(ctx)
	if err == nil {
		return fmt.Errorf("expected Test to fail for a failing test")
	}
	if !strings.Contains(err.Error(), "deliberately failing") {
		return fmt.Errorf("Test error does not mention the failing test: %w", err)
	}

	junit, err := dag.Flutter().
		Test(app, dagger.FlutterTestOpts{IgnoreFailures: true}).
		File("junit.xml").
		Contents(ctx)
	if err != nil {
		return fmt.Errorf("junit.xml with ignoreFailures: %w", err)
	}
	if !strings.Contains(junit, "<failure") || !strings.Contains(junit, "deliberately failing") {
		return fmt.Errorf("junit.xml does not report the failure:\n%s", junit)
	}
	return nil
}

// Analyze runs flutter analyze with fatal infos.
func (m *Tests) Analyze(ctx context.Context) error {
	_, err := dag.Flutter().Analyze(ctx, m.App, dagger.FlutterAnalyzeOpts{FatalInfos: true})
	return err
}

// Format checks that the fixture is formatted.
func (m *Tests) Format(ctx context.Context) error {
	_, err := dag.Flutter().Format(ctx, m.App)
	return err
}

// FormatDetectsUnformatted checks that the format check fails on badly formatted code.
func (m *Tests) FormatDetectsUnformatted(ctx context.Context) error {
	app := m.App.WithNewFile("lib/ugly.dart", "int   ugly( int a ){return a;}\n")
	_, err := dag.Flutter().Format(ctx, app)
	if err == nil {
		return fmt.Errorf("expected format check to fail for lib/ugly.dart")
	}
	if !strings.Contains(err.Error(), "ugly.dart") {
		return fmt.Errorf("format error does not mention ugly.dart: %w", err)
	}
	return nil
}

// ApkDebug builds a debug APK without any signing configuration.
func (m *Tests) ApkDebug(ctx context.Context) error {
	apk := dag.Flutter().BuildApk(m.App, dagger.FlutterBuildApkOpts{Mode: "debug"})
	name, err := apk.Name(ctx)
	if err != nil {
		return err
	}
	if name != "app-debug.apk" {
		return fmt.Errorf("unexpected apk name %q", name)
	}
	_, err = verifier().
		WithMountedFile("/work/app.apk", apk).
		WithExec([]string{"sh", "-c", apkSigner + " verify /work/app.apk"}).
		Sync(ctx)
	return err
}

// ApkRelease builds a release APK signed with a throwaway keystore and verifies the signature.
func (m *Tests) ApkRelease(ctx context.Context) error {
	keystore, password := keystore()
	apk := dag.Flutter().BuildApk(m.App, dagger.FlutterBuildApkOpts{
		Mode:          "release",
		BuildName:     "1.2.3",
		BuildNumber:   "42",
		Keystore:      keystore,
		StorePassword: password,
		KeyAlias:      keyAlias,
	})
	name, err := apk.Name(ctx)
	if err != nil {
		return err
	}
	if name != "app-release.apk" {
		return fmt.Errorf("unexpected apk name %q", name)
	}
	out, err := verifier().
		WithMountedFile("/work/app.apk", apk).
		WithExec([]string{"sh", "-c", apkSigner + " verify --print-certs /work/app.apk" +
			` && aapt="$(ls -d "$ANDROID_HOME"/build-tools/* | sort -V | tail -n 1)/aapt2"` +
			` && "$aapt" dump badging /work/app.apk | head -n 1`}).
		Stdout(ctx)
	if err != nil {
		return err
	}
	if !strings.Contains(out, signerCN) {
		return fmt.Errorf("release apk is not signed with the test keystore:\n%s", out)
	}
	if !strings.Contains(out, "versionCode='42'") || !strings.Contains(out, "versionName='1.2.3'") {
		return fmt.Errorf("build name/number not applied:\n%s", out)
	}
	return nil
}

// AppBundleRelease builds a release app bundle signed with a throwaway keystore and verifies it.
func (m *Tests) AppBundleRelease(ctx context.Context) error {
	keystore, password := keystore()
	aab := dag.Flutter().BuildAppBundle(m.App, dagger.FlutterBuildAppBundleOpts{
		Keystore:      keystore,
		StorePassword: password,
		KeyAlias:      keyAlias,
	})
	name, err := aab.Name(ctx)
	if err != nil {
		return err
	}
	if name != "app-release.aab" {
		return fmt.Errorf("unexpected bundle name %q", name)
	}
	out, err := verifier().
		WithMountedFile("/work/app.aab", aab).
		WithExec([]string{"jarsigner", "-verify", "-verbose", "-certs", "/work/app.aab"}).
		Stdout(ctx)
	if err != nil {
		return err
	}
	if !strings.Contains(out, "jar verified") || !strings.Contains(out, signerCN) {
		return fmt.Errorf("bundle is not signed with the test keystore:\n%s", tail(out, 30))
	}
	return nil
}

// IntegrationTest runs the fixture's integration_test/ on the Android emulator
// and checks that KVM acceleration was used.
func (m *Tests) IntegrationTest(ctx context.Context) error {
	reports := dag.Flutter().IntegrationTest(m.App)

	boot, err := reports.File("boot.txt").Contents(ctx)
	if err != nil {
		return fmt.Errorf("boot.txt: %w", err)
	}
	if !strings.Contains(boot, "sdk=36") {
		return fmt.Errorf("emulator does not run the default API level 36: %s", boot)
	}
	if !strings.Contains(boot, "accel=on") {
		return fmt.Errorf("emulator did not use KVM: %s", boot)
	}
	junit, err := reports.File("junit.xml").Contents(ctx)
	if err != nil {
		return fmt.Errorf("junit.xml: %w", err)
	}
	if !strings.Contains(junit, "tapping the button increments the counter on device") {
		return fmt.Errorf("junit.xml does not contain the integration test:\n%s", junit)
	}
	if strings.Contains(junit, "<failure") || strings.Contains(junit, "<error") {
		return fmt.Errorf("junit.xml contains failures:\n%s", junit)
	}
	return nil
}

const (
	oldFlutterVersion = "3.41.9"
	oldAndroidVersion = "35"
)

// versionProbe is a minimal project that works on older Flutter SDKs. Its only
// test is named after the SDK version it runs on, so the JUnit report shows
// which SDK Test used.
func versionProbe() *dagger.Directory {
	return dag.Directory().
		WithNewFile("pubspec.yaml", `name: version_probe
publish_to: 'none'
environment:
  sdk: '>=3.0.0 <4.0.0'
dev_dependencies:
  flutter_test:
    sdk: flutter
`).
		WithNewFile("test/version_test.dart", `import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

String sdkVersion() {
  final root = Platform.environment['FLUTTER_ROOT']!;
  final json = File('$root/bin/cache/flutter.version.json');
  if (json.existsSync()) {
    final data = jsonDecode(json.readAsStringSync()) as Map<String, dynamic>;
    return data['frameworkVersion'] as String;
  }
  return File('$root/version').readAsStringSync().trim();
}

void main() {
  final version = sdkVersion();
  test('runs on flutter $version', () {
    expect(version, isNotEmpty);
  });
}
`)
}

// VersionOverride checks that WithFlutterVersion selects the Flutter SDK used by
// the image and by Test/Analyze, and that WithAndroidVersion selects the
// emulator system image and the API level the integration tests run on.
// (AndroidVersion does not affect Android builds: compileSdk/targetSdk come from
// the project and the Flutter Gradle plugin.)
func (m *Tests) VersionOverride(ctx context.Context) error {
	p := pool.New().WithErrors().WithContext(ctx)

	p.Go(func(ctx context.Context) error {
		out, err := dag.Flutter().WithFlutterVersion(oldFlutterVersion).
			FlutterImage().
			WithExec([]string{"flutter", "--version"}).
			Stdout(ctx)
		if err != nil {
			return err
		}
		if !strings.Contains(out, "Flutter "+oldFlutterVersion) {
			return fmt.Errorf("flutter --version does not report %s:\n%s", oldFlutterVersion, out)
		}
		return nil
	})

	p.Go(func(ctx context.Context) error {
		junit, err := dag.Flutter().WithFlutterVersion(oldFlutterVersion).
			Test(versionProbe()).
			File("junit.xml").
			Contents(ctx)
		if err != nil {
			return err
		}
		if !strings.Contains(junit, "runs on flutter "+oldFlutterVersion) {
			return fmt.Errorf("Test did not run on Flutter %s:\n%s", oldFlutterVersion, junit)
		}
		return nil
	})

	p.Go(func(ctx context.Context) error {
		_, err := dag.Flutter().WithFlutterVersion(oldFlutterVersion).
			Analyze(ctx, versionProbe(), dagger.FlutterAnalyzeOpts{FatalInfos: true})
		return err
	})

	p.Go(func(ctx context.Context) error {
		emu := dag.Flutter().WithAndroidVersion(oldAndroidVersion).EmulatorImage()
		env, err := emu.EnvVariable(ctx, "ANDROID_PLATFORM_VERSION")
		if err != nil {
			return err
		}
		if env != oldAndroidVersion {
			return fmt.Errorf("ANDROID_PLATFORM_VERSION is %q, want %q", env, oldAndroidVersion)
		}
		cfg, err := emu.File("/root/.android/avd/emulator.avd/config.ini").Contents(ctx)
		if err != nil {
			return err
		}
		if !strings.Contains(cfg, "system-images/android-"+oldAndroidVersion+"/") {
			return fmt.Errorf("AVD does not use the android-%s system image:\n%s", oldAndroidVersion, cfg)
		}
		return nil
	})

	p.Go(func(ctx context.Context) error {
		boot, err := dag.Flutter().WithAndroidVersion(oldAndroidVersion).
			IntegrationTest(m.App).
			File("boot.txt").
			Contents(ctx)
		if err != nil {
			return err
		}
		if !strings.Contains(boot, "sdk="+oldAndroidVersion) {
			return fmt.Errorf("integration tests did not run on API level %s: %s", oldAndroidVersion, boot)
		}
		return nil
	})

	return p.Wait()
}

// verifier returns the Android SDK image (keytool, jarsigner, apksigner).
func verifier() *dagger.Container {
	return dag.FlutterContainer().Android().WithWorkdir("/work")
}

// keystore generates a throwaway PKCS12 upload keystore.
func keystore() (*dagger.File, *dagger.Secret) {
	password := dag.SetSecret("flutter-test-store-password", "dagger-test-password")
	ks := verifier().
		WithSecretVariable("STORE_PASSWORD", password).
		WithExec([]string{"keytool", "-genkeypair", "-noprompt",
			"-keystore", "/work/upload.p12", "-storetype", "PKCS12",
			"-storepass:env", "STORE_PASSWORD",
			"-alias", keyAlias, "-keyalg", "RSA", "-keysize", "2048", "-validity", "10000",
			"-dname", signerDN}).
		File("/work/upload.p12")
	return ks, password
}

// tail returns the last n lines of s.
func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
