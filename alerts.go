package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"hinatracer/i18n"
	"hinatracer/internal/config"

	"github.com/egoist/mygo/ui"
)

// Desktop alerts: system
// notifications when a target goes down (downAfter consecutive failures)
// or recovers, and a tray icon. All are off by default and only run in
// the desktop app; they work in remote data-source mode too.

const defaultDownAfter = 3

// downThreshold is the configured failures-in-a-row count (1–100).
func downThreshold(v int) int {
	if v <= 0 {
		return defaultDownAfter
	}
	return min(100, v)
}

// alertRow is one monitored target as the alert watcher sees it.
type alertRow struct {
	Key, Name, Host, Reason string
	Down                    bool
}

// trailingFails counts the lost probes (-1) at the end of hist.
func trailingFails(hist []float64) int {
	n := 0
	for i := len(hist) - 1; i >= 0 && hist[i] < 0; i-- {
		n++
	}
	return n
}

// alertRows lists the enabled targets of running monitors.
func alertRows(st wsState, downAfter int) []alertRow {
	var out []alertRow
	if st.Ping.Running {
		for _, r := range st.Ping.Rows {
			if !r.Enabled {
				continue
			}
			name := r.Alias
			if name == "" {
				name = r.Host
			}
			out = append(out, alertRow{Key: fmt.Sprintf("p:%d", r.ID), Name: name, Host: r.Host, Reason: r.LastErr,
				Down: trailingFails(r.Hist) >= downAfter})
		}
	}
	if st.HTTP.Running {
		for _, r := range st.HTTP.Rows {
			if !r.Enabled {
				continue
			}
			name := r.Alias
			if name == "" {
				name = r.Spec.URL
			}
			reason := r.Err
			if reason == "" && r.Code > 0 {
				reason = fmt.Sprintf("HTTP %d", r.Code)
			}
			out = append(out, alertRow{Key: fmt.Sprintf("h:%d", r.ID), Name: name, Host: r.Spec.URL, Reason: reason,
				Down: trailingFails(r.Hist) >= downAfter})
		}
	}
	return out
}

// alertEvent is a down or recovery transition.
type alertEvent struct {
	Row  alertRow
	Down bool
}

// alertWatch remembers which targets are down and queues transitions.
type alertWatch struct {
	mu      sync.Mutex
	down    map[string]bool
	queue   []alertEvent
	firstAt time.Time // first queued event
	lastAt  time.Time // last notification shown
	failing int
}

// step takes the current rows and returns the failing count; it queues
// down events, and recoveries when withRecover.
func (w *alertWatch) step(rows []alertRow, now time.Time, withRecover bool) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.down == nil {
		w.down = map[string]bool{}
	}
	seen := map[string]bool{}
	n := 0
	for _, r := range rows {
		seen[r.Key] = true
		was := w.down[r.Key]
		if r.Down {
			n++
		}
		if r.Down != was {
			if r.Down || withRecover {
				if len(w.queue) == 0 {
					w.firstAt = now
				}
				w.queue = append(w.queue, alertEvent{Row: r, Down: r.Down})
			}
			w.down[r.Key] = r.Down
		}
	}
	for k := range w.down {
		if !seen[k] {
			delete(w.down, k) // removed or monitoring stopped: no recovery note
		}
	}
	w.failing = n
	return n
}

// Bursts within alertGroup are shown as one notification, and at most one
// notification is shown per alertGap.
const alertGroup, alertGap = 2 * time.Second, 10 * time.Second

// flush returns the notification to show now, if any.
func (w *alertWatch) flush(now time.Time) (title, body string, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.queue) == 0 || now.Sub(w.firstAt) < alertGroup || now.Sub(w.lastAt) < alertGap {
		return "", "", false
	}
	evs := w.queue
	w.queue, w.lastAt = nil, now
	title, body = alertText(evs)
	return title, body, true
}

func alertLine(e alertEvent) string {
	if e.Down {
		s := i18n.Tf("alert.down", e.Row.Name)
		if e.Row.Reason != "" {
			s += " — " + e.Row.Reason
		}
		return s
	}
	return i18n.Tf("alert.up", e.Row.Name)
}

// alertText is the notification for a burst of events.
func alertText(evs []alertEvent) (string, string) {
	if len(evs) == 1 {
		e := evs[0]
		body := e.Row.Host
		if e.Down && e.Row.Reason != "" {
			body += "\n" + e.Row.Reason
		}
		if e.Down {
			return i18n.Tf("alert.down", e.Row.Name), body
		}
		return i18n.Tf("alert.up", e.Row.Name), body
	}
	var lines []string
	for i, e := range evs {
		if i == 5 {
			lines = append(lines, i18n.Tf("alert.more", len(evs)-5))
			break
		}
		lines = append(lines, alertLine(e))
	}
	return i18n.Tf("alert.many", len(evs)), strings.Join(lines, "\n")
}

// Desktop hooks, set by main for the desktop app (no-ops in tests and
// headless mode).
var (
	showNotice  = func(title, body string) {}
	syncTray    = func(a *app, on bool, failing int) {}
	alertsDelay = time.Second
)

// startAlerts runs the watcher (desktop app only).
func (a *app) startAlerts() {
	safeGo("alerts", func() {
		t := time.NewTicker(alertsDelay)
		defer t.Stop()
		for range t.C {
			a.alertTick(time.Now())
		}
	})
}

// alertTick is one watcher pass.
func (a *app) alertTick(now time.Time) {
	var notify, recov, tray bool
	var st wsState
	thr := defaultDownAfter
	if err := a.run(func() {
		notify, recov, tray = a.cfg.NotifyDown, a.cfg.NotifyRecover, a.cfg.ShowTray
		thr = downThreshold(a.cfg.DownThreshold)
		if !notify && !tray {
			return
		}
		if a.remote != nil {
			st = a.remoteState()
		} else {
			st = a.webBuildState()
		}
	}); err != nil {
		return
	}
	n := 0
	if notify || tray {
		n = a.alerts.step(alertRows(st, thr), now, recov)
	}
	if notify {
		if title, body, ok := a.alerts.flush(now); ok {
			showNotice(title, body)
		}
	}
	syncTray(a, tray, n)
}

// viewSettingsAlerts is the 「桌面提醒」 card (this computer's settings).
func (a *app) viewSettingsAlerts(c *ui.Context) {
	t := c.Theme()
	card(c, func() {
		cardTitle(c, i18n.T("settings.alerts.title"))
		thr := downThreshold(a.cfg.DownThreshold)
		ui.Text(c, i18n.Tf("settings.alerts.desc", thr)).FontSize(12).TextColor(t.TextMuted)
		changed := false
		if a.alertThresh == 0 {
			a.alertThresh = float64(thr)
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, i18n.T("settings.alerts.threshold")).FontSize(13)
			if ui.NumberInput(c, &a.alertThresh, 1, 100, 1).Width(110).Changed() {
				v := max(1, min(100, int(a.alertThresh+0.5)))
				a.alertThresh = float64(v)
				a.cfg.DownThreshold = v
				changed = true
			}
		})
		sub := func(v *bool, label string, parent bool) {
			ui.Row(c).Padding(0, 0, 0, 24).Children(func() {
				if ui.Checkbox(c, v, label).Disabled(!parent).Changed() {
					changed = true
				}
			})
		}
		if ui.Checkbox(c, &a.cfg.NotifyDown, i18n.T("settings.alerts.notify")).Changed() {
			changed = true
		}
		sub(&a.cfg.NotifyRecover, i18n.T("settings.alerts.notify_recover"), a.cfg.NotifyDown)
		if ui.Checkbox(c, &a.cfg.ShowTray, i18n.T("settings.alerts.tray")).Changed() {
			changed = true
		}
		sub(&a.cfg.CloseToTray, i18n.T("settings.alerts.close_to_tray"), a.cfg.ShowTray)
		if changed {
			a.saveAlertSettings()
		}
	})
}

// saveAlertSettings writes the alert options to the local config (also in
// remote mode, where they are still this computer's).
func (a *app) saveAlertSettings() {
	v := a.cfg
	_ = a.store.Update(func(c *config.Config) {
		c.NotifyDown, c.NotifyRecover = v.NotifyDown, v.NotifyRecover
		c.ShowTray, c.CloseToTray, c.DownThreshold = v.ShowTray, v.CloseToTray, v.DownThreshold
	})
}
