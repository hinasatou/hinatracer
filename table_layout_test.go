package main

import (
	"slices"
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

func TestMigratePingLayoutInsertsIPAfterHost(t *testing.T) {
	old := config.TableLayout{Order: []string{"#", "alias", "host", "rdns", "location"}, Widths: map[string]float32{"host": 200}}
	got := migratePingLayout(old)
	want := []string{"#", "alias", "host", "ip", "rdns", "location"}
	if !slices.Equal(got.Order, want) {
		t.Fatalf("order = %v, want %v", got.Order, want)
	}
	if !slices.Equal(old.Order, []string{"#", "alias", "host", "rdns", "location"}) {
		t.Fatalf("input mutated: %v", old.Order)
	}
	if again := migratePingLayout(got); !slices.Equal(again.Order, want) {
		t.Fatalf("not idempotent: %v", again.Order)
	}
	if e := migratePingLayout(config.TableLayout{Widths: map[string]float32{"host": 1}}); len(e.Order) != 0 {
		t.Fatalf("empty order should stay empty: %v", e.Order)
	}
	moved := migratePingLayout(config.TableLayout{Order: []string{"host", "#", "alias"}})
	if !slices.Equal(moved.Order, []string{"host", "ip", "#", "alias"}) {
		t.Fatalf("moved host: %v", moved.Order)
	}
}

func TestCmpIPTextNumericV4BeforeV6(t *testing.T) {
	in := []string{"2001:db8::1", "10.0.0.10", "—", "10.0.0.9", "::1", "1.2.3.10"}
	slices.SortStableFunc(in, cmpIPText)
	want := []string{"1.2.3.10", "10.0.0.9", "10.0.0.10", "::1", "2001:db8::1", "—"}
	if !slices.Equal(in, want) {
		t.Fatalf("got %v, want %v", in, want)
	}
}
