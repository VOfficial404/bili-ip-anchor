package main

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

func (a *app) view(c *ui.Context) {
	for _, msg := range a.toasts {
		c.Toast(msg)
	}
	a.toasts = nil
	a.width, _ = c.Size()
	a.narrow = a.width < 700

	if a.cl == nil || a.st == nil {
		a.connectView(c)
		return
	}
	t := c.Theme()
	if a.narrow {
		// 手机：页面在上，标签栏在下
		ui.Column(c).Fill().Background(t.Background).Children(func() {
			ui.Column(c).Grow(1).MinHeight(0).Children(func() { a.pageView(c) })
			a.tabBar(c)
		})
	} else {
		ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
			a.sidebar(c)
			ui.Column(c).Grow(1).MinWidth(0).Background(t.Background).Children(func() {
				a.pageView(c)
			})
		})
	}
	a.qrDialog(c)
	if ui.AlertDialog(c, &a.confirmOpen, a.confirmTitle, a.confirmText, "取消", a.confirmOK) == 1 && a.confirmAction != nil {
		a.confirmAction()
	}
}

func muted(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).TextColor(c.Theme().TextMuted)
}

// card 是有边框的白底分组
func card(c *ui.Context) ui.Element {
	t := c.Theme()
	return ui.Column(c).Padding(16).Gap(8).Radius(t.Radius*1.5).Background(t.Background).Border(1, t.Border)
}

// iconButton 是只有图标的按钮，label 给读屏和提示用
func iconButton(c *ui.Context, key string, svg *ui.SVG, label string) ui.Element {
	return ui.Button(c.Key(key), "").Label(label).Tooltip(label).Children(func() { ui.Icon(c, svg) })
}

func (a *app) probeView(c *ui.Context, proxy string) {
	t := c.Theme()
	p := a.probe
	if p.proxy != strings.TrimSpace(proxy) && p.err == "" {
		return
	}
	switch {
	case p.running:
		muted(c, "正在经过代理访问 B 站…").FontSize(12)
	case p.err != "":
		ui.Text(c, p.err).FontSize(12).TextColor(t.Danger).Selectable()
	case p.zone != nil:
		z := p.zone
		ui.Text(c, fmt.Sprintf("可用。B 站看到的是 %s · %s，主页属地会变成%s。", zonePlace(z), z.Addr, zoneRegion(z))).FontSize(12).TextColor(t.Success).Selectable()
	}
}

/* 连接页 */

func (a *app) connectView(c *ui.Context) {
	t := c.Theme()
	outer, inner := float32(24), float32(28)
	if a.narrow {
		outer, inner = 12, 20
	}
	ui.Column(c).Fill().Center().Padding(outer).Background(t.Surface).Children(func() {
		card(c).Width(min(420, a.width-2*outer)).Padding(inner).Gap(16).Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.06)).Children(func() {
			ui.Row(c).Gap(10).Children(func() {
				ui.Icon(c, iconAnchor).FontSize(22).TextColor(t.Accent)
				ui.Text(c, "Bili IP Anchor").FontSize(18).Bold()
			})
			if ui.Segmented(c.Key("mode"), &a.modeIdx, "连接服务器", "在本机运行").Changed() {
				a.connectErr = ""
			}
			if a.modeIdx == 0 {
				a.remoteForm(c)
			} else {
				a.localForm(c)
			}
			if a.connectErr != "" {
				ui.Text(c, a.connectErr).TextColor(t.Danger).FontSize(13).Selectable()
			}
			label, run := "连接", a.connect
			disabled := a.password == ""
			if a.modeIdx == 1 {
				label, run = "开始运行", a.startLocal
				disabled = strings.TrimSpace(a.proxy) == ""
			}
			ui.PrimaryButton(c.Key("connect"), "").Disabled(a.connecting || disabled).OnClick(run).Children(func() {
				if a.connecting {
					ui.Spinner(c).TextColor(t.AccentText)
					ui.Text(c, "正在"+label).SingleLine()
				} else {
					ui.Text(c, label).SingleLine()
				}
			})
		})
	})
}

func (a *app) remoteForm(c *ui.Context) {
	muted(c, "定时任务在你的云服务器上跑。填 bili-ip-anchor WebUI 的地址和面板密码；通过 SSH 隧道访问时，地址是 http://127.0.0.1:8080。").FontSize(13)
	ui.Field(c, "服务器地址", func() {
		ui.TextInput(c.Key("server"), &a.server).Placeholder("http://127.0.0.1:8080").Disabled(a.connecting)
	})
	ui.Field(c, "面板密码", func() {
		if ui.TextInput(c.Key("password"), &a.password).Password().AutoFocus().Disabled(a.connecting).Submitted() {
			a.connect()
		}
	}).Description("启动服务器时 WEBUI_PASSWORD 里设置的那个。")
	ui.Checkbox(c.Key("remember"), &a.remember, "记住密码，下次打开自动连接")
}

func (a *app) localForm(c *ui.Context) {
	muted(c, "没有服务器也可以用："+a.device()+"定时以账号身份发请求，经过代理出去，B 站看到的是代理的出口 IP。看视频不经过它。关掉窗口后程序在托盘里继续运行。").FontSize(13)
	ui.Field(c, "代理", func() {
		ui.Row(c).Gap(8).Children(func() {
			if ui.TextInput(c.Key("proxy"), &a.proxy).Placeholder("http://127.0.0.1:7890").Font("monospace").Disabled(a.connecting).Grow(1).Submitted() {
				a.startLocal()
			}
			ui.Button(c.Key("probe"), "测试").Disabled(a.connecting || a.probe.running).OnClick(func() { a.testProxy(a.proxy) })
		})
		a.probeView(c, a.proxy)
	}).Description("代理软件的本地端口，支持 http:// 和 socks5://。")
	ui.Checkbox(c.Key("auto-local"), &a.autoLocal, "下次打开时自动开始运行")
}

/* 侧边栏 */

func (a *app) sidebar(c *ui.Context) {
	t := c.Theme()
	bad := 0
	for _, acc := range a.st.Accounts {
		if !acc.LoggedIn {
			bad++
		}
	}
	ui.Column(c).Width(220).Background(t.Surface).BorderWidth(0, 1, 0, 0).BorderColor(t.Border).Children(func() {
		ui.Row(c).Gap(8).Padding(18, 16, 14).Children(func() {
			ui.Icon(c, iconAnchor).FontSize(18).TextColor(t.Accent)
			ui.Text(c, "Bili IP Anchor").Bold().SingleLine()
		})
		ui.Sidebar(c, &a.page, func() {
			ui.SidebarItem(c, pageOverview, iconOverview, "概览")
			ui.SidebarItem(c, pageAccounts, iconUsers, "账号").Children(func() {
				if bad > 0 {
					ui.Badge(c, fmt.Sprint(bad)).Background(t.Danger).TextColor(t.AccentText)
				} else if n := len(a.st.Accounts); n > 0 {
					ui.Badge(c, fmt.Sprint(n))
				}
			})
			ui.SidebarItem(c, pageSettings, iconSettings, "设置")
			ui.SidebarItem(c, pageLogs, iconLogs, "日志")
		}).Grow(1).Background(t.Surface)
		ui.Column(c).Padding(12, 16, 16).Gap(6).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
			a.connInfo(c)
		})
	})
}

// connInfo 是连接状态：桌面上在侧边栏底部，手机上在概览页最后
func (a *app) connInfo(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(6).Children(func() {
		dot := t.Success
		if a.loadErr != "" {
			dot = t.Danger
		}
		ui.Box(c).Size(7, 7).Radius(4).Background(dot)
		label := "已连接 · " + a.st.Version
		switch {
		case a.loadErr != "":
			label = "连接中断，正在重试"
		case a.local():
			label = "本机运行 · " + a.st.Version
		}
		muted(c, label).FontSize(12).SingleLine()
	})
	sideLine(c, iconServer, "服务器地址", serverHost(a.server)).Hidden(a.local())
	sideLine(c, iconLaptop, "在"+a.device()+"上运行", "本机").Hidden(!a.local())
	sideLine(c, iconGlobe, "B 站看到的出口 IP", a.egressText())
	if a.local() {
		ui.Button(c.Key("disconnect"), "").OnClick(a.disconnect).Children(func() {
			ui.Icon(c, iconStop)
			ui.Text(c, "停止运行").SingleLine()
		}).Margin(4, 0, 0, 0)
	} else {
		ui.Button(c.Key("disconnect"), "").OnClick(a.disconnect).Children(func() {
			ui.Icon(c, iconLogout)
			ui.Text(c, "断开连接").SingleLine()
		}).Margin(4, 0, 0, 0)
	}
}

var navPages = []struct {
	page, label string
	icon        *ui.SVG
}{
	{pageOverview, "概览", iconOverview},
	{pageAccounts, "账号", iconUsers},
	{pageSettings, "设置", iconSettings},
	{pageLogs, "日志", iconLogs},
}

// tabBar 是手机上代替侧边栏的底部标签栏
func (a *app) tabBar(c *ui.Context) {
	t := c.Theme()
	bad := 0
	for _, acc := range a.st.Accounts {
		if !acc.LoggedIn {
			bad++
		}
	}
	ui.Row(c).Padding(4, 0, 6).Background(t.Surface).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
		for _, p := range navPages {
			col := t.TextMuted
			if a.page == p.page {
				col = t.Accent
			}
			ui.ButtonBase(c.Key("tab-"+p.page)).Column().Grow(1).Basis(0).Gap(2).Padding(6, 0).Label(p.label).
				OnClick(func() { a.page = p.page }).Children(func() {
				ui.Box(c).Children(func() {
					ui.Icon(c, p.icon).FontSize(20).TextColor(col)
					if p.page == pageAccounts && bad > 0 {
						ui.Box(c).Size(8, 8).Radius(4).Background(t.Danger).Absolute().Top(-2).Right(-4)
					}
				})
				ui.Text(c, p.label).FontSize(11).TextColor(col).SingleLine()
			})
		}
	})
}

// pad 是页面四周的留白，手机上小一些
func (a *app) pad() float32 {
	if a.narrow {
		return 12
	}
	return 24
}

// padded 给卡片加内边距，手机上小一些
func (a *app) padded(e ui.Element) ui.Element {
	if a.narrow {
		return e.Padding(16)
	}
	return e.Padding(20, 24)
}

func sideLine(c *ui.Context, svg *ui.SVG, tip, text string) ui.Element {
	return ui.Row(c).Gap(6).Tooltip(tip).Children(func() {
		ui.Icon(c, svg).FontSize(13).TextColor(c.Theme().TextMuted)
		muted(c, text).FontSize(12).SingleLine().Shrink(1)
	})
}

func (a *app) egressText() string {
	s := "出口 IP 查询中"
	switch {
	case a.st.Zone != nil:
		s = a.st.Zone.Addr + " · " + zoneRegion(a.st.Zone)
	case a.st.ZoneError != "":
		s = "出口 IP 查询失败"
	}
	if a.st.Settings.Proxy != "" {
		s = "经代理 " + s
	}
	return s
}

func serverHost(s string) string {
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		return u.Host
	}
	return s
}

/* 页面 */

func (a *app) pageView(c *ui.Context) {
	t := c.Theme()
	titles := map[string]string{pageOverview: "概览", pageAccounts: "账号", pageSettings: "设置", pageLogs: "日志"}
	pad := a.pad()
	head := ui.Row(c).Gap(8).Padding(18, pad).MinHeight(70)
	title := float32(20)
	if a.narrow {
		head.Padding(10, pad).MinHeight(56)
		title = 18
	}
	head.Children(func() {
		ui.Text(c, titles[a.page]).FontSize(title).Bold().Grow(1)
		switch a.page {
		case pageOverview, pageAccounts:
			if len(a.st.Accounts) > 0 {
				ui.Button(c.Key("run-all"), "").Label("全部立即运行").Disabled(a.runningAll || !anyIdle(a.st.Accounts)).OnClick(a.runAll).Children(func() {
					ui.Icon(c, iconPlay)
					ui.Text(c, "全部立即运行").SingleLine().Hidden(a.narrow)
				})
			}
			add := "添加账号"
			if a.narrow {
				add = "添加"
			}
			ui.PrimaryButton(c.Key("add"), "").Label("添加账号").OnClick(func() { a.openQR("") }).Children(func() {
				ui.Icon(c, iconPlus)
				ui.Text(c, add).SingleLine()
			})
		}
	})
	banner := func(msg string) {
		ui.Text(c, msg).FontSize(13).TextColor(t.Danger).Selectable().
			Padding(8, 12).Margin(0, pad, 12).Radius(t.Radius).Background(t.Danger.Alpha(0.08))
	}
	if a.loadErr != "" {
		banner("连不上服务器，显示的是之前的数据：" + a.loadErr)
	}
	if a.oldServer() {
		banner(fmt.Sprintf("服务器是 %s。账号详情、多个心跳视频、动态文字和图片、代理需要服务器更新到 v0.6：docker compose pull && docker compose up -d", a.st.Version))
	}
	switch a.page {
	case pageAccounts:
		ui.Scroll(c.Key("accounts")).Grow(1).Padding(0, pad, pad).Gap(12).Children(func() { a.accountsPage(c) })
	case pageSettings:
		ui.Scroll(c.Key("settings")).Grow(1).Padding(0, pad, pad).Children(func() { a.settingsPage(c) })
	case pageLogs:
		a.logsPage(c)
	default:
		ui.Scroll(c.Key("overview")).Grow(1).Padding(0, pad, pad).Gap(min(16, pad)).Children(func() { a.overviewPage(c) })
	}
}

func anyIdle(list []account) bool {
	for _, acc := range list {
		if acc.LoggedIn && !acc.Running && acc.Busy == "" {
			return true
		}
	}
	return false
}

func zonePlace(z *zone) string {
	parts := []string{z.Country, z.Province}
	if z.Country == "中国" {
		parts = []string{z.Province, z.City}
	}
	var out []string
	for _, p := range parts {
		if p != "" && (len(out) == 0 || out[len(out)-1] != p) {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return "未知地区"
	}
	return strings.Join(out, " ")
}

// zoneRegion 是主页属地显示的那一级：国内是省份，国外是国家
func zoneRegion(z *zone) string {
	if z.Country == "中国" && z.Province != "" {
		return z.Province
	}
	return z.Country
}

func (a *app) overviewPage(c *ui.Context) {
	t := c.Theme()
	st := a.st
	big, gap := float32(36), float32(12)
	if a.narrow {
		big, gap = 28, 10
	}
	a.padded(card(c)).Gap(10).Children(func() {
		where := "这台服务器"
		if a.local() {
			where = "本机"
		}
		if st.Settings.Proxy != "" {
			where += "（经代理）"
		}
		muted(c, "B 站把"+where+"识别为")
		switch {
		case st.Zone != nil:
			ui.Text(c, zonePlace(st.Zone)).FontSize(big).Bold().LetterSpacing(1)
			ui.Row(c).Gap(6).Wrap().Children(func() {
				if st.Zone.ISP != "" {
					ui.Badge(c, st.Zone.ISP)
				}
				if st.Zone.Addr != "" {
					ui.Badge(c, st.Zone.Addr).Font("monospace")
				}
				if st.ZoneCheckedAt != nil {
					muted(c, "查询于 "+a.fmtTime(*st.ZoneCheckedAt)).FontSize(12)
				}
			})
		case st.ZoneError != "":
			ui.Text(c, "暂时查不到").FontSize(28).Bold().TextColor(t.TextMuted)
			ui.Text(c, "查询失败："+st.ZoneError).FontSize(13).TextColor(t.Danger)
		default:
			ui.Text(c, "查询中").FontSize(28).Bold().TextColor(t.TextMuted)
		}
		muted(c, "这就是目标属地。主页的 IP 属地按省份（国外按国家）显示；在「账号」页授权查询后，可以看到每个账号主页现在显示的属地。").FontSize(13).MaxWidth(560)
	})

	var bad, authed, matched int
	var next *time.Time
	for _, acc := range st.Accounts {
		if !acc.LoggedIn || (acc.Last != nil && !acc.Last.OK) {
			bad++
		}
		if acc.AppAuth {
			authed++
			if acc.Home != nil && acc.Home.MatchedSince != nil {
				matched++
			}
		}
		if acc.Next != nil && (next == nil || acc.Next.Before(*next)) {
			next = acc.Next
		}
	}
	stats := ui.Row(c).Gap(gap).AlignItems(ui.Stretch)
	if a.narrow {
		stats.Wrap()
	}
	stats.Children(func() {
		a.stat(c, "账号", fmt.Sprint(len(st.Accounts)), "各自排程、同时进行", t.Text)
		if authed > 0 {
			a.stat(c, "主页已是目标属地", fmt.Sprintf("%d / %d", matched, authed), fmt.Sprintf("%d 个账号已授权查询", authed), t.Success)
		} else {
			a.stat(c, "主页已是目标属地", "—", "授权查询后显示", t.Success)
		}
		badColor := t.Text
		if bad > 0 {
			badColor = t.Danger
		}
		a.stat(c, "需要处理", fmt.Sprint(bad), "登录失效或上次失败", badColor)
		nextText, nextSub := "—", "没有排程"
		if next != nil {
			nextText, nextSub = a.fmtTime(*next), a.fmtRelative(*next)
		}
		a.stat(c, "最近一次运行", nextText, nextSub, t.Text)
	})

	if len(st.Accounts) == 0 {
		a.emptyAccounts(c)
	}
	if a.narrow {
		card(c).Gap(6).Children(func() { a.connInfo(c) })
	}
}

func (a *app) stat(c *ui.Context, label, value, sub string, color ui.Color) {
	basis := float32(0)
	if a.narrow {
		basis = 140 // 一行两个
	}
	card(c).Grow(1).Basis(basis).MinWidth(0).Gap(2).Children(func() {
		muted(c, label).FontSize(13).SingleLine()
		ui.Text(c, value).FontSize(24).Bold().TextColor(color).SingleLine().FontFeatures("tnum")
		muted(c, sub).FontSize(12).SingleLine()
	})
}

func (a *app) emptyAccounts(c *ui.Context) {
	t := c.Theme()
	card(c).Padding(28).Gap(12).AlignItems(ui.Center).BorderStyle(ui.BorderDashed).Children(func() {
		ui.Icon(c, iconUsers).FontSize(28).TextColor(t.TextMuted)
		ui.Text(c, "还没有账号").Bold()
		muted(c, "扫码登录一个 B 站账号，它就会按设置定时运行。可以添加多个，每个账号各自排程、同时进行。").TextAlign(ui.Center).MaxWidth(420).FontSize(13)
		ui.PrimaryButton(c.Key("add-empty"), "扫码添加第一个账号").OnClick(func() { a.openQR("") })
	})
}

/* 账号 */

func (a *app) accountsPage(c *ui.Context) {
	if len(a.st.Accounts) == 0 {
		a.emptyAccounts(c)
		return
	}
	for _, acc := range a.st.Accounts {
		a.accountCard(c, acc)
	}
}

func accountState(acc account) (string, func(t *ui.Theme) ui.Color) {
	switch {
	case acc.Running:
		return "运行中", func(t *ui.Theme) ui.Color { return t.Accent }
	case acc.Busy != "":
		return "正在" + acc.Busy, func(t *ui.Theme) ui.Color { return t.Accent }
	case !acc.LoggedIn:
		return "登录失效", func(t *ui.Theme) ui.Color { return t.Danger }
	case acc.Last == nil:
		return "等待首次运行", func(t *ui.Theme) ui.Color { return t.TextMuted }
	case acc.Last.OK:
		return "正常", func(t *ui.Theme) ui.Color { return t.Success }
	default:
		return "上次失败", func(t *ui.Theme) ui.Color { return t.Warning }
	}
}

func tint(c *ui.Context, text string, col ui.Color) ui.Element {
	return ui.Badge(c, text).Background(col.Alpha(0.12)).TextColor(col)
}

// tile 是账号卡片里的一格：小标题，然后是 body
func tile(c *ui.Context, svg *ui.SVG, title string, body func()) ui.Element {
	t := c.Theme()
	return ui.Column(c).Grow(1).Basis(0).MinWidth(0).Padding(10, 12).Gap(3).Radius(t.Radius).Background(t.Surface).Children(func() {
		ui.Row(c).Gap(5).Children(func() {
			ui.Icon(c, svg).FontSize(12).TextColor(t.TextMuted)
			muted(c, title).FontSize(12)
		})
		body()
	})
}

func tileValue(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(17).Bold().SingleLine().Shrink(1)
}

func tilePending(c *ui.Context, s string) ui.Element {
	return muted(c, s).FontSize(14).SingleLine()
}

func tileSub(c *ui.Context, s string) {
	if s != "" {
		muted(c, s).FontSize(12).Selectable()
	}
}

func (a *app) accountCard(c *ui.Context, acc account) {
	t := c.Theme()
	name := acc.Uname
	if name == "" {
		name = "UID " + acc.Mid
	}
	busy := a.busy[acc.Mid] || acc.Running || acc.Busy != ""
	actions := func() {
		if acc.LoggedIn {
			label := "立即运行"
			if acc.Running {
				label = "运行中"
			}
			ui.Button(c.Key("run"), "").Disabled(busy).OnClick(func() { a.accountAction(acc.Mid, "POST", "run", "") }).Children(func() {
				ui.Icon(c, iconPlay)
				ui.Text(c, label).SingleLine()
			})
			if !a.oldServer() {
				iconButton(c, "check", iconRefresh, "刷新资料：只查询硬币、Cookie 和主页属地，不发心跳").Disabled(busy).
					OnClick(func() { a.accountAction(acc.Mid, "POST", "check", "资料已刷新") })
			}
		} else {
			ui.PrimaryButton(c.Key("relogin"), "重新扫码").OnClick(func() { a.openQR("") })
		}
		iconButton(c, "remove", iconLogout, "退出这个账号").TextColor(t.Danger).Disabled(a.busy[acc.Mid]).OnClick(func() { a.askRemove(acc.Mid) })
	}
	// 手机上一行放不下两格，各占一行
	tiles := func(body func()) {
		r := ui.Row(c).Gap(10).AlignItems(ui.Stretch)
		if a.narrow {
			r.Wrap()
		}
		r.Children(body)
	}
	fit := func(e ui.Element) {
		if a.narrow {
			e.Basis(240)
		}
	}
	card(c.Key(acc.Mid)).Gap(14).Children(func() {
		ui.Row(c).Gap(12).Children(func() {
			ui.Avatar(c, name, a.faces[acc.Face])
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				ui.Row(c).Gap(8).Children(func() {
					ui.Text(c, name).Bold().FontSize(15).SingleLine().Shrink(1)
					label, color := accountState(acc)
					tint(c, label, color(t))
				})
				ui.Row(c).Gap(4).Wrap().Children(func() {
					ui.Badge(c, "UID "+acc.Mid).Font("monospace")
					if s := acc.Stats; s != nil {
						lv := t.Text
						if s.Level >= 5 {
							lv = t.Warning
						}
						ui.Badge(c, fmt.Sprintf("Lv%d", s.Level)).TextColor(lv)
						if s.VIP != "" {
							tint(c, s.VIP, ui.Hex("#e0527a"))
						}
						ui.Badge(c, "硬币 "+fmtNum(s.Coins))
						if s.BCoins > 0 {
							ui.Badge(c, "B币 "+fmtNum(s.BCoins))
						}
					}
				})
			})
			if !a.narrow {
				ui.Row(c).Gap(6).Children(actions)
			}
		})
		if a.narrow {
			ui.Row(c).Gap(6).Wrap().Children(actions)
		}

		if !a.oldServer() {
			ui.Column(c).Gap(10).Children(func() {
				tiles(func() {
					fit(tile(c, iconPin, "主页属地", func() { a.homeTile(c, acc, busy) }))
					fit(tile(c, iconGlobe, "目标属地", func() {
						if z := a.st.Zone; z != nil {
							tileValue(c, zoneRegion(z))
							tileSub(c, strings.Join(nonEmpty(z.Addr, z.ISP), " · "))
						} else {
							tilePending(c, "查询中")
						}
					}))
				})
				tiles(func() {
					fit(tile(c, iconPlay, "运行", func() { a.runTile(c, acc) }))
					fit(tile(c, iconCookie, "Cookie", func() { a.cookieTile(c, acc, busy) }))
				})
			})
		}

		if !a.oldServer() {
			a.favRow(c, acc, busy)
		}

		msg := ""
		switch {
		case !acc.LoggedIn:
			msg = "这个账号的会话已失效，点「重新扫码」用同一个账号登录即可恢复。"
		case acc.Last != nil && !acc.Last.OK:
			msg = acc.Last.Error
		}
		if msg != "" {
			ui.Text(c, msg).FontSize(12).TextColor(t.Danger).Selectable()
		}
	})
}

func (a *app) homeTile(c *ui.Context, acc account, busy bool) {
	t := c.Theme()
	h := acc.Home
	sub := ""
	switch {
	case !acc.AppAuth:
		tilePending(c, "未授权查询")
		sub = "B 站只在 App 接口里提供主页属地，需要额外授权一次才能显示。"
	case h == nil || (h.Text == "" && h.Error == ""):
		tilePending(c, "查询中")
	case h.Text == "":
		tilePending(c, "查不到")
		sub = h.Error
	default:
		target := ""
		if a.st.Zone != nil {
			target = zoneRegion(a.st.Zone)
		}
		ui.Row(c).Gap(6).Wrap().Children(func() {
			tileValue(c, h.Text)
			switch {
			case h.MatchedSince != nil:
				tint(c, "已是目标属地", t.Success)
			case target != "":
				tint(c, "还不是"+target, t.Warning)
			}
		})
		switch {
		case h.MatchedSince != nil:
			sub = a.fmtTime(*h.MatchedSince) + " 起一致"
		case target != "":
			next := "下一轮"
			if acc.Next != nil {
				next = fmt.Sprintf("下一轮（%s）", a.fmtTime(*acc.Next))
			}
			sub = fmt.Sprintf("预计%s之后几小时内变为%s，B 站没有公开具体规则", next, target)
		}
		if h.Error != "" {
			sub = "上次查询失败：" + h.Error
		} else if !h.CheckedAt.IsZero() {
			if sub != "" {
				sub += "。"
			}
			sub += "查询于 " + a.fmtTime(h.CheckedAt)
		}
	}
	tileSub(c, sub)
	ui.Row(c).Gap(6).Margin(4, 0, 0).Children(func() {
		if acc.AppAuth {
			ui.Button(c.Key("unauth"), "取消授权").Disabled(busy).OnClick(func() { a.askUnauth(acc.Mid) })
		} else {
			ui.Button(c.Key("auth"), "").Disabled(busy || !acc.LoggedIn).Tooltip("用 B 站 App 再扫一次码，授权本程序查询这个账号主页显示的 IP 属地").
				OnClick(func() { a.openQR(acc.Mid) }).Children(func() {
				ui.Icon(c, iconShield)
				ui.Text(c, "授权查询").SingleLine()
			})
		}
	})
}

func (a *app) runTile(c *ui.Context, acc account) {
	switch {
	case acc.Running:
		tileValue(c, "正在运行")
	case acc.Next != nil:
		tileValue(c, "下次 "+a.fmtTime(*acc.Next))
	default:
		tileValue(c, "—")
	}
	var lines []string
	if acc.Next != nil && !acc.Running {
		lines = append(lines, a.fmtRelative(*acc.Next))
	}
	if acc.Last != nil {
		res := "成功"
		if !acc.Last.OK {
			res = "失败"
		}
		lines = append(lines, fmt.Sprintf("上次 %s %s", a.fmtTime(acc.Last.At), res))
	}
	if acc.LastOK != nil {
		lines = append(lines, "上次从目标 IP 发出请求："+a.fmtTime(*acc.LastOK))
	}
	if len(lines) == 0 {
		lines = []string{"还没有运行过"}
	}
	tileSub(c, strings.Join(lines, " · "))
}

func (a *app) cookieTile(c *ui.Context, acc account, busy bool) {
	t := c.Theme()
	if exp := acc.Cookie.Expires; exp != nil {
		days := int(math.Ceil(exp.Sub(a.now()).Hours() / 24))
		v := tileValue(c, fmt.Sprintf("还有 %d 天", days))
		switch {
		case days <= 0:
			v.TextColor(t.Danger)
			ui.Text(c, "已过期").FontSize(12).TextColor(t.Danger)
		case days < 14:
			v.TextColor(t.Warning)
		}
		sub := "到 " + exp.In(a.now().Location()).Format("2006-01-02")
		if acc.Cookie.CanRefresh {
			sub += "，B 站要求时会自动刷新"
		}
		tileSub(c, sub)
	} else {
		tilePending(c, "有效期未知")
		if acc.Cookie.CanRefresh {
			tileSub(c, "下次运行时会查询")
		} else {
			tileSub(c, "旧版本扫码登录的账号，重新扫码后可以看到有效期并刷新")
		}
	}
	label := "刷新 Cookie"
	if acc.Busy == "刷新 Cookie" {
		label = "正在刷新"
	}
	tip := "现在就换一份新的 Cookie"
	if !acc.Cookie.CanRefresh {
		tip = "这个账号是旧版本扫码登录的，重新扫码一次之后才能刷新"
	}
	ui.Row(c).Margin(4, 0, 0).Children(func() {
		ui.Button(c.Key("cookie"), "").Disabled(busy || !acc.LoggedIn || !acc.Cookie.CanRefresh).Tooltip(tip).
			OnClick(func() { a.accountAction(acc.Mid, "POST", "cookie/refresh", "Cookie 已刷新") }).Children(func() {
			ui.Icon(c, iconRefresh)
			ui.Text(c, label).SingleLine()
		})
	})
}

// favRow 是账号卡片底部的收藏夹一栏：导出、导入和导入进度
func (a *app) favRow(c *ui.Context, acc account, busy bool) {
	t := c.Theme()
	fb := a.favBusy[acc.Mid]
	off := busy || !acc.LoggedIn || fb != ""
	ui.Column(c.Key("fav")).Gap(6).Children(func() {
		ui.Row(c).Gap(8).Wrap().Children(func() {
			ui.Row(c).Gap(6).Grow(1).Basis(200).Children(func() {
				ui.Icon(c, iconFolder).TextColor(t.TextMuted)
				muted(c, "收藏夹 · 导出成文件，可以导入到这个或别的账号").FontSize(12).Shrink(1)
			})
			label := "导出"
			if fb == "export" {
				label = "正在导出"
			}
			ui.Button(c.Key("fav-export"), label).Disabled(off).OnClick(func() { a.exportFav(acc.Mid) })
			ui.Button(c.Key("fav-import"), "导入").Disabled(off).OnClick(func() { a.importFav(acc.Mid) })
		})
		j := acc.Fav
		if j == nil {
			return
		}
		head := "导入完成"
		switch {
		case j.Running && j.Folder != "":
			head = "正在导入「" + j.Folder + "」"
		case j.Running:
			head = "正在导入"
		case j.Error != "":
			head = "导入中途停下"
		}
		ui.Row(c).Gap(8).Children(func() {
			h := ui.Text(c, head).FontSize(12).Bold().Grow(1).Shrink(1)
			if j.Error != "" {
				h.TextColor(t.Danger)
			}
			n := fmt.Sprintf("%d/%d", j.Done, j.Total)
			if j.Finished != nil {
				n += " · " + a.fmtTime(*j.Finished)
			}
			muted(c, n).FontSize(12)
		})
		v := 1.0
		if j.Total > 0 {
			v = float64(j.Done) / float64(j.Total)
		}
		ui.Progress(c, v)
		counts := []string{fmt.Sprintf("新增 %d", j.Added), fmt.Sprintf("已有 %d", j.Skipped)}
		if j.Failed > 0 {
			counts = append(counts, fmt.Sprintf("失败 %d", j.Failed))
		}
		if len(j.Created) > 0 {
			counts = append(counts, "新建了「"+strings.Join(j.Created, "」「")+"」")
		}
		muted(c, strings.Join(counts, " · ")).FontSize(12)
		if j.Error != "" {
			ui.Text(c, j.Error).FontSize(12).TextColor(t.Danger).Selectable()
		}
		for _, f := range j.Failures {
			muted(c, fmt.Sprintf("%s %s — %s", f.BVID, f.Title, f.Reason)).FontSize(12).Selectable()
		}
	})
}

func nonEmpty(ss ...string) []string {
	var out []string
	for _, s := range ss {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func fmtNum(f float64) string {
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.1f", f), "0"), ".")
}

/* 设置 */

func durationError(s string, min time.Duration) string {
	d, err := time.ParseDuration(strings.TrimSpace(s))
	switch {
	case err != nil:
		return "写成 90m、6h 这样"
	case d < 0:
		return "不能是负数"
	case d < min:
		return "最少 " + strings.TrimSuffix(min.String(), "0s")
	}
	return ""
}

var proxyRe = regexp.MustCompile(`^(?:(?:https?|socks5h?)://)?(?:[^@/\s]+@)?[^:/\s]+:\d{1,5}/?$`)

func (a *app) settingsPage(c *ui.Context) {
	t := c.Theme()
	intervalErr := durationError(a.form.Interval, 10*time.Minute)
	jitterErr := durationError(a.form.Jitter, 0)
	proxyErr := ""
	if p := strings.TrimSpace(a.form.Proxy); p != "" && !proxyRe.MatchString(p) {
		proxyErr = "写成 http://127.0.0.1:7890 或 socks5://127.0.0.1:7891"
	} else if p == "" && a.local() {
		proxyErr = "本机运行时不填代理，B 站看到的就是你自己的 IP"
	}
	videosErr := ""
	if len(a.form.Videos) == 0 {
		videosErr = "至少要有一个心跳视频"
	}
	edited := false
	muted(c, "所有账号共用，保存后立即生效。").FontSize(13).Margin(0, 0, 16)
	ui.Column(c).MaxWidth(680).Gap(min(16, a.pad())).Children(func() {
		a.padded(card(c)).Gap(12).Children(func() {
			ui.Text(c, "运行").FontSize(15).Bold()
			ui.Form(c, func() {
				ui.Field(c, "每轮间隔", func() {
					edited = ui.TextInput(c.Key("interval"), &a.form.Interval).Width(200).Changed() || edited
				}).Description("写成 90m、6h、12h 这样，最少 10m。").Error(intervalErr)
				ui.Field(c, "随机抖动", func() {
					edited = ui.TextInput(c.Key("jitter"), &a.form.Jitter).Width(200).Changed() || edited
				}).Description("每轮额外随机等 0 到这么久，让时间不那么规律。").Error(jitterErr)
				ui.Field(c, "代理", func() {
					ui.Row(c).Gap(8).Children(func() {
						edited = ui.TextInput(c.Key("proxy"), &a.form.Proxy).Placeholder("留空表示直连").Font("monospace").Grow(1).Changed() || edited
						ui.Button(c.Key("probe"), "测试").Disabled(a.probe.running).OnClick(func() { a.testProxy(a.form.Proxy) })
					})
					a.probeView(c, a.form.Proxy)
				}).Description("所有以账号身份发出的 B 站请求都经过它，B 站看到的就是代理的出口 IP。支持 http://、socks5://。").Error(proxyErr)
				if a.levelLogs() {
					ui.Field(c, "日志级别", func() {
						i := max(0, slices.Index(logLevels, a.form.LogLevel))
						if ui.Segmented(c.Key("log-level"), &i, logLevelNames...).Label("日志级别").Changed() {
							a.form.LogLevel = logLevels[i]
							edited = true
						}
					}).Description("记录到哪一级。排查问题时选「调试」，会记下每个 B 站请求的地址、结果和耗时（不含 Cookie 和 access_key）。")
				}
			})
		})

		a.padded(card(c)).Gap(10).Children(func() {
			ui.Text(c, "心跳视频").FontSize(15).Bold()
			muted(c, fmt.Sprintf("每轮从下面随机选一个视频上报观看心跳。任意公开视频都可以，最多 %d 个。", maxVideos)).FontSize(13)
			for _, bvid := range a.form.Videos {
				a.videoRow(c, bvid)
			}
			ui.Row(c).Gap(8).Children(func() {
				if ui.TextInput(c.Key("video-add"), &a.videoAdd).Placeholder("BV 号或视频链接，可以一次粘贴多个").Font("monospace").Grow(1).Submitted() {
					a.addVideos()
				}
				ui.Button(c.Key("video-add-btn"), "").Disabled(a.videoAdding || strings.TrimSpace(a.videoAdd) == "").OnClick(a.addVideos).Children(func() {
					if a.videoAdding {
						ui.Spinner(c)
					} else {
						ui.Icon(c, iconPlus)
					}
					ui.Text(c, "添加").SingleLine()
				})
			})
			for _, msg := range nonEmpty(videosErr, a.videoErr) {
				ui.Text(c, msg).FontSize(12).TextColor(t.Danger).Selectable()
			}
		})

		a.padded(card(c)).Gap(12).Children(func() {
			ui.Text(c, "动态").FontSize(15).Bold()
			ui.Form(c, func() {
				ui.Field(c, "发动态", func() {
					edited = ui.Switch(c.Key("dynamic"), &a.form.DoDynamic).Changed() || edited
				}).Description("每轮发一条动态，几秒后删掉。只发心跳不能改变属地时再打开；动态会短暂出现在粉丝的动态流里。")
				ui.Field(c, "文字", func() {
					edited = ui.TextArea(c.Key("dynamic-text"), &a.form.DynamicText).Lines(3, 8).Placeholder("可以不写").Changed() || edited
				})
				ui.Field(c, "图片", func() { a.imagesField(c) }).Description("图片上传后立即生效，不需要点保存。最多 9 张，每张不超过 10 MB。")
			})
			ui.Text(c, a.dynamicPreview()).FontSize(13).Padding(8, 10).Radius(t.Radius).Background(t.Accent.Alpha(0.08))
		})

		if edited {
			a.formEdited = true
		}
		if a.formErr != "" {
			ui.Text(c, a.formErr).FontSize(13).TextColor(t.Danger)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			ui.Button(c.Key("reset"), "还原").Disabled(!a.formEdited || a.saving).OnClick(a.resetSettings)
			invalid := intervalErr != "" || jitterErr != "" || videosErr != "" || (proxyErr != "" && strings.TrimSpace(a.form.Proxy) != "")
			label := "保存设置"
			if a.saving {
				label = "正在保存"
			}
			ui.PrimaryButton(c.Key("save"), label).Disabled(!a.formEdited || a.saving || invalid || a.oldServer()).OnClick(a.saveSettings)
		})
	})
}

func (a *app) videoRow(c *ui.Context, bvid string) {
	t := c.Theme()
	v := a.videos[bvid]
	ui.Row(c.Key("video-"+bvid)).Gap(10).Padding(6).Radius(t.Radius).Border(1, t.Border).Children(func() {
		ui.Box(c).Size(80, 50).Radius(4).Background(t.Surface).Center().Children(func() {
			if v != nil && v.cover != nil {
				ui.Image(c, v.cover).Size(80, 50).Fit(ui.Cover).Radius(4)
			} else {
				ui.Icon(c, iconPlay).TextColor(t.TextMuted)
			}
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			title := bvid
			if v != nil && v.info != nil {
				title = v.info.Title
			}
			ui.Link(c, title, "https://www.bilibili.com/video/"+bvid).SingleLine().Shrink(1)
			meta := bvid
			switch {
			case v == nil || (v.info == nil && v.err == ""):
			case v.err != "":
				meta += " · " + v.err
			default:
				meta += fmt.Sprintf(" · %s · %s · %s 播放", v.info.Owner, fmtDuration(v.info.Duration), fmtCount(v.info.Views))
			}
			muted(c, meta).FontSize(12).SingleLine().Shrink(1)
		})
		iconButton(c, "remove", iconTrash, "移除这个视频").Disabled(len(a.form.Videos) <= 1).OnClick(func() { a.removeVideo(bvid) })
	})
}

func (a *app) imagesField(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(8).Wrap().Children(func() {
		for _, img := range a.st.Images {
			name := img.Name
			ui.Column(c.Key("img-" + name)).Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(88, 88).Radius(t.Radius).Background(t.Surface).Border(1, t.Border).Center().Children(func() {
					if bm := a.images[name]; bm != nil {
						ui.Image(c, bm).Size(86, 86).Fit(ui.Cover).Radius(t.Radius)
					} else {
						ui.Icon(c, iconImage).TextColor(t.TextMuted)
					}
				})
				iconButton(c, "del", iconTrash, "删除这张图片").OnClick(func() { a.removeImage(name) })
			})
		}
		if len(a.st.Images) < 9 {
			ui.Button(c.Key("add-image"), "").Width(88).Height(88).Disabled(a.uploading).OnClick(a.addImages).Children(func() {
				ui.Column(c).Gap(4).AlignItems(ui.Center).Children(func() {
					if a.uploading {
						ui.Spinner(c)
						muted(c, "正在上传").FontSize(12)
					} else {
						ui.Icon(c, iconImage).FontSize(22).TextColor(t.TextMuted)
						muted(c, "添加图片").FontSize(12)
					}
				})
			})
		}
	})
}

func (a *app) dynamicPreview() string {
	if !a.form.DoDynamic {
		return "现在不发动态，只上报心跳。"
	}
	var parts []string
	if text := strings.TrimSpace(a.form.DynamicText); text != "" {
		if r := []rune(text); len(r) > 24 {
			text = string(r[:24]) + "…"
		}
		parts = append(parts, "文字「"+text+"」")
	}
	if n := len(a.st.Images); n > 0 {
		parts = append(parts, fmt.Sprintf("%d 张图片", n))
	}
	if len(parts) == 0 {
		return "没有文字也没有图片时，每轮发一张内置图片，几秒后删掉。"
	}
	return "每轮会发：" + strings.Join(parts, " + ") + "，几秒后删掉。"
}

func fmtDuration(sec int) string {
	m := sec / 60
	if m >= 60 {
		return fmt.Sprintf("%d:%02d:%02d", m/60, m%60, sec%60)
	}
	return fmt.Sprintf("%d:%02d", m, sec%60)
}

func fmtCount(n int64) string {
	switch {
	case n >= 1e8:
		return fmt.Sprintf("%.1f 亿", float64(n)/1e8)
	case n >= 1e4:
		return fmt.Sprintf("%.1f 万", float64(n)/1e4)
	}
	return fmt.Sprint(n)
}

/* 日志 */

var (
	logLevels     = []string{"debug", "info", "warn", "error"}
	logLevelNames = []string{"调试", "信息", "警告", "错误"}
	logMins       = []string{"全部", "信息及以上", "警告及以上", "只看错误"}
)

const allAccounts = "全部账号"

func levelColor(t *ui.Theme, level string) ui.Color {
	switch level {
	case "debug":
		return t.TextMuted
	case "warn":
		return t.Warning
	case "error":
		return t.Danger
	}
	return t.Accent
}

var logLineRe = regexp.MustCompile(`^\d{4}/(\d\d/\d\d \d\d:\d\d:\d\d) (?:(\[[^\]]*\]) )?(.*)$`)

func (a *app) logsPage(c *ui.Context) {
	if a.levelLogs() {
		a.levelLogsPage(c)
		return
	}
	t := c.Theme()
	muted(c, "最近 300 行，每 5 秒更新。").FontSize(13).Margin(0, 24, 12)
	if a.logScroll.Y >= a.logScroll.MaxY {
		a.logScroll.Y = math.MaxFloat32 // 停在底部时跟着新日志走
	}
	ui.Scroll(c.Key("logs")).TrackScroll(&a.logScroll).Grow(1).Margin(0, 24, 24).Padding(12, 14).
		Radius(t.Radius*1.5).Background(t.Surface).Border(1, t.Border).Selectable().
		Font("monospace").FontSize(12).Children(func() {
		if len(a.st.Logs) == 0 {
			muted(c, "还没有日志")
		}
		for i, line := range a.st.Logs {
			bad := strings.Contains(line, "失败") || strings.Contains(line, "失效") || strings.Contains(line, "错误")
			ui.Row(c.Key(i)).Gap(8).AlignItems(ui.Start).Children(func() {
				m := logLineRe.FindStringSubmatch(line)
				if m == nil {
					ui.Text(c, line).Grow(1).Shrink(1)
					return
				}
				ui.Text(c, m[1]).TextColor(t.TextMuted).NoWrap()
				if m[2] != "" {
					ui.Text(c, m[2]).TextColor(t.Accent).NoWrap()
				}
				msg := ui.Text(c, m[3]).Grow(1).Shrink(1).MinWidth(0)
				if bad {
					msg.TextColor(t.Danger)
				}
			})
		}
	})
}

/* 扫码 */

func (a *app) qrDialog(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.qrOpen, func() {
		ui.Column(c).Width(min(320, a.width-64)).Gap(12).AlignItems(ui.Center).Children(func() {
			if a.qrMid == "" {
				ui.Text(c, "添加 B 站账号").FontSize(17).Bold()
				muted(c, "用 B 站 App 扫码并确认。已经添加过的账号再扫一次，会更新它的会话。").FontSize(13).TextAlign(ui.Center)
			} else {
				ui.Text(c, "授权查询主页属地").FontSize(17).Bold()
				muted(c, fmt.Sprintf("用「%s」的 B 站 App 扫码并确认。这份授权只用来查询这个账号主页显示的 IP 属地，随时可以取消。", a.accountName(a.qrMid))).FontSize(13).TextAlign(ui.Center)
			}
			ui.Box(c).Size(232, 232).Center().Padding(8).Radius(t.Radius*1.5).Background(ui.Hex("#ffffff")).Border(1, t.Border).Children(func() {
				switch {
				case a.qrImage != nil && a.qrState != "expired" && a.qrState != "error":
					ui.Image(c, a.qrImage).Size(214, 214)
				case a.qrState == "loading":
					ui.Spinner(c).Label("正在获取二维码")
				default:
					muted(c, "二维码已失效")
				}
			})
			text, color := "正在获取二维码", t.TextMuted
			switch a.qrState {
			case "waiting":
				text, color = "用 B 站 App 扫一扫", t.Text
			case "scanned":
				text, color = "已扫码，请在手机上确认", t.Accent
			case "expired":
				text = "二维码已过期"
			case "error":
				text, color = a.qrErr, t.Danger
			}
			ui.Text(c, text).TextColor(color).TextAlign(ui.Center)
			ui.Row(c).Gap(8).Children(func() {
				if a.qrState == "expired" || a.qrState == "error" {
					ui.PrimaryButton(c.Key("qr-renew"), "换一张").OnClick(a.startQR)
				}
				ui.Button(c.Key("qr-close"), "关闭").OnClick(func() { a.qrOpen = false })
			})
		})
	})
}

/* 时间 */

func (a *app) fmtTime(at time.Time) string {
	now := a.now()
	at = at.In(now.Location())
	hm := at.Format("15:04")
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	switch day := at.Sub(today); {
	case day >= 0 && day < 24*time.Hour:
		return "今天 " + hm
	case day >= 24*time.Hour && day < 48*time.Hour:
		return "明天 " + hm
	case day < 0 && day >= -24*time.Hour:
		return "昨天 " + hm
	}
	return fmt.Sprintf("%d月%d日 %s", at.Month(), at.Day(), hm)
}

func (a *app) fmtRelative(at time.Time) string {
	d := at.Sub(a.now()).Round(time.Minute)
	abs := d
	if abs < 0 {
		abs = -abs
	}
	var text string
	switch {
	case abs < time.Minute:
		text = "不到 1 分钟"
	case abs < time.Hour:
		text = fmt.Sprintf("%d 分钟", int(abs.Minutes()))
	case abs < 48*time.Hour:
		h, m := int(abs.Hours()), int(abs.Minutes())%60
		text = fmt.Sprintf("%d 小时", h)
		if m > 0 {
			text += fmt.Sprintf(" %d 分钟", m)
		}
	default:
		text = fmt.Sprintf("%d 天", int(abs.Hours()/24))
	}
	if d >= 0 {
		return text + "后"
	}
	return text + "前"
}

// levelLogsPage 是 v0.7 起的日志页：按级别和账号筛选，每 2 秒拉一次新增的
func (a *app) levelLogsPage(c *ui.Context) {
	t := c.Theme()
	if !a.logsTried {
		a.fetchLogs()
	}
	accounts := []string{allAccounts}
	for _, acc := range a.st.Accounts {
		if name := a.accountName(acc.Mid); !slices.Contains(accounts, name) {
			accounts = append(accounts, name)
		}
	}
	if a.logAccount == "" || !slices.Contains(accounts, a.logAccount) {
		a.logAccount = allAccounts
	}
	pad := float32(24)
	if a.narrow {
		pad = 12
	}
	ui.Row(c).Gap(8).Wrap().Margin(0, pad, 12).Children(func() {
		ui.Segmented(c.Key("log-min"), &a.logMin, logMins...).Label("级别")
		if len(accounts) > 2 {
			ui.Select(c.Key("log-account"), &a.logAccount, accounts).Width(160)
		}
		level := logLevelNames[max(0, slices.Index(logLevels, a.st.LogLevel))]
		muted(c, "现在记录到「"+level+"」，在设置里改").FontSize(12).Grow(1).TextAlign(ui.End)
	})
	if a.logScroll.Y >= a.logScroll.MaxY {
		a.logScroll.Y = math.MaxFloat32 // 停在底部时跟着新日志走
	}
	min := logLevels[a.logMin]
	ui.Scroll(c.Key("level-logs")).TrackScroll(&a.logScroll).Grow(1).Margin(0, pad, pad).Padding(12, 14).
		Radius(t.Radius*1.5).Background(t.Surface).Border(1, t.Border).Selectable().
		Font("monospace").FontSize(12).Gap(2).Children(func() {
		shown := 0
		for _, en := range a.logs {
			if slices.Index(logLevels, en.Level) < slices.Index(logLevels, min) ||
				(a.logAccount != allAccounts && en.Account != a.logAccount) {
				continue
			}
			shown++
			meta := func() {
				ui.Text(c, en.Time.Local().Format("01/02 15:04:05")).TextColor(t.TextMuted).NoWrap()
				ui.Text(c, strings.ToUpper(en.Level)).TextColor(levelColor(t, en.Level)).Width(40).NoWrap()
				if en.Account != "" {
					ui.Text(c, "["+en.Account+"]").TextColor(t.Accent).SingleLine().Shrink(1)
				}
			}
			msg := func() {
				m := ui.Text(c, en.Msg).Grow(1).Shrink(1).MinWidth(0)
				if en.Level == "warn" || en.Level == "error" {
					m.TextColor(levelColor(t, en.Level))
				}
			}
			if a.narrow {
				// 手机上时间、级别、账号一行，消息另起一行
				ui.Column(c.Key(en.Seq)).Gap(1).Padding(0, 0, 4).Children(func() {
					ui.Row(c).Gap(8).Children(meta)
					msg()
				})
				continue
			}
			ui.Row(c.Key(en.Seq)).Gap(8).AlignItems(ui.Start).Children(func() {
				meta()
				msg()
			})
		}
		if shown == 0 {
			if a.logsLoading && len(a.logs) == 0 {
				muted(c, "正在读取日志…")
			} else {
				muted(c, "没有符合条件的日志")
			}
		}
	})
}
