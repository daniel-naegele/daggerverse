package main

import (
	"context"

	"dagger/flutter/internal/dagger"
)

// Analyze runs static analysis (flutter analyze) using the project's
// analysis_options.yaml and returns the analyzer output.
//
// Errors always fail the analysis; warnings and infos fail it depending on
// fatalWarnings and fatalInfos.
func (m *Flutter) Analyze(
	ctx context.Context,
	// Flutter project directory.
	// +ignore=["build", ".dart_tool", "android/.gradle", "ios/Pods"]
	project *dagger.Directory,
	// Treat info level issues as fatal.
	// +optional
	fatalInfos bool,
	// Treat warning level issues as fatal.
	// +optional
	// +default=true
	fatalWarnings bool,
	// Pre-built Flutter image to use instead of building one.
	// +optional
	flutterImage *dagger.Container,
) (string, error) {
	args := []string{"flutter", "analyze"}
	if fatalInfos {
		args = append(args, "--fatal-infos")
	} else {
		args = append(args, "--no-fatal-infos")
	}
	if fatalWarnings {
		args = append(args, "--fatal-warnings")
	} else {
		args = append(args, "--no-fatal-warnings")
	}
	return runChecked(ctx, m.flutterCtr(project, flutterImage), "flutter analyze", args)
}
