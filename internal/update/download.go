package update

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Progress reports download progress.
type Progress struct {
	Name     string
	Bytes    int64
	Total    int64 // -1 when unknown
	SpeedBPS float64
}

// Download fetches url into dir/name (written to name+".part" then renamed).
// onProgress is throttled to ~10/s.
func Download(ctx context.Context, c *http.Client, url, dir, name string, onProgress func(Progress)) (string, error) {
	if c == nil {
		c = NewHTTPClient(0)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %d", name, resp.StatusCode)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	final := filepath.Join(dir, name)
	part := final + ".part"
	f, err := os.Create(part)
	if err != nil {
		return "", err
	}
	total := resp.ContentLength
	start := time.Now()
	var last time.Time
	var n int64
	buf := make([]byte, 64<<10)
	for {
		k, rerr := resp.Body.Read(buf)
		if k > 0 {
			if _, werr := f.Write(buf[:k]); werr != nil {
				f.Close()
				os.Remove(part)
				return "", werr
			}
			n += int64(k)
			if onProgress != nil && time.Since(last) > 100*time.Millisecond {
				last = time.Now()
				onProgress(Progress{Name: name, Bytes: n, Total: total, SpeedBPS: float64(n) / time.Since(start).Seconds()})
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			os.Remove(part)
			return "", rerr
		}
	}
	if err := f.Close(); err != nil {
		os.Remove(part)
		return "", err
	}
	if total > 0 && n != total {
		os.Remove(part)
		return "", fmt.Errorf("download %s: got %d of %d bytes", name, n, total)
	}
	if onProgress != nil {
		onProgress(Progress{Name: name, Bytes: n, Total: total, SpeedBPS: float64(n) / time.Since(start).Seconds()})
	}
	if err := os.Rename(part, final); err != nil {
		os.Remove(part)
		return "", err
	}
	return final, nil
}

// FetchText downloads a small text file (e.g. SHA256SUMS.txt).
func FetchText(ctx context.Context, c *http.Client, url string) (string, error) {
	if c == nil {
		c = NewHTTPClient(60 * time.Second)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return string(b), err
}
