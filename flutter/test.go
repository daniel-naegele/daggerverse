package main

import (
	"context"

	"dagger/flutter/internal/dagger"
)

// testScript runs flutter test with a human-readable reporter on stdout plus a
// JSON file reporter, converts the JSON report to JUnit XML and optionally
// exports lcov coverage (raw and as HTML).
const testScript = `set -u
coverage="$1"; shift
flags=""
if [ "$coverage" = "true" ]; then flags="--coverage"; fi

flutter test --reporter expanded --file-reporter "json:` + reportsDir + `/test-results.json" $flags "$@"
status=$?

if [ -s ` + reportsDir + `/test-results.json ]; then
  dart pub global run junitreport:tojunit \
    --input ` + reportsDir + `/test-results.json \
    --output ` + reportsDir + `/junit.xml \
    --base ` + workspaceDir + `/ || status=1
fi

if [ "$coverage" = "true" ] && [ -f coverage/lcov.info ]; then
  cp coverage/lcov.info ` + reportsDir + `/lcov.info
  genhtml --quiet coverage/lcov.info --output-directory ` + reportsDir + `/coverage-html || true
fi

exit $status
`

// Test runs the project's unit and widget tests (flutter test).
//
// Returns a reports directory containing:
//   - test-results.json: Dart JSON reporter output
//   - junit.xml: JUnit XML report (for GitLab/GitHub test reporting)
//   - lcov.info and coverage-html/: coverage, when coverage is enabled
//
// Fails when a test fails, unless ignoreFailures is set.
func (m *Flutter) Test(
	ctx context.Context,
	// Flutter project directory.
	// +ignore=["build", ".dart_tool", "android/.gradle", "ios/Pods"]
	project *dagger.Directory,
	// Test files or directories to run (defaults to the test/ directory).
	// +optional
	targets []string,
	// Collect coverage and export lcov.info plus an HTML report.
	// +optional
	coverage bool,
	// Return the reports even when tests fail.
	// +optional
	ignoreFailures bool,
	// Pre-built Flutter image to use instead of building one (e.g. ghcr.io/daniel-naegele/flutter:<version>).
	// +optional
	flutterImage *dagger.Container,
) (*dagger.Directory, error) {
	ctr := withJunitReport(m.flutterCtr(project, flutterImage))
	cov := "false"
	if coverage {
		cov = "true"
	}
	return runReporting(ctx, ctr, "flutter test", testScript, append([]string{cov}, targets...), ignoreFailures)
}
