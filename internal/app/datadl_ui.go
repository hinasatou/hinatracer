package app

import (
	"context"
	"fmt"
	"strings"

	"hinatracer/i18n"
	"hinatracer/internal/config"
	"hinatracer/internal/datadl"
)

func formatDataDLProgress(p datadl.Progress) string {
	name := p.Filename
	if name == "" {
		name = string(p.Kind)
	}
	state := p.State
	switch state {
	case "pending":
		state = i18n.T("settings.dl_all.state.pending")
	case "running":
		state = i18n.T("settings.dl_all.state.running")
	case "ok":
		state = i18n.T("settings.dl_all.state.ok")
	case "error":
		state = i18n.T("settings.dl_all.state.error")
	case "canceled":
		state = i18n.T("settings.dl_all.state.canceled")
	}
	var prog string
	if p.Total > 0 {
		prog = fmt.Sprintf("%s / %s (%.0f%%)", formatBytes(p.Bytes), formatBytes(p.Total), p.Percent)
	} else if p.Bytes > 0 {
		prog = formatBytes(p.Bytes)
	}
	speed := ""
	if p.SpeedBPS > 0 && p.State == "running" {
		speed = " · " + formatBytes(int64(p.SpeedBPS)) + "/s"
	}
	errPart := ""
	if p.Err != "" {
		errPart = " — " + p.Err
	}
	if prog != "" {
		return fmt.Sprintf("%s: %s · %s%s%s", name, state, prog, speed, errPart)
	}
	return fmt.Sprintf("%s: %s%s", name, state, errPart)
}

func formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%.2f MB", float64(n)/(1024*1024))
}

func (a *app) startDataDownload() {
	if a.dataDL == nil || a.dataDL.Running() {
		return
	}
	cfgDir := a.appDataDir()
	specs := datadl.DefaultSpecs(a.draftIPDB, a.draftGeoIP, a.draftASN)
	a.dataDLStatus = i18n.T("settings.dl_all.status.running")
	a.taskbarEnd(a.dlTB)
	a.dlTB = a.taskbarBegin()
	ok := a.dataDL.Start(context.Background(), cfgDir, specs, func(results []datadl.Result) {
		a.onDataDownloadDone(results)
	})
	if !ok {
		a.taskbarEnd(a.dlTB)
		a.dataDLStatus = i18n.T("settings.dl_all.status.busy")
	}
	a.requestUIUpdate()
}

// onDataDownloadDone runs on the download goroutine. App state is updated
// via a.run (the UI thread when a window exists); the slow database reload
// happens afterwards, off the UI thread.
func (a *app) onDataDownloadDone(results []datadl.Result) {
	var okN int
	_ = a.run(func() {
		okN = a.applyDataDownloadResults(results)
		a.taskbarEnd(a.dlTB)
	})
	if okN > 0 {
		a.reloadDBs()
		a.refreshAllHostMeta()
	}
	a.requestUIUpdate()
}

// applyDataDownloadResults stores downloaded paths and the status text and
// returns how many files succeeded.
func (a *app) applyDataDownloadResults(results []datadl.Result) int {
	okN, errN, cancelN := 0, 0, 0
	for _, r := range results {
		if r.Err != nil {
			if strings.Contains(r.Err.Error(), "context canceled") || strings.Contains(r.Err.Error(), "canceled") {
				cancelN++
			} else {
				errN++
			}
			continue
		}
		okN++
		if r.Filled {
			switch r.Kind {
			case datadl.KindIPDB:
				a.draftIPDB = r.Path
				a.cfg.IPDBPath = r.Path
			case datadl.KindGeoIP:
				a.draftGeoIP = r.Path
				a.cfg.GeoIPPath = r.Path
			case datadl.KindASN:
				a.draftASN = r.Path
				a.cfg.ASNPath = r.Path
			}
		} else {
			// Keep drafts in sync with successful overwrite of configured paths.
			switch r.Kind {
			case datadl.KindIPDB:
				if strings.TrimSpace(a.draftIPDB) == "" {
					a.draftIPDB = r.Path
				}
			case datadl.KindGeoIP:
				if strings.TrimSpace(a.draftGeoIP) == "" {
					a.draftGeoIP = r.Path
				}
			case datadl.KindASN:
				if strings.TrimSpace(a.draftASN) == "" {
					a.draftASN = r.Path
				}
			}
		}
	}
	if okN > 0 {
		_ = a.store.Update(func(c *config.Config) {
			c.IPDBPath = a.cfg.IPDBPath
			c.GeoIPPath = a.cfg.GeoIPPath
			c.ASNPath = a.cfg.ASNPath
		})
		a.cfg = a.store.Get()
	}
	switch {
	case cancelN > 0 && okN == 0 && errN == 0:
		a.dataDLStatus = i18n.T("settings.dl_all.status.canceled")
	case errN > 0 && okN == 0:
		a.dataDLStatus = i18n.Tf("settings.dl_all.status.failed", errN)
	case errN > 0:
		a.dataDLStatus = i18n.Tf("settings.dl_all.status.partial", okN, errN)
	default:
		a.dataDLStatus = i18n.Tf("settings.dl_all.status.ok", okN)
	}
	return okN
}

// syncDataDLTaskbar mirrors download progress onto the taskbar (UI thread).
func (a *app) syncDataDLTaskbar() {
	if a.dataDL == nil || !a.dataDL.Running() || a.dlTB == 0 {
		return
	}
	var sumPct, n float64
	for _, p := range a.dataDL.Snapshot() {
		if p.Total > 0 {
			sumPct += p.Percent
			n++
		}
	}
	if n == 0 {
		return // keep indeterminate
	}
	a.taskbarSet(a.dlTB, sumPct/(n*100))
}
