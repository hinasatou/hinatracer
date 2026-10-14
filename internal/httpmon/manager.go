package httpmon

import (
	"context"
	"sync"
	"time"

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
	go m.loop(ctx)
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

func (m *Manager) loop(ctx context.Context) {
	next := map[int]time.Time{}
	busy := map[int]bool{}
	done := make(chan int, 64)
	tick := time.NewTimer(0)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-done:
			delete(busy, id)
		case <-m.wake:
		case <-tick.C:
		}
		now := time.Now()
		m.mu.Lock()
		check := m.check
		soonest := now.Add(time.Second)
		for _, t := range m.targets {
			if !t.Enabled || busy[t.ID] {
				continue
			}
			at, ok := next[t.ID]
			if ok && now.Before(at) {
				if at.Before(soonest) {
					soonest = at
				}
				continue
			}
			busy[t.ID] = true
			next[t.ID] = now.Add(time.Duration(t.Spec.Interval * float64(time.Second)))
			id, spec := t.ID, t.Spec
			go func() {
				r := check(ctx, spec)
				m.record(id, spec, r)
				select {
				case done <- id:
				case <-ctx.Done():
				}
			}()
		}
		m.mu.Unlock()
		if !tick.Stop() {
			select {
			case <-tick.C:
			default:
			}
		}
		tick.Reset(max(10*time.Millisecond, time.Until(soonest)))
	}
}

func (m *Manager) record(id int, spec Spec, r Result) {
	m.mu.Lock()
	t := m.find(id)
	if t == nil || !m.running || t.Spec.URL != spec.URL {
		m.mu.Unlock()
		return
	}
	if r.OK {
		t.Stats.AddSuccess(r.Ms, r.At)
	} else {
		t.Stats.AddFailure(r.At)
	}
	t.Last = r
	t.Checks++
	snap := Snapshot{ID: t.ID, Alias: t.Alias, Spec: t.Spec, Enabled: t.Enabled, Last: r}
	onS, onU := m.onSample, m.onUpdate
	m.mu.Unlock()
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
