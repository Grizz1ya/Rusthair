package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestNewerVersion(t *testing.T) {
	for _, tc := range []struct {
		tag, current string
		want         bool
	}{
		{"v1.10.0", "1.9.9", true},
		{"v2.0.0", "1.99.99", true},
		{"v1.2.1", "1.2.0", true},
		{"1.2.0", "v1.2.0", false},
		{"v1.1.0", "1.2.0", false},
		{"v0.9.99", "1.0.0", false},
	} {
		t.Run(tc.tag+"/"+tc.current, func(t *testing.T) {
			got, err := newerVersion(tc.tag, tc.current)
			if err != nil || got != tc.want {
				t.Fatalf("got %v, %v, want %v", got, err, tc.want)
			}
		})
	}
	for _, bad := range []string{"", "v1.2", "v1.2.3.4", "v1.2.0-rc.1", "v1.2.0+build", "v01.2.0", "v+1.2.0", "v1.2.-1", "v1.2.18446744073709551616", "https://example.com"} {
		if _, err := stableVersion(bad); err == nil {
			t.Errorf("accepted invalid stable tag %q", bad)
		}
	}
}

func TestFetchLatestRelease(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		wantError bool
	}{
		{"stable", 200, `{"tag_name":"v1.10.0","html_url":"https://untrusted.example/"}`, false},
		{"rate limit", 403, `{"message":"rate limit exceeded"}`, true},
		{"unavailable", 503, "", true},
		{"no release", 404, "{}", true},
		{"invalid JSON", 200, "broken", true},
		{"missing tag", 200, "{}", true},
		{"draft", 200, `{"tag_name":"v1.10.0","draft":true}`, true},
		{"prerelease", 200, `{"tag_name":"v1.10.0","prerelease":true}`, true},
		{"release candidate", 200, `{"tag_name":"v1.10.0-rc.1"}`, true},
		{"too large", 200, strings.Repeat(" ", 1<<20) + `{"tag_name":"v1.10.0"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("User-Agent") != "Rusthair/"+appVersion {
					t.Error("missing app user-agent")
				}
				if r.Header.Get("Accept") != "application/vnd.github+json" {
					t.Error("missing API accept header")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			got, err := fetchLatestRelease(context.Background(), server.Client(), server.URL)
			if (err != nil) != tc.wantError {
				t.Fatalf("got %v, error %v", got, err)
			}
			if err == nil && got.pageURL() != "https://github.com/Grizz1ya/Rusthair/releases/tag/v1.10.0" {
				t.Fatalf("untrusted URL used: %q", got.pageURL())
			}
		})
	}
}

func TestUpdateCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := fetchLatestRelease(ctx, http.DefaultClient, latestReleaseAPI)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want canceled", err)
	}
}

func TestWindowsResourceVersion(t *testing.T) {
	data, err := os.ReadFile("winres/winres.json")
	if err != nil {
		t.Fatal(err)
	}
	var resource map[string]map[string]map[string]json.RawMessage
	if err = json.Unmarshal(data, &resource); err != nil {
		t.Fatal(err)
	}
	var version struct {
		Fixed struct {
			File    string `json:"file_version"`
			Product string `json:"product_version"`
		} `json:"fixed"`
		Info map[string]map[string]string `json:"info"`
	}
	if err = json.Unmarshal(resource["RT_VERSION"]["#1"]["0000"], &version); err != nil {
		t.Fatal(err)
	}
	if version.Fixed.File != appVersion+".0" || version.Fixed.Product != appVersion+".0" {
		t.Fatal("fixed EXE resource version differs from update checker")
	}
	if version.Info["0419"]["FileVersion"] != appVersion || version.Info["0419"]["ProductVersion"] != appVersion {
		t.Fatal("display EXE resource version differs from update checker")
	}
}
