package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
)

const (
	codemagicPyPIURL           = "https://pypi.org/pypi/codemagic-cli-tools/json"
	bundletoolLatestReleaseURL = "https://github.com/google/bundletool/releases/latest"
)

// LatestCodemagicVersion returns the latest codemagic-cli-tools version published on PyPI.
// +cache="never"
func (m *CodemagicCliTools) LatestCodemagicVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, codemagicPyPIURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch codemagic-cli-tools from PyPI: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch codemagic-cli-tools from PyPI: %s", resp.Status)
	}
	var body struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decode PyPI response: %w", err)
	}
	if body.Info.Version == "" {
		return "", fmt.Errorf("no version in PyPI response")
	}
	return body.Info.Version, nil
}

// LatestBundletoolVersion returns the latest bundletool release version from GitHub.
//
// Resolves the github.com/google/bundletool/releases/latest redirect instead of the REST API to avoid
// unauthenticated API rate limits.
// +cache="never"
func (m *CodemagicCliTools) LatestBundletoolVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, bundletoolLatestReleaseURL, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("resolve latest bundletool release: %w", err)
	}
	defer resp.Body.Close()
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "/releases/tag/") {
		return "", fmt.Errorf("resolve latest bundletool release: unexpected response %s (Location %q)", resp.Status, loc)
	}
	return strings.TrimPrefix(path.Base(loc), "v"), nil
}
