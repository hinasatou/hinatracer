package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"hinatracer/internal/config"
)

func TestTableLayoutPersist(t *testing.T) {
	a := freshApp(t)
	a.noteLayout("ping", ui.TableLayout{Widths: map[string]float32{"host": 222}})
	a.noteLayout("trace", ui.TableLayout{Order: []string{"ttl", "ip"}, Widths: map[string]float32{"ip": 190}})
	a.layoutMu.Lock()
	if a.layoutTimer != nil {
		a.layoutTimer.Stop()
	}
	a.layoutMu.Unlock()
	a.flushLayouts()
	st := config.NewStore(a.store.Path())
	if err := st.Load(); err != nil {
		t.Fatal(err)
	}
	got := st.Get().TableLayouts
	if got["ping"].Widths["host"] != 222 || got["trace"].Widths["ip"] != 190 {
		t.Fatalf("%+v", got)
	}
	// restore into a fresh app
	b := newApp()
	if b.pingTable.Columns.Widths["host"] != 222 || b.activeTrace().Table.Columns.Widths["ip"] != 190 {
		t.Fatalf("not restored: %+v", b.pingTable.Columns)
	}
	// unchanged layout does not reschedule
	b.layoutTimer = nil
	b.noteLayout("ping", b.pingTable.Columns)
	if b.layoutTimer != nil {
		t.Fatal("should not save unchanged layout")
	}
}
