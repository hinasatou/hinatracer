package main

import (
	"maps"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/internal/config"
)

const layoutSaveDelay = 800 * time.Millisecond

func toCfgLayout(l ui.TableLayout) config.TableLayout {
	return config.TableLayout{Order: slices.Clone(l.Order), Widths: maps.Clone(l.Widths)}
}

func fromCfgLayout(l config.TableLayout) ui.TableLayout {
	return ui.TableLayout{Order: slices.Clone(l.Order), Widths: maps.Clone(l.Widths)}
}

func layoutEqual(a, b config.TableLayout) bool {
	return slices.Equal(a.Order, b.Order) && maps.Equal(a.Widths, b.Widths)
}

// restoreLayouts applies saved column layouts to the tables.
func (a *app) restoreLayouts() {
	if l, ok := a.cfg.TableLayouts["ping"]; ok {
		a.pingTable.Columns = fromCfgLayout(l)
	}
	if l, ok := a.cfg.TableLayouts["lookup"]; ok {
		a.lookupTable.Columns = fromCfgLayout(l)
	}
	if l, ok := a.cfg.TableLayouts["trace"]; ok {
		for _, tab := range a.traceTabs {
			tab.Table.Columns = fromCfgLayout(l)
		}
	}
}

// noteLayout records a table's current column layout (UI thread) and
// schedules a debounced save when it changed.
func (a *app) noteLayout(name string, l ui.TableLayout) {
	cur := toCfgLayout(l)
	if len(cur.Order) == 0 && len(cur.Widths) == 0 {
		return
	}
	if old, ok := a.cfg.TableLayouts[name]; ok && layoutEqual(old, cur) {
		return
	}
	if a.cfg.TableLayouts == nil {
		a.cfg.TableLayouts = map[string]config.TableLayout{}
	}
	a.cfg.TableLayouts[name] = cur
	if name == "trace" {
		// Keep every trace tab in step with the one the user resized.
		for _, tab := range a.traceTabs {
			tab.Table.Columns = fromCfgLayout(cur)
		}
	}
	snapshot := map[string]config.TableLayout{}
	for k, v := range a.cfg.TableLayouts {
		snapshot[k] = toCfgLayout(fromCfgLayout(v))
	}
	a.layoutMu.Lock()
	a.layoutPending = snapshot
	if a.layoutTimer == nil {
		a.layoutTimer = time.AfterFunc(layoutSaveDelay, a.flushLayouts)
	} else {
		a.layoutTimer.Reset(layoutSaveDelay)
	}
	a.layoutMu.Unlock()
}

func (a *app) flushLayouts() {
	a.layoutMu.Lock()
	pending := a.layoutPending
	a.layoutPending = nil
	a.layoutMu.Unlock()
	if pending == nil {
		return
	}
	_ = a.store.Update(func(c *config.Config) { c.TableLayouts = pending })
}

func (a *app) notePingLayout() { a.noteLayout("ping", a.pingTable.Columns) }
