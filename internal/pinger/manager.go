package pinger

import (
	"context"
	"sync"
	"time"

	"hinatracer/internal/crashlog"
)

// Target is a live ping target with stats.
type Target struct {
	ID      int
	Host    string
	Alias   string
	Enabled bool
	Stats   Stats
	LastErr string
}

// Snapshot is a UI-safe copy of a target.
type Snapshot struct {
	ID      int
	Host    string
	Alias   string
	Enabled bool
	Stats   Stats
	Running bool
	LastErr string
	Proto   Proto
}

// Manager periodically pings a list of targets.
type Manager struct {
	mu       sync.Mutex
	targets  []*Target
	nextID   int
	interval time.Duration
	timeout  time.Duration
	cancel   context.CancelFunc
	running  bool
	onUpdate func()
}

// NewManager creates a manager with the given interval.
func NewManager(interval time.Duration) *Manager {
	if interval <= 0 {
		interval = time.Second
	}
	return &Manager{
		interval: interval,
		timeout:  2 * time.Second,
		nextID:   1,
	}
}

// SetOnUpdate registers a callback invoked after stats change (any goroutine).
func (m *Manager) SetOnUpdate(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onUpdate = fn
}

// SetInterval updates the ping interval (takes effect next loop).
func (m *Manager) SetInterval(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d <= 0 {
		d = time.Second
	}
	m.interval = d
}

// Interval returns the current interval.
func (m *Manager) Interval() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.interval
}

// Add adds a target (enabled) and returns its ID.
func (m *Manager) Add(host, alias string) int {
	return m.AddEnabled(host, alias, true)
}

// AddEnabled adds a target with the given enabled flag.
func (m *Manager) AddEnabled(host, alias string, enabled bool) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextID
	m.nextID++
	m.targets = append(m.targets, &Target{ID: id, Host: host, Alias: alias, Enabled: enabled})
	return id
}

// FindByHost returns the target ID matching normalized host, or -1.
func (m *Manager) FindByHost(host string) (id int, alias string, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := NormalizeHost(host)
	if key == "" {
		return -1, "", false
	}
	for _, t := range m.targets {
		if NormalizeHost(t.Host) == key {
			return t.ID, t.Alias, true
		}
	}
	return -1, "", false
}

// SetEnabled sets whether a target is pinged.
func (m *Manager) SetEnabled(id int, enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.targets {
		if t.ID == id {
			t.Enabled = enabled
			return
		}
	}
}

// ReplaceAlias sets alias for an existing host by ID.
func (m *Manager) ReplaceAlias(id int, alias string) {
	m.SetAlias(id, alias)
}

// Remove removes a target by ID.
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

// SetAlias sets the alias for a target.
func (m *Manager) SetAlias(id int, alias string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.targets {
		if t.ID == id {
			t.Alias = alias
			return
		}
	}
}

// ResetStats clears stats for one or all (id=0) targets.
func (m *Manager) ResetStats(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.targets {
		if id == 0 || t.ID == id {
			t.Stats.Reset()
		}
	}
}

// Snapshots returns UI-safe copies.
func (m *Manager) Snapshots() []Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Snapshot, len(m.targets))
	for i, t := range m.targets {
		st := t.Stats
		st.Latencies = append([]float64(nil), t.Stats.Latencies...)
		out[i] = Snapshot{
			ID:      t.ID,
			Host:    t.Host,
			Alias:   t.Alias,
			Enabled: t.Enabled,
			Stats:   st,
			Running: m.running,
			LastErr: t.LastErr,
			Proto:   ProtoOf(t.Host),
		}
	}
	return out
}

// ConfigTarget is a host/alias/enabled triple for persistence.
type ConfigTarget struct {
	Host, Alias string
	Enabled     bool
}

// TargetsForConfig returns hosts/aliases/enabled for persistence.
func (m *Manager) TargetsForConfig() []ConfigTarget {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ConfigTarget, len(m.targets))
	for i, t := range m.targets {
		out[i] = ConfigTarget{Host: t.Host, Alias: t.Alias, Enabled: t.Enabled}
	}
	return out
}

// Start begins the ping loop.
func (m *Manager) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.running = true
	m.mu.Unlock()
	go m.loop(ctx)
}

// Stop stops the ping loop.
func (m *Manager) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	cancel := m.cancel
	m.running = false
	m.cancel = nil
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Running reports whether the loop is active.
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

func (m *Manager) loop(ctx context.Context) {
	for {
		m.pingAll()
		m.mu.Lock()
		d := m.interval
		fn := m.onUpdate
		m.mu.Unlock()
		if fn != nil {
			fn()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(d):
		}
	}
}

func (m *Manager) pingAll() {
	m.mu.Lock()
	targets := append([]*Target(nil), m.targets...)
	timeout := m.timeout
	m.mu.Unlock()

	var wg sync.WaitGroup
	for _, t := range targets {
		if !t.Enabled {
			continue
		}
		wg.Add(1)
		go func(t *Target) {
			defer wg.Done()
			defer func() {
				if rec := crashlog.Recover("pinger.ping"); rec != nil {
					now := time.Now()
					m.mu.Lock()
					defer m.mu.Unlock()
					for _, cur := range m.targets {
						if cur.ID == t.ID {
							cur.Stats.AddFailure(now)
							return
						}
					}
				}
			}()
			res := Probe(t.Host, timeout)
			now := time.Now()
			m.mu.Lock()
			defer m.mu.Unlock()
			// ensure still present
			found := false
			for _, cur := range m.targets {
				if cur.ID == t.ID {
					found = true
					t = cur
					break
				}
			}
			if !found {
				return
			}
			if !res.OK {
				t.Stats.AddFailure(now)
				if res.Err != nil {
					t.LastErr = res.Err.Error()
				} else {
					t.LastErr = "timeout"
				}
			} else {
				t.Stats.AddSuccess(float64(res.RTT)/float64(time.Millisecond), now)
				t.LastErr = ""
			}
		}(t)
	}
	wg.Wait()
}

// IndexOf returns the config-order index of id, or -1.
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

// MoveByIndex moves the target at from to before to (to may be len).
// No-op when from is invalid or the move would not change order.
func (m *Manager) MoveByIndex(from, to int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := len(m.targets)
	if from < 0 || from >= n {
		return
	}
	if to < 0 {
		to = 0
	}
	if to > n {
		to = n
	}
	if to == from || to == from+1 {
		return
	}
	item := m.targets[from]
	m.targets = append(m.targets[:from], m.targets[from+1:]...)
	if to > from {
		to--
	}
	m.targets = append(m.targets[:to], append([]*Target{item}, m.targets[to:]...)...)
}

// ReorderByIndices moves the given config-order indices to before to,
// matching ui.ListState.Reorder semantics.
func (m *Manager) ReorderByIndices(rows []int, to int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := len(m.targets)
	if n == 0 || len(rows) == 0 {
		return
	}
	seen := make(map[int]bool, len(rows))
	var moving []*Target
	for _, i := range rows {
		if i < 0 || i >= n || seen[i] {
			continue
		}
		seen[i] = true
		moving = append(moving, m.targets[i])
	}
	if len(moving) == 0 {
		return
	}
	remain := make([]*Target, 0, n-len(moving))
	for i, t := range m.targets {
		if !seen[i] {
			remain = append(remain, t)
		}
	}
	// Adjust to for removed rows before the insertion point.
	adj := to
	for _, i := range rows {
		if i >= 0 && i < to {
			adj--
		}
	}
	if adj < 0 {
		adj = 0
	}
	if adj > len(remain) {
		adj = len(remain)
	}
	out := make([]*Target, 0, n)
	out = append(out, remain[:adj]...)
	out = append(out, moving...)
	out = append(out, remain[adj:]...)
	m.targets = out
}

// MoveUp swaps the target with the previous one in config order.
func (m *Manager) MoveUp(id int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := -1
	for j, t := range m.targets {
		if t.ID == id {
			i = j
			break
		}
	}
	if i <= 0 {
		return false
	}
	m.targets[i-1], m.targets[i] = m.targets[i], m.targets[i-1]
	return true
}

// MoveDown swaps the target with the next one in config order.
func (m *Manager) MoveDown(id int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := -1
	for j, t := range m.targets {
		if t.ID == id {
			i = j
			break
		}
	}
	if i < 0 || i >= len(m.targets)-1 {
		return false
	}
	m.targets[i], m.targets[i+1] = m.targets[i+1], m.targets[i]
	return true
}
