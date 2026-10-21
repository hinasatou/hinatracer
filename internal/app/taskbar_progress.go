package app

import "github.com/egoist/mygo"

// taskbarProgress mirrors long tasks (exports, downloads, the updater, …)
// onto the main window's taskbar / Dock / launcher progress bar. Each task
// holds a token; the most recently begun task still running is shown, and
// the bar clears when the last one ends, so overlapping tasks never leave
// it stuck. UI thread only (goroutines go through a.run / tbRun).
type taskbarProgress struct {
	next  int
	tasks []tbTask // in begin order
	shown string   // last state sent, to skip repeats
}

type tbTask struct {
	id    int
	value float64 // 0..1, <0 = indeterminate
}

// taskbarBegin starts a task of unknown length and returns its token.
func (a *app) taskbarBegin() int {
	a.tbProg.next++
	id := a.tbProg.next
	a.tbProg.tasks = append(a.tbProg.tasks, tbTask{id: id, value: -1})
	a.syncTaskbar()
	return id
}

// taskbarSet sets task id's fraction done (0..1; <0 = indeterminate).
// Unknown or ended tokens are ignored.
func (a *app) taskbarSet(id int, v float64) {
	for i := range a.tbProg.tasks {
		if a.tbProg.tasks[i].id == id {
			if v >= 0 {
				v = min(v, 1)
			}
			a.tbProg.tasks[i].value = v
			a.syncTaskbar()
			return
		}
	}
}

// taskbarEnd ends task id (success, error or cancel alike). Ending a
// token twice, or 0, is harmless.
func (a *app) taskbarEnd(id int) {
	t := a.tbProg.tasks[:0]
	for _, k := range a.tbProg.tasks {
		if k.id != id {
			t = append(t, k)
		}
	}
	a.tbProg.tasks = t
	a.syncTaskbar()
}

// taskbarClear ends every task and removes the bar (before quitting).
func (a *app) taskbarClear() {
	a.tbProg.tasks = nil
	a.syncTaskbar()
}

// tbRun runs fn on the UI thread from a goroutine (directly without a window).
func (a *app) tbRun(fn func()) {
	if a.win == nil {
		fn()
		return
	}
	_ = a.run(fn)
}

// taskbarState is what the bar shows: "" none, "i" indeterminate, else
// the fraction of the newest task.
func (p *taskbarProgress) state() (string, float64) {
	if len(p.tasks) == 0 {
		return "", 0
	}
	k := p.tasks[len(p.tasks)-1]
	if k.value < 0 {
		return "i", 0
	}
	return "v", k.value
}

func (a *app) syncTaskbar() {
	st, v := a.tbProg.state()
	key := st
	if st == "v" {
		key = st + string(rune('0'+int(v*100)/10)) + string(rune('0'+int(v*100)%10))
	}
	if key == a.tbProg.shown {
		return
	}
	a.tbProg.shown = key
	w := a.win
	if w == nil {
		return
	}
	switch st {
	case "":
		w.SetProgressBar(mygo.ProgressBar{})
	case "i":
		w.SetProgressBar(mygo.ProgressBar{State: mygo.ProgressIndeterminate})
	default:
		w.SetProgressBar(mygo.ProgressBar{State: mygo.ProgressNormal, Value: v})
	}
}
