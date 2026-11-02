// bili-ip-anchor-desktop 是 bili-ip-anchor 的桌面客户端：
// 通过服务器 WebUI 的接口查看状态、扫码添加账号、修改设置和看日志。
// 定时任务仍然在服务器上跑，这个程序只是一个遥控器。
package main

import (
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	var win *mygo.Window
	a := newApp(systemStore{}, func(fn func()) { win.Update(fn) })
	mygo.App.WhenReady(func() {
		win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "Bili IP Anchor",
			Width:     1040,
			Height:    700,
			MinWidth:  760,
			MinHeight: 520,
			StateKey:  "main", // 下次在上次关闭的位置打开
			Content:   ui.View(a.view),
		})
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
