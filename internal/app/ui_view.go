package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/flagx"
	"hinatracer/internal/pinger"
	"hinatracer/internal/version"
)

const (
	appVersion = version.Version // single source: internal/version

	// Default window sizes (a saved window-state.json overrides them).
	mainWinW      = 1460
	mainWinH      = 980
	detailWinW    = 690
	detailWinH    = 900
	detailWinMinW = 560
	detailWinMinH = 840
	// Glass details window: the chart pane holds the range selector, the
	// plot (at least glassDetailPlotMinH), the time axis and the loss line.
	glassDetailPlotMinH      = float32(120)
	glassDetailChartPaneMinH = float32(16 + 36 + 6 + 120 + 6 + 18 + 16 + 2)
	appRepoURL               = version.RepoURL
	contentMinH              = float32(200)
	pageTableMinH            = float32(280)
)

func (a *app) view(c *ui.Context) {
	a.viewGlass(c)
}

// detailAction is an export or copy chosen in a detail window's menu.
type detailAction struct {
	format string // "" = nothing chosen
	copy   bool
}

// detailExportMenu is a detail window's 「导出」 menu: CSV, TXT and
// Markdown files, and TXT and Markdown to the clipboard.
func detailExportMenu(c *ui.Context) detailAction {
	var act detailAction
	ui.MenuButton(c, i18n.T("detail.export"), func(m *ui.Menu) {
		for _, it := range []struct {
			key    string
			format string
			copy   bool
		}{
			{"history.export.button", "csv", false}, {"history.export.txt", "txt", false}, {"history.export.md", "md", false},
			{"", "", false},
			{"copy.txt", "txt", true}, {"copy.md", "md", true},
		} {
			if it.key == "" {
				m.Separator()
				continue
			}
			if m.Item(i18n.T(it.key)).Chosen() {
				act = detailAction{format: it.format, copy: it.copy}
			}
		}
	})
	return act
}

// statusPill is a top-bar status badge: a soft horizontal wash of col
// with a hairline of it, legible on light and dark.
func statusPill(c *ui.Context, text string, col ui.Color) ui.Element {
	return ui.Badge(c, text).TextColor(col).Border(1, col.Alpha(0.28)).
		LinearGradient(ui.LinearGradient{From: col.Alpha(0.24), To: col.Alpha(0.10), Angle: 90, Oklab: true})
}

func (a *app) viewTopBar(c *ui.Context) {
	t := c.Theme()
	title, sub := i18n.T("top.trace.title"), i18n.T("top.trace.sub")
	switch a.nav {
	case "ping":
		title, sub = i18n.T("top.ping.title"), i18n.T("top.ping.sub")
	case "http":
		title, sub = i18n.T("top.http.title"), i18n.T("top.http.sub")
	case "iplookup":
		title, sub = i18n.T("top.lookup.title"), i18n.T("top.lookup.sub")
	case "settings":
		title, sub = i18n.T("top.settings.title"), i18n.T("top.settings.sub")
	case "webui":
		title, sub = i18n.T("top.webui.title"), i18n.T("top.webui.sub")
	case "about":
		title, sub = i18n.T("top.about.title"), i18n.T("top.about.sub")
	}
	bar := ui.Row(c).Padding(14, 20).Gap(12).AlignItems(ui.Center).Shrink(0)
	bar.Padding(0, 0, 0, 0).DragWindow()

	bar.Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(16).Bold()
			ui.Text(c, sub).FontSize(12).TextColor(t.TextMuted)
		})
		ui.Spacer(c)
		if a.nav == "trace" && a.anyTraceRunning() {
			statusPill(c, i18n.T("badge.tracing"), t.Accent)
		}
		if a.notice != "" && time.Since(a.noticeAt) < 8*time.Second {
			ui.Text(c, a.notice).FontSize(12).TextColor(t.Danger).SingleLine().MinWidth(0)
		}
		if a.remote != nil {
			st, _ := a.remote.Status()
			col := t.Success
			if st != "connected" {
				col = t.Danger
			}
			statusPill(c, i18n.T("remote.badge")+" · "+i18n.T("remote.status."+st), col)
		}
		if a.nav == "http" && a.remote == nil && a.httpMgr.Running() {
			statusPill(c, i18n.T("badge.monitoring"), t.Success)
		}
		if st := a.web.srv.Status(); st.Running {
			statusPill(c, i18n.T("badge.web"), t.Accent)
		}
		if a.nav == "ping" && a.pingMgr.Running() {
			statusPill(c, i18n.T("badge.monitoring"), t.Success)
		}
		glassWindowButtons(c, a.glassMainWin())
	})
}

func card(c *ui.Context, body func()) ui.Element {
	return glassCardChildren(c, glassCard(c), body)
}

func regionCell(c *ui.Context, iso, name string) {
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		if bmp := flagx.Bitmap(iso); bmp != nil {
			ui.Image(c, bmp).Size(20, 14).Radius(2).Label(iso)
		} else if flag := flagx.Emoji(iso); flag != "" {
			ui.Text(c, flag).Font("Segoe UI Emoji, Segoe UI").FontSize(14).SingleLine()
		}
		label := flagx.NameOnly(iso, name)
		ui.Text(c, label).SingleLine().Grow(1)
	})
}

func mutedIfPlaceholder(c *ui.Context, el ui.Element, val string) {
	if val == "" || val == "…" || val == i18n.T("cell.loading") || val == i18n.T("cell.resolving") {
		el.TextColor(c.Theme().TextMuted)
	}
}

func clearListSelection(tbl ui.Element, selected *int) {
	if tbl.Clicked() && selected != nil && *selected >= 0 {
		*selected = -1
	}
}

func (a *app) viewTraceroute(c *ui.Context) {
	ui.Row(c).Gap(8).AlignItems(ui.Stretch).Grow(1).MinWidth(0).MinHeight(contentMinH).Children(func() {
		glassPaneEl(c).Width(196).Shrink(0).Padding(12, 8).Children(func() { a.viewTraceTabList(c) })
		ui.Column(c).Grow(1).MinWidth(0).Children(func() { a.viewTraceContent(c) })
	})
}

func (a *app) viewTraceTabList(c *ui.Context) {
	t := c.Theme()
	// Snapshot so close/reorder during this frame cannot panic mid-loop.
	tabs := append([]*traceTab(nil), a.traceTabs...)
	n := len(tabs)
	pendingClose := -1
	pendingSelect := -1
	var pendingCtx struct {
		closeOthers bool
		duplicate   bool
		start       bool
		stop        bool
		idx         int
	}
	pendingAdd := false
	pendingStartAll := false
	pendingStopAll := false
	var pendingExport traceExportReq

	ro := a.readOnly()
	pendingMove := [2]int{-1, -1}
	list := ui.Column(c).Gap(8).Padding(8, 8).FillHeight().MinWidth(0)
	list.Padding(0).Gap(4)

	list.Children(func() {
		ui.Row(c).Gap(2).AlignItems(ui.Center).Padding(0, 0, 4, 4).Children(func() {
			ui.Text(c, i18n.T("nav.trace")).FontSize(12).TextColor(t.TextMuted).Grow(1).SingleLine()
			if ro {
				ui.Badge(c, i18n.T("remote.readonly")).Background(t.TextMuted.Alpha(0.18)).TextColor(t.TextMuted)
				return
			}
			if glassGhostIcon(c, icoPlay, i18n.T("trace.tab.start_all"), !traceTabsCanStartAll(tabs)) {
				pendingStartAll = true
			}
			if glassGhostIcon(c, icoStop, i18n.T("trace.tab.stop_all"), !traceTabsCanStopAll(tabs)) {
				pendingStopAll = true
			}
			if glassGhostIcon(c, icoPlus, i18n.T("trace.tab.add"), false) {
				pendingAdd = true
			}
		})
		scroll := ui.Scroll(c).Grow(1).MinHeight(0).MinWidth(0)
		scroll.Children(func() {
			col := ui.Column(c).Gap(4).MinWidth(0).Focusable()
			// Keyboard: Up/Down change active tab when the list column has focus.
			if col.Shortcut(0, ui.KeyDown) && a.traceActive+1 < n {
				pendingSelect = a.traceActive + 1
			}
			if col.Shortcut(0, ui.KeyUp) && a.traceActive > 0 {
				pendingSelect = a.traceActive - 1
			}
			if col.Shortcut(0, ui.KeyHome) && n > 0 {
				pendingSelect = 0
			}
			if col.Shortcut(0, ui.KeyEnd) && n > 0 {
				pendingSelect = n - 1
			}
			col.Children(func() {
				for i := 0; i < n; i++ {
					ti := tabs[i]
					on := i == a.traceActive
					idx := i
					// Every state shares one fixed height, padding and border width;
					// hover/selection only change background, shadow and border colour.
					row := ui.Row(c).Gap(8).Padding(0, 10).Radius(8).AlignItems(ui.Center).MinWidth(0).
						Height(traceTabRowH()).Shrink(0).Border(1, ui.Color{})
					row.Padding(0, 8, 0, 12).Radius(10)

					if on {
						row.Background(glassSelBG(t)).Shadow(0, 1, 6, 0, glassSelShadow(t))
					} else if row.Hovered() {
						row.Background(t.SurfaceHover)
					}
					row.Children(func() {
						dotColor := t.TextMuted
						if ti.isRunning() {
							dotColor = t.Success
						}
						ui.Box(c).Size(8, 8).Radius(4).Background(dotColor).Shrink(0)
						label := ti.title()
						runes := []rune(label)
						if len(runes) > 22 {
							label = string(runes[:20]) + "…"
						}
						txt := ui.Text(c, label).FontSize(12).SingleLine().Grow(1).MinWidth(0)
						txt.FontSize(13)
						if !on {
							txt.TextColor(t.Text.Alpha(0.78))
						}

						// The close slot is always reserved at a fixed size; the ×
						// is only drawn inside it when hovered/selected.
						showClose := !ro && (on || row.Hovered())
						cs := traceTabCloseSlot()
						ui.Row(c).Size(cs, cs).Shrink(0).Center().Children(func() {
							if !showClose {
								return
							}
							if glassGhostIcon(c, icoX, i18n.T("trace.tab.ctx.close"), false) {
								pendingClose = idx
							}
						})
					})
					if row.Clicked() && pendingClose < 0 {
						pendingSelect = idx
					}
					row.ContextMenu(func(m *ui.Menu) {
						traceExportMenu(m, ti.ID, &pendingExport)
						m.Separator()
						if ro {
							m.Item(i18n.T("remote.readonly")).Disabled(true)
							return
						}
						if m.Item(i18n.T("ping.ctx.move_up")).Disabled(idx == 0).Chosen() {
							pendingMove = [2]int{idx, idx - 1}
						}
						if m.Item(i18n.T("ping.ctx.move_down")).Disabled(idx >= n-1).Chosen() {
							pendingMove = [2]int{idx, idx + 1}
						}
						m.Separator()
						if m.Item(i18n.T("trace.tab.ctx.close")).Chosen() {
							pendingClose = idx
						}
						if m.Item(i18n.T("trace.tab.ctx.close_others")).Disabled(n <= 1).Chosen() {
							pendingCtx.closeOthers = true
							pendingCtx.idx = idx
						}
						if m.Item(i18n.T("trace.tab.ctx.duplicate")).Chosen() {
							pendingCtx.duplicate = true
							pendingCtx.idx = idx
						}
						m.Separator()
						if ti.isRunning() {
							if m.Item(i18n.T("trace.tab.ctx.stop")).Chosen() {
								pendingCtx.stop = true
								pendingCtx.idx = idx
							}
						} else {
							if m.Item(i18n.T("trace.tab.ctx.start")).Disabled(strings.TrimSpace(ti.Host) == "").Chosen() {
								pendingCtx.start = true
								pendingCtx.idx = idx
							}
						}
					})
				}
			})
		})
	})

	if pendingExport.set { // guests may export too
		pendingExport.run(a)
	}
	if ro { // guests only switch tabs
		if pendingSelect >= 0 && pendingSelect < len(a.traceTabs) {
			a.traceActive = pendingSelect
		}
		return
	}
	if pendingMove[0] >= 0 {
		a.moveTraceTabTo(pendingMove[0], pendingMove[1])
	} else if pendingAdd {
		a.addTraceTabEmpty()
		a.persist()
	} else if pendingStartAll {
		a.startAllTraces()
		a.persist()
	} else if pendingStopAll {
		a.stopAllTraceTabs()
		a.persist()
	} else if pendingClose >= 0 {
		a.closeTraceTab(pendingClose)
	} else if pendingSelect >= 0 && pendingSelect < len(a.traceTabs) {
		a.traceActive = pendingSelect
		a.persist()
	} else if pendingCtx.closeOthers {
		a.closeOtherTraceTabs(pendingCtx.idx)
	} else if pendingCtx.duplicate {
		a.duplicateTraceTab(pendingCtx.idx)
	} else if pendingCtx.start && pendingCtx.idx >= 0 && pendingCtx.idx < len(a.traceTabs) {
		a.traceActive = pendingCtx.idx
		a.startTraceTab(a.traceTabs[pendingCtx.idx])
		a.persist()
	} else if pendingCtx.stop && pendingCtx.idx >= 0 && pendingCtx.idx < len(a.traceTabs) {
		a.stopTraceTab(a.traceTabs[pendingCtx.idx])
		a.traceTabs[pendingCtx.idx].Status = i18n.T("trace.status.canceled")
		a.persist()
	}
}

func (a *app) viewTraceContent(c *ui.Context) {
	t := c.Theme()
	tab := a.activeTrace()
	ro := a.readOnly()

	tc := ui.Column(c).Gap(14).Padding(0, 4).MinWidth(0).FillHeight()
	tc.Gap(8).Padding(0)

	tc.Children(func() {
		card(c, func() {
			ui.Text(c, i18n.T("trace.target")).FontSize(13).Bold()
			ui.Text(c, i18n.T("trace.desc")).FontSize(12).TextColor(t.TextMuted)
			ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().MinWidth(0).Children(func() {
				ui.TextInput(c, &tab.Host).Placeholder(i18n.T("trace.placeholder")).Label(i18n.T("trace.label")).Grow(1).MinWidth(180).Disabled(ro)
				if ui.TextInput(c, &tab.Alias).Placeholder(i18n.T("trace.alias_ph")).Label(i18n.T("trace.alias")).Width(140).Disabled(ro).Changed() {
					a.persist()
				}
				if ro {
					// guests watch only
				} else if tab.isRunning() {
					if ui.Button(c, i18n.T("trace.stop")).Clicked() {
						a.stopTrace()
					}
				} else {
					if ui.PrimaryButton(c, i18n.T("trace.start")).Clicked() {
						a.persist()
						a.startTrace()
					}
				}
			})
			ui.Row(c).Gap(12).AlignItems(ui.Center).Wrap().MinWidth(0).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, i18n.T("trace.mtr")).FontSize(12)
					if ui.Switch(c, &tab.MTR).Disabled(ro).Changed() {
						a.persist()
					}
				})
				ivl := ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(6, 10).Radius(8)
				ivl.Padding(0, 8, 0, 0)

				ivl.Children(func() {
					ui.Text(c, i18n.T("trace.interval")).FontSize(12).TextColor(t.TextMuted)
					if ui.NumberInput(c, &tab.Interval, 0.2, 3600, 0.5).Disabled(ro).Changed() {
						a.applyTraceInterval(tab)
					}
					ui.Text(c, i18n.T("ping.seconds")).FontSize(12).TextColor(t.TextMuted)
				})
				if !ro && ui.Button(c, i18n.T("trace.reset")).Disabled(tab.isRunning()).Clicked() {
					a.clearTraceTab(tab)
				}
			})
			if tab.Status != "" {
				ui.Text(c, tab.Status).FontSize(12).TextColor(t.TextMuted)
			}
		}).Shrink(0)

		card(c, func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, i18n.T("trace.hops")).FontSize(14).Bold()
				ui.Spacer(c)
				if tab.isRunning() {
					ui.Spinner(c)
				}
				var req traceExportReq
				ui.MenuButton(c, i18n.T("trace.export"), func(m *ui.Menu) {
					traceExportItems(m, tab.ID, &req)
				})
				if req.set {
					req.run(a)
				}
			})
			a.viewRangeBar(c, &a.rangeTrace, "trace", tab.ID, nil)
			display := a.sortedTraceHops(tab)
			cols := traceColumns()
			nRows := len(display)
			if nRows == 0 {
				ui.Column(c).Padding(16).AlignItems(ui.Center).Gap(4).Children(func() {
					ui.Text(c, i18n.T("trace.empty")).FontSize(12).TextColor(t.TextMuted)
				})
			}
			keys := make([]int, len(display))
			for i, h := range display {
				keys[i] = h.TTL
			}
			// rows re-sort as samples arrive: keep the scroll offset, not the top row
			tab.Table.KeepOffset = true
			tab.Table.Key = func(i int) any {
				if i >= 0 && i < len(keys) {
					return keys[i]
				}
				return -1 - i
			}
			var post []func()
			tbl := ui.Table(c, &tab.Table, cols, nRows, func(row, col int) {
				selCell(c, cols[col].Align == ui.End, func() {
					h := display[row]
					failing := hopIsFailing(h)
					switch col {
					case 0:
						ui.Text(c, fmt.Sprintf("%d", h.TTL)).SingleLine()
					case 1:
						addr := h.Addr
						if h.Timeout && h.Stats.Success == 0 {
							addr = "*"
						}
						txt := ui.Text(c, addr).SingleLine()
						if h.Reached {
							txt.TextColor(t.Accent).Bold()
						} else if h.Timeout {
							txt.TextColor(t.Warning)
						}
					case 2:
						txt := ui.Text(c, h.RDNS).SingleLine()
						mutedIfPlaceholder(c, txt, h.RDNS)
					case 3:
						txt := ui.Text(c, h.Location).SingleLine()
						mutedIfPlaceholder(c, txt, h.Location)
					case 4:
						regionCell(c, h.ISO, h.Region)
					case 5:
						txt := ui.Text(c, emptyDash(h.ASN)).SingleLine()
						mutedIfPlaceholder(c, txt, h.ASN)
					case 6:
						regionCell(c, h.ASNISO, h.ASNRegion)
					case 7:
						ui.Textf(c, "%d", h.Stats.Success).SingleLine()
					case 8:
						txt := ui.Textf(c, "%d", h.Stats.Failure).SingleLine()
						if h.Stats.Failure > 0 {
							txt.TextColor(t.Danger)
						}
					case 9:
						rate := h.Stats.SuccessRate()
						ui.Row(c).Gap(4).AlignItems(ui.Center).Justify(ui.End).Children(func() {
							if failing {
								ui.Badge(c, "⚠").Background(t.Danger.Alpha(0.14)).TextColor(t.Danger)
							}
							txt := ui.Textf(c, "%.1f%%", rate).SingleLine()
							if failing {
								txt.TextColor(t.Danger).Bold()
							} else if h.Stats.Success+h.Stats.Failure > 0 && rate >= 99.5 {
								txt.TextColor(t.Success)
							}
						})
					case 10:
						lr := h.Stats.LossRate()
						txt := ui.Textf(c, "%.1f%%", lr).SingleLine()
						if lr > 0 {
							txt.TextColor(t.Danger)
						}
					case 11:
						ui.Text(c, fmtLatency(h.Stats.Last, h.Stats.Success > 0)).SingleLine()
					case 12:
						ui.Text(c, fmtLatency(h.Stats.Avg, h.Stats.Success > 0)).SingleLine()
					case 13:
						ui.Text(c, fmtLatency(h.Stats.Min, h.Stats.Success > 0)).SingleLine()
					case 14:
						ui.Text(c, fmtLatency(h.Stats.Max, h.Stats.Success > 0)).SingleLine()
					case 15:
						ui.Text(c, fmtLatency(h.Stats.Median, h.Stats.Success > 0)).SingleLine()
					case 16:
						ui.Text(c, fmtTime(h.Stats.LastOK)).SingleLine()
					case 17:
						txt := ui.Text(c, fmtTime(h.Stats.LastFail)).SingleLine()
						if failing {
							txt.TextColor(t.Danger)
						}
					}
				}, func(m *ui.Menu) {
					rightSelect(&tab.Sel, &tab.Selected, keys[row], row)
					a.traceMenu(m, tab, display, keys, &post)
				})
			}).Grow(1).MinWidth(0).MinHeight(pageTableMinH).Label(i18n.T("trace.hops"))
			clearTableSelection(tbl, &tab.Selected, &tab.Sel)
			a.noteLayout("trace", tab.Table.Columns)

			if tbl.Submitted() && submitAllowed(keys, &tab.Sel, tab.Selected) {
				ids := selectedKeys(keys, &tab.Sel, tab.Selected)
				for _, h := range display {
					if h.TTL == ids[0] {
						a.openHopDetail(h)
						break
					}
				}
			}

			tbl.ContextMenu(func(m *ui.Menu) {
				a.traceMenu(m, tab, display, keys, &post)
			})
			for _, f := range post {
				f()
			}
		}).Grow(1).MinWidth(0).MinHeight(contentMinH)
	})
}

func hopIsFailing(h traceHopView) bool {
	st := h.Stats
	if st.Success+st.Failure == 0 {
		return false
	}
	if st.Success == 0 && st.Failure > 0 {
		return true
	}
	if st.Failure > 0 && !st.LastFail.IsZero() && (st.LastOK.IsZero() || st.LastFail.After(st.LastOK)) {
		return true
	}
	if st.SuccessRate() < 80 && st.Failure >= 2 {
		return true
	}
	return false
}

func (a *app) sortedTraceHops(tab *traceTab) []traceHopView {
	if tab == nil {
		return nil
	}
	tab.mu.Lock()
	src := append([]traceHopView(nil), tab.Hops...)
	tab.mu.Unlock()
	src = a.rangeOverlayHops(tab, src)
	n := len(src)
	if n == 0 {
		return src
	}
	col := tab.Sort.Column
	desc := tab.Sort.Descending
	if col == "" || col == "ttl" {
		if !desc {
			return src
		}
		out := make([]traceHopView, n)
		for i := range src {
			out[i] = src[n-1-i]
		}
		return out
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	less := func(i, j int) bool {
		sa, sb := src[idx[i]], src[idx[j]]
		var cmp int
		switch col {
		case "ip":
			cmp = strings.Compare(sa.Addr, sb.Addr)
		case "rdns":
			cmp = strings.Compare(strings.ToLower(sa.RDNS), strings.ToLower(sb.RDNS))
		case "location":
			cmp = strings.Compare(strings.ToLower(sa.Location), strings.ToLower(sb.Location))
		case "region":
			cmp = strings.Compare(strings.ToLower(sa.Region), strings.ToLower(sb.Region))
			if cmp == 0 {
				cmp = strings.Compare(sa.ISO, sb.ISO)
			}
		case "asn":
			cmp = strings.Compare(strings.ToLower(sa.ASN), strings.ToLower(sb.ASN))
		case "asn_region":
			cmp = strings.Compare(strings.ToLower(sa.ASNRegion), strings.ToLower(sb.ASNRegion))
			if cmp == 0 {
				cmp = strings.Compare(sa.ASNISO, sb.ASNISO)
			}
		case "success":
			cmp = cmpInt(sa.Stats.Success, sb.Stats.Success)
		case "failure":
			cmp = cmpInt(sa.Stats.Failure, sb.Stats.Failure)
		case "success_rate":
			cmp = cmpFloat(sa.Stats.SuccessRate(), sb.Stats.SuccessRate())
		case "loss_rate":
			cmp = cmpFloat(sa.Stats.LossRate(), sb.Stats.LossRate())
		case "last":
			cmp = cmpFloat(sa.Stats.Last, sb.Stats.Last)
		case "avg":
			cmp = cmpFloat(sa.Stats.Avg, sb.Stats.Avg)
		case "min":
			cmp = cmpFloat(sa.Stats.Min, sb.Stats.Min)
		case "max":
			cmp = cmpFloat(sa.Stats.Max, sb.Stats.Max)
		case "median":
			cmp = cmpFloat(sa.Stats.Median, sb.Stats.Median)
		case "last_ok":
			cmp = cmpTime(sa.Stats.LastOK, sb.Stats.LastOK)
		case "last_fail":
			cmp = cmpTime(sa.Stats.LastFail, sb.Stats.LastFail)
		default:
			cmp = cmpInt(sa.TTL, sb.TTL)
		}
		if cmp == 0 {
			return idx[i] < idx[j]
		}
		if desc {
			return cmp > 0
		}
		return cmp < 0
	}
	for i := 1; i < n; i++ {
		for j := i; j > 0 && less(j, j-1); j-- {
			idx[j], idx[j-1] = idx[j-1], idx[j]
		}
	}
	out := make([]traceHopView, n)
	for i, j := range idx {
		out[i] = src[j]
	}
	return out
}

func (a *app) viewPing(c *ui.Context) {
	t := c.Theme()
	if a.pingSnaps == nil {
		a.pingSnaps = a.pingMgr.Snapshots()
	}

	ro := a.readOnly()
	card(c, func() {
		if ro {
			ui.Badge(c, i18n.T("remote.readonly")).Background(t.TextMuted.Alpha(0.18)).TextColor(t.TextMuted)
			return
		}
		ui.Text(c, i18n.T("ping.add_title")).FontSize(13).Bold()
		ui.Row(c).Gap(10).AlignItems(ui.End).Wrap().MinWidth(0).Children(func() {
			ui.TextInput(c, &a.pingHost).Placeholder(i18n.T("ping.host_ph")).Label(i18n.T("ping.host")).Width(glassPick(220, 300))
			ui.TextInput(c, &a.pingAlias).Placeholder(i18n.T("ping.alias_ph")).Label(i18n.T("ping.alias")).Width(glassPick(140, 180))
			if ui.PrimaryButton(c, i18n.T("ping.add")).Clicked() {
				a.addPingTarget()
			}
			if ui.Button(c, i18n.T("ping.import")).Clicked() {
				a.beginImport()
			}
		})

		ui.Divider(c)

		ui.Column(c).Gap(8).MinWidth(0).Children(func() {
			ui.Text(c, i18n.T("ping.control")).FontSize(13).Bold()
			ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().MinWidth(0).Children(func() {
				ivl := ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(6, 10).Radius(8)
				ivl.Padding(0, 8, 0, 0)

				ivl.Children(func() {
					ui.Text(c, i18n.T("ping.interval")).FontSize(12).TextColor(t.TextMuted)
					if ui.NumberInput(c, &a.pingInterval, 0.2, 3600, 0.5).Changed() {
						a.applyInterval()
					}
					ui.Text(c, i18n.T("ping.seconds")).FontSize(12).TextColor(t.TextMuted)
				})

				running := a.pingMgr.Running()
				if running {
					if ui.Button(c, i18n.T("ping.stop")).Clicked() {
						a.pingMgr.Stop()
					}
				} else {
					if ui.PrimaryButton(c, i18n.T("ping.start")).Clicked() {
						a.applyInterval()
						a.pingMgr.Start()
						a.pingSnaps = a.pingMgr.Snapshots()
						a.refreshAllHostMeta()
						a.refreshAllPingRDNS()
					}
				}
				if ui.Button(c, i18n.T("ping.reset")).Clicked() {
					a.confirmReset(len(a.pingSnaps), func() {
						a.resetPingStats(0)
						a.pingSnaps = a.pingMgr.Snapshots()
					})
				}
				selIDs := selectedKeys(pingKeys(a.sortedPingSnaps()), &a.pingSel, a.pingSelected)
				if ui.Button(c, i18n.T("ping.delete_sel")).Disabled(len(selIDs) == 0).Clicked() {
					a.deletePingIDs(selIDs)
				}
			})
		})

		status := i18n.T("ping.status.stopped")
		statusColor := t.TextMuted
		if a.pingMgr.Running() {
			status = i18n.Tf("ping.status.running", a.pingInterval)
			statusColor = t.Success
		}
		ui.Text(c, status).FontSize(12).TextColor(statusColor)
		if loading, dbStat := a.pingDBStatus(); loading {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c)
				ui.Text(c, i18n.T("db.loading")).FontSize(12).TextColor(t.TextMuted)
			})
		} else if dbStat != "" {
			ui.Text(c, dbStat).FontSize(11).TextColor(t.TextMuted)
		}
		if a.importResult != "" {
			ui.Text(c, a.importResult).FontSize(12).TextColor(t.Accent)
		}
	}).Shrink(0)

	tbl := card(c, func() {
		display := a.sortedPingSnaps()
		keys := pingKeys(display)
		var exportPick tableExportPick
		a.viewRangeBar(c, &a.rangePing, "ping", 0, func() {
			exportPick = tableExportMenu(c, len(display), selectionCount(keys, &a.pingSel, a.pingSelected))
		})
		cols := pingColumns()
		n := len(display)
		if n == 0 {
			ui.Column(c).Padding(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, i18n.T("ping.empty")).FontSize(12).TextColor(t.TextMuted)
			})
		}
		if !ro && a.pingSort.Column == "#" && !a.pingSort.Descending {
			a.pingTable.Reorder = func(rows []int, to int) {
				a.reorderPingRows(rows, to)
			}
		} else {
			a.pingTable.Reorder = nil
		}
		a.syncAliasEditsFor(display)
		// rows re-sort as samples arrive: keep the scroll offset, not the top row
		a.pingTable.KeepOffset = true
		a.pingTable.Key = func(i int) any {
			if i >= 0 && i < len(keys) {
				return keys[i]
			}
			return -1 - i
		}
		var post []func()
		tbl := ui.Table(c, &a.pingTable, cols, n, func(row, col int) {
			selCell(c, cols[col].Align == ui.End, func() {
				s := display[row]
				meta := a.hostMetaOf(s.Host)
				loc, region, iso := meta.Loc, meta.Region, meta.ISO
				failing := pingIsFailing(s)
				cfgIdx := a.pingMgr.IndexOf(s.ID)
				muted := !s.Enabled
				cellColor := func(el ui.Element) {
					if muted {
						el.TextColor(t.TextMuted)
					}
				}
				switch col {
				case 0:
					ord := "—"
					if cfgIdx >= 0 {
						ord = fmt.Sprintf("%d", cfgIdx+1)
					}
					ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
						txt := ui.Text(c, ord).SingleLine()
						cellColor(txt)
						if muted {
							ui.Badge(c, i18n.T("badge.disabled")).Background(t.TextMuted.Alpha(0.18)).TextColor(t.TextMuted)
						}
					})
				case 1:
					if ro {
						ui.Text(c, emptyDash(s.Alias)).SingleLine()
					} else if ui.EditableText(c, &a.aliasEdits[row]).Changed() {
						a.pingMgr.SetAlias(s.ID, a.aliasEdits[row])
						a.refreshPingSnaps()
						a.persist()
					}
				case 2:
					ui.Row(c).Gap(4).AlignItems(ui.Center).MinWidth(0).Children(func() {
						if s.Proto == pinger.ProtoTCP {
							ui.Badge(c, "TCP").Background(t.Accent.Alpha(0.14)).TextColor(t.Accent)
						}
						txt := ui.Text(c, s.Host).SingleLine()
						cellColor(txt)
					})
				case 3:
					ip := a.pingIPOf(s)
					txt := ui.Text(c, ip).SingleLine()
					if muted {
						cellColor(txt)
					} else {
						mutedIfPlaceholder(c, txt, ip)
					}
				case 4:
					rdns := a.pingRDNSOf(s.Host)
					txt := ui.Text(c, rdns).SingleLine()
					if muted {
						cellColor(txt)
					} else {
						mutedIfPlaceholder(c, txt, rdns)
					}
				case 5:
					txt := ui.Text(c, loc).SingleLine()
					if muted {
						cellColor(txt)
					} else {
						mutedIfPlaceholder(c, txt, loc)
					}
				case 6:
					regionCell(c, iso, region)
				case 7:
					txt := ui.Text(c, meta.ASN).SingleLine()
					if muted {
						cellColor(txt)
					} else {
						mutedIfPlaceholder(c, txt, meta.ASN)
					}
				case 8:
					regionCell(c, meta.ASNISO, meta.ASNRegion)
				case 9:
					txt := ui.Textf(c, "%d", s.Stats.Success).SingleLine()
					cellColor(txt)
				case 10:
					txt := ui.Textf(c, "%d", s.Stats.Failure).SingleLine()
					if s.Stats.Failure > 0 && !muted {
						txt.TextColor(t.Danger)
					} else {
						cellColor(txt)
					}
				case 11:
					rate := s.Stats.SuccessRate()
					ui.Row(c).Gap(4).AlignItems(ui.Center).Justify(ui.End).Children(func() {
						if failing && !muted {
							ui.Badge(c, "⚠").Background(t.Danger.Alpha(0.14)).TextColor(t.Danger)
						}
						txt := ui.Textf(c, "%.1f%%", rate).SingleLine()
						if muted {
							txt.TextColor(t.TextMuted)
						} else if failing {
							txt.TextColor(t.Danger).Bold()
						} else if s.Stats.Success+s.Stats.Failure > 0 && rate >= 99.5 {
							txt.TextColor(t.Success)
						}
					})
				case 12:
					lr := s.Stats.LossRate()
					txt := ui.Textf(c, "%.1f%%", lr).SingleLine()
					if lr > 0 && !muted {
						txt.TextColor(t.Danger)
					} else {
						cellColor(txt)
					}
				case 13:
					txt := ui.Text(c, fmtLatency(s.Stats.Last, s.Stats.Success > 0)).SingleLine()
					cellColor(txt)
				case 14:
					txt := ui.Text(c, fmtLatency(s.Stats.Avg, s.Stats.Success > 0)).SingleLine()
					cellColor(txt)
				case 15:
					txt := ui.Text(c, fmtLatency(s.Stats.Min, s.Stats.Success > 0)).SingleLine()
					cellColor(txt)
				case 16:
					txt := ui.Text(c, fmtLatency(s.Stats.Max, s.Stats.Success > 0)).SingleLine()
					cellColor(txt)
				case 17:
					txt := ui.Text(c, fmtLatency(s.Stats.Median, s.Stats.Success > 0)).SingleLine()
					cellColor(txt)
				case 18:
					txt := ui.Text(c, fmtTime(s.Stats.LastOK)).SingleLine()
					cellColor(txt)
				case 19:
					txt := ui.Text(c, fmtTime(s.Stats.LastFail)).SingleLine()
					if failing && !muted {
						txt.TextColor(t.Danger)
					} else {
						cellColor(txt)
					}
				}
			}, func(m *ui.Menu) {
				rightSelect(&a.pingSel, &a.pingSelected, keys[row], row)
				a.pingMenu(m, display, keys, &post)
			})
		}).Grow(1).MinWidth(0).MinHeight(pageTableMinH).Label(i18n.T("ping.list_label"))
		clearTableSelection(tbl, &a.pingSelected, &a.pingSel)
		a.notePingLayout()

		if tbl.Submitted() && submitAllowed(keys, &a.pingSel, a.pingSelected) {
			ids := selectedKeys(keys, &a.pingSel, a.pingSelected)
			for _, s := range display {
				if len(ids) == 1 && s.ID == ids[0] {
					a.openPingDetail(s)
					break
				}
			}
		}

		tbl.ContextMenu(func(m *ui.Menu) {
			a.pingMenu(m, display, keys, &post)
		})
		for _, f := range post {
			f()
		}
		if exportPick.set {
			ks := keys
			if exportPick.selected {
				ks = selectedKeys(keys, &a.pingSel, a.pingSelected)
			}
			a.runTableExport("ping", exportPick, tableExportQuery(exportPick, arrangedColumnIDs(cols, a.pingTable.Columns), ks, a.pingSort, a.rangePing.activeQuery("ping")))
		}

		a.viewAliasEditModal(c)
		a.viewImportModal(c)
		a.viewDupModal(c)
		a.viewConflictModal(c)
	}).Grow(1).MinWidth(0).MinHeight(glassMinH(contentMinH)).Clip()
	_ = tbl
	if glassPingChart != nil {
		glassPingChart(c)
	}
}

func (a *app) viewAliasEditModal(c *ui.Context) {
	ui.Modal(c, &a.aliasEditOpen, func() {
		ui.Text(c, i18n.T("alias.title")).FontSize(15).Bold()
		ui.Text(c, i18n.T("alias.desc")).FontSize(12).TextColor(c.Theme().TextMuted)
		ui.TextInput(c, &a.aliasEditText).Placeholder(i18n.T("ping.alias_ph")).Label(i18n.T("ping.alias")).MinWidth(260)
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			if ui.Button(c, i18n.T("alias.cancel")).Clicked() {
				a.aliasEditOpen = false
			}
			if ui.PrimaryButton(c, i18n.T("alias.ok")).Clicked() {
				a.applyAliasEdit()
			}
		})
	})
}

func (a *app) viewImportModal(c *ui.Context) {
	ui.Modal(c, &a.importOpen, func() {
		ui.Text(c, i18n.T("import.title")).FontSize(15).Bold()
		ui.Text(c, i18n.T("import.desc")).FontSize(12).TextColor(c.Theme().TextMuted)
		ui.TextArea(c, &a.importText).Placeholder(i18n.T("import.placeholder")).MinWidth(420).Height(200)
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			if ui.Button(c, i18n.T("import.cancel")).Clicked() {
				a.importOpen = false
			}
			if ui.PrimaryButton(c, i18n.T("import.submit")).Clicked() {
				a.submitImport()
			}
		})
	})
}

func (a *app) viewDupModal(c *ui.Context) {
	ui.Modal(c, &a.dupOpen, func() {
		ui.Text(c, i18n.T("dup.title")).FontSize(15).Bold()
		ui.Text(c, i18n.Tf("dup.desc", a.dupHost, emptyDash(a.dupOldAlias), emptyDash(a.dupNewAlias))).
			FontSize(12).TextColor(c.Theme().TextMuted)
		ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
			if ui.Button(c, i18n.T("dup.skip")).Clicked() {
				a.dupOpen = false
				a.pingHost = ""
				a.pingAlias = ""
			}
			if ui.PrimaryButton(c, i18n.T("dup.replace")).Clicked() {
				a.applyDupReplace()
			}
		})
	})
}

func (a *app) viewConflictModal(c *ui.Context) {
	ui.Modal(c, &a.conflictOpen, func() {
		t := c.Theme()
		ui.Text(c, i18n.T("import.conflict.title")).FontSize(15).Bold()
		ui.Text(c, i18n.T("import.conflict.desc")).FontSize(12).TextColor(t.TextMuted)
		ui.Column(c).Gap(4).MaxHeight(180).Children(func() {
			for _, conf := range a.conflicts {
				ui.Text(c, i18n.Tf("import.conflict.item", conf.Host, emptyDash(conf.OldAlias), emptyDash(conf.NewAlias))).
					FontSize(12)
			}
		})
		ui.Row(c).Gap(10).Justify(ui.End).Wrap().Children(func() {
			if ui.Button(c, i18n.T("import.conflict.cancel")).Clicked() {
				a.conflictOpen = false
				a.conflicts = nil
				a.pendingAdds = nil
			}
			if ui.Button(c, i18n.T("import.conflict.skip_all")).Clicked() {
				a.resolveConflicts(false)
			}
			if ui.PrimaryButton(c, i18n.T("import.conflict.replace_all")).Clicked() {
				a.resolveConflicts(true)
			}
		})
	})
}

func pingIsFailing(s pinger.Snapshot) bool {
	st := s.Stats
	if st.Success+st.Failure == 0 {
		return false
	}
	if st.Success == 0 && st.Failure > 0 {
		return true
	}
	if st.Failure > 0 && !st.LastFail.IsZero() && (st.LastOK.IsZero() || st.LastFail.After(st.LastOK)) {
		return true
	}
	if st.SuccessRate() < 80 && st.Failure >= 2 {
		return true
	}
	return false
}

func detailShowDisabledBadge(isHop bool, enabled bool) bool {
	return !isHop && !enabled
}

func (a *app) viewPingDetail(c *ui.Context) {
	t := c.Theme()
	s := a.detailSnap
	meta := a.hostMetaOf(s.Host)
	loc, region, iso := meta.Loc, meta.Region, meta.ISO
	// names its subject, as the window/taskbar title and trend window do
	title := detailWinTitle(s, a.detailIsHop, a.detailHopTTL)
	var root ui.Element
	// pane groups the sections; on glass each is a floating pane.
	pane := func(grow bool, body func()) { body() }
	dark := t.Dark
	c.SetTheme(glassTheme(dark))
	t = c.Theme()
	glassGround(c, dark)
	a.detailFit.apply(winOrNil(a.detailWin), detailMinW())
	root = glassRootPad(c, ui.Column(c).Fill().Gap(8))
	root.Draw(func(_ *ui.Painter, r ui.Rect) { a.detailFit.have = r.H })
	pane = func(grow bool, body func()) {
		el := glassPaneEl(c).Gap(10)
		if grow {
			// Grow into spare height but never shrink below the content
			// (chart + axis labels), so the action bar can't cover it.
			el.Grow(1).MinHeight(glassDetailChartPaneMinH)
		}
		el.Children(body)
	}

	root.Children(func() {
		// macOS: the title sits on the traffic lights' row.
		mac := glassMacHeader(c, title, nil)
		// The whole title pane moves the window (as the main window's
		// title bar does); its buttons stay clickable.
		glassPaneEl(c).Gap(10).DragWindow().Children(func() {
			if !mac {
				ui.Row(c).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
					ui.Text(c, title).FontSize(16).Bold()
					ui.Spacer(c)
					glassWindowButtons(c, winOrNil(a.detailWin))
				})
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, s.Host).FontSize(13).TextColor(t.TextMuted).Grow(1)
				if detailShowDisabledBadge(a.detailIsHop, s.Enabled) {
					ui.Badge(c, i18n.T("badge.disabled")).Background(t.TextMuted.Alpha(0.18)).TextColor(t.TextMuted)
				}
				if pingIsFailing(s) {
					ui.Badge(c, i18n.T("badge.abnormal")).Background(t.Danger.Alpha(0.14)).TextColor(t.Danger)
				}
			})
		})
		firstLabel, firstVal := i18n.T("detail.alias"), emptyDash(s.Alias)
		if a.detailIsHop {
			firstLabel, firstVal = i18n.T("detail.ttl"), fmt.Sprintf("%d", a.detailHopTTL)
		}
		rows := [][2]string{
			{firstLabel, firstVal},
			{i18n.T("detail.host"), s.Host},
			{i18n.T("detail.ip"), a.pingIPOf(s)},
			{i18n.T("detail.proto"), string(pinger.ProtoOf(s.Host))},
			{i18n.T("detail.rdns"), a.pingRDNSOf(s.Host)},
			{i18n.T("detail.location"), loc},
			{i18n.T("detail.region"), flagx.NameOnly(iso, region)},
			{i18n.T("detail.asn"), meta.ASN},
			{i18n.T("detail.asn_region"), flagx.NameOnly(meta.ASNISO, meta.ASNRegion)},
			{i18n.T("detail.success"), fmt.Sprintf("%d", s.Stats.Success)},
			{i18n.T("detail.failure"), fmt.Sprintf("%d", s.Stats.Failure)},
			{i18n.T("detail.rate"), fmt.Sprintf("%.1f%%", s.Stats.SuccessRate())},
			{i18n.T("detail.loss"), fmt.Sprintf("%.1f%%", s.Stats.LossRate())},
			{i18n.T("detail.last"), fmtLatency(s.Stats.Last, s.Stats.Success > 0)},
			{i18n.T("detail.avg"), fmtLatency(s.Stats.Avg, s.Stats.Success > 0)},
			{i18n.T("detail.min"), fmtLatency(s.Stats.Min, s.Stats.Success > 0)},
			{i18n.T("detail.max"), fmtLatency(s.Stats.Max, s.Stats.Success > 0)},
			{i18n.T("detail.median"), fmtLatency(s.Stats.Median, s.Stats.Success > 0)},
			{i18n.T("detail.last_ok"), fmtTime(s.Stats.LastOK)},
			{i18n.T("detail.last_fail"), fmtTime(s.Stats.LastFail)},
			{i18n.T("detail.samples"), fmt.Sprintf("%d", len(s.Stats.Latencies))},
		}
		row := func(r [2]string, labelW float32) {
			ui.Row(c).Gap(12).AlignItems(ui.Center).MinWidth(0).Children(func() {
				ui.Text(c, r[0]).Width(labelW).TextColor(t.TextMuted).FontSize(13)
				if r[0] == i18n.T("detail.region") {
					regionCell(c, iso, region)
				} else if r[0] == i18n.T("detail.asn_region") {
					regionCell(c, meta.ASNISO, meta.ASNRegion)
				} else {
					txt := ui.Text(c, r[1]).FontSize(13).Grow(1)
					if r[0] == i18n.T("detail.failure") && s.Stats.Failure > 0 {
						txt.TextColor(t.Danger)
					}
					if r[0] == i18n.T("detail.rate") && pingIsFailing(s) {
						txt.TextColor(t.Danger).Bold()
					}
				}
			})
		}
		// Identity rows (long values) take the full width; the statistics
		// rows go in two columns so the latency chart fits the window.
		const identityRows = 9
		pane(false, func() {
			for _, r := range rows[:identityRows] {
				row(r, 100)
			}
			ui.Divider(c)
			statRows := rows[identityRows:]
			half := (len(statRows) + 1) / 2
			ui.Row(c).Gap(16).MinWidth(0).Children(func() {
				for _, part := range [][][2]string{statRows[:half], statRows[half:]} {
					ui.Column(c).Gap(10).Grow(1).MinWidth(0).Children(func() {
						for _, r := range part {
							row(r, 100)
						}
					})
				}
			})
		})
		pane(true, func() { a.latencyChartView(c, s.Stats.History) })

		pane(false, func() {
			ui.Row(c).Gap(10).Justify(ui.End).AlignItems(ui.Center).MinWidth(0).Children(func() {
				if a.detailExportMsg != "" {
					ui.Text(c, a.detailExportMsg).FontSize(11).TextColor(t.TextMuted).Grow(1).MinWidth(0).Selectable()
				}
				if act := detailExportMenu(c); act.format != "" {
					if act.copy {
						a.copyDetail(act.format)
					} else {
						a.exportDetail(act.format)
					}
				}
				if ui.Button(c, i18n.T("detail.copy_all")).Clicked() {
					copyText(formatPingDetail(s, loc, iso, region, meta.ASN, meta.ASNISO, meta.ASNRegion, a.pingRDNSOf(s.Host), a.pingIPOf(s), a.detailIsHop, a.detailHopTTL))
				}
				if ui.Button(c, i18n.T("detail.traceroute")).Clicked() {
					w := a.detailWin
					a.jumpToTrace(s.Host)
					if w != nil {
						w.Close() // the main window shows the trace
					}
				}
				if ui.PrimaryButton(c, i18n.T("detail.close")).Clicked() {
					if w := a.detailWin; w != nil {
						w.Close()
					}
				}
			})
		})
		// Marks where the pane stack ends (after the root's padding).
		ui.Box(c).Height(0).Shrink(0).Draw(func(_ *ui.Painter, r ui.Rect) { a.detailFit.need = r.Y + 8 })
	})
}

func formatPingDetail(s pinger.Snapshot, loc, iso, region, asnText, asnISO, asnRegion, rdns, ip string, isHop bool, hopTTL int) string {
	var b strings.Builder
	if isHop {
		b.WriteString(i18n.T("detail.hop_title"))
	} else {
		b.WriteString(i18n.T("detail.title"))
	}
	b.WriteByte('\n')
	firstLabel, firstVal := i18n.T("detail.alias"), emptyDash(s.Alias)
	if isHop {
		firstLabel, firstVal = i18n.T("detail.ttl"), fmt.Sprintf("%d", hopTTL)
	}
	pairs := [][2]string{
		{firstLabel, firstVal},
		{i18n.T("detail.host"), s.Host},
		{i18n.T("detail.ip"), emptyDash(ip)},
		{i18n.T("detail.proto"), string(pinger.ProtoOf(s.Host))},
		{i18n.T("detail.rdns"), emptyDash(rdns)},
		{i18n.T("detail.location"), loc},
		{i18n.T("detail.region"), flagx.CopyLabel(iso, region)},
		{i18n.T("detail.asn"), emptyDash(asnText)},
		{i18n.T("detail.asn_region"), flagx.CopyLabel(asnISO, asnRegion)},
		{i18n.T("detail.success"), fmt.Sprintf("%d", s.Stats.Success)},
		{i18n.T("detail.failure"), fmt.Sprintf("%d", s.Stats.Failure)},
		{i18n.T("detail.rate"), fmt.Sprintf("%.1f%%", s.Stats.SuccessRate())},
		{i18n.T("detail.loss"), fmt.Sprintf("%.1f%%", s.Stats.LossRate())},
		{i18n.T("detail.last"), fmtLatency(s.Stats.Last, s.Stats.Success > 0)},
		{i18n.T("detail.avg"), fmtLatency(s.Stats.Avg, s.Stats.Success > 0)},
		{i18n.T("detail.min"), fmtLatency(s.Stats.Min, s.Stats.Success > 0)},
		{i18n.T("detail.max"), fmtLatency(s.Stats.Max, s.Stats.Success > 0)},
		{i18n.T("detail.median"), fmtLatency(s.Stats.Median, s.Stats.Success > 0)},
		{i18n.T("detail.last_ok"), fmtTime(s.Stats.LastOK)},
		{i18n.T("detail.last_fail"), fmtTime(s.Stats.LastFail)},
		{i18n.T("detail.samples"), fmt.Sprintf("%d", len(s.Stats.Latencies))},
	}
	for _, p := range pairs {
		b.WriteString(p[0])
		b.WriteString(": ")
		b.WriteString(p[1])
		b.WriteByte('\n')
	}
	return b.String()
}

func emptyDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func fmtLatency(ms float64, ok bool) string {
	if !ok {
		return "—"
	}
	return fmt.Sprintf("%.1f ms", ms)
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func (a *app) viewSettings(c *ui.Context) {
	t := c.Theme()
	ui.Scroll(c).Grow(1).Fill().MinWidth(0).MinHeight(0).TrackScroll(&a.settingsScroll).Children(func() {
		ui.Column(c).Gap(14).MinWidth(0).Padding(0, 0, 8, 0).Children(func() {
			if a.remote != nil {
				cardScope = scopeLocal
				defer func() { cardScope = scopeNone }()
				ui.Text(c, i18n.T("settings.scope.intro")).FontSize(12).TextColor(t.TextMuted)
				settingsGroupHeader(c, i18n.T("settings.scope.local_group"))
			}
			a.viewDataSource(c)
			card(c, func() {
				cardTitle(c, i18n.T("settings.appearance"))
				ui.Form(c, func() {
					ui.Field(c, i18n.T("settings.language"), func() {
						names := i18n.Names()
						if a.langName == "" {
							a.langName = i18n.Name(i18n.Language())
						}
						if ui.Select(c, &a.langName, names).Changed() {
							a.applyLanguage(a.langName)
						}
					}).Description(i18n.T("settings.language_desc"))

					ui.Field(c, i18n.T("settings.theme"), func() {
						opts := []string{
							i18n.T("settings.theme.light"),
							i18n.T("settings.theme.dark"),
							i18n.T("settings.theme.system"),
						}
						if a.themeName == "" {
							a.themeName = themeDisplayName(a.cfg.Theme)
						}
						if ui.Select(c, &a.themeName, opts).Changed() {
							a.applyTheme(a.themeName)
						}
					}).Description(i18n.T("settings.theme_desc"))
					a.glassPalettePicker(c)
				})
				langDir := filepath.Join(configDirHint(), "lang")
				ui.Text(c, i18n.Tf("settings.lang_packs", langDir)).FontSize(12).TextColor(t.TextMuted)
			})
			a.viewSettingsAlerts(c)
			if a.remote != nil {
				a.viewSettingsRemoteGroups(c)
				return
			}
			a.viewLocalOnlySettings(c)
		})
	})
}

// viewLocalOnlySettings are the cards that configure this computer's own
// monitoring (data files, startup, history, downloads, config).
func (a *app) viewLocalOnlySettings(c *ui.Context) {
	t := c.Theme()
	card(c, func() {
		cardTitle(c, i18n.T("settings.data"))
		ui.Text(c, i18n.T("settings.data_desc")).FontSize(12).TextColor(t.TextMuted)

		ui.Form(c, func() {
			ui.Field(c, i18n.T("settings.ipdb"), func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
					ui.TextInput(c, &a.draftIPDB).Placeholder(i18n.T("settings.ipdb_ph")).Grow(1).MinWidth(0)
					if ui.Button(c, i18n.T("settings.browse")).Clicked() {
						a.pickFile(i18n.T("settings.pick_ipdb"), []string{"ipdb"}, &a.draftIPDB)
					}
				})
			}).Description(i18n.T("settings.ipdb_desc"))

			ui.Field(c, i18n.T("settings.geoip"), func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
					ui.TextInput(c, &a.draftGeoIP).Placeholder(i18n.T("settings.geoip_ph")).Grow(1).MinWidth(0)
					if ui.Button(c, i18n.T("settings.browse")).Clicked() {
						a.pickFile(i18n.T("settings.pick_geoip"), []string{"mmdb"}, &a.draftGeoIP)
					}
				})
			}).Description(i18n.T("settings.geoip_desc"))

			ui.Field(c, i18n.T("settings.asn"), func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
					ui.TextInput(c, &a.draftASN).Placeholder(i18n.T("settings.asn_ph")).Grow(1).MinWidth(0)
					if ui.Button(c, i18n.T("settings.browse")).Clicked() {
						a.pickFile(i18n.T("settings.pick_asn"), []string{"tsv", "gz"}, &a.draftASN)
					}
				})
			}).Description(i18n.T("settings.asn_desc"))
		})

		ui.Row(c).Gap(10).Wrap().Children(func() {
			if ui.PrimaryButton(c, i18n.T("settings.save_load")).Clicked() {
				a.saveSettings()
			}
			if ui.Button(c, i18n.T("settings.reload")).Clicked() {
				a.reloadDBsAsync("db.reload")
			}
		})
		if _, dbStat := a.getDBStatus(); dbStat != "" {
			ui.Text(c, dbStat).FontSize(12).TextColor(t.TextMuted)
		}
	})

	card(c, func() {
		cardTitle(c, i18n.T("settings.startup"))
		ui.Text(c, i18n.T("settings.startup_desc")).FontSize(12).TextColor(t.TextMuted)
		ui.Checkbox(c, &a.cfg.AutoStartPing, i18n.T("settings.autostart"))
		ui.Checkbox(c, &a.cfg.AutoStartHTTP, i18n.T("settings.autostart_http"))
		ui.Checkbox(c, &a.cfg.AutoStartTrace, i18n.T("settings.autostart_trace"))
		if ui.Button(c, i18n.T("settings.save_startup")).Clicked() {
			a.persist()
		}
	})

	if a.remote == nil {
		a.viewSettingsUpdate(c)
		a.viewSettingsLocTr(c)
	}
	a.viewSettingsHistory(c)

	card(c, func() {
		cardTitle(c, i18n.T("settings.dl_all.title"))
		ui.Text(c, i18n.T("settings.dl_all.desc")).FontSize(12).TextColor(t.TextMuted)
		running := a.dataDL != nil && a.dataDL.Running()
		ui.Row(c).Gap(10).Wrap().Children(func() {
			btn := ui.PrimaryButton(c, i18n.T("settings.dl_all.start"))
			if running {
				btn.Disabled(true)
			}
			if btn.Clicked() && !running {
				a.startDataDownload()
			}
			cbtn := ui.Button(c, i18n.T("settings.dl_all.cancel"))
			if !running {
				cbtn.Disabled(true)
			}
			if cbtn.Clicked() && running {
				a.dataDL.Cancel()
				a.taskbarEnd(a.dlTB)
			}
		})
		if a.dataDL != nil {
			if running {
				a.syncDataDLTaskbar()
			}
			for _, p := range a.dataDL.Snapshot() {
				line := formatDataDLProgress(p)
				col := t.TextMuted
				if p.State == "error" {
					col = t.Danger
				} else if p.State == "ok" {
					col = t.Success
				}
				ui.Text(c.Key("datadl-"+string(p.Kind)), line).FontSize(12).TextColor(col)
			}
		}
		if a.dataDLStatus != "" {
			ui.Text(c, a.dataDLStatus).FontSize(12).TextColor(t.TextMuted)
		}
	})

	card(c, func() {
		cardTitle(c, i18n.T("settings.downloads"))
		ui.Column(c).Gap(8).Children(func() {
			ui.Text(c, i18n.T("settings.dl.ipdb")).FontSize(13)
			ui.Text(c, "  https://github.com/nmgliangwei/qqwry.ipdb").FontSize(12).TextColor(t.Accent)
			ui.Text(c, "  https://cdn.bili33.top/gh/nmgliangwei/qqwry.ipdb@main/qqwry.ipdb").FontSize(12).TextColor(t.TextMuted)
			ui.Text(c, "  https://cdn.jsdelivr.net/npm/qqwry.ipdb/qqwry.ipdb").FontSize(12).TextColor(t.TextMuted)
			if ui.Button(c, i18n.T("settings.dl.open_ipdb")).Clicked() {
				safeGo("shell.open", func() { mygoShellOpen("https://github.com/nmgliangwei/qqwry.ipdb") })
			}
			ui.Text(c, i18n.T("settings.dl.geoip")).FontSize(13)
			ui.Text(c, "  https://github.com/Loyalsoldier/geoip/releases").FontSize(12).TextColor(t.Accent)
			ui.Text(c, "  https://cdn.jsdelivr.net/gh/Loyalsoldier/geoip@release/Country.mmdb").FontSize(12).TextColor(t.TextMuted)
			if ui.Button(c, i18n.T("settings.dl.open_geoip")).Clicked() {
				safeGo("shell.open", func() { mygoShellOpen("https://github.com/Loyalsoldier/geoip/releases") })
			}
			ui.Text(c, i18n.T("settings.dl.asn")).FontSize(13)
			ui.Text(c, "  https://iptoasn.com/data/ip2asn-combined.tsv.gz").FontSize(12).TextColor(t.Accent)
			ui.Text(c, "  https://iptoasn.com/").FontSize(12).TextColor(t.TextMuted)
			if ui.Button(c, i18n.T("settings.dl.open_asn")).Clicked() {
				safeGo("shell.open", func() { mygoShellOpen("https://iptoasn.com/") })
			}
		})
	})

	card(c, func() {
		cardTitle(c, i18n.T("settings.flags"))
		ui.Text(c, i18n.T("settings.flags_desc")).FontSize(12).TextColor(t.TextMuted)
	})

	card(c, func() {
		cardTitle(c, i18n.T("settings.config"))
		ui.Text(c, i18n.T("settings.config_desc")).FontSize(12).TextColor(t.TextMuted)
		ui.Text(c, a.store.Path()).FontSize(12)
		ui.Text(c, i18n.Tf("settings.config_dir", configDirHint())).FontSize(12).TextColor(t.TextMuted)
	})
}

func (a *app) viewAbout(c *ui.Context) {
	t := c.Theme()
	card(c, func() {
		ui.Text(c, i18n.T("about.name")).FontSize(20).Bold()
		ui.Text(c, i18n.Tf("about.version", appVersion)).FontSize(13).TextColor(t.TextMuted)
		a.viewAboutUpdate(c)
		ui.Divider(c)
		ui.Text(c, i18n.T("about.desc")).FontSize(13)
		ui.Text(c, i18n.T("about.license")).FontSize(12).TextColor(t.TextMuted)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
			ui.Text(c, i18n.T("about.homepage")).FontSize(13)
			link := ui.Text(c, appRepoURL).FontSize(13).TextColor(t.Accent)
			if link.Clicked() {
				safeGo("shell.open", func() { mygoShellOpen(appRepoURL) })
			}
		})
		if ui.Button(c, i18n.T("about.open_github")).Clicked() {
			safeGo("shell.open", func() { mygoShellOpen(appRepoURL) })
		}
	})
}

var mygoShellOpen = func(url string) {}

// pingDBStatus is the IP database state shown on the Ping page: the
// server's in remote mode (its lookups use its databases), else ours.
func (a *app) pingDBStatus() (loading bool, status string) {
	if a.remote != nil {
		st := a.remoteState()
		return st.Settings.DBLoading, st.Settings.DBStatus
	}
	return a.getDBStatus()
}

// traceColumns are the table columns (also listed by the copy-column menu).
func traceColumns() []ui.TableColumn { return traceColumnsT(i18n.T) }

// traceColumnsT is traceColumns with titles from tr (an export language).
func traceColumnsT(tr func(string) string) []ui.TableColumn {
	return []ui.TableColumn{
		{Title: tr("trace.col.ttl"), ID: "ttl", Width: 56, Align: ui.Center, Sortable: true, Fixed: true},
		{Title: tr("trace.col.ip"), ID: "ip", Width: 140, Sortable: true},
		{Title: tr("trace.col.rdns"), ID: "rdns", Width: 150, Sortable: true},
		{Title: tr("trace.col.location"), ID: "location", Width: 130, Sortable: true},
		{Title: tr("trace.col.region"), ID: "region", Width: 130, Sortable: true},
		{Title: tr("trace.col.asn"), ID: "asn", Width: 160, Sortable: true},
		{Title: tr("trace.col.asn_region"), ID: "asn_region", Width: 130, Sortable: true},
		{Title: tr("ping.col.success"), ID: "success", Width: 56, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.failure"), ID: "failure", Width: 56, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.rate"), ID: "success_rate", Width: 88, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.loss"), ID: "loss_rate", Width: 80, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.last"), ID: "last", Width: 72, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.avg"), ID: "avg", Width: 72, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.min"), ID: "min", Width: 72, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.max"), ID: "max", Width: 72, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.median"), ID: "median", Width: 72, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.last_ok"), ID: "last_ok", Width: 140, Sortable: true},
		{Title: tr("ping.col.last_fail"), ID: "last_fail", Width: 140, Sortable: true},
	}
}

// pingColumns are the table columns (also listed by the copy-column menu).
func pingColumns() []ui.TableColumn { return pingColumnsT(i18n.T) }

// pingColumnsT is pingColumns with titles from tr (an export language).
func pingColumnsT(tr func(string) string) []ui.TableColumn {
	return []ui.TableColumn{
		{Title: tr("ping.col.n"), ID: "#", Width: 44, Align: ui.Center, Sortable: true, Fixed: true},
		{Title: tr("ping.col.alias"), ID: "alias", Width: 100, Sortable: true},
		{Title: tr("ping.col.host"), ID: "host", Width: 148, Sortable: true},
		{Title: tr("ping.col.ip"), ID: "ip", Width: 128, Sortable: true},
		{Title: tr("ping.col.rdns"), ID: "rdns", Width: 160, Sortable: true},
		{Title: tr("ping.col.location"), ID: "location", Width: 140, Sortable: true},
		{Title: tr("ping.col.region"), ID: "region", Width: 140, Sortable: true},
		{Title: tr("ping.col.asn"), ID: "asn", Width: 180, Sortable: true},
		{Title: tr("ping.col.asn_region"), ID: "asn_region", Width: 140, Sortable: true},
		{Title: tr("ping.col.success"), ID: "success", Width: 56, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.failure"), ID: "failure", Width: 56, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.rate"), ID: "success_rate", Width: 88, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.loss"), ID: "loss_rate", Width: 80, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.last"), ID: "last", Width: 72, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.avg"), ID: "avg", Width: 72, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.min"), ID: "min", Width: 72, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.max"), ID: "max", Width: 72, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.median"), ID: "median", Width: 72, Align: ui.End, Sortable: true},
		{Title: tr("ping.col.last_ok"), ID: "last_ok", Width: 140, Sortable: true},
		{Title: tr("ping.col.last_fail"), ID: "last_fail", Width: 140, Sortable: true},
	}
}

// traceTabCloseSlot is the fixed square reserved for a trace tab's × button.
func traceTabCloseSlot() float32 { return glassPick(22, 28) }

// traceTabRowH is the fixed height of every trace tab row in every state.
func traceTabRowH() float32 { return traceTabCloseSlot() + 16 }

// detailMinH is the details windows' minimum height: the glass pane stack
// (header, info, stats, chart with its axis, action bar, 8px gaps).
func detailMinH() int {
	return glassDetailWinMinH
}

// glassDetailWinMinH fits the glass Ping details with the chart at its
// smallest (TestGlassDetailFits checks it).
const glassDetailWinMinH = 880

// paneFit grows a window whose pane stack is taller than the window (large
// fonts, a long title): need is where the stack ends, have the window's
// content height, both from the last frame's paint.
type paneFit struct {
	need, have float32
	tries      int
}

// apply raises the minimum height and the window when the last frame did
// not fit; it gives up after a few tries (e.g. capped by the screen).
func (f *paneFit) apply(w glassWin, minW int) {
	if f.have <= 0 || f.need <= f.have+0.5 || f.tries >= 3 {
		return
	}
	f.tries++
	ws, ok := w.(interface {
		Size() (int, int)
		SetSize(int, int)
		SetMinimumSize(int, int)
	})
	if !ok || ws == nil {
		return
	}
	cw, ch := ws.Size()
	add := int(f.need-f.have) + 1
	ws.SetMinimumSize(minW, ch+add)
	ws.SetSize(cw, ch+add)
}

// detailMinW: on glass the chart's range selector needs the default width.
func detailMinW() int {
	return detailWinW
}

// trendMinW: on glass the range selector must fit inside the chart pane.
func trendMinW() int {
	return 700
}
