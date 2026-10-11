package main

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"

	"dagger/codemagic-cli-tools/internal/dagger"
)

// envGooglePlayCredentials is the environment variable codemagic reads the service account JSON from.
const envGooglePlayCredentials = "GOOGLE_PLAY_SERVICE_ACCOUNT_CREDENTIALS"

// GooglePlay wraps the `google-play` tool (Google Play Developer API) authenticated with a service account.
type GooglePlay struct {
	// +private
	Ctr *dagger.Container
}

// GooglePlay returns a `google-play` wrapper authenticated with the given service account credentials.
func (m *CodemagicCliTools) GooglePlay(
	// Google Play service account credentials (JSON key file contents).
	credentials *dagger.Secret,
) *GooglePlay {
	return &GooglePlay{
		Ctr: m.base().WithSecretVariable(envGooglePlayCredentials, credentials),
	}
}

func (g *GooglePlay) run(ctx context.Context, args ...string) (string, error) {
	return output(ctx, g.Ctr, append([]string{"google-play"}, args...))
}

func (g *GooglePlay) withBundle(ctx context.Context, bundle *dagger.File) (*GooglePlay, string, error) {
	name, err := bundle.Name(ctx)
	if err != nil {
		return nil, "", err
	}
	p := path.Join(workspaceDir, name)
	return &GooglePlay{Ctr: g.Ctr.WithFile(p, bundle, dagger.ContainerWithFileOpts{Owner: user})}, p, nil
}

// LatestBuildNumber returns the highest version code of the app on Google Play, optionally restricted to the
// given tracks (`google-play get-latest-build-number`).
func (g *GooglePlay) LatestBuildNumber(
	ctx context.Context,
	// Application package name, e.g. "com.example.app".
	packageName string,
	// Only consider these tracks (e.g. ["internal", "production"]). Defaults to all tracks.
	// +optional
	tracks []string,
) (int, error) {
	args := []string{"get-latest-build-number", "--package-name", packageName}
	if len(tracks) > 0 {
		args = append(append(args, "--tracks"), tracks...)
	}
	out, err := g.run(ctx, args...)
	if err != nil {
		return 0, err
	}
	lines := strings.Split(out, "\n")
	n, err := strconv.Atoi(strings.TrimSpace(lines[len(lines)-1]))
	if err != nil {
		return 0, fmt.Errorf("unexpected get-latest-build-number output %q: %w", out, err)
	}
	return n, nil
}

// PublishBundle uploads an App Bundle and releases it to a track (`google-play bundles publish`).
// The package name is read from the bundle. Returns the release information as JSON.
func (g *GooglePlay) PublishBundle(
	ctx context.Context,
	// Android App Bundle (.aab) to publish.
	bundle *dagger.File,
	// Release track, e.g. "internal", "alpha", "beta" or "production".
	track string,
	// Release name. Defaults to the bundle's versionName.
	// +optional
	releaseName string,
	// Localised release notes as JSON, e.g. '[{"language": "en-US", "text": "Bug fixes"}]'.
	// +optional
	releaseNotes string,
	// In-app update priority in range [0, 5]. -1 leaves it unset.
	// +optional
	// +default=-1
	inAppUpdatePriority int,
	// Staged rollout user fraction in range (0, 1). 0 leaves it unset (full rollout).
	// +optional
	rolloutFraction float64,
	// Create the release as a draft (mutually exclusive with rolloutFraction).
	// +optional
	draft bool,
	// Do not send the changes for review automatically.
	// +optional
	changesNotSentForReview bool,
) (string, error) {
	gp, p, err := g.withBundle(ctx, bundle)
	if err != nil {
		return "", err
	}
	args := []string{"bundles", "publish", "--json", "--bundle", p, "--track", track}
	args = append(args, releaseArgs(releaseName, releaseNotes, inAppUpdatePriority, rolloutFraction, draft)...)
	if changesNotSentForReview {
		args = append(args, "--changes-not-sent-for-review")
	}
	return gp.run(ctx, args...)
}

// UploadBundle uploads an App Bundle without releasing it to a track (`google-play bundles upload`).
// Returns the uploaded bundle information as JSON.
func (g *GooglePlay) UploadBundle(
	ctx context.Context,
	// Android App Bundle (.aab) to upload.
	bundle *dagger.File,
) (string, error) {
	gp, p, err := g.withBundle(ctx, bundle)
	if err != nil {
		return "", err
	}
	return gp.run(ctx, "bundles", "upload", "--json", "--bundle", p)
}

// UploadToInternalAppSharing uploads an App Bundle to Internal App Sharing
// (`google-play internal-app-sharing upload-bundle`). Returns the artifact (incl. download URL) as JSON.
func (g *GooglePlay) UploadToInternalAppSharing(
	ctx context.Context,
	// Android App Bundle (.aab) to upload.
	bundle *dagger.File,
) (string, error) {
	gp, p, err := g.withBundle(ctx, bundle)
	if err != nil {
		return "", err
	}
	return gp.run(ctx, "internal-app-sharing", "upload-bundle", "--json", "--bundle", p)
}

// PromoteRelease promotes a release from one track to another (`google-play tracks promote-release`).
// Returns the updated target track as JSON.
func (g *GooglePlay) PromoteRelease(
	ctx context.Context,
	// Application package name, e.g. "com.example.app".
	packageName string,
	// Track to promote from, e.g. "internal".
	sourceTrack string,
	// Track to promote to, e.g. "production".
	targetTrack string,
	// Status of the promoted release: completed, inProgress, halted or draft.
	// +optional
	// +default="completed"
	releaseStatus string,
	// User fraction in range (0, 1) for staged releases (status inProgress or halted). 0 leaves it unset.
	// +optional
	userFraction float64,
	// Promote only the source release containing this version code.
	// +optional
	versionCodeFilter string,
	// Promote only a source release with this status.
	// +optional
	releaseStatusFilter string,
) (string, error) {
	args := []string{
		"tracks", "promote-release", "--json",
		"--package-name", packageName,
		"--source-track", sourceTrack,
		"--target-track", targetTrack,
	}
	if releaseStatus != "" {
		args = append(args, "--release-status", releaseStatus)
	}
	if userFraction > 0 {
		args = append(args, "--user-fraction", strconv.FormatFloat(userFraction, 'f', -1, 64))
	}
	if versionCodeFilter != "" {
		args = append(args, "--version-code-filter", versionCodeFilter)
	}
	if releaseStatusFilter != "" {
		args = append(args, "--release-status-filter", releaseStatusFilter)
	}
	return g.run(ctx, args...)
}

// GetTrack returns information about a release track as JSON (`google-play tracks get`).
func (g *GooglePlay) GetTrack(
	ctx context.Context,
	// Application package name, e.g. "com.example.app".
	packageName string,
	// Track name, e.g. "internal".
	track string,
) (string, error) {
	return g.run(ctx, "tracks", "get", "--json", "--package-name", packageName, "--track", track)
}

// ListTracks returns information about all release tracks as JSON (`google-play tracks list`).
func (g *GooglePlay) ListTracks(
	ctx context.Context,
	// Application package name, e.g. "com.example.app".
	packageName string,
) (string, error) {
	return g.run(ctx, "tracks", "list", "--json", "--package-name", packageName)
}

func releaseArgs(releaseName, releaseNotes string, inAppUpdatePriority int, rolloutFraction float64, draft bool) []string {
	var args []string
	if releaseName != "" {
		args = append(args, "--release-name", releaseName)
	}
	if releaseNotes != "" {
		args = append(args, "--release-notes", releaseNotes)
	}
	if inAppUpdatePriority >= 0 {
		args = append(args, "--in-app-update-priority", strconv.Itoa(inAppUpdatePriority))
	}
	if rolloutFraction > 0 {
		args = append(args, "--rollout-fraction", strconv.FormatFloat(rolloutFraction, 'f', -1, 64))
	}
	if draft {
		args = append(args, "--draft")
	}
	return args
}
