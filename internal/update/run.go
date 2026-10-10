package update

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Plan describes how this copy of the app updates itself.
type Plan struct {
	GOOS, GOARCH string
	Exe          string // running executable
	Bundle       string // macOS .app root
	Kind         Kind
}

// MakePlan inspects the running executable to choose the package kind.
func MakePlan(exe, goos, goarch string) Plan {
	p := Plan{GOOS: goos, GOARCH: goarch, Exe: exe}
	switch goos {
	case "windows":
		p.Kind = KindFor(goos, IsNSISInstall(exe), false)
	case "darwin":
		p.Bundle = BundleRoot(exe)
		p.Kind = KindMacZip
	case "linux":
		p.Kind = KindFor(goos, false, IsSystemInstall(exe))
	}
	return p
}

// Outcome is what the app must do after Run succeeds.
type Outcome struct {
	// Action: "relaunch" (start Launch and quit), "setup" (run the
	// installer Launch silently, then start Exe, and quit), "saved" (a
	// package was saved to Launch for the user to install).
	Action string
	Launch string
}

// ErrNeedManual means self-update is not possible here (e.g. not writable);
// the UI should offer the release page.
var ErrNeedManual = errors.New("automatic update is not possible for this installation")

// DownloadsDir returns ~/Downloads (or the temp dir).
func DownloadsDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		d := filepath.Join(h, "Downloads")
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d
		}
	}
	return os.TempDir()
}

// Run downloads, verifies and installs rel according to plan. workDir is a
// scratch directory (created). Verification against SHA256SUMS.txt is
// mandatory; a mismatch aborts before anything is replaced.
func Run(ctx context.Context, c *http.Client, rel Release, plan Plan, workDir string, onProgress func(Progress)) (Outcome, error) {
	if plan.Kind == "" {
		return Outcome{}, ErrNeedManual
	}
	asset, err := SelectAsset(rel, plan.Kind, plan.GOARCH)
	if err != nil {
		return Outcome{}, err
	}
	sumsAsset, ok := FindAsset(rel, "SHA256SUMS.txt")
	if !ok {
		return Outcome{}, &ErrChecksum{Name: "SHA256SUMS.txt"}
	}
	// Check writability before spending time on the download.
	switch plan.Kind {
	case KindWinZip, KindLinuxTGZ:
		if !DirWritable(filepath.Dir(plan.Exe)) {
			return Outcome{}, ErrNeedManual
		}
	case KindMacZip:
		if plan.Bundle == "" || !DirWritable(filepath.Dir(plan.Bundle)) {
			return Outcome{}, ErrNeedManual
		}
	}
	sumsText, err := FetchText(ctx, c, sumsAsset.URL)
	if err != nil {
		return Outcome{}, err
	}
	sums := ParseSums(sumsText)
	dlDir := workDir
	if plan.Kind == KindLinuxDeb {
		dlDir = DownloadsDir()
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return Outcome{}, err
	}
	path, err := Download(ctx, c, asset.URL, dlDir, asset.Name, onProgress)
	if err != nil {
		return Outcome{}, err
	}
	if err := Verify(sums, asset.Name, path); err != nil {
		os.Remove(path)
		return Outcome{}, err
	}
	if err := ctx.Err(); err != nil {
		return Outcome{}, err
	}
	switch plan.Kind {
	case KindWinSetup:
		return Outcome{Action: "setup", Launch: path}, nil
	case KindLinuxDeb:
		return Outcome{Action: "saved", Launch: path}, nil
	case KindWinZip:
		// Stage next to the exe (same volume) so the swap is a rename.
		staged := filepath.Join(filepath.Dir(plan.Exe), ".hinatracer-update.exe")
		if err := ExtractZipFile(path, []string{"HinaTracer.exe", "hinatracer.exe"}, staged); err != nil {
			return Outcome{}, err
		}
		if err := ReplaceFile(plan.Exe, staged); err != nil {
			os.Remove(staged)
			return Outcome{}, err
		}
		return Outcome{Action: "relaunch", Launch: plan.Exe}, nil
	case KindLinuxTGZ:
		staged := filepath.Join(filepath.Dir(plan.Exe), ".hinatracer-update")
		if err := ExtractTarGzFile(path, "hinatracer", staged); err != nil {
			return Outcome{}, err
		}
		if err := ReplaceFile(plan.Exe, staged); err != nil {
			os.Remove(staged)
			return Outcome{}, err
		}
		// A running binary on Linux can be unlinked; drop the old copy now.
		CleanupOld(plan.Exe)
		return Outcome{Action: "relaunch", Launch: plan.Exe}, nil
	case KindMacZip:
		stage, err := os.MkdirTemp(filepath.Dir(plan.Bundle), ".hinatracer-update-")
		if err != nil {
			return Outcome{}, err
		}
		app, err := ExtractZipApp(path, stage)
		if err != nil {
			os.RemoveAll(stage)
			return Outcome{}, err
		}
		if err := ReplaceDir(plan.Bundle, app); err != nil {
			os.RemoveAll(stage)
			return Outcome{}, err
		}
		os.RemoveAll(stage)
		return Outcome{Action: "relaunch", Launch: plan.Bundle}, nil
	}
	return Outcome{}, fmt.Errorf("unsupported package kind %q", plan.Kind)
}

// PSQuote quotes s as a PowerShell single-quoted string.
func PSQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// SetupScript is the PowerShell helper that waits for the app to exit,
// runs the NSIS installer silently (/S) into the folder the app currently
// lives in (/D=, which NSIS requires last and unquoted), and starts the
// updated app from that same folder.
func SetupScript(setup, exe string, pid int) string {
	args := "/S /D=" + winDir(exe)
	return fmt.Sprintf("Wait-Process -Id %d -Timeout 60 -ErrorAction SilentlyContinue; "+
		"Start-Process -Wait -WindowStyle Hidden -FilePath %s -ArgumentList %s; Start-Process -FilePath %s",
		pid, PSQuote(setup), PSQuote(args), PSQuote(exe))
}

// winDir returns the directory part of a Windows path without a trailing
// separator, independent of the host OS (so it is testable everywhere).
func winDir(p string) string {
	i := strings.LastIndexAny(p, `\/`)
	if i <= 0 {
		return p
	}
	d := p[:i]
	if len(d) == 2 && d[1] == ':' {
		d += `\`
	}
	return d
}
