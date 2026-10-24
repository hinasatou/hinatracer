package app

import (
	"github.com/egoist/mygo/ui"

	"path/filepath"

	"hinatracer/i18n"
	"hinatracer/internal/loctr"
)

// locTrCacheName is the translation cache next to the config (a separate
// file; history.db is not touched).
const locTrCacheName = "location-translations.json"

func (a *app) initLocTr() {
	a.locTr = loctr.New(filepath.Join(a.appDataDir(), locTrCacheName))
	a.locTr.SetEnabled(a.cfg.TranslateLocOn())
	a.locTr.OnTranslated = a.onLocTranslated
}

// trLoc returns the IPDB location text in the UI language when translation
// is on and cached; otherwise the original (a request is queued). Never blocks.
func (a *app) trLoc(s string) string {
	if a.locTr == nil {
		return s
	}
	return a.locTr.Get(s, loctr.TargetFor(i18n.Language()))
}

// setTranslateLoc is the settings switch (desktop and WebUI).
func (a *app) setTranslateLoc(on bool) {
	a.cfg.TranslateLoc = &on
	if a.locTr != nil {
		a.locTr.SetEnabled(on)
	}
	a.persist()
	a.invalidateHostMeta() // re-lookup: queues translations (or restores Chinese)
	if on {
		a.retranslateTraces()
	}
	a.requestUIUpdate()
}

// retranslateTraces queues translations for trace hops already shown
// (cached ones apply at once). Runs after the render loop.
func (a *app) retranslateTraces() {
	run := func() {
		for _, tab := range a.traceTabs {
			tab.mu.Lock()
			// Only looked-up IPDB location text (tab.meta), never
			// placeholders, rDNS or other hop fields.
			locs := map[string]bool{}
			for _, m := range tab.meta {
				locs[m.Location] = true
			}
			tab.mu.Unlock()
			for loc := range locs {
				if tr := a.trLoc(loc); tr != loc {
					a.patchLoc(loc, tr)
				}
			}
		}
	}
	if a.win != nil {
		a.win.Update(func() {
			defer recoverAndLog("loctr.retrace", false)
			run()
		})
	} else {
		run()
	}
}

// onLocTranslated swaps the shown text after a translation lands: host
// meta is patched in place and trace hops after the render loop.
func (a *app) onLocTranslated(orig, lang, translated string) {
	if lang != loctr.TargetFor(i18n.Language()) {
		return
	}
	a.hostMetaMu.Lock()
	for k, m := range a.hostMeta {
		if m.Loc == orig {
			m.Loc = translated
			a.hostMeta[k] = m
		}
	}
	a.hostMetaMu.Unlock()
	if a.win != nil {
		a.win.Update(func() {
			defer recoverAndLog("loctr.patch", false)
			a.patchLoc(orig, translated)
		})
	} else {
		a.patchLoc(orig, translated)
	}
	a.requestUIUpdate()
}

// patchLoc replaces shown trace hop location text (UI thread).
func (a *app) patchLoc(orig, translated string) {
	{ //nolint:gocritic
		for _, tab := range a.traceTabs {
			tab.mu.Lock()
			for k, m := range tab.meta {
				if m.Location == orig {
					m.Location = translated
					tab.meta[k] = m
				}
			}
			for i := range tab.Hops {
				if tab.Hops[i].Location == orig {
					tab.Hops[i].Location = translated
				}
			}
			tab.mu.Unlock()
		}
	}
}

// viewSettingsLocTr is the settings card for the location translation switch.
func (a *app) viewSettingsLocTr(c *ui.Context) {
	card(c, func() {
		cardTitle(c, i18n.T("settings.loctr.title"))
		on := a.cfg.TranslateLocOn()
		if ui.Checkbox(c, &on, i18n.T("settings.loctr.enable")).Changed() {
			a.setTranslateLoc(on)
		}
		ui.Text(c, i18n.T("settings.loctr.desc")).FontSize(12).TextColor(c.Theme().TextMuted).Wrap()
	})
}
