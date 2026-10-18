package app

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"hinatracer/i18n"
	"hinatracer/internal/webserver"
)

// viewWebUI is the 「WebUI」 page: bind address, password, start/stop,
// status and the URL to open. No network calls happen here; LAN URLs are
// gathered in the background when the server starts.
func (a *app) viewWebUI(c *ui.Context) {
	t := c.Theme()
	a.webUIInit()
	st := a.web.srv.Status()
	hasPW := a.cfg.WebPasswordHash != "" || webEnvPasswordHash() != ""
	ui.Scroll(c).Grow(1).Fill().MinWidth(0).MinHeight(0).Children(func() {
		ui.Column(c).Gap(14).MinWidth(0).Padding(0, 0, 8, 0).Children(func() {
			card(c, func() {
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Text(c, i18n.T("web.status.title")).FontSize(14).Bold()
					if st.Running {
						ui.Badge(c, i18n.T("web.status.running")).Background(t.Success.Alpha(0.18)).TextColor(t.Success)
					} else {
						ui.Badge(c, i18n.T("web.status.stopped")).Background(t.TextMuted.Alpha(0.15)).TextColor(t.TextMuted)
					}
				})
				ui.Text(c, i18n.T("web.desc")).FontSize(12).TextColor(t.TextMuted)
				if st.Running {
					url := st.URL
					ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
						ui.Text(c, i18n.T("web.url")).FontSize(13)
						link := ui.Text(c, url).FontSize(13).TextColor(t.Accent)
						if link.Clicked() {
							safeGo("shell.open", func() { mygoShellOpen(url) })
						}
						if ui.Button(c, i18n.T("web.copy")).Clicked() {
							copyText(url)
						}
						if ui.Button(c, i18n.T("web.open")).Clicked() {
							safeGo("shell.open", func() { mygoShellOpen(url) })
						}
					})
					for _, u := range a.web.lanURLs {
						ui.Text(c.Key("lan-"+u), i18n.Tf("web.lan_url", u)).FontSize(12).TextColor(t.TextMuted)
					}
				}
				if a.web.msg != "" {
					col := t.TextMuted
					if a.web.msgErr {
						col = t.Danger
					}
					ui.Text(c, a.web.msg).FontSize(12).TextColor(col)
				} else if st.Err != "" && !st.Running {
					ui.Text(c, i18n.Tf("web.err.start", st.Err)).FontSize(12).TextColor(t.Danger)
				}
			})

			card(c, func() {
				ui.Text(c, i18n.T("web.listen.title")).FontSize(14).Bold()
				ui.Form(c, func() {
					ui.Field(c, i18n.T("web.listen.host"), func() {
						ui.TextInput(c, &a.web.hostText).Placeholder(i18n.T("ph.web.host")).Grow(1).MinWidth(0)
					}).Description(i18n.T("web.listen.host_desc"))
					ui.Field(c, i18n.T("web.listen.port"), func() {
						ui.TextInput(c, &a.web.portText).Placeholder(i18n.T("ph.web.port")).Width(140)
					})
				})
				if st.Running {
					if cur, saved := webserver.JoinListen(st.Host, st.Port), webserver.JoinListen(a.webHostPort()); cur != saved {
						ui.Text(c, i18n.Tf("web.listen.active_differs", cur, saved)).FontSize(12).TextColor(t.Accent)
					}
				}
				host, _ := webserver.NormalizeHost(a.web.hostText)
				if host != "" && !webserver.IsLoopback(host) {
					col := t.Accent
					msg := i18n.T("web.warn.exposed")
					if !hasPW {
						col, msg = t.Danger, i18n.T("web.warn.need_password")
					}
					ui.Text(c, msg).FontSize(12).TextColor(col)
				}
				if ui.Checkbox(c, &a.cfg.WebAutoStart, i18n.T("web.autostart")).Changed() {
					a.persist()
				}
				ui.Row(c).Gap(10).Wrap().Children(func() {
					label := i18n.T("web.start")
					if st.Running {
						label = i18n.T("web.restart")
					}
					if ui.PrimaryButton(c, label).Clicked() {
						a.webUIApply(true)
					}
					if ui.Button(c, i18n.T("web.stop")).Disabled(!st.Running).Clicked() {
						a.webUIStop()
					}
					if ui.Button(c, i18n.T("web.save")).Clicked() {
						a.webUIApply(false)
					}
				})
			})

			card(c, func() {
				ui.Text(c, i18n.T("web.pw.title")).FontSize(14).Bold()
				ui.Text(c, i18n.T("web.pw.desc")).FontSize(12).TextColor(t.TextMuted)
				state := i18n.T("web.pw.none")
				if hasPW {
					state = i18n.T("web.pw.set")
				}
				ui.Text(c, state).FontSize(12)
				ui.Form(c, func() {
					ui.Field(c, i18n.T("web.pw.new"), func() {
						ui.TextInput(c, &a.web.pwText).Password().Placeholder(i18n.T("ph.web.pw_new")).Grow(1).MinWidth(0)
					})
					ui.Field(c, i18n.T("web.pw.confirm"), func() {
						ui.TextInput(c, &a.web.pwText2).Password().Placeholder(i18n.T("ph.web.pw_confirm")).Grow(1).MinWidth(0)
					})
				})
				ui.Row(c).Gap(10).Wrap().Children(func() {
					if ui.PrimaryButton(c, i18n.T("web.pw.save")).Clicked() {
						a.webUISetPassword()
					}
					if hasPW && ui.Button(c, i18n.T("web.pw.clear")).Clicked() {
						a.web.pwText, a.web.pwText2 = "", ""
						a.webUISetPassword()
					}
				})
				ui.Divider(c)
				guest := a.cfg.WebGuest && hasPW
				if ui.Checkbox(c, &guest, i18n.T("web.guest.allow")).Disabled(!hasPW).Changed() {
					if err := a.webSetGuest(guest); err != nil {
						a.web.msg, a.web.msgErr = err.Error(), true
					} else if guest {
						a.web.msg, a.web.msgErr = i18n.T("web.guest.on"), false
					} else {
						a.web.msg, a.web.msgErr = i18n.T("web.guest.off"), false
					}
				}
				desc := i18n.T("web.guest.desc")
				col := t.TextMuted
				if !hasPW {
					desc, col = i18n.T("web.guest.need_password"), t.Danger
				}
				ui.Text(c, desc).FontSize(12).TextColor(col)
			})

			a.viewWebTokens(c, st)

			card(c, func() {
				ui.Text(c, i18n.T("web.cli.title")).FontSize(14).Bold()
				ui.Text(c, i18n.T("web.cli.desc")).FontSize(12).TextColor(t.TextMuted)
				mono := "Consolas, Menlo, monospace"
				ui.Text(c, "hinatracer --web --listen 127.0.0.1:8790").FontSize(12).Font(mono)
				ui.Text(c, i18n.T("web.cli.client")).FontSize(12).TextColor(t.TextMuted)
				ui.Text(c, "hinatracer ping list --token <API token>").FontSize(12).Font(mono)
				ui.Text(c, "hinatracer help").FontSize(12).Font(mono)
			})
		})
	})
}

// viewWebTokens is the API token card plus the MCP address. Revoking is
// recorded during the loop and applied after it (no list mutation while
// the list is being built).
func (a *app) viewWebTokens(c *ui.Context, st webserver.Status) {
	t := c.Theme()
	card(c, func() {
		ui.Text(c, i18n.T("web.mcp.title")).FontSize(14).Bold()
		ui.Text(c, i18n.T("web.mcp.desc")).FontSize(12).TextColor(t.TextMuted)
		if st.Running {
			mcpURL := st.URL + "mcp"
			if !strings.HasSuffix(st.URL, "/") {
				mcpURL = st.URL + "/mcp"
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
				ui.Text(c, i18n.T("web.mcp.url")).FontSize(13)
				ui.Text(c, mcpURL).FontSize(13).TextColor(t.Accent).Font("Consolas, Menlo, monospace")
				if ui.Button(c, i18n.T("web.copy")).Clicked() {
					copyText(mcpURL)
				}
			})
		} else {
			ui.Text(c, i18n.T("web.mcp.stopped")).FontSize(12).TextColor(t.TextMuted)
		}
	})
	card(c, func() {
		ui.Text(c, i18n.T("web.token.title")).FontSize(14).Bold()
		ui.Text(c, i18n.T("web.token.desc")).FontSize(12).TextColor(t.TextMuted)
		if a.web.tokNew != "" {
			ui.Column(c).Gap(8).Padding(10).Radius(8).Background(t.Accent.Alpha(0.1)).MinWidth(0).Children(func() {
				ui.Text(c, i18n.Tf("web.token.created", a.web.tokNewNm)).FontSize(12)
				ui.Text(c, a.web.tokNew).FontSize(12).Font("Consolas, Menlo, monospace").Selectable()
				ui.Row(c).Gap(10).Children(func() {
					if ui.PrimaryButton(c, i18n.T("web.token.copy")).Clicked() {
						copyText(a.web.tokNew)
					}
					if ui.Button(c, i18n.T("web.token.dismiss")).Clicked() {
						a.web.tokNew, a.web.tokNewNm = "", ""
					}
				})
			})
		}
		ui.Form(c, func() {
			ui.Field(c, i18n.T("web.token.name"), func() {
				ui.TextInput(c, &a.web.tokName).Placeholder(i18n.T("web.token.name_ph")).Grow(1).MinWidth(0)
			})
			ui.Field(c, i18n.T("web.token.role"), func() {
				ui.Row(c).Children(func() {
					ui.Segmented(c, &a.web.tokRole, i18n.T("web.token.role.guest"), i18n.T("web.token.role.admin"))
				})
			})
		})
		ui.Row(c).Children(func() {
			if ui.PrimaryButton(c, i18n.T("web.token.create")).Clicked() {
				a.webUICreateToken()
			}
		})
		toks := a.webTokenList()
		if len(toks) == 0 {
			ui.Text(c, i18n.T("web.token.none")).FontSize(12).TextColor(t.TextMuted)
		}
		revoke := ""
		for _, tk := range toks {
			ui.Row(c.Key("tok-" + tk.ID)).Gap(10).AlignItems(ui.Center).MinWidth(0).Children(func() {
				ui.Text(c, tk.Name).FontSize(13).Bold().MinWidth(0)
				role := i18n.T("web.token.role.guest")
				bg, fg := t.TextMuted.Alpha(0.15), t.TextMuted
				if tk.Role == string(webserver.RoleAdmin) {
					role, bg, fg = i18n.T("web.token.role.admin"), t.Accent.Alpha(0.14), t.Accent
				}
				ui.Badge(c, role).Background(bg).TextColor(fg)
				ui.Text(c, i18n.Tf("web.token.created_at", fmtTokenCreated(tk.Created))).FontSize(12).TextColor(t.TextMuted).Grow(1)
				if ui.Button(c, i18n.T("web.token.revoke")).Clicked() {
					revoke = tk.ID
				}
			})
		}
		// applied after the loop over the copied list
		if revoke != "" && a.webRevokeToken(revoke) {
			a.web.msg, a.web.msgErr = i18n.T("web.token.revoked"), false
		}
	})
}
