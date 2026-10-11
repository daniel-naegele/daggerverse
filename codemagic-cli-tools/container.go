package main

import (
	"fmt"
	"strconv"
	"strings"

	"dagger/codemagic-cli-tools/internal/dagger"
)

// javaModules is the set of JDK modules linked into the minimal Java runtime.
// java.base provides java + keytool, jdk.jartool provides jarsigner, the rest
// cover what bundletool loads reflectively (XML/XPath for dump, logging, zipfs, crypto).
var javaModules = []string{
	"java.base",
	"java.logging",
	"java.management",
	"java.naming",
	"java.sql",
	"java.xml",
	"jdk.charsets",
	"jdk.crypto.cryptoki",
	"jdk.crypto.ec",
	"jdk.jartool",
	"jdk.unsupported",
	"jdk.zipfs",
}

// googleAPIDiscoveryKeep lists the googleapiclient static discovery documents
// codemagic-cli-tools actually uses (google-play, firebase-app-distribution);
// all others are removed to keep the image small.
var googleAPIDiscoveryKeep = []string{"androidpublisher.*", "firebaseappdistribution.*", "index.json"}

// Container returns the codemagic-cli-tools container.
//
// Contents: python:<PythonVersion>-slim-trixie, codemagic-cli-tools==<CodemagicVersion>,
// a jlink'ed Temurin <JavaVersion> runtime (java, keytool, jarsigner) at /opt/java, bundletool
// <BundletoolVersion> at /opt/bundletool/bundletool.jar (with a `bundletool` wrapper on PATH, also
// used by codemagic in place of its bundled jar) and git. Runs as the non-root user "codemagic"
// (uid 1000) in /workspace.
//
// Note: bundletool ships an x86_64-only aapt2, so APK generation from bundles (build-apks,
// build-universal-apk) only works on linux/amd64.
func (m *CodemagicCliTools) Container(
	// Target platform (e.g. "linux/amd64"). Defaults to the engine's native platform.
	// +optional
	platform dagger.Platform,
) *dagger.Container {
	jar := dag.HTTP(fmt.Sprintf(
		"https://github.com/google/bundletool/releases/download/%s/bundletool-all-%s.jar",
		m.BundletoolVersion, m.BundletoolVersion,
	))

	keep := make([]string, 0, len(googleAPIDiscoveryKeep))
	for _, k := range googleAPIDiscoveryKeep {
		keep = append(keep, fmt.Sprintf("! -name '%s'", k))
	}

	return newContainer(platform).
		From(fmt.Sprintf("python:%s-slim-trixie", m.PythonVersion)).
		WithEnvVariable("PIP_NO_CACHE_DIR", "1").
		WithEnvVariable("PIP_DISABLE_PIP_VERSION_CHECK", "1").
		WithEnvVariable("PIP_ROOT_USER_ACTION", "ignore").
		WithEnvVariable("PYTHONDONTWRITEBYTECODE", "1").
		WithExec([]string{"sh", "-c", strings.Join([]string{
			"apt-get update",
			"apt-get install -y --no-install-recommends git",
			"rm -rf /var/lib/apt/lists/*",
			"git config --system --add safe.directory '*'",
		}, " && ")}).
		WithDirectory(javaHome, m.javaRuntime(platform)).
		WithEnvVariable("JAVA_HOME", javaHome).
		WithEnvVariable("PATH", javaHome+"/bin:${PATH}", dagger.ContainerWithEnvVariableOpts{Expand: true}).
		WithFile(bundletoolJar, jar, dagger.ContainerWithFileOpts{Permissions: 0o644}).
		WithNewFile("/usr/local/bin/bundletool",
			"#!/bin/sh\nexec java -jar "+bundletoolJar+" \"$@\"\n",
			dagger.ContainerWithNewFileOpts{Permissions: 0o755}).
		WithEnvVariable("ANDROID_APP_BUNDLE_BUNDLETOOL", bundletoolJar).
		// Install and prune in a single layer so removed files never end up in the image.
		WithExec([]string{"sh", "-c", strings.Join([]string{
			"pip install codemagic-cli-tools==" + m.CodemagicVersion,
			// Replace the bundletool jar vendored by codemagic with a symlink to the pinned one, so every
			// code path (including google-play's internal App Bundle parsing) uses the same bundletool.
			`jars="$(python -c 'import codemagic, pathlib; print(pathlib.Path(codemagic.__file__).parent / "data" / "jars")')"`,
			`rm -f "$jars"/bundletool*.jar`,
			`ln -s ` + bundletoolJar + ` "$jars/bundletool.jar"`,
			// Drop unused Google API discovery documents (~100 MB).
			`disc="$(python -c 'import googleapiclient, pathlib; print(pathlib.Path(googleapiclient.__file__).parent / "discovery_cache" / "documents")')"`,
			`find "$disc" -type f ` + strings.Join(keep, " ") + ` -delete`,
		}, " && ")}).
		WithExec([]string{"sh", "-c", strings.Join([]string{
			"useradd --create-home --uid " + uid + " --shell /bin/sh " + user,
			"mkdir -p " + workspaceDir + " " + secretsDir,
			"chown " + user + ":" + user + " " + workspaceDir + " " + secretsDir,
		}, " && ")}).
		WithLabel("org.opencontainers.image.title", "codemagic-cli-tools").
		WithLabel("org.opencontainers.image.description", "Codemagic CLI tools with Java runtime, bundletool and git").
		WithLabel("org.opencontainers.image.source", "https://github.com/daniel-naegele/daggerverse").
		WithLabel("org.opencontainers.image.version", m.CodemagicVersion).
		WithLabel("dev.naegele.bundletool.version", m.BundletoolVersion).
		WithUser(user).
		WithWorkdir(workspaceDir).
		WithDefaultArgs([]string{"codemagic-cli-tools", "--help"})
}

// javaRuntime builds a minimal Java runtime image with jlink from the Temurin JDK.
func (m *CodemagicCliTools) javaRuntime(platform dagger.Platform) *dagger.Directory {
	return newContainer(platform).
		From(fmt.Sprintf("eclipse-temurin:%s-jdk-noble", m.JavaVersion)).
		WithExec([]string{
			"jlink",
			"--add-modules", strings.Join(javaModules, ","),
			"--strip-debug",
			"--no-man-pages",
			"--no-header-files",
			"--compress=" + jlinkCompression(m.JavaVersion),
			"--output", "/javaruntime",
		}).
		Directory("/javaruntime")
}

// jlinkCompression returns the strongest jlink --compress value supported by the given JDK major version:
// JDK 21+ takes zip-<level> (numeric levels are deprecated), older JDKs only know 0-2.
func jlinkCompression(javaVersion string) string {
	major, err := strconv.Atoi(strings.SplitN(javaVersion, ".", 2)[0])
	if err == nil && major < 21 {
		return "2"
	}
	return "zip-6"
}

// newContainer returns an empty container for the given platform (native if empty).
func newContainer(platform dagger.Platform) *dagger.Container {
	if platform == "" {
		return dag.Container()
	}
	return dag.Container(dagger.ContainerOpts{Platform: platform})
}
