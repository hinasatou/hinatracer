package httpmon

import (
	"context"
	"fmt"
	"sync"
	"time"

	"hinatracer/internal/crashlog"
	"hinatracer/internal/pinger"
)

// Target is one monitored URL.
type Target struct {
	ID      int
	Alias   string
	Spec    Spec
	Enabled bool
	Stats   pinger.Stats
	Last    Result
	Checks  int
}

// Snapshot is a UI-safe copy of a target.
type Snapshot struct {
	ID      int
	Alias   string
	Spec    Spec
	Enabled bool
	Stats   pinger.Stats
	Last    Result
	Checks  int
	Running bool
}

// Manager runs the checks; each target has its own interval.
type Manager struct {
	mu       sync.Mutex
	targets  []*Target
	nextID   int
	running  bool
	cancel   context.CancelFunc
	onUpdate func()
	onSample func(t Snapshot, r Result)
	check    func(context.Context, Spec) Result
	wake     chan struct{}
}

// NewManager makes an idle manager.
func NewManager() *Manager {
	return &Manager{nextID: 1, check: Check, wake: make(chan struct{}, 1)}
}

// SetCheck replaces the checker (tests).
func (m *Manager) SetCheck(fn func(context.Context, Spec) Result) {
	m.mu.Lock()
	m.check = fn
	m.mu.Unlock()
}

// SetOnUpdate is called (without the lock) after results change.
func (m *Manager) SetOnUpdate(fn func()) {
	m.mu.Lock()
	m.onUpdate = fn
	m.mu.Unlock()
}

// SetOnSample is called (without the lock) for every finished check.
func (m *Manager) SetOnSample(fn func(t Snapshot, r Result)) {
	m.mu.Lock()
	m.onSample = fn
	m.mu.Unlock()
}

// Add appends a target and returns its id.
func (m *Manager) Add(alias string, spec Spec, enabled bool) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextID
	m.nextID++
	m.targets = append(m.targets, &Target{ID: id, Alias: alias, Spec: spec.Defaults(), Enabled: enabled, Last: Result{CertDays: -1}})
	m.poke()
	return id
}

// FindByURL returns the id of a target with this URL.
func (m *Manager) FindByURL(u string) (int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.targets {
		if t.Spec.URL == u {
			return t.ID, true
		}
	}
	return 0, false
}

func (m *Manager) find(id int) *Target {
	for _, t := range m.targets {
		if t.ID == id {
			return t
		}
	}
	return nil
}

// Update replaces alias and spec; stats reset when the URL changes.
func (m *Manager) Update(id int, alias string, spec Spec) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.find(id)
	if t == nil {
		return false
	}
	spec = spec.Defaults()
	if spec.URL != t.Spec.URL {
		t.Stats.Reset()
		t.Last = Result{CertDays: -1}
		t.Checks = 0
	}
	t.Alias, t.Spec = alias, spec
	m.poke()
	return true
}

// SetAlias renames.
func (m *Manager) SetAlias(id int, alias string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t := m.find(id); t != nil {
		t.Alias = alias
	}
}

// SetEnabled toggles one target.
func (m *Manager) SetEnabled(id int, on bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t := m.find(id); t != nil {
		t.Enabled = on
		m.poke()
	}
}

// Remove deletes a target.
func (m *Manager) Remove(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, t := range m.targets {
		if t.ID == id {
			m.targets = append(m.targets[:i], m.targets[i+1:]...)
			return
		}
	}
}

// ResetStats clears statistics (0 = all).
func (m *Manager) ResetStats(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.targets {
		if id == 0 || t.ID == id {
			t.Stats.Reset()
			t.Last = Result{CertDays: -1}
			t.Checks = 0
		}
	}
}

// IndexOf is the position of id, or -1.
func (m *Manager) IndexOf(id int) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, t := range m.targets {
		if t.ID == id {
			return i
		}
	}
	return -1
}

// Move moves id by delta positions (clamped).
func (m *Manager) Move(id, delta int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	from := -1
	for i, t := range m.targets {
		if t.ID == id {
			from = i
		}
	}
	if from < 0 {
		return false
	}
	to := max(0, min(len(m.targets)-1, from+delta))
	if to == from {
		return false
	}
	t := m.targets[from]
	m.targets = append(m.targets[:from], m.targets[from+1:]...)
	m.targets = append(m.targets[:to], append([]*Target{t}, m.targets[to:]...)...)
	return true
}

// ReorderByIDs moves the given ids (in list order) before position to.
func (m *Manager) ReorderByIDs(ids []int, to int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pick := map[int]bool{}
	for _, id := range ids {
		pick[id] = true
	}
	var moved, rest []*Target
	before := 0
	for i, t := range m.targets {
		if pick[t.ID] {
			moved = append(moved, t)
			if i < to {
				before++
			}
		} else {
			rest = append(rest, t)
		}
	}
	at := max(0, min(len(rest), to-before))
	out := append([]*Target{}, rest[:at]...)
	out = append(out, moved...)
	m.targets = append(out, rest[at:]...)
}

// Snapshots returns copies in list order.
func (m *Manager) Snapshots() []Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Snapshot, len(m.targets))
	for i, t := range m.targets {
		st := t.Stats
		st.Latencies = append([]float64(nil), t.Stats.Latencies...)
		st.History = append([]float64(nil), t.Stats.History...)
		out[i] = Snapshot{ID: t.ID, Alias: t.Alias, Spec: t.Spec, Enabled: t.Enabled, Stats: st, Last: t.Last, Checks: t.Checks, Running: m.running}
	}
	return out
}

// Start begins checking.
func (m *Manager) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.running, m.cancel = true, cancel
	m.mu.Unlock()
	go func() {
		defer crashlog.Recover("httpmon.loop")
		m.loop(ctx)
	}()
}

// Stop stops checking.
func (m *Manager) Stop() {
	m.mu.Lock()
	cancel := m.cancel
	m.running, m.cancel = false, nil
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Running reports whether checks run.
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

func (m *Manager) poke() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// loop schedules the checks. A panic in a scheduling pass is written to
// the crash log and the loop goes on (after a growing extra wait if it
// repeats); a panic in a check counts as a failed check.
func (m *Manager) loop(ctx context.Context) {
	next := map[int]time.Time{}
	tick := time.NewTimer(0)
	defer tick.Stop()
	var backoff crashlog.Backoff
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.wake:
		case <-tick.C:
		}
		soonest := time.Now().Add(time.Second)
		rec := crashlog.Pass("httpmon.loop", func() {
			soonest = m.schedule(ctx, next)
		})
		if !tick.Stop() {
			select {
			case <-tick.C:
			default:
			}
		}
		tick.Reset(max(10*time.Millisecond, time.Until(soonest)) + backoff.After(rec))
	}
}

// schedule starts the checks that are due and returns when the next one is.
// Checks run at a fixed rate, start-to-start: the next check of a target is
// due at start+interval even if the previous one is still running, so
// checks of the same target may overlap and a slow check never delays the
// schedule. Each check records exactly one result when it finishes.
func (m *Manager) schedule(ctx context.Context, next map[int]time.Time) time.Time {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	check := m.check
	soonest := now.Add(time.Second)
	for _, t := range m.targets {
		if !t.Enabled {
			continue
		}
		at, ok := next[t.ID]
		if ok && now.Before(at) {
			if at.Before(soonest) {
				soonest = at
			}
			continue
		}
		next[t.ID] = now.Add(time.Duration(t.Spec.Interval * float64(time.Second)))
		id, spec := t.ID, t.Spec
		go func() {
			defer func() {
				if rec := crashlog.Logged("httpmon.check", recover()); rec != nil {
					failed := Result{At: time.Now(), Err: fmt.Sprintf("internal error: %v", rec), CertDays: -1}
					crashlog.Pass("httpmon.record", func() { m.record(id, spec, failed) })
				}
			}()
			r := check(ctx, spec)
			m.record(id, spec, r)
		}()
	}
	return soonest
}

func (m *Manager) record(id int, spec Spec, r Result) {
	snap, onS, onU, ok := m.recordLocked(id, spec, r)
	if !ok {
		return
	}
	if onS != nil {
		onS(snap, r)
	}
	if onU != nil {
		onU()
	}
}

// EditStats runs f on a target's stats under the lock (restoring saved
// totals). Returns false for an unknown id.
func (m *Manager) EditStats(id int, f func(*pinger.Stats)) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.find(id)
	if t == nil {
		return false
	}
	f(&t.Stats)
	if t.Checks == 0 && t.Stats.Success+t.Stats.Failure > 0 {
		// restored: show the last saved outcome until the next check
		t.Checks = t.Stats.Success + t.Stats.Failure
		st := t.Stats
		t.Last.OK = !st.LastOK.Before(st.LastFail)
		t.Last.At = st.LastOK
		if st.LastFail.After(st.LastOK) {
			t.Last.At = st.LastFail
		}
	}
	return true
}

// recordLocked adds r to the target's stats under the lock (released by
// defer, so a panic cannot leave it held). ok is false when the target is
// gone, stopped or changed meanwhile.
func (m *Manager) recordLocked(id int, spec Spec, r Result) (snap Snapshot, onS func(Snapshot, Result), onU func(), ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.find(id)
	if t == nil || !m.running || t.Spec.URL != spec.URL {
		return snap, nil, nil, false
	}
	if r.OK {
		t.Stats.AddSuccess(r.Ms, r.At)
	} else {
		t.Stats.AddFailure(r.At)
	}
	t.Last = r
	t.Checks++
	snap = Snapshot{ID: t.ID, Alias: t.Alias, Spec: t.Spec, Enabled: t.Enabled, Last: r}
	return snap, m.onSample, m.onUpdate, true
}
