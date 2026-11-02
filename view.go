package main

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

func (a *app) view(c *ui.Context) {
	for _, msg := range a.toasts {
		c.Toast(msg)
	}
	a.toasts = nil

	if a.cl == nil || a.st == nil {
		a.connectView(c)
		return
	}
	t := c.Theme()
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.sidebar(c)
		ui.Column(c).Grow(1).MinWidth(0).Background(t.Background).Children(func() {
			a.pageView(c)
		})
	})
	a.qrDialog(c)
	if ui.AlertDialog(c, &a.removeOpen, fmt.Sprintf("退出「%s」？", a.accountName(a.removeMid)),
		"服务器上的这份会话会在 B 站注销并删除，之后不再为这个账号运行。", "取消", "退出") == 1 {
		a.removeAccount(a.removeMid)
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

/* 连接页 */

func (a *app) connectView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Padding(24).Background(t.Surface).Children(func() {
		card(c).Width(380).Padding(28).Gap(16).Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.06)).Children(func() {
			ui.Row(c).Gap(10).Children(func() {
				ui.Icon(c, iconAnchor).FontSize(22).TextColor(t.Accent)
				ui.Text(c, "连接到服务器").FontSize(18).Bold()
			})
			muted(c, "填 bili-ip-anchor WebUI 的地址和面板密码。通过 SSH 隧道访问时，地址是 http://127.0.0.1:8080。").FontSize(13)
			ui.Field(c, "服务器地址", func() {
				ui.TextInput(c.Key("server"), &a.server).Placeholder("http://127.0.0.1:8080").Disabled(a.connecting)
			})
			ui.Field(c, "面板密码", func() {
				if ui.TextInput(c.Key("password"), &a.password).Password().AutoFocus().Disabled(a.connecting).Submitted() {
					a.connect()
				}
			}).Description("启动服务器时 WEBUI_PASSWORD 里设置的那个。")
			ui.Checkbox(c.Key("remember"), &a.remember, "记住密码，下次打开自动连接")
			if a.connectErr != "" {
				ui.Text(c, a.connectErr).TextColor(t.Danger).FontSize(13).Selectable()
			}
			ui.PrimaryButton(c.Key("connect"), "").Disabled(a.connecting || a.password == "").OnClick(a.connect).Children(func() {
				if a.connecting {
					ui.Spinner(c).TextColor(t.AccentText)
					ui.Text(c, "正在连接").SingleLine()
				} else {
					ui.Text(c, "连接").SingleLine()
				}
			})
		})
	})
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
	ui.Column(c).Width(212).Background(t.Surface).BorderWidth(0, 1, 0, 0).BorderColor(t.Border).Children(func() {
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
			ui.Row(c).Gap(6).Children(func() {
				ui.Icon(c, iconServer).FontSize(13).TextColor(t.TextMuted)
				muted(c, serverHost(a.server)).FontSize(12).SingleLine().Shrink(1)
			})
			ui.Row(c).Gap(6).Children(func() {
				dot := t.Success
				if a.loadErr != "" {
					dot = t.Danger
				}
				ui.Box(c).Size(7, 7).Radius(4).Background(dot)
				label := "已连接 · " + a.st.Version
				if a.loadErr != "" {
					label = "连接中断，正在重试"
				}
				muted(c, label).FontSize(12).SingleLine()
			})
			ui.Button(c.Key("disconnect"), "").OnClick(a.disconnect).Children(func() {
				ui.Icon(c, iconLogout)
				ui.Text(c, "断开连接").SingleLine()
			}).Margin(4, 0, 0, 0)
		})
	})
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
	ui.Row(c).Gap(8).Padding(18, 24).MinHeight(70).Children(func() {
		ui.Text(c, titles[a.page]).FontSize(20).Bold().Grow(1)
		switch a.page {
		case pageOverview, pageAccounts:
			if len(a.st.Accounts) > 0 {
				ui.Button(c.Key("run-all"), "").Disabled(a.runningAll || !anyIdle(a.st.Accounts)).OnClick(a.runAll).Children(func() {
					ui.Icon(c, iconPlay)
					ui.Text(c, "全部立即运行").SingleLine()
				})
			}
			ui.PrimaryButton(c.Key("add"), "").OnClick(a.openQR).Children(func() {
				ui.Icon(c, iconPlus)
				ui.Text(c, "添加账号").SingleLine()
			})
		}
	})
	if a.loadErr != "" {
		ui.Text(c, "连不上服务器，显示的是之前的数据："+a.loadErr).FontSize(13).TextColor(t.Danger).
			Padding(8, 12).Margin(0, 24, 12).Radius(t.Radius).Background(t.Danger.Alpha(0.08))
	}
	switch a.page {
	case pageAccounts:
		ui.Scroll(c.Key("accounts")).Grow(1).Padding(0, 24, 24).Gap(12).Children(func() { a.accountsPage(c) })
	case pageSettings:
		ui.Scroll(c.Key("settings")).Grow(1).Padding(0, 24, 24).Children(func() { a.settingsPage(c) })
	case pageLogs:
		a.logsPage(c)
	default:
		ui.Scroll(c.Key("overview")).Grow(1).Padding(0, 24, 24).Gap(16).Children(func() { a.overviewPage(c) })
	}
}

func anyIdle(list []account) bool {
	for _, acc := range list {
		if acc.LoggedIn && !acc.Running {
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

func (a *app) overviewPage(c *ui.Context) {
	t := c.Theme()
	st := a.st
	card(c).Padding(20, 24).Gap(10).Children(func() {
		muted(c, "B 站把这台服务器识别为")
		switch {
		case st.Zone != nil:
			ui.Text(c, zonePlace(st.Zone)).FontSize(36).Bold().LetterSpacing(1)
			ui.Row(c).Gap(6).Wrap().Children(func() {
				if st.Zone.ISP != "" {
					ui.Badge(c, st.Zone.ISP)
				}
				if st.Zone.Addr != "" {
					ui.Badge(c, st.Zone.Addr).Font("monospace")
				}
			})
		case st.ZoneError != "":
			ui.Text(c, "暂时查不到").FontSize(28).Bold().TextColor(t.TextMuted)
			ui.Text(c, "查询失败："+st.ZoneError).FontSize(13).TextColor(t.Danger)
		default:
			ui.Text(c, "查询中").FontSize(28).Bold().TextColor(t.TextMuted)
		}
		muted(c, "主页的 IP 属地按省份显示。这里是服务器出口 IP 在 B 站那边的归属；主页有没有更新，请用无痕窗口看个人空间。").FontSize(13).MaxWidth(560)
	})

	var ok, bad, failed int
	var next *time.Time
	for _, acc := range st.Accounts {
		switch {
		case !acc.LoggedIn:
			bad++
		case acc.Last != nil && !acc.Last.OK:
			failed++
		case acc.Last != nil:
			ok++
		}
		if acc.Next != nil && (next == nil || acc.Next.Before(*next)) {
			next = acc.Next
		}
	}
	ui.Row(c).Gap(12).AlignItems(ui.Stretch).Children(func() {
		stat(c, "账号", fmt.Sprint(len(st.Accounts)), "各自排程、同时进行", t.Text)
		stat(c, "上次正常", fmt.Sprint(ok), fmt.Sprintf("%d 个上次失败", failed), t.Success)
		needColor := t.Text
		if bad > 0 {
			needColor = t.Danger
		}
		stat(c, "需要重新扫码", fmt.Sprint(bad), "登录失效的账号", needColor)
		nextText, nextSub := "—", "没有排程"
		if next != nil {
			nextText, nextSub = a.fmtTime(*next), a.fmtRelative(*next)
		}
		stat(c, "最近一次运行", nextText, nextSub, t.Text)
	})

	if len(st.Accounts) == 0 {
		a.emptyAccounts(c)
	}
}

func stat(c *ui.Context, label, value, sub string, color ui.Color) {
	card(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
		muted(c, label).FontSize(13)
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
		ui.PrimaryButton(c.Key("add-empty"), "扫码添加第一个账号").OnClick(a.openQR)
	})
}

/* 账号 */

func (a *app) accountsPage(c *ui.Context) {
	if len(a.st.Accounts) == 0 {
		a.emptyAccounts(c)
		return
	}
	t := c.Theme()
	card(c).Padding(0).Gap(0).Dividers(1, t.Border).Children(func() {
		for _, acc := range a.st.Accounts {
			a.accountRow(c, acc)
		}
	})
}

func accountState(acc account) (string, func(t *ui.Theme) ui.Color) {
	switch {
	case acc.Running:
		return "运行中", func(t *ui.Theme) ui.Color { return t.Accent }
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

func (a *app) accountRow(c *ui.Context, acc account) {
	t := c.Theme()
	name := acc.Uname
	if name == "" {
		name = "UID " + acc.Mid
	}
	ui.Column(c.Key(acc.Mid)).Padding(14, 16).Gap(6).Children(func() {
		ui.Row(c).Gap(16).Children(func() {
			ui.Row(c).Gap(12).Width(240).MinWidth(0).Children(func() {
				ui.Avatar(c, name, a.faces[acc.Face])
				ui.Column(c).MinWidth(0).Shrink(1).Gap(2).Children(func() {
					ui.Row(c).Gap(8).Children(func() {
						ui.Text(c, name).Bold().SingleLine().Shrink(1)
						label, color := accountState(acc)
						col := color(t)
						ui.Badge(c, label).Background(col.Alpha(0.12)).TextColor(col)
					})
					if acc.Uname != "" {
						muted(c, "UID "+acc.Mid).FontSize(12)
					}
				})
			})
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Row(c).Gap(10).Children(func() {
					muted(c, "上次运行").FontSize(13).Width(56)
					if acc.Last == nil {
						ui.Text(c, "还没有").FontSize(13)
					} else {
						ui.Text(c, a.fmtTime(acc.Last.At)).FontSize(13)
						if acc.Last.OK {
							ui.Text(c, "成功").FontSize(13).TextColor(t.Success)
						} else {
							ui.Text(c, "失败").FontSize(13).TextColor(t.Danger)
						}
					}
				})
				ui.Row(c).Gap(10).Children(func() {
					muted(c, "下次运行").FontSize(13).Width(56)
					next := "—"
					switch {
					case acc.Running:
						next = "正在运行"
					case acc.Next != nil:
						next = fmt.Sprintf("%s（%s）", a.fmtTime(*acc.Next), a.fmtRelative(*acc.Next))
					}
					ui.Text(c, next).FontSize(13).SingleLine()
				})
			})
			ui.Row(c).Gap(6).Children(func() {
				busy := a.busy[acc.Mid]
				if acc.LoggedIn {
					label := "立即运行"
					if acc.Running {
						label = "运行中"
					}
					ui.Button(c.Key("run"), label).Disabled(acc.Running || busy).OnClick(func() { a.runAccount(acc) })
				} else {
					ui.PrimaryButton(c.Key("relogin"), "重新扫码").OnClick(a.openQR)
				}
				ui.Button(c.Key("remove"), "退出").Disabled(busy).TextColor(t.Danger).OnClick(func() { a.askRemove(acc.Mid) })
			})
		})
		msg := ""
		switch {
		case !acc.LoggedIn:
			msg = "这个账号的会话已失效，点「重新扫码」用同一个账号登录即可恢复。"
		case acc.Last != nil && !acc.Last.OK:
			msg = acc.Last.Error
		}
		if msg != "" {
			ui.Text(c, msg).FontSize(12).TextColor(t.Danger).Padding(0, 0, 0, 44).Selectable()
		}
	})
}

/* 设置 */

var bvidRe = regexp.MustCompile(`^BV[0-9A-Za-z]{10}$`)

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

func (a *app) settingsPage(c *ui.Context) {
	t := c.Theme()
	intervalErr := durationError(a.form.Interval, 10*time.Minute)
	jitterErr := durationError(a.form.Jitter, 0)
	bvidErr := ""
	if !bvidRe.MatchString(strings.TrimSpace(a.form.BVID)) {
		bvidErr = "BV 号是 BV 开头的 12 位"
	}
	muted(c, "所有账号共用，保存后立即生效。").FontSize(13).Margin(0, 0, 16)
	card(c).Padding(20, 24).MaxWidth(640).Children(func() {
		ui.Form(c, func() {
			edited := false
			ui.Field(c, "每轮间隔", func() {
				edited = ui.TextInput(c.Key("interval"), &a.form.Interval).Width(200).Changed() || edited
			}).Description("写成 90m、6h、12h 这样，最少 10m。").Error(intervalErr)
			ui.Field(c, "随机抖动", func() {
				edited = ui.TextInput(c.Key("jitter"), &a.form.Jitter).Width(200).Changed() || edited
			}).Description("每轮额外随机等 0 到这么久，让时间不那么规律。").Error(jitterErr)
			ui.Field(c, "心跳视频", func() {
				edited = ui.TextInput(c.Key("bvid"), &a.form.BVID).Width(200).Font("monospace").Changed() || edited
			}).Description("任意一个公开视频的 BV 号。").Error(bvidErr)
			ui.Field(c, "图片动态", func() {
				edited = ui.Switch(c.Key("dynamic"), &a.form.DoDynamic).Changed() || edited
			}).Description("每轮发一条图片动态，几秒后删掉。只发心跳不能改变属地时再打开；动态会短暂出现在粉丝的动态流里。换图片：把 dynamic.jpg 或 dynamic.png 放进服务器的数据目录。")
			if edited {
				a.formEdited = true
			}
		})
		if a.formErr != "" {
			ui.Text(c, a.formErr).FontSize(13).TextColor(t.Danger).Margin(4, 0, 0)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Margin(8, 0, 0).Children(func() {
			ui.Button(c.Key("reset"), "还原").Disabled(!a.formEdited || a.saving).OnClick(a.resetSettings)
			invalid := intervalErr != "" || jitterErr != "" || bvidErr != ""
			label := "保存设置"
			if a.saving {
				label = "正在保存"
			}
			ui.PrimaryButton(c.Key("save"), label).Disabled(!a.formEdited || a.saving || invalid).OnClick(a.saveSettings)
		})
	})
}

/* 日志 */

var logLineRe = regexp.MustCompile(`^\d{4}/(\d\d/\d\d \d\d:\d\d:\d\d) (?:(\[[^\]]*\]) )?(.*)$`)

func (a *app) logsPage(c *ui.Context) {
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

/* 扫码添加账号 */

func (a *app) qrDialog(c *ui.Context) {
	t := c.Theme()
	ui.Modal(c, &a.qrOpen, func() {
		ui.Column(c).Width(320).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "添加 B 站账号").FontSize(17).Bold()
			muted(c, "用 B 站 App 扫码并确认。已经添加过的账号再扫一次，会更新它的会话。").FontSize(13).TextAlign(ui.Center)
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
				text, color = "已扫码，请在手机上确认登录", t.Accent
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
	default:
		h, m := int(abs.Hours()), int(abs.Minutes())%60
		text = fmt.Sprintf("%d 小时", h)
		if m > 0 {
			text += fmt.Sprintf(" %d 分钟", m)
		}
	}
	if d >= 0 {
		return text + "后"
	}
	return text + "前"
}
