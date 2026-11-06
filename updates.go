package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// The Windows version resource is checked against this value by the tests.
var appVersion = "1.2.0"

const latestReleaseAPI = "https://api.github.com/repos/Grizz1ya/Rusthair/releases/latest"
const releasePage = "https://github.com/Grizz1ya/Rusthair/releases/tag/"
const updateInterval = 6 * time.Hour

type releaseInfo struct {
	Tag        string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

func stableVersion(tag string) ([3]uint64, error) {
	var version [3]uint64
	parts := strings.Split(strings.TrimPrefix(tag, "v"), ".")
	if len(parts) != 3 {
		return version, fmt.Errorf("unsupported version %q", tag)
	}
	for i, part := range parts {
		if part == "" || len(part) > 1 && part[0] == '0' {
			return version, fmt.Errorf("invalid version %q", tag)
		}
		for _, digit := range part {
			if digit < '0' || digit > '9' {
				return version, fmt.Errorf("invalid version %q", tag)
			}
		}
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return version, err
		}
		version[i] = n
	}
	return version, nil
}

func newerVersion(tag, current string) (bool, error) {
	candidate, err := stableVersion(tag)
	if err != nil {
		return false, err
	}
	installed, err := stableVersion(current)
	if err != nil {
		return false, err
	}
	for i := range candidate {
		if candidate[i] != installed[i] {
			return candidate[i] > installed[i], nil
		}
	}
	return false, nil
}

func fetchLatestRelease(ctx context.Context, client *http.Client, endpoint string) (releaseInfo, error) {
	var release releaseInfo
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return release, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "Rusthair/"+appVersion)
	response, err := client.Do(request)
	if err != nil {
		return release, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return release, fmt.Errorf("GitHub HTTP %d", response.StatusCode)
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&release); err != nil {
		return release, err
	}
	if release.Draft || release.Prerelease {
		return release, fmt.Errorf("release is not stable")
	}
	if _, err = stableVersion(release.Tag); err != nil {
		return release, err
	}
	return release, nil
}

func (r releaseInfo) pageURL() string { return releasePage + url.PathEscape(r.Tag) }
