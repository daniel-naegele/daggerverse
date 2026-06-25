package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"
)

const flutterReleasesURL = "https://storage.googleapis.com/flutter_infra_release/releases/releases_linux.json"

type flutterReleasesJSON struct {
	CurrentRelease struct {
		Stable string `json:"stable"`
	} `json:"current_release"`
	Releases []struct {
		Hash        string `json:"hash"`
		Channel     string `json:"channel"`
		Version     string `json:"version"`
		ReleaseDate string `json:"release_date"`
	} `json:"releases"`
}

func fetchFlutterReleases() (*flutterReleasesJSON, error) {
	resp, err := http.Get(flutterReleasesURL)
	if err != nil {
		return nil, fmt.Errorf("fetch flutter releases: %w", err)
	}
	defer resp.Body.Close()
	var rel flutterReleasesJSON
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode flutter releases: %w", err)
	}
	return &rel, nil
}

// LatestVersion returns the current latest stable Flutter version string.
func (m *FlutterContainer) LatestVersion(_ context.Context) (string, error) {
	rel, err := fetchFlutterReleases()
	if err != nil {
		return "", err
	}
	stableHash := rel.CurrentRelease.Stable
	for _, r := range rel.Releases {
		if r.Hash == stableHash {
			return r.Version, nil
		}
	}
	return "", fmt.Errorf("no release found for stable hash %s", stableHash)
}

// StableVersionsSince returns stable Flutter versions released in the last two years, oldest-first.
func (m *FlutterContainer) StableVersionsSince(_ context.Context) ([]string, error) {
	return stableVersionsSince(time.Now().AddDate(-2, 0, 0))
}

// stableVersionsSince returns stable Flutter versions released after the given time, sorted oldest-first.
func stableVersionsSince(since time.Time) ([]string, error) {
	rel, err := fetchFlutterReleases()
	if err != nil {
		return nil, err
	}
	type entry struct {
		version string
		date    time.Time
	}
	var entries []entry
	for _, r := range rel.Releases {
		if r.Channel != "stable" {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, r.ReleaseDate)
		if err != nil {
			t, err = time.Parse(time.RFC3339, r.ReleaseDate)
			if err != nil {
				continue
			}
		}
		if t.After(since) {
			entries = append(entries, entry{r.Version, t})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].date.Before(entries[j].date)
	})
	versions := make([]string, len(entries))
	for i, e := range entries {
		versions[i] = e.version
	}
	return versions, nil
}
