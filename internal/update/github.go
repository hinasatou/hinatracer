package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// DefaultReleasesURL is the GitHub Releases API for the project.
const DefaultReleasesURL = "https://api.github.com/repos/hinasatou/hinatracer/releases"

// ReleasesPage is the human release list.
const ReleasesPage = "https://github.com/hinasatou/hinatracer/releases"

// UserAgent identifies the app to GitHub.
var UserAgent = "HinaTracer-Updater"

// Asset is a release file.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// Release is one GitHub release.
type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	HTMLURL     string    `json:"html_url"`
	Assets      []Asset   `json:"assets"`
}

// Version returns the release version without the leading v.
func (r Release) Version() string { return strings.TrimPrefix(strings.TrimSpace(r.TagName), "v") }

// RateLimitError is returned for HTTP 403/429 rate limiting.
type RateLimitError struct {
	Status int
	Reset  time.Time // zero if unknown
}

func (e *RateLimitError) Error() string {
	if !e.Reset.IsZero() {
		return fmt.Sprintf("GitHub API rate limit (HTTP %d), resets at %s", e.Status, e.Reset.Local().Format("15:04"))
	}
	return fmt.Sprintf("GitHub API rate limit (HTTP %d)", e.Status)
}

// NewHTTPClient returns a client with timeouts that honors proxy env vars.
func NewHTTPClient(timeout time.Duration) *http.Client {
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       60 * time.Second,
	}
	return &http.Client{Transport: tr, Timeout: timeout}
}

// FetchReleases gets the release list (newest first, as GitHub returns it).
func FetchReleases(ctx context.Context, c *http.Client, url string) ([]Release, error) {
	if c == nil {
		c = NewHTTPClient(30 * time.Second)
	}
	if url == "" {
		url = DefaultReleasesURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests ||
		(resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0") {
		e := &RateLimitError{Status: resp.StatusCode}
		var sec int64
		if _, err := fmt.Sscan(resp.Header.Get("X-RateLimit-Reset"), &sec); err == nil && sec > 0 {
			e.Reset = time.Unix(sec, 0)
		}
		return nil, e
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("GitHub API: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var out []Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("GitHub API: %w", err)
	}
	return out, nil
}

// Pick returns the newest release newer than current, honoring the
// channel (includePre) and skipping drafts, unparsable tags and the
// skipped version (when skip is non-empty). ok is false when up to date.
func Pick(releases []Release, current string, includePre bool, skip string) (Release, bool) {
	cur := ParseVersion(current)
	var best Release
	var bestV Version
	found := false
	for _, r := range releases {
		if r.Draft {
			continue
		}
		v := ParseVersion(r.TagName)
		if !v.Valid {
			continue
		}
		if (r.Prerelease || v.IsPrerelease()) && !includePre {
			continue
		}
		if Compare(v, cur) <= 0 {
			continue
		}
		if !found || Compare(v, bestV) > 0 {
			best, bestV, found = r, v, true
		}
	}
	if found && skip != "" && CompareStrings(best.TagName, skip) == 0 && ParseVersion(skip).Valid {
		return Release{}, false
	}
	return best, found
}

// ErrNoAsset means the release has no file for this platform.
var ErrNoAsset = errors.New("no download for this platform in the release")
