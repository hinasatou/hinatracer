package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
)

// remoteServerUI is the draft state of the 「远程服务器设置」 card.
type remoteServerUI struct {
	inited                   bool
	lang, theme              string
	ipdb, geoip, asn         string
	startPing, startTrace    bool
	channel                  string
	checkOnRun               bool
	histSave                 bool
	histDays                 string
	webHost, webPort         string
	webAuto, guest           bool
	tokens                   []webTokenView
	tokName, tokRole, newTok string
	msg                      string
}

// syncRemoteServerDraft fills the drafts from the pushed state.
func (a *app) syncRemoteServerDraft(st wsState) {
	s := &a.rsrv
	s.lang = i18n.Name(st.Lang)
	s.theme = themeDisplayName(st.Settings.Theme)
	s.ipdb, s.geoip, s.asn = st.Settings.IPDB, st.Settings.GeoIP, st.Settings.ASN
	s.startPing, s.startTrace = st.Settings.AutoStartPing, st.Settings.AutoStartTrace
	s.channel = i18n.T("update.channel.stable")
	if st.Settings.UpdateChannel == "prerelease" {
		s.channel = i18n.T("update.channel.prerelease")
	}
	s.checkOnRun = st.Settings.UpdateCheckOnStart
	s.histSave, s.histDays = st.Settings.HistorySave, strconv.Itoa(st.Settings.HistoryDays)
	s.webHost, s.webAuto, s.guest = st.Web.Host, st.Web.AutoStart, st.Web.Guest
	s.webPort = ""
	if st.Web.Port > 0 {
		s.webPort = strconv.Itoa(st.Web.Port)
	}
	if s.tokRole == "" {
		s.tokRole = i18n.T("web.token.role.guest")
	}
	s.inited = true
}

// remoteSet posts a server setting and reports the outcome in the card.
func (a *app) remoteSet(path string, body any) {
	a.beAsync(path, body, func(r apiResult, err error) {
		if err != nil {
			a.rsrv.msg = err.Error()
			return
		}
		a.rsrv.msg = i18n.T("remote.server.saved")
		if r.Message != "" {
			a.rsrv.msg = r.Message
		}
	})
}

// loadRemoteTokens refreshes the token list in the background.
func (a *app) loadRemoteTokens() {
	rc := a.remote
	if rc == nil || a.readOnly() {
		return
	}
	safeGo("remote.tokens", func() {
		data, err := rc.Get("/api/web/tokens")
		var r struct {
			Data []webTokenView `json:"data"`
		}
		if err == nil && json.Unmarshal(data, &r) == nil {
			_ = a.run(func() { a.rsrv.tokens = r.Data })
			a.requestUIUpdate()
		}
	})
}

func (a *app) viewRemoteServerSettings(c *ui.Context) {
	t := c.Theme()
	st, have := a.remote.State()
	s := &a.rsrv
	if !have {
		return
	}
	ro := a.readOnly()
	if !s.inited {
		a.syncRemoteServerDraft(st)
		a.loadRemoteTokens()
	}
	card(c, func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, i18n.T("remote.server.title")).FontSize(14).Bold()
			if ro {
				ui.Badge(c, i18n.T("remote.readonly")).Background(t.TextMuted.Alpha(0.18)).TextColor(t.TextMuted)
			}
		})
		ui.Text(c, i18n.T("remote.server.desc")).FontSize(12).TextColor(t.TextMuted)
		if s.msg != "" {
			ui.Text(c, s.msg).FontSize(12).TextColor(t.Accent)
		}
		ui.Form(c, func() {
			ui.Field(c, i18n.T("remote.server.language"), func() {
				if ro {
					ui.Text(c, i18n.Name(st.Lang))
					return
				}
				if ui.Select(c, &s.lang, i18n.Names()).Changed() {
					for _, code := range i18n.Codes() {
						if i18n.Name(code) == s.lang {
							a.remoteSet("/api/settings/language", map[string]string{"code": code})
						}
					}
				}
			})
			ui.Field(c, i18n.T("remote.server.theme"), func() {
				if ro {
					ui.Text(c, themeDisplayName(st.Settings.Theme))
					return
				}
				opts := []string{i18n.T("settings.theme.light"), i18n.T("settings.theme.dark"), i18n.T("settings.theme.system")}
				if ui.Select(c, &s.theme, opts).Changed() {
					a.remoteSet("/api/settings/theme", map[string]string{"theme": themeFromDisplay(s.theme)})
				}
			})
		})
	})
	if !ro {
		card(c, func() {
			ui.Text(c, i18n.T("settings.data")).FontSize(14).Bold()
			ui.Form(c, func() {
				ui.Field(c, i18n.T("settings.ipdb"), func() { ui.TextInput(c, &s.ipdb).Placeholder(i18n.T("settings.ipdb_ph")).MinWidth(320) })
				ui.Field(c, i18n.T("settings.geoip"), func() { ui.TextInput(c, &s.geoip).Placeholder(i18n.T("settings.geoip_ph")).MinWidth(320) })
				ui.Field(c, i18n.T("settings.asn"), func() { ui.TextInput(c, &s.asn).Placeholder(i18n.T("settings.asn_ph")).MinWidth(320) })
			})
			ui.Row(c).Gap(8).Children(func() {
				if ui.PrimaryButton(c, i18n.T("settings.save_load")).Clicked() {
					a.remoteSet("/api/settings/data", map[string]string{"ipdb": s.ipdb, "geoip": s.geoip, "asn": s.asn})
				}
				if ui.Button(c, i18n.T("settings.reload")).Clicked() {
					a.remoteSet("/api/settings/reloaddb", nil)
				}
			})
			if st.Settings.DBStatus != "" {
				ui.Text(c, st.Settings.DBStatus).FontSize(11).TextColor(t.TextMuted)
			}
		})
	}
	card(c, func() {
		ui.Text(c, i18n.T("settings.dl_all.title")).FontSize(14).Bold()
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if !ro {
				if st.DataDL.Running {
					if ui.Button(c, i18n.T("settings.dl_all.cancel")).Clicked() {
						a.remoteSet("/api/datadl/cancel", nil)
					}
				} else if ui.PrimaryButton(c, i18n.T("settings.dl_all.start")).Clicked() {
					a.remoteSet("/api/datadl/start", nil)
				}
			}
			if st.DataDL.Status != "" {
				ui.Text(c, st.DataDL.Status).FontSize(12).TextColor(t.TextMuted)
			}
		})
		for _, f := range st.DataDL.Files {
			ui.Column(c.Key("rdl-" + f.Kind)).Gap(4).MinWidth(0).Children(func() {
				line := f.Filename
				if f.Text != "" {
					line += " · " + f.Text
				} else if f.State != "" {
					line += " · " + i18n.T("settings.dl_all.state."+f.State)
				}
				col := t.TextMuted
				if f.State == "error" {
					col = t.Danger
				}
				ui.Text(c, line).FontSize(12).TextColor(col).SingleLine()
				ui.Progress(c, max(0, min(1, f.Percent/100)))
			})
		}
	})
	card(c, func() {
		ui.Text(c, i18n.T("settings.startup")).FontSize(14).Bold()
		if ui.Checkbox(c, &s.startPing, i18n.T("settings.autostart")).Disabled(ro).Changed() ||
			ui.Checkbox(c, &s.startTrace, i18n.T("settings.autostart_trace")).Disabled(ro).Changed() {
			a.remoteSet("/api/settings/startup", map[string]bool{"autoStartPing": s.startPing, "autoStartTrace": s.startTrace})
		}
	})
	card(c, func() {
		ui.Text(c, i18n.T("settings.history.title")).FontSize(14).Bold()
		ui.Row(c).Gap(10).AlignItems(ui.End).Wrap().Children(func() {
			ui.Checkbox(c, &s.histSave, i18n.T("settings.history.save")).Disabled(ro)
			ui.TextInput(c, &s.histDays).Placeholder(i18n.T("ph.history.days")).Label(i18n.T("settings.history.days")).Width(120).Disabled(ro)
			if !ro && ui.Button(c, i18n.T("settings.history.apply")).Clicked() {
				d, err := strconv.Atoi(strings.TrimSpace(s.histDays))
				if err != nil || d < 1 || d > 3650 {
					s.msg = i18n.T("history.err.days")
				} else {
					a.remoteSet("/api/settings/history", map[string]any{"save": s.histSave, "days": d})
				}
			}
		})
	})
	card(c, func() {
		ui.Text(c, i18n.T("update.settings.title")).FontSize(14).Bold()
		ui.Form(c, func() {
			ui.Field(c, i18n.T("update.settings.channel"), func() {
				if ui.Select(c, &s.channel, []string{i18n.T("update.channel.stable"), i18n.T("update.channel.prerelease")}).Disabled(ro).Changed() {
					a.remoteUpdateSettings(false)
				}
			})
		})
		if ui.Checkbox(c, &s.checkOnRun, i18n.T("update.settings.on_start")).Disabled(ro).Changed() {
			a.remoteUpdateSettings(false)
		}
		if st.Settings.UpdateSkip != "" && !ro {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, i18n.Tf("update.settings.skipped", st.Settings.UpdateSkip)).FontSize(12)
				if ui.Button(c, i18n.T("update.settings.unskip")).Clicked() {
					a.remoteUpdateSettings(true)
				}
			})
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
			if !ro && ui.Button(c, i18n.T("update.check")).Clicked() {
				a.remoteSet("/api/update/check", nil)
			}
			if st.Update.Phase == "available" && !ro {
				if ui.Button(c, i18n.T("update.skip")).Clicked() {
					a.remoteSet("/api/update/skip", nil)
				}
				if st.Update.CanInstall && ui.PrimaryButton(c, i18n.T("update.now")).Clicked() {
					a.remoteSet("/api/update/install", nil)
				}
			}
			if st.Update.Text != "" {
				ui.Text(c, st.Update.Text).FontSize(12).TextColor(t.TextMuted)
			}
		})
	})
	if ro {
		return
	}
	card(c, func() {
		ui.Text(c, i18n.T("web.settings.title")).FontSize(14).Bold()
		ui.Row(c).Gap(10).AlignItems(ui.End).Wrap().Children(func() {
			ui.TextInput(c, &s.webHost).Placeholder(i18n.T("ph.web.host")).Label(i18n.T("web.listen.host")).Width(180)
			ui.TextInput(c, &s.webPort).Placeholder(i18n.T("ph.web.port")).Label(i18n.T("web.listen.port")).Width(100)
			ui.Checkbox(c, &s.webAuto, i18n.T("web.autostart"))
			if ui.Button(c, i18n.T("web.save")).Clicked() {
				p, err := strconv.Atoi(strings.TrimSpace(s.webPort))
				if err != nil || p < 1 || p > 65535 {
					s.msg = i18n.T("web.err.bad_port")
				} else {
					h, auto := strings.TrimSpace(s.webHost), s.webAuto
					a.remoteSet("/api/web/listen", map[string]any{"host": h, "port": p, "autoStart": auto})
				}
			}
		})
		ui.Text(c, i18n.T("remote.server.listen_note")).FontSize(11).TextColor(t.TextMuted)
		if ui.Checkbox(c, &s.guest, i18n.T("web.guest.allow")).Changed() {
			a.remoteSet("/api/web/guest", map[string]bool{"enabled": s.guest})
		}
		ui.Text(c, i18n.T("web.token.title")).FontSize(13).Bold()
		if s.newTok != "" {
			ui.Row(c).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
				ui.Text(c, s.newTok).FontSize(12).Selectable().SingleLine().MinWidth(0)
				if ui.Button(c, i18n.T("web.token.copy")).Clicked() {
					copyText(s.newTok)
				}
				if ui.Button(c, i18n.T("web.token.dismiss")).Clicked() {
					s.newTok = ""
				}
			})
			ui.Text(c, i18n.T("web.token.once")).FontSize(11).TextColor(t.Danger)
		}
		ui.Row(c).Gap(8).AlignItems(ui.End).Wrap().Children(func() {
			ui.TextInput(c, &s.tokName).Label(i18n.T("web.token.name")).Placeholder(i18n.T("web.token.name_ph")).Width(200)
			ui.Select(c, &s.tokRole, []string{i18n.T("web.token.role.guest"), i18n.T("web.token.role.admin")})
			if ui.Button(c, i18n.T("web.token.create")).Disabled(strings.TrimSpace(s.tokName) == "").Clicked() {
				role := "guest"
				if s.tokRole == i18n.T("web.token.role.admin") {
					role = "admin"
				}
				name := strings.TrimSpace(s.tokName)
				s.tokName = ""
				a.beAsync("/api/web/tokens/create", map[string]string{"name": name, "role": role}, func(r apiResult, err error) {
					if err != nil {
						s.msg = err.Error()
						return
					}
					if m, ok := r.Data.(map[string]any); ok {
						s.newTok, _ = m["token"].(string)
					}
					a.loadRemoteTokens()
				})
			}
		})
		if len(s.tokens) == 0 {
			ui.Text(c, i18n.T("web.token.none")).FontSize(12).TextColor(t.TextMuted)
		}
		for _, tk := range s.tokens {
			ui.Row(c.Key("rtok-" + tk.ID)).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
				ui.Text(c, tk.Name).FontSize(12).Bold().SingleLine()
				ui.Text(c, i18n.T("web.token.role."+tk.Role)).FontSize(12).TextColor(t.TextMuted)
				ui.Text(c, i18n.Tf("web.token.created_at", time.UnixMilli(tk.Created).Format("2006-01-02 15:04"))).FontSize(11).TextColor(t.TextMuted)
				ui.Spacer(c)
				if ui.Button(c, i18n.T("web.token.revoke")).Clicked() {
					id := tk.ID
					a.beAsync("/api/web/tokens/revoke", map[string]string{"id": id}, func(apiResult, error) { a.loadRemoteTokens() })
				}
			})
		}
	})
}

func (a *app) remoteUpdateSettings(unskip bool) {
	s := &a.rsrv
	ch := "stable"
	if s.channel == i18n.T("update.channel.prerelease") {
		ch = "prerelease"
	}
	a.remoteSet("/api/settings/update", map[string]any{"channel": ch, "checkOnRun": s.checkOnRun, "unskip": unskip})
}
