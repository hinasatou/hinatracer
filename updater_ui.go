package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/update"
)

// Update checker state (guarded by updMu; written by workers, read by view).
type updState struct {
	mu        sync.Mutex
	phase     string // "" idle | checking | uptodate | available | error | downloading | ready
	rel       update.Release
	err       string
	manual    bool // last check was from the About button
	prompt    bool // show the update panel
	progress  update.Progress
	cancel    context.CancelFunc
	savedPath string // Linux .deb saved for the user
	startedAt bool   // startup check already ran this launch

	notesScroll ui.ScrollState
	chanName    string
	checkOnRun  bool
	inited      bool
}

// updReleasesURL is overridable in tests.
var updReleasesURL = update.DefaultReleasesURL

// updRelaunch starts the updated app; replaced in tests.
var updRelaunch = relaunchAfterUpdate

// updQuit quits the app after launching the update; replaced in tests.
var updQuit = func(a *app) {}

// updView is a lock-free copy of updState for the view.
type updView struct {
	phase     string
	rel       update.Release
	err       string
	manual    bool
	prompt    bool
	progress  update.Progress
	savedPath string
}

func (a *app) updSnapshot() updView {
	a.upd.mu.Lock()
	defer a.upd.mu.Unlock()
	return updView{phase: a.upd.phase, rel: a.upd.rel, err: a.upd.err, manual: a.upd.manual,
		prompt: a.upd.prompt, progress: a.upd.progress, savedPath: a.upd.savedPath}
}

func (a *app) updSet(fn func(s *updState)) {
	a.upd.mu.Lock()
	fn(&a.upd)
	a.upd.mu.Unlock()
	a.requestUIUpdate()
}

// scheduleStartupUpdateCheck runs one check a few seconds after launch
// (never for dev builds, never when disabled).
func (a *app) scheduleStartupUpdateCheck(delay time.Duration) {
	if !a.cfg.CheckUpdatesOnStart() || update.IsDevVersion(appVersion) {
		return
	}
	a.upd.mu.Lock()
	if a.upd.startedAt {
		a.upd.mu.Unlock()
		return
	}
	a.upd.startedAt = true
	a.upd.mu.Unlock()
	pre, skip := a.cfg.IncludePrereleases(), a.cfg.UpdateSkipVersion
	safeGo("update.startup", func() {
		time.Sleep(delay)
		a.checkForUpdates(false, pre, skip)
	})
}

// checkForUpdates queries GitHub (worker goroutine). A manual check
// ignores the skipped version and always reports the result.
func (a *app) checkForUpdates(manual, includePre bool, skip string) {
	a.updSet(func(s *updState) {
		s.phase, s.err, s.manual = "checking", "", manual
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rels, err := update.FetchReleases(ctx, update.NewHTTPClient(30*time.Second), updReleasesURL)
	if err != nil {
		msg := err.Error()
		var rl *update.RateLimitError
		if errors.As(err, &rl) {
			msg = i18n.T("update.err.ratelimit")
		}
		a.updSet(func(s *updState) { s.phase, s.err = "error", msg })
		return
	}
	if manual {
		skip = ""
	}
	rel, ok := update.Pick(rels, appVersion, includePre, skip)
	a.updSet(func(s *updState) {
		if !ok {
			s.phase = "uptodate"
			return
		}
		s.phase, s.rel, s.prompt = "available", rel, true
	})
}

func (a *app) skipUpdateVersion(v string) {
	a.cfg.UpdateSkipVersion = v
	a.persist()
	a.updSet(func(s *updState) { s.prompt = false; s.phase = "" })
}

// startUpdateInstall downloads, verifies and installs (worker goroutine).
func (a *app) startUpdateInstall() {
	snap := a.updSnapshot()
	if snap.phase == "downloading" {
		return
	}
	exe, err := os.Executable()
	if err == nil {
		if r, e := filepath.EvalSymlinks(exe); e == nil {
			exe = r
		}
	}
	if err != nil {
		a.updSet(func(s *updState) { s.phase, s.err = "error", err.Error() })
		return
	}
	plan := update.MakePlan(exe, runtime.GOOS, runtime.GOARCH)
	ctx, cancel := context.WithCancel(context.Background())
	a.updSet(func(s *updState) {
		s.phase, s.err, s.cancel, s.progress = "downloading", "", cancel, update.Progress{}
	})
	rel := snap.rel
	safeGo("update.install", func() {
		defer cancel()
		work := filepath.Join(os.TempDir(), "hinatracer-update")
		out, err := update.Run(ctx, update.NewHTTPClient(0), rel, plan, work, func(p update.Progress) {
			a.updSet(func(s *updState) { s.progress = p })
		})
		if err != nil {
			msg := err.Error()
			switch {
			case errors.Is(err, context.Canceled):
				msg = i18n.T("update.err.canceled")
			case errors.Is(err, update.ErrNeedManual):
				msg = i18n.T("update.err.manual")
			}
			a.updSet(func(s *updState) { s.phase, s.err, s.cancel = "error", msg, nil })
			return
		}
		if out.Action == "saved" {
			a.updSet(func(s *updState) { s.phase, s.savedPath, s.cancel = "ready", out.Launch, nil })
			return
		}
		a.updSet(func(s *updState) { s.phase, s.cancel = "ready", nil })
		// Save config/state before restarting.
		a.flushLayouts()
		a.persistNow()
		if err := updRelaunch(out, plan.Exe); err != nil {
			a.updSet(func(s *updState) { s.phase, s.err = "error", err.Error() })
			return
		}
		updQuit(a)
	})
}

func (a *app) cancelUpdate() {
	a.upd.mu.Lock()
	c := a.upd.cancel
	a.upd.mu.Unlock()
	if c != nil {
		c()
	}
}

// persistNow saves the config via the UI thread when a window exists, so
// config writes never race with the view.
func (a *app) persistNow() {
	done := make(chan struct{})
	if a.win == nil {
		a.persist()
		return
	}
	a.win.Update(func() {
		a.persist()
		close(done)
	})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}

func relaunchAfterUpdate(out update.Outcome, exe string) error {
	switch out.Action {
	case "setup":
		script := update.SetupScript(out.Launch, exe, os.Getpid())
		cmd := exec.Command("powershell.exe", "-NoProfile", "-WindowStyle", "Hidden", "-Command", script)
		return cmd.Start()
	case "relaunch":
		if runtime.GOOS == "darwin" {
			return exec.Command("open", "-n", out.Launch).Start()
		}
		return exec.Command(out.Launch).Start()
	}
	return nil
}

// cleanupAfterUpdate removes the previous executable left by a self-update.
func cleanupAfterUpdate() {
	if exe, err := os.Executable(); err == nil {
		update.CleanupOld(exe)
		if b := update.BundleRoot(exe); b != "" {
			update.CleanupOld(b)
		}
	}
}

func (a *app) openReleasePage(rel update.Release) {
	u := rel.HTMLURL
	if u == "" {
		u = update.ReleasesPage
	}
	go mygoShellOpen(u)
}

func updStatusText(s updView) string {
	switch s.phase {
	case "checking":
		return i18n.T("update.status.checking")
	case "uptodate":
		return i18n.T("update.status.uptodate")
	case "available":
		return i18n.Tf("update.status.available", s.rel.Version())
	case "error":
		return i18n.Tf("update.status.error", s.err)
	case "downloading":
		return i18n.T("update.status.downloading") + " " + formatUpdProgress(s.progress)
	case "ready":
		if s.savedPath != "" {
			return i18n.Tf("update.status.saved", s.savedPath)
		}
		return i18n.T("update.status.restarting")
	}
	return ""
}

func formatUpdProgress(p update.Progress) string {
	if p.Bytes == 0 {
		return ""
	}
	speed := ""
	if p.SpeedBPS > 0 {
		speed = " · " + formatBytes(int64(p.SpeedBPS)) + "/s"
	}
	if p.Total > 0 {
		return fmt.Sprintf("%s / %s (%.0f%%)%s", formatBytes(p.Bytes), formatBytes(p.Total), float64(p.Bytes)*100/float64(p.Total), speed)
	}
	return formatBytes(p.Bytes) + speed
}

// viewUpdatePanel shows the non-modal update prompt above page content.
func (a *app) viewUpdatePanel(c *ui.Context) {
	s := a.updSnapshot()
	if !s.prompt {
		return
	}
	t := c.Theme()
	rel := s.rel
	card(c, func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, i18n.Tf("update.title", rel.Version())).FontSize(14).Bold()
			if rel.Prerelease {
				ui.Badge(c, i18n.T("update.prerelease"))
			}
			ui.Spacer(c)
			link := ui.Text(c, i18n.T("update.open_page")).FontSize(12).TextColor(t.Accent)
			if link.Clicked() {
				a.openReleasePage(rel)
			}
		})
		date := "—"
		if !rel.PublishedAt.IsZero() {
			date = rel.PublishedAt.Local().Format("2006-01-02 15:05")
		}
		ui.Text(c, i18n.Tf("update.meta", appVersion, rel.Version(), date)).FontSize(12).TextColor(t.TextMuted)
		notes := strings.TrimSpace(rel.Body)
		if notes == "" {
			notes = i18n.T("update.no_notes")
		}
		ui.Scroll(c).TrackScroll(&a.upd.notesScroll).MaxHeight(180).MinWidth(0).Padding(8).Radius(6).
			Background(t.Surface).Border(1, t.Border).Children(func() {
			ui.Text(c, notes).FontSize(12).Selectable()
		})
		if st := updStatusText(s); st != "" && s.phase != "available" {
			color := t.TextMuted
			if s.phase == "error" {
				color = t.Danger
			}
			ui.Text(c, st).FontSize(12).TextColor(color)
		}
		busy := s.phase == "downloading" || (s.phase == "ready" && s.savedPath == "")
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if s.phase == "downloading" {
				if ui.Button(c, i18n.T("update.cancel")).Clicked() {
					a.cancelUpdate()
				}
			} else if ui.PrimaryButton(c, i18n.T("update.now")).Disabled(busy).Clicked() {
				a.startUpdateInstall()
			}
			if ui.Button(c, i18n.T("update.later")).Disabled(busy).Clicked() {
				a.updSet(func(s *updState) { s.prompt = false })
			}
			if ui.Button(c, i18n.T("update.skip")).Disabled(busy).Clicked() {
				a.skipUpdateVersion(rel.Version())
			}
			if s.phase == "error" || s.savedPath != "" {
				if ui.Button(c, i18n.T("update.open_download")).Clicked() {
					a.openReleasePage(rel)
				}
			}
		})
	}).Shrink(0)
}

// viewAboutUpdate is the About page "check for updates" block.
func (a *app) viewAboutUpdate(c *ui.Context) {
	t := c.Theme()
	s := a.updSnapshot()
	ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Children(func() {
		busy := s.phase == "checking" || s.phase == "downloading"
		if ui.Button(c, i18n.T("update.check")).Disabled(busy).Clicked() {
			pre := a.cfg.IncludePrereleases()
			safeGo("update.manual", func() { a.checkForUpdates(true, pre, "") })
		}
		if s.phase == "checking" {
			ui.Spinner(c)
		}
		if st := updStatusText(s); st != "" {
			color := t.TextMuted
			switch s.phase {
			case "error":
				color = t.Danger
			case "available":
				color = t.Accent
			case "uptodate":
				color = t.Success
			}
			txt := ui.Text(c, st).FontSize(12).TextColor(color)
			if s.phase == "available" && txt.Clicked() {
				a.updSet(func(s *updState) { s.prompt = true })
			}
		}
	})
}

// viewSettingsUpdate is the settings card for the update checker.
func (a *app) viewSettingsUpdate(c *ui.Context) {
	t := c.Theme()
	if !a.upd.inited {
		a.upd.inited = true
		a.upd.checkOnRun = a.cfg.CheckUpdatesOnStart()
	}
	stable, pre := i18n.T("update.channel.stable"), i18n.T("update.channel.prerelease")
	a.upd.chanName = stable
	if a.cfg.IncludePrereleases() {
		a.upd.chanName = pre
	}
	card(c, func() {
		ui.Text(c, i18n.T("update.settings.title")).FontSize(14).Bold()
		ui.Text(c, i18n.T("update.settings.desc")).FontSize(12).TextColor(t.TextMuted)
		if ui.Checkbox(c, &a.upd.checkOnRun, i18n.T("update.settings.on_start")).Changed() {
			v := a.upd.checkOnRun
			a.cfg.UpdateCheckOnStart = &v
			a.persist()
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, i18n.T("update.settings.channel")).FontSize(13)
			if ui.Select(c, &a.upd.chanName, []string{stable, pre}).Changed() {
				if a.upd.chanName == pre {
					a.cfg.UpdateChannel = "prerelease"
				} else {
					a.cfg.UpdateChannel = "stable"
				}
				a.persist()
			}
		})
		if v := a.cfg.UpdateSkipVersion; v != "" {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, i18n.Tf("update.settings.skipped", v)).FontSize(12).TextColor(t.TextMuted)
				if ui.Button(c, i18n.T("update.settings.unskip")).Clicked() {
					a.cfg.UpdateSkipVersion = ""
					a.persist()
				}
			})
		}
	})
}
