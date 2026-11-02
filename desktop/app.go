package main

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	pageOverview = "overview"
	pageAccounts = "accounts"
	pageSettings = "settings"
	pageLogs     = "logs"
)

// app 是窗口显示的全部状态。只在主线程读写：
// 后台请求的结果通过 post 交回主线程（正式运行时是 Window.Update）。
type app struct {
	post  func(func())
	store store
	now   func() time.Time
	wg    sync.WaitGroup // 进行中的后台请求，测试用来等它们结束

	// 连接表单
	server     string
	password   string
	remember   bool
	connecting bool
	connectErr string

	cl        *client // nil 表示还没连上
	st        *status
	loading   bool
	loadErr   string
	page      string
	connected time.Time

	// 设置表单
	form       settings
	formEdited bool
	saving     bool
	formErr    string

	busy       map[string]bool // 正在操作的账号
	runningAll bool

	removeOpen bool
	removeMid  string

	qrOpen    bool
	qrImage   *ui.Bitmap
	qrState   string // loading / waiting / scanned / expired / error
	qrErr     string
	qrGen     int
	qrPolling bool

	faces       map[string]*ui.Bitmap // 头像地址 → 图片
	faceLoading map[string]bool

	logScroll ui.ScrollState
	toasts    []string
	ticks     int
}

func newApp(s store, post func(func())) *app {
	p := s.load()
	a := &app{
		post:        post,
		store:       s,
		now:         time.Now,
		server:      p.Server,
		remember:    p.Remember,
		page:        pageOverview,
		busy:        map[string]bool{},
		faces:       map[string]*ui.Bitmap{},
		faceLoading: map[string]bool{},
	}
	if a.server == "" {
		a.server = "http://127.0.0.1:8080"
	}
	return a
}

// async 在后台跑 work，再把它返回的 apply 交回主线程执行
func (a *app) async(work func() (apply func())) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		apply := work()
		if apply != nil {
			a.post(apply)
		}
	}()
}

func (a *app) toast(msg string) { a.toasts = append(a.toasts, msg) }

// autoConnect 在启动时用记住的密码直接连接
func (a *app) autoConnect() {
	if !a.remember {
		return
	}
	pw, err := a.store.password()
	if err != nil {
		if !errors.Is(err, mygo.ErrSecretNotFound) {
			log.Printf("读取保存的密码失败：%v", err)
		}
		return
	}
	a.password = pw
	a.connect()
}

func (a *app) connect() {
	if a.connecting {
		return
	}
	a.connecting, a.connectErr = true, ""
	server, password, remember := a.server, a.password, a.remember
	a.async(func() func() {
		cl, err := newClient(server, password)
		var st status
		if err == nil {
			err = cl.login()
		}
		if err == nil {
			st, err = cl.status()
		}
		return func() {
			a.connecting = false
			if err != nil {
				a.connectErr = err.Error()
				return
			}
			a.server, a.cl, a.connected = cl.base, cl, a.now()
			a.loadErr, a.formEdited, a.formErr = "", false, ""
			a.applyStatus(st)
			a.savePrefs(remember, password)
		}
	})
}

func (a *app) savePrefs(remember bool, password string) {
	if err := a.store.save(prefs{Server: a.server, Remember: remember}); err != nil {
		log.Printf("保存连接信息失败：%v", err)
	}
	var err error
	if remember {
		err = a.store.setPassword(password)
	} else {
		err = a.store.forgetPassword()
	}
	if err != nil {
		log.Printf("保存密码失败：%v", err)
		if remember {
			a.toast("系统凭据管理器不可用，密码没有记住")
		}
	}
}

func (a *app) disconnect() {
	if cl := a.cl; cl != nil {
		go cl.logout()
	}
	a.cl, a.st, a.loadErr = nil, nil, ""
	a.qrOpen, a.removeOpen = false, false
	if !a.remember {
		a.password = ""
	}
}

func (a *app) applyStatus(st status) {
	a.st = &st
	if !a.formEdited {
		a.form = st.Settings
	}
	for _, acc := range st.Accounts {
		a.loadFace(acc.Face)
	}
}

// refresh 拉一次状态；已经在拉就跳过
func (a *app) refresh() {
	cl := a.cl
	if cl == nil || a.loading {
		return
	}
	a.loading = true
	a.async(func() func() {
		st, err := cl.status()
		return func() {
			a.loading = false
			if a.cl != cl {
				return // 期间断开或换了服务器
			}
			if errors.Is(err, errUnauthorized) {
				a.disconnect()
				a.connectErr = "面板密码已经改了，请重新输入"
				return
			}
			if err != nil {
				a.loadErr = err.Error()
				return
			}
			a.loadErr = ""
			a.applyStatus(st)
		}
	})
}

// tick 每秒在主线程跑一次：每 5 秒刷新状态，扫码时每 2 秒查一次扫码结果
func (a *app) tick() {
	a.ticks++
	if a.cl == nil {
		return
	}
	if a.ticks%5 == 0 {
		a.refresh()
	}
	if a.qrOpen && a.ticks%2 == 0 {
		a.pollQR()
	}
}

func (a *app) loadFace(face string) {
	if face == "" || a.faces[face] != nil || a.faceLoading[face] || faceURL(face) == "" {
		return
	}
	a.faceLoading[face] = true
	a.async(func() func() {
		data, err := fetchFace(face)
		return func() {
			if err != nil {
				return // 留着 faceLoading，不反复重试；用名字首字代替
			}
			if bm, err := ui.DecodeBitmap(data); err == nil {
				a.faces[face] = bm
			}
		}
	})
}

// action 在后台对服务器做一件事，完成后刷新状态
func (a *app) action(work func(cl *client) error, done func(err error)) {
	cl := a.cl
	if cl == nil {
		return
	}
	a.async(func() func() {
		err := work(cl)
		return func() {
			done(err)
			if a.cl == cl {
				a.refresh()
			}
		}
	})
}

func (a *app) runAll() {
	a.runningAll = true
	var n int
	a.action(func(cl *client) (err error) {
		n, err = cl.runAll()
		return err
	}, func(err error) {
		a.runningAll = false
		if err != nil {
			a.toast(err.Error())
			return
		}
		a.toast(fmt.Sprintf("已让 %d 个账号立即运行", n))
	})
}

func (a *app) runAccount(acc account) {
	a.busy[acc.Mid] = true
	a.action(func(cl *client) error { return cl.runAccount(acc.Mid) }, func(err error) {
		delete(a.busy, acc.Mid)
		if err != nil {
			a.toast(err.Error())
		}
	})
}

func (a *app) askRemove(mid string) {
	a.removeMid, a.removeOpen = mid, true
}

func (a *app) removeAccount(mid string) {
	name := a.accountName(mid)
	a.busy[mid] = true
	a.action(func(cl *client) error { return cl.removeAccount(mid) }, func(err error) {
		delete(a.busy, mid)
		if err != nil {
			a.toast(err.Error())
			return
		}
		a.toast("已退出 " + name)
	})
}

func (a *app) accountName(mid string) string {
	if a.st != nil {
		for _, acc := range a.st.Accounts {
			if acc.Mid == mid && acc.Uname != "" {
				return acc.Uname
			}
		}
	}
	return "UID " + mid
}

func (a *app) saveSettings() {
	a.saving, a.formErr = true, ""
	form := a.form
	var saved settings
	a.action(func(cl *client) (err error) {
		saved, err = cl.saveSettings(form)
		return err
	}, func(err error) {
		a.saving = false
		if err != nil {
			a.formErr = err.Error()
			return
		}
		a.form, a.formEdited = saved, false
		a.toast("设置已保存")
	})
}

func (a *app) resetSettings() {
	if a.st != nil {
		a.form = a.st.Settings
	}
	a.formEdited, a.formErr = false, ""
}

func (a *app) openQR() {
	a.qrOpen = true
	a.startQR()
}

func (a *app) startQR() {
	cl := a.cl
	if cl == nil {
		return
	}
	a.qrGen++
	gen := a.qrGen
	a.qrImage, a.qrState, a.qrErr = nil, "loading", ""
	a.async(func() func() {
		png, err := cl.startQR()
		return func() {
			if gen != a.qrGen || !a.qrOpen {
				return
			}
			if err == nil {
				a.qrImage, err = ui.DecodeBitmap(png)
			}
			if err != nil {
				a.qrState, a.qrErr = "error", err.Error()
				return
			}
			a.qrImage.Pixelated()
			a.qrState = "waiting"
		}
	})
}

func (a *app) pollQR() {
	cl := a.cl
	if cl == nil || a.qrPolling || (a.qrState != "waiting" && a.qrState != "scanned") {
		return
	}
	a.qrPolling = true
	gen := a.qrGen
	a.async(func() func() {
		state, mid, err := cl.pollQR()
		return func() {
			a.qrPolling = false
			if gen != a.qrGen || !a.qrOpen {
				return
			}
			switch {
			case err != nil:
				a.qrState, a.qrErr = "error", err.Error()
			case state == "success":
				a.qrOpen = false
				a.page = pageAccounts
				a.toast(fmt.Sprintf("已添加账号 UID %s，正在运行第一轮", mid))
				a.refresh()
			case state == "none":
				a.qrState = "expired"
			default:
				a.qrState = state
			}
		}
	})
}
