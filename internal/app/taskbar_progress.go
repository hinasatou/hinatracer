package app

import "github.com/egoist/mygo"

// taskbarProgress tracks concurrent long tasks and mirrors them onto the
// main window's taskbar / Dock / launcher progress bar. UI thread only.
type taskbarProgress struct {
	n     int     // active tasks
	value float64 // 0..1 while determined; <0 = indeterminate
}

// taskbarBeginIndeterminate marks a long task of unknown length (export,
// first range query). Pair every begin with taskbarEnd.
func (a *app) taskbarBeginIndeterminate() {
	a.tbProg.n++
	a.tbProg.value = -1
	a.syncTaskbar()
}

// taskbarSetValue updates the fraction done (0..1). No-op with no task.
func (a *app) taskbarSetValue(v float64) {
	if a.tbProg.n == 0 {
		return
	}
	a.tbProg.value = max(0, min(1, v))
	a.syncTaskbar()
}

// taskbarEnd finishes one task and clears the bar when none remain.
// failed is reserved for a red flash; we clear either way so the bar
// never sticks after an error or cancel.
func (a *app) taskbarEnd(failed bool) {
	_ = failed
	if a.tbProg.n > 0 {
		a.tbProg.n--
	}
	if a.tbProg.n == 0 {
		a.taskbarClear()
		return
	}
	a.syncTaskbar()
}

// taskbarClear removes the bar (cancel, done, or quit).
func (a *app) taskbarClear() {
	a.tbProg = taskbarProgress{value: -1}
	if a.win != nil {
		a.win.SetProgressBar(mygo.ProgressBar{})
	}
}

func (a *app) syncTaskbar() {
	w := a.win
	if w == nil || a.tbProg.n == 0 {
		return
	}
	if a.tbProg.value < 0 {
		w.SetProgressBar(mygo.ProgressBar{State: mygo.ProgressIndeterminate})
		return
	}
	w.SetProgressBar(mygo.ProgressBar{Value: a.tbProg.value})
}
