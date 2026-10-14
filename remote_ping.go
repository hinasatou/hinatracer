package main

import (
	"strings"
	"sync"
	"time"

	"hinatracer/internal/pinger"
)

// remotePing mirrors a remote WebUI's ping list as a pingEngine. Reads come
// from the mirror; every change is applied to the mirror at once (so the
// page reacts immediately) and queued as the matching API call. The next
// pushed state replaces the mirror once the queue is idle.
type remotePing struct {
	rc       *remoteClient
	mu       sync.Mutex
	rows     []pinger.Snapshot
	running  bool
	interval time.Duration
	tmpID    int
}

func newRemotePing(rc *remoteClient) *remotePing { return &remotePing{rc: rc, interval: time.Second} }

func wsToPingSnap(r wsPing) pinger.Snapshot {
	s := pinger.Snapshot{ID: r.ID, Host: r.Host, Alias: r.Alias, Enabled: r.Enabled, LastErr: r.LastErr,
		Proto: pinger.Proto(r.Proto), IP: r.IP}
	st := &s.Stats
	st.Success, st.Failure = r.Success, r.Failure
	st.Last, st.Avg, st.Min, st.Max, st.Median = r.Last, r.Avg, r.Min, r.Max, r.Median
	st.LastOK, st.LastFail = msTime(r.LastOK), msTime(r.LastFail)
	st.Latencies = append([]float64(nil), r.Lat...)
	st.History = append([]float64(nil), r.Hist...)
	return s
}

func msTime(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

// apply takes a pushed state (skipped while actions are in flight).
func (p *remotePing) apply(st wsState) bool {
	if p.rc.Busy() {
		return false
	}
	rows := make([]pinger.Snapshot, len(st.Ping.Rows))
	for i, r := range st.Ping.Rows {
		rows[i] = wsToPingSnap(r)
		rows[i].Running = st.Ping.Running
	}
	p.mu.Lock()
	p.rows, p.running = rows, st.Ping.Running
	if st.Ping.Interval > 0 {
		p.interval = time.Duration(st.Ping.Interval * float64(time.Second))
	}
	p.mu.Unlock()
	return true
}

func (p *remotePing) find(id int) int {
	for i, r := range p.rows {
		if r.ID == id {
			return i
		}
	}
	return -1
}

func (p *remotePing) post(path string, body any) { p.rc.Enqueue(path, body, nil) }

func (p *remotePing) SetOnSample(func(host, ip string, at time.Time, ms float64)) {}
func (p *remotePing) SetOnUpdate(func())                                          {}

func (p *remotePing) SetInterval(d time.Duration) {
	p.mu.Lock()
	same := d == p.interval
	p.interval = d
	p.mu.Unlock()
	if !same {
		p.post("/api/ping/interval", map[string]float64{"seconds": d.Seconds()})
	}
}

func (p *remotePing) Interval() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.interval
}

func (p *remotePing) Add(host, alias string) int { return p.AddEnabled(host, alias, true) }

func (p *remotePing) AddEnabled(host, alias string, enabled bool) int {
	host = strings.TrimSpace(host)
	p.mu.Lock()
	p.tmpID--
	id := p.tmpID
	p.rows = append(p.rows, pinger.Snapshot{ID: id, Host: host, Alias: alias, Enabled: enabled, Running: p.running})
	p.mu.Unlock()
	p.post("/api/ping/add", map[string]any{"host": host, "alias": alias, "replace": true})
	return id
}

func (p *remotePing) FindByHost(host string) (int, string, bool) {
	key := pinger.NormalizeHost(host)
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, r := range p.rows {
		if pinger.NormalizeHost(r.Host) == key {
			return r.ID, r.Alias, true
		}
	}
	return 0, "", false
}

func (p *remotePing) SetEnabled(id int, enabled bool) {
	p.mu.Lock()
	if i := p.find(id); i >= 0 {
		p.rows[i].Enabled = enabled
	}
	p.mu.Unlock()
	p.post("/api/ping/enable", idsReq{IDs: []int{id}, Enabled: enabled})
}

func (p *remotePing) ReplaceAlias(id int, alias string) { p.SetAlias(id, alias) }

func (p *remotePing) SetAlias(id int, alias string) {
	p.mu.Lock()
	if i := p.find(id); i >= 0 {
		p.rows[i].Alias = alias
	}
	p.mu.Unlock()
	p.post("/api/ping/alias", map[string]any{"id": id, "alias": alias})
}

func (p *remotePing) Remove(id int) {
	p.mu.Lock()
	if i := p.find(id); i >= 0 {
		p.rows = append(p.rows[:i:i], p.rows[i+1:]...)
	}
	p.mu.Unlock()
	p.post("/api/ping/delete", idsReq{IDs: []int{id}})
}

func (p *remotePing) ResetStats(id int) {
	p.mu.Lock()
	for i := range p.rows {
		if id == 0 || p.rows[i].ID == id {
			p.rows[i].Stats = pinger.Stats{}
		}
	}
	p.mu.Unlock()
	if id == 0 {
		p.post("/api/ping/reset", idsReq{Confirm: true}) // the GUI asked first
	} else {
		p.post("/api/ping/reset", idsReq{IDs: []int{id}, Confirm: true})
	}
}

func (p *remotePing) Snapshots() []pinger.Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]pinger.Snapshot(nil), p.rows...)
}

// TargetsForConfig is empty: remote targets never go into the local config.
func (p *remotePing) TargetsForConfig() []pinger.ConfigTarget { return nil }

func (p *remotePing) Start() {
	p.mu.Lock()
	p.running = true
	p.mu.Unlock()
	p.post("/api/ping/start", nil)
}

func (p *remotePing) Stop() {
	p.mu.Lock()
	was := p.running
	p.running = false
	p.mu.Unlock()
	if was {
		p.post("/api/ping/stop", nil)
	}
}

func (p *remotePing) Running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

func (p *remotePing) IndexOf(id int) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.find(id)
}

func (p *remotePing) MoveByIndex(from, to int) {
	p.mu.Lock()
	ok := from >= 0 && from < len(p.rows)
	id := 0
	if ok {
		id = p.rows[from].ID
	}
	p.mu.Unlock()
	if ok {
		p.ReorderByIndices([]int{from}, to)
		_ = id
	}
}

// ReorderByIndices mirrors pinger.Manager.ReorderByIndices and sends the
// moved ids with the id they now sit before.
func (p *remotePing) ReorderByIndices(rows []int, to int) {
	p.mu.Lock()
	n := len(p.rows)
	seen := map[int]bool{}
	var moving []pinger.Snapshot
	var ids []int
	for _, i := range rows {
		if i >= 0 && i < n && !seen[i] {
			seen[i] = true
			moving = append(moving, p.rows[i])
			ids = append(ids, p.rows[i].ID)
		}
	}
	if len(moving) == 0 {
		p.mu.Unlock()
		return
	}
	var remain []pinger.Snapshot
	for i, r := range p.rows {
		if !seen[i] {
			remain = append(remain, r)
		}
	}
	adj := to
	for i := range seen {
		if i < to {
			adj--
		}
	}
	adj = max(0, min(adj, len(remain)))
	before := 0 // 0 = at the end
	if adj < len(remain) {
		before = remain[adj].ID
	}
	out := append(append(append([]pinger.Snapshot(nil), remain[:adj]...), moving...), remain[adj:]...)
	p.rows = out
	p.mu.Unlock()
	p.post("/api/ping/reorder", map[string]any{"ids": ids, "before": before})
}

func (p *remotePing) move(id int, up bool) bool {
	p.mu.Lock()
	i := p.find(id)
	j := i + 1
	if up {
		j = i - 1
	}
	ok := i >= 0 && j >= 0 && j < len(p.rows)
	if ok {
		p.rows[i], p.rows[j] = p.rows[j], p.rows[i]
	}
	p.mu.Unlock()
	if ok {
		p.post("/api/ping/move", map[string]any{"id": id, "up": up})
	}
	return ok
}

func (p *remotePing) MoveUp(id int) bool   { return p.move(id, true) }
func (p *remotePing) MoveDown(id int) bool { return p.move(id, false) }
