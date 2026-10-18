package app

import (
	"net/url"

	"hinatracer/i18n"

	"github.com/egoist/mygo/ui"
)

// Settings cards in remote data-source mode are marked as this
// computer's (local) or the remote server's.
const (
	scopeNone = iota
	scopeLocal
	scopeRemote
)

// cardScope is the scope of the settings cards being built (render
// thread only); scopeNone outside remote mode.
var cardScope = scopeNone

// settingsGroupHeader is a group title above settings cards.
func settingsGroupHeader(c *ui.Context, title string) {
	t := c.Theme()
	ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(6, 0, 0, 0).Children(func() {
		ui.Text(c, title).FontSize(15).Bold()
		ui.Column(c).Height(1).Grow(1).Background(t.Border)
	}).Label(title)
}

// cardTitle is a settings card title, with a 「本机」/「远程」 badge in
// remote mode.
func cardTitle(c *ui.Context, title string) {
	t := c.Theme()
	ui.Row(c).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
		ui.Text(c, title).FontSize(14).Bold()
		switch cardScope {
		case scopeLocal:
			ui.Badge(c, i18n.T("settings.scope.badge_local")).Background(t.TextMuted.Alpha(0.16)).TextColor(t.Text)
		case scopeRemote:
			ui.Badge(c, i18n.T("settings.scope.badge_remote")).Background(t.Accent).TextColor(ui.RGB(255, 255, 255))
		}
	})
}

// remoteHost is the remote server's host[:port] for the group title.
func (a *app) remoteHost() string {
	if a.remote == nil {
		return ""
	}
	if u, err := url.Parse(a.remote.Base); err == nil && u.Host != "" {
		return u.Host
	}
	return a.remote.Base
}

// viewSettingsRemoteGroups is the rest of Settings in remote mode: the
// local app update card, the remote server's settings and, collapsed, the
// local-only settings that apply after switching back to local.
func (a *app) viewSettingsRemoteGroups(c *ui.Context) {
	cardScope = scopeLocal
	a.viewSettingsUpdate(c)
	settingsGroupHeader(c, i18n.Tf("settings.scope.remote_group", a.remoteHost()))
	cardScope = scopeRemote
	a.viewRemoteServerSettings(c)
	cardScope = scopeLocal
	ui.Collapsible(c, i18n.T("settings.scope.inactive_group"), &a.settingsInactiveOpen, func() {
		ui.Column(c).Gap(14).MinWidth(0).Padding(8, 0, 0, 0).Children(func() {
			card(c, func() {
				cardTitle(c, i18n.T("settings.local_webui.title"))
				ui.Text(c, i18n.T("settings.local_webui.desc")).FontSize(12).TextColor(c.Theme().TextMuted)
			})
			a.viewLocalOnlySettings(c)
		})
	})
}
