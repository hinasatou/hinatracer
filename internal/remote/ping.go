package remote

import (
	"strings"
	"sync"
	"time"

	"hinatracer/internal/api"
	"hinatracer/internal/pinger"
)

// Ping mirrors a remote WebUI's ping list as a pingEngine. Reads come
// from the mirror; every change is applied to the mirror at once (so the
// page reacts immediately) and queued as the matching API call. The next
// pushed state replaces the mirror once the queue is idle.
type Ping struct {
	rc       *Client
	mu       sync.Mutex
	rows     []pinger.Snapshot
	running  bool
	interval time.Duration
	tmpID    int
}

func NewPing(rc *Client) *Ping { return &Ping{rc: rc, interval: time.Second} }

func PingSnap(r api.Ping) pinger.Snapshot {
	s := pinger.Snapshot{ID: r.ID, Host: r.Host, Alias: r.Alias, Enabled: r.Enabled, LastErr: r.LastErr,
		Proto: pinger.Proto(r.Proto), IP: r.IP}
	st := &s.Stats
	st.Success, st.Failure = r.Success, r.Failure
	st.Last, st.Avg, st.Min, st.Max, st.Median = r.Last, r.Avg, r.Min, r.Max, r.Median
	st.LastOK, st.LastFail = api.MsTime(r.LastOK), api.MsTime(r.LastFail)
	st.Latencies = append([]float64(nil), r.Lat...)
	st.History = append([]float64(nil), r.Hist...)
	return s
}

// Apply takes a pushed state (skipped while actions are in flight).
func (p *Ping) Apply(st api.State) bool {
	if p.rc.Busy() {
		return false
	}
	rows := make([]pinger.Snapshot, len(st.Ping.Rows))
	for i, r := range st.Ping.Rows {
		rows[i] = PingSnap(r)
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

func (p *Ping) find(id int) int {
	for i, r := range p.rows {
		if r.ID == id {
			return i
		}
	}
	return -1
}

func (p *Ping) post(path string, body any) { p.rc.Enqueue(path, body, nil) }

func (p *Ping) SetOnSample(func(host, ip string, at time.Time, ms float64)) {}
func (p *Ping) SetOnUpdate(func())                                          {}

func (p *Ping) SetInterval(d time.Duration) {
	p.mu.Lock()
	same := d == p.interval
	p.interval = d
	p.mu.Unlock()
	if !same {
		p.post("/api/ping/interval", map[string]float64{"seconds": d.Seconds()})
	}
}

func (p *Ping) Interval() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.interval
}

func (p *Ping) Add(host, alias string) int { return p.AddEnabled(host, alias, true) }

func (p *Ping) AddEnabled(host, alias string, enabled bool) int {
	host = strings.TrimSpace(host)
	p.mu.Lock()
	p.tmpID--
	id := p.tmpID
	p.rows = append(p.rows, pinger.Snapshot{ID: id, Host: host, Alias: alias, Enabled: enabled, Running: p.running})
	p.mu.Unlock()
	p.post("/api/ping/add", map[string]any{"host": host, "alias": alias, "replace": true})
	return id
}

func (p *Ping) FindByHost(host string) (int, string, bool) {
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

func (p *Ping) SetEnabled(id int, enabled bool) {
	p.mu.Lock()
	if i := p.find(id); i >= 0 {
		p.rows[i].Enabled = enabled
	}
	p.mu.Unlock()
	p.post("/api/ping/enable", api.IDsReq{IDs: []int{id}, Enabled: enabled})
}

func (p *Ping) ReplaceAlias(id int, alias string) { p.SetAlias(id, alias) }

func (p *Ping) SetAlias(id int, alias string) {
	p.mu.Lock()
	if i := p.find(id); i >= 0 {
		p.rows[i].Alias = alias
	}
	p.mu.Unlock()
	p.post("/api/ping/alias", map[string]any{"id": id, "alias": alias})
}

func (p *Ping) Remove(id int) {
	p.mu.Lock()
	if i := p.find(id); i >= 0 {
		p.rows = append(p.rows[:i:i], p.rows[i+1:]...)
	}
	p.mu.Unlock()
	p.post("/api/ping/delete", api.IDsReq{IDs: []int{id}})
}

func (p *Ping) ResetStats(id int) {
	p.mu.Lock()
	for i := range p.rows {
		if id == 0 || p.rows[i].ID == id {
			p.rows[i].Stats = pinger.Stats{}
		}
	}
	p.mu.Unlock()
	if id == 0 {
		p.post("/api/ping/reset", api.IDsReq{Confirm: true}) // the GUI asked first
	} else {
		p.post("/api/ping/reset", api.IDsReq{IDs: []int{id}, Confirm: true})
	}
}

func (p *Ping) Snapshots() []pinger.Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]pinger.Snapshot(nil), p.rows...)
}

// TargetsForConfig is empty: remote targets never go into the local config.
func (p *Ping) TargetsForConfig() []pinger.ConfigTarget { return nil }

func (p *Ping) Start() {
	p.mu.Lock()
	p.running = true
	p.mu.Unlock()
	p.post("/api/ping/start", nil)
}

func (p *Ping) Stop() {
	p.mu.Lock()
	was := p.running
	p.running = false
	p.mu.Unlock()
	if was {
		p.post("/api/ping/stop", nil)
	}
}

func (p *Ping) Running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

func (p *Ping) IndexOf(id int) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.find(id)
}

func (p *Ping) MoveByIndex(from, to int) {
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
func (p *Ping) ReorderByIndices(rows []int, to int) {
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

func (p *Ping) move(id int, up bool) bool {
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

func (p *Ping) MoveUp(id int) bool   { return p.move(id, true) }
func (p *Ping) MoveDown(id int) bool { return p.move(id, false) }
