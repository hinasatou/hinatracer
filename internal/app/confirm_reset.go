package app

import (
	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
)

// confirmUI is the "permanently clear saved stats?" dialog.
type confirmUI struct {
	open bool
	n    int
	run  func()
}

// statsSaved reports whether table stats are saved to disk (here, or on
// the remote server in remote mode), so a reset is permanent.
func (a *app) statsSaved() bool {
	if rc := a.remote; rc != nil {
		st, _ := rc.State()
		return st.Settings.HistorySave
	}
	return a.hist != nil && a.hist.Enabled()
}

// confirmReset runs reset at once when nothing is saved; otherwise it asks
// first (n = number of targets). Cancel does nothing.
func (a *app) confirmReset(n int, reset func()) {
	if n <= 0 {
		return
	}
	if !a.statsSaved() {
		reset()
		return
	}
	a.resetAsk = confirmUI{open: true, n: n, run: reset}
}

func (a *app) viewConfirmResetModal(c *ui.Context) {
	q := &a.resetAsk
	if !q.open {
		return
	}
	ui.Modal(c, &q.open, func() {
		t := c.Theme()
		ui.Text(c, i18n.T("reset.confirm.title")).FontSize(15).Bold()
		ui.Text(c, i18n.Tf("reset.confirm.desc", q.n)).FontSize(12).TextColor(t.TextMuted).MaxWidth(440)
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			if ui.Button(c, i18n.T("reset.confirm.cancel")).Clicked() {
				*q = confirmUI{}
			}
			if ui.PrimaryButton(c, i18n.T("reset.confirm.ok")).Background(t.Danger).Clicked() {
				run := q.run
				*q = confirmUI{}
				if run != nil {
					run()
				}
			}
		})
	})
}
