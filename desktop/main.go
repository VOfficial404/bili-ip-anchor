//go:build !js

// bili-ip-anchor 的桌面客户端，两种用法：
//   - 连接服务器：通过服务器 WebUI 的接口查看状态、扫码添加账号、修改设置和看日志，定时任务在服务器上跑；
//   - 在本机运行：程序自己跑定时任务，请求经过本机的代理出去，关闭窗口后在托盘里继续运行。
package main

import (
	_ "embed"
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

//go:embed resources/tray.png
var trayIcon []byte

func main() {
	var (
		win      *mygo.Window
		tray     *mygo.Tray
		quitting bool
	)
	a := newApp(systemStore{}, func(fn func()) { win.Update(fn) })

	// 本机运行时显示托盘图标；关掉窗口后从这里打开或退出
	a.onLocal = func(running bool) {
		switch {
		case running && tray == nil:
			t, err := mygo.NewTray(mygo.TrayOptions{
				ID:             "main",
				Icon:           trayIcon,
				IconIsTemplate: true,
				ToolTip:        "Bili IP Anchor：本机运行中",
				Menu: mygo.NewMenu([]*mygo.MenuItem{
					{Label: "打开 Bili IP Anchor", Click: func(*mygo.MenuItem, *mygo.Window) { win.Show(); win.Focus() }},
					{Label: "全部立即运行", Click: func(*mygo.MenuItem, *mygo.Window) { win.Update(a.runAll) }},
					mygo.Separator(),
					{Label: "退出", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
				}),
			})
			if err != nil {
				log.Printf("托盘图标不可用，关闭窗口会退出程序：%v", err)
				return
			}
			tray = t
		case !running && tray != nil:
			tray.Destroy()
			tray = nil
		}
	}

	mygo.App.WhenReady(func() {
		win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "Bili IP Anchor",
			Width:     1080,
			Height:    760,
			MinWidth:  780,
			MinHeight: 540,
			StateKey:  "main", // 下次在上次关闭的位置打开
			Content:   ui.View(a.view),
		})
		win.OnClose(func(e *mygo.CloseEvent) {
			// 本机运行时关掉窗口只是藏起来，定时任务继续跑
			if !quitting && a.local() && tray != nil {
				e.PreventDefault()
				win.Hide()
			}
		})
		a.autoConnect()
		go func() {
			for range time.Tick(time.Second) {
				win.Update(a.tick)
			}
		}()
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { quitting = true })
	mygo.App.OnQuit(func() {
		a.host.stopLocal()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
