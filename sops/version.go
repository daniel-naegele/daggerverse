package main

import (
	"context"
	"fmt"

	"dagger/sops/internal/dagger"
)

// latestRelease returns the tag of the latest GitHub release of repo, without a "v" prefix.
func latestRelease(ctx context.Context, repo string, token *dagger.Secret) (string, error) {
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := githubAPI(ctx, "https://api.github.com/repos/"+repo+"/releases/latest", token, &rel); err != nil {
		return "", fmt.Errorf("fetch latest %s release: %w", repo, err)
	}
	if rel.TagName == "" {
		return "", fmt.Errorf("latest %s release has no tag", repo)
	}
	return trimV(rel.TagName), nil
}

// LatestSopsVersion returns the latest released sops version (e.g. "3.13.3").
func (m *Sops) LatestSopsVersion(
	ctx context.Context,
	// GitHub token to avoid API rate limits.
	// +optional
	token *dagger.Secret,
) (string, error) {
	return latestRelease(ctx, "getsops/sops", token)
}

// LatestAgeVersion returns the latest released age version (e.g. "1.3.2").
func (m *Sops) LatestAgeVersion(
	ctx context.Context,
	// GitHub token to avoid API rate limits.
	// +optional
	token *dagger.Secret,
) (string, error) {
	return latestRelease(ctx, "FiloSottile/age", token)
}

// LatestSshToAgeVersion returns the latest released ssh-to-age version (e.g. "1.3.0").
func (m *Sops) LatestSshToAgeVersion(
	ctx context.Context,
	// GitHub token to avoid API rate limits.
	// +optional
	token *dagger.Secret,
) (string, error) {
	return latestRelease(ctx, "Mic92/ssh-to-age", token)
}
