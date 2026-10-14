package main

import (
	"time"

	"hinatracer/internal/pinger"
)

// pingEngine is what the Ping page drives: the local pinger.Manager, or a
// mirror of a remote WebUI's ping list (remotePing) in remote mode. The same
// page code runs on both.
type pingEngine interface {
	SetOnSample(fn func(host, ip string, at time.Time, ms float64))
	SetOnUpdate(fn func())
	SetInterval(d time.Duration)
	Interval() time.Duration
	Add(host, alias string) int
	AddEnabled(host, alias string, enabled bool) int
	FindByHost(host string) (id int, alias string, ok bool)
	SetEnabled(id int, enabled bool)
	ReplaceAlias(id int, alias string)
	Remove(id int)
	SetAlias(id int, alias string)
	ResetStats(id int)
	Snapshots() []pinger.Snapshot
	TargetsForConfig() []pinger.ConfigTarget
	Start()
	Stop()
	Running() bool
	IndexOf(id int) int
	MoveByIndex(from, to int)
	ReorderByIndices(rows []int, to int)
	MoveUp(id int) bool
	MoveDown(id int) bool
}

var _ pingEngine = (*pinger.Manager)(nil)
var _ pingEngine = (*remotePing)(nil)
