//go:build js

// 手机 App 的界面：和桌面客户端同一份代码，编译成 WebAssembly，在 App 外壳的 WebView 里运行。
// 没有托盘，也不需要隐藏窗口：本机运行时 Android 外壳用前台服务让定时任务在后台继续。
package main

import (
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	state, err := loadHostState()
	if err != nil {
		log.Printf("读取 App 保存的连接信息失败：%v", err)
	}
	if state.Prefs.Server == "" {
		state.Prefs.Server = "https://"
	}
	var win *mygo.Window
	a := newApp(&webStore{p: state.Prefs, pw: state.Password}, func(fn func()) { win.Update(fn) })
	a.phone = true
	a.platform = state.Platform
	// 界面重新打开（例如 Android 上从通知回来）时，本机的定时任务可能还在跑，直接接上
	a.resumeLocal = state.LocalRunning

	mygo.App.WhenReady(func() {
		win = mygo.NewWindow(mygo.WindowOptions{Title: "Bili IP Anchor", Content: ui.View(a.view)})
		a.autoConnect()
		go func() {
			for range time.Tick(time.Second) {
				win.Update(a.tick)
			}
		}()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
