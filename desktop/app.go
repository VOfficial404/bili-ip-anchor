package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/VOfficial404/bili-ip-anchor/core"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// version 要和 mygo.json 的 version 一致（version_test.go 检查）
const version = "v0.6.0"

const (
	pageOverview = "overview"
	pageAccounts = "accounts"
	pageSettings = "settings"
	pageLogs     = "logs"

	modeRemote = "remote"
	modeLocal  = "local"
)

// proxyProbe 是一次代理测试的结果，连接页和设置页共用
type proxyProbe struct {
	proxy   string
	running bool
	zone    *zone
	err     string
}

type videoEntry struct {
	info  *videoInfo
	err   string
	cover *ui.Bitmap
}

// app 是窗口显示的全部状态。只在主线程读写：
// 后台请求的结果通过 post 交回主线程（正式运行时是 Window.Update）。
type app struct {
	post  func(func())
	store store
	now   func() time.Time
	wg    sync.WaitGroup // 进行中的后台请求，测试用来等它们结束

	onLocal   func(running bool)       // 本机运行开始、停止时调用，main 用它显示托盘图标
	pickFiles func() ([]string, error) // 选择图片，在后台 goroutine 里调用
	transport http.RoundTripper        // 不为 nil 时本机模式用它代替网络，给测试用

	// 连接页
	modeIdx    int // 0 连接服务器，1 在本机运行
	server     string
	password   string
	remember   bool
	proxy      string
	autoLocal  bool
	connecting bool
	connectErr string
	probe      proxyProbe

	engine    *core.Engine // 本机运行时不为 nil
	cl        *client      // nil 表示还没连上
	st        *status
	loading   bool
	loadErr   string
	page      string
	connected time.Time

	// 设置表单
	form        settings
	formEdited  bool
	saving      bool
	formErr     string
	videoAdd    string
	videoErr    string
	videoAdding bool
	videos      map[string]*videoEntry // BV 号 → 视频信息
	images      map[string]*ui.Bitmap  // 动态图片文件名 → 缩略图
	imageTried  map[string]bool
	uploading   bool

	busy       map[string]bool // 正在操作的账号
	runningAll bool

	confirmOpen   bool
	confirmTitle  string
	confirmText   string
	confirmOK     string
	confirmAction func()

	qrOpen    bool
	qrMid     string // 空表示添加账号，否则是给这个账号授权查询主页属地
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
		pickFiles:   pickImages,
		server:      p.Server,
		remember:    p.Remember,
		proxy:       p.Proxy,
		autoLocal:   p.AutoLocal,
		page:        pageOverview,
		busy:        map[string]bool{},
		videos:      map[string]*videoEntry{},
		images:      map[string]*ui.Bitmap{},
		imageTried:  map[string]bool{},
		faces:       map[string]*ui.Bitmap{},
		faceLoading: map[string]bool{},
	}
	if p.Mode == modeLocal {
		a.modeIdx = 1
	}
	if a.server == "" {
		a.server = "http://127.0.0.1:8080"
	}
	return a
}

func pickImages() ([]string, error) {
	return mygo.Dialog.Open(mygo.OpenDialogOptions{
		Title:    "选择动态图片",
		Multiple: true,
		Filters:  []mygo.FileFilter{{Name: "图片", Extensions: []string{"jpg", "jpeg", "png", "gif", "webp"}}},
	})
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

func (a *app) local() bool { return a.engine != nil }

// oldServer 表示连的是 v0.5 及更早的服务器，没有账号详情、多个视频等功能
func (a *app) oldServer() bool { return a.st != nil && a.st.Mode == "" }

// autoConnect 在启动时自动连接：上次是本机运行并勾选了自动开始，或者记住了服务器密码
func (a *app) autoConnect() {
	if a.modeIdx == 1 {
		if a.autoLocal && a.proxy != "" {
			a.startLocal()
		}
		return
	}
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
			a.server = cl.base
			a.connected = a.now()
			a.useClient(cl, st)
			a.savePrefs(func(p *prefs) { p.Mode, p.Server, p.Remember = modeRemote, a.server, remember })
			a.savePassword(remember, password)
			if a.oldServer() {
				a.toast(fmt.Sprintf("服务器是 %s，账号详情、多个视频等功能需要服务器更新到 v0.6", st.Version))
			}
		}
	})
}

func (a *app) useClient(cl *client, st status) {
	a.cl = cl
	a.loadErr, a.formEdited, a.formErr = "", false, ""
	a.applyStatus(st)
}

// startLocal 在这台电脑上运行定时任务：先确认代理可用，再启动核心和只在本机监听的 WebUI，
// 界面像连服务器一样连过去
func (a *app) startLocal() {
	if a.connecting {
		return
	}
	proxy := strings.TrimSpace(a.proxy)
	if proxy == "" {
		a.connectErr = "请填写本机代理的地址"
		return
	}
	if _, err := core.ParseProxy(proxy); err != nil {
		a.connectErr = err.Error()
		return
	}
	a.connecting, a.connectErr = true, ""
	autoLocal, rt := a.autoLocal, a.transport
	a.async(func() func() {
		var (
			e       *core.Engine
			cl      *client
			st      status
			warning string
		)
		via, err := core.ProbeProxy(proxy, rt)
		if err == nil && rt == nil {
			// 和直连比一下：一样说明代理没有接管这些请求
			if direct, derr := core.ProbeProxy("", nil); derr == nil && direct.Addr == via.Addr {
				warning = fmt.Sprintf("注意：经过代理和直连时 B 站看到的都是 %s，代理可能没有生效", via.Addr)
			}
		}
		var dir string
		if err == nil {
			dir, err = a.store.dataDir()
		}
		if err == nil {
			e = core.New(core.Config{
				DataDir:   dir,
				Version:   version,
				Password:  core.RandomPassword(),
				Local:     true,
				Defaults:  core.DefaultSettings(),
				LogOutput: os.Stderr,
				Transport: rt,
			})
			if s := e.Settings(); s.Proxy != proxy {
				s.Proxy = proxy
				err = e.UpdateSettings(s)
			}
		}
		if err == nil {
			e.Start()
			var addr string
			if addr, err = e.ServeLocal(); err == nil {
				cl, err = newClient(addr, e.Password())
			}
			if err == nil {
				err = cl.login()
			}
			if err == nil {
				st, err = cl.status()
			}
			if err != nil {
				e.Stop()
			}
		}
		return func() {
			a.connecting = false
			if err != nil {
				a.connectErr = err.Error()
				return
			}
			z := zone{Addr: via.Addr, Country: via.Country, Province: via.Province, City: via.City, ISP: via.ISP}
			a.probe = proxyProbe{proxy: proxy, zone: &z}
			a.engine, a.connected = e, a.now()
			a.useClient(cl, st)
			a.savePrefs(func(p *prefs) { p.Mode, p.Proxy, p.AutoLocal = modeLocal, proxy, autoLocal })
			if warning != "" {
				a.toast(warning)
			}
			if a.onLocal != nil {
				a.onLocal(true)
			}
		}
	})
}

func (a *app) savePrefs(fn func(p *prefs)) {
	p := a.store.load()
	fn(&p)
	if err := a.store.save(p); err != nil {
		log.Printf("保存连接信息失败：%v", err)
	}
}

func (a *app) savePassword(remember bool, password string) {
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

// disconnect 断开服务器；本机运行时停下定时任务
func (a *app) disconnect() {
	if cl := a.cl; cl != nil {
		go cl.logout()
	}
	if e := a.engine; e != nil {
		a.engine = nil
		go e.Stop()
		a.toast("已停止本机运行，账号和设置都还在")
		if a.onLocal != nil {
			a.onLocal(false)
		}
	}
	a.cl, a.st, a.loadErr = nil, nil, ""
	a.qrOpen, a.confirmOpen = false, false
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
	for _, v := range a.form.Videos {
		a.loadVideo(v)
	}
	a.loadImages(st.Images)
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
	if face == "" || a.faces[face] != nil || a.faceLoading[face] || hdslbURL(face, "") == "" {
		return
	}
	a.faceLoading[face] = true
	a.async(func() func() {
		data, err := fetchHdslb(face, faceSize)
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

// loadVideo 查询视频标题和封面，每个 BV 号只查一次
func (a *app) loadVideo(bvid string) {
	cl := a.cl
	if cl == nil || a.videos[bvid] != nil {
		return
	}
	v := &videoEntry{}
	a.videos[bvid] = v
	a.async(func() func() {
		info, err := cl.video(bvid)
		var cover []byte
		if err == nil {
			cover, _ = fetchHdslb(info.Cover, coverSize)
		}
		return func() {
			if err != nil {
				v.err = err.Error()
				return
			}
			v.info = &info
			if bm, err := ui.DecodeBitmap(cover); err == nil {
				v.cover = bm
			}
		}
	})
}

// loadImages 下载还没有缩略图的动态图片
func (a *app) loadImages(list []imageInfo) {
	cl := a.cl
	for _, img := range list {
		name := img.Name
		if cl == nil || a.imageTried[name] {
			continue
		}
		a.imageTried[name] = true
		a.async(func() func() {
			data, err := cl.image(name)
			return func() {
				if err != nil {
					return
				}
				if bm, err := ui.DecodeBitmap(data); err == nil {
					a.images[name] = bm
				}
			}
		})
	}
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

// accountAction 对账号做一件事（立即运行、刷新资料、刷新 Cookie、取消授权、退出），成功时提示 done
func (a *app) accountAction(mid, method, action, done string) {
	a.busy[mid] = true
	a.action(func(cl *client) error { return cl.accountAction(method, mid, action) }, func(err error) {
		delete(a.busy, mid)
		if err != nil {
			a.toast(err.Error())
			return
		}
		if done != "" {
			a.toast(done)
		}
	})
}

func (a *app) askConfirm(title, text, ok string, fn func()) {
	a.confirmTitle, a.confirmText, a.confirmOK, a.confirmAction = title, text, ok, fn
	a.confirmOpen = true
}

func (a *app) askRemove(mid string) {
	name := a.accountName(mid)
	where := "服务器上的"
	if a.local() {
		where = "这台电脑上的"
	}
	a.askConfirm(fmt.Sprintf("退出「%s」？", name), where+"这份会话会在 B 站注销并删除，之后不再为这个账号运行。", "退出",
		func() { a.accountAction(mid, "DELETE", "", "已退出 "+name) })
}

func (a *app) askUnauth(mid string) {
	a.askConfirm(fmt.Sprintf("取消「%s」的主页属地授权？", a.accountName(mid)), "会在 B 站撤销这份 App 授权，之后不再显示主页属地。不影响定时任务。", "取消授权",
		func() { a.accountAction(mid, "DELETE", "appauth", "已取消授权") })
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

/* 设置 */

func (a *app) saveSettings() {
	a.saving, a.formErr = true, ""
	form := a.form
	form.Proxy = strings.TrimSpace(form.Proxy)
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
		if a.local() {
			a.savePrefs(func(p *prefs) { p.Proxy = saved.Proxy })
			a.proxy = saved.Proxy
		}
		a.toast("设置已保存")
	})
}

func (a *app) resetSettings() {
	if a.st != nil {
		a.form = a.st.Settings
	}
	a.formEdited, a.formErr, a.videoErr = false, "", ""
}

var bvidFindRe = regexp.MustCompile(`BV[0-9A-Za-z]{10}`)

const maxVideos = 20

// addVideos 从输入框里找出 BV 号，查到视频后加进列表
func (a *app) addVideos() {
	cl := a.cl
	var found []string
	for _, v := range bvidFindRe.FindAllString(a.videoAdd, -1) {
		if !slices.Contains(found, v) && !slices.Contains(a.form.Videos, v) {
			found = append(found, v)
		}
	}
	if len(found) == 0 {
		if bvidFindRe.MatchString(a.videoAdd) {
			a.videoErr, a.videoAdd = "已经在列表里了", ""
		} else {
			a.videoErr = "没找到 BV 号。粘贴视频链接或 BV 开头的 12 位编号"
		}
		return
	}
	if cl == nil || a.videoAdding {
		return
	}
	a.videoAdding, a.videoErr = true, ""
	a.async(func() func() {
		infos := make([]*videoInfo, len(found))
		errs := make([]error, len(found))
		for i, bvid := range found {
			info, err := cl.video(bvid)
			infos[i], errs[i] = &info, err
		}
		return func() {
			a.videoAdding = false
			var bad []string
			for i, bvid := range found {
				switch {
				case errs[i] != nil:
					bad = append(bad, bvid+"："+errs[i].Error())
				case len(a.form.Videos) >= maxVideos:
					bad = append(bad, fmt.Sprintf("最多 %d 个视频", maxVideos))
				default:
					a.videos[bvid] = &videoEntry{info: infos[i]}
					a.form.Videos = append(a.form.Videos, bvid)
					a.formEdited = true
					a.loadCover(bvid)
				}
			}
			a.videoAdd = ""
			a.videoErr = strings.Join(slices.Compact(bad), "；")
		}
	})
}

func (a *app) loadCover(bvid string) {
	v := a.videos[bvid]
	if v == nil || v.info == nil || v.cover != nil {
		return
	}
	src := v.info.Cover
	a.async(func() func() {
		data, err := fetchHdslb(src, coverSize)
		return func() {
			if err == nil {
				if bm, err := ui.DecodeBitmap(data); err == nil {
					v.cover = bm
				}
			}
		}
	})
}

func (a *app) removeVideo(bvid string) {
	a.form.Videos = slices.DeleteFunc(slices.Clone(a.form.Videos), func(v string) bool { return v == bvid })
	a.formEdited = true
}

// testProxy 查询经过代理时 B 站看到的出口 IP。连接页还没有核心在跑，直接在本机查。
func (a *app) testProxy(proxy string) {
	proxy = strings.TrimSpace(proxy)
	if proxy == "" {
		a.probe = proxyProbe{err: "先填写代理地址"}
		return
	}
	if a.probe.running {
		return
	}
	a.probe = proxyProbe{proxy: proxy, running: true}
	cl, rt := a.cl, a.transport
	a.async(func() func() {
		var z zone
		var err error
		if cl != nil {
			z, err = cl.testProxy(proxy)
		} else {
			var cz core.Zone
			cz, err = core.ProbeProxy(proxy, rt)
			z = zone{Addr: cz.Addr, Country: cz.Country, Province: cz.Province, City: cz.City, ISP: cz.ISP}
		}
		return func() {
			if a.probe.proxy != proxy {
				return
			}
			a.probe.running = false
			if err != nil {
				a.probe.err = err.Error()
				return
			}
			a.probe.zone = &z
		}
	})
}

const maxImageSize = 10 << 20

// addImages 选择图片并上传到动态图片里
func (a *app) addImages() {
	cl := a.cl
	if cl == nil || a.uploading {
		return
	}
	a.uploading = true
	pick := a.pickFiles
	a.async(func() func() {
		paths, err := pick()
		var bad []string
		n := 0
		if err != nil {
			bad = append(bad, err.Error())
		}
		for _, p := range paths {
			data, err := readLimited(p, maxImageSize)
			if err == nil {
				err = cl.uploadImage(data)
			}
			if err != nil {
				bad = append(bad, fmt.Sprintf("%s：%v", baseName(p), err))
				continue
			}
			n++
		}
		return func() {
			a.uploading = false
			if n > 0 {
				a.toast(fmt.Sprintf("已添加 %d 张图片", n))
			}
			for _, msg := range bad {
				a.toast(msg)
			}
			a.refresh()
		}
	})
}

func readLimited(path string, max int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > max {
		return nil, fmt.Errorf("图片不能超过 %d MB", max>>20)
	}
	return os.ReadFile(path)
}

func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

func (a *app) removeImage(name string) {
	a.action(func(cl *client) error { return cl.removeImage(name) }, func(err error) {
		if err != nil {
			a.toast(err.Error())
			return
		}
		delete(a.images, name)
	})
}

/* 扫码：添加账号，或给某个账号授权查询主页属地 */

func (a *app) openQR(mid string) {
	a.qrOpen, a.qrMid = true, mid
	a.startQR()
}

func (a *app) startQR() {
	cl := a.cl
	if cl == nil {
		return
	}
	a.qrGen++
	gen, mid := a.qrGen, a.qrMid
	a.qrImage, a.qrState, a.qrErr = nil, "loading", ""
	a.async(func() func() {
		png, err := cl.startQR(mid)
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
	gen, mid := a.qrGen, a.qrMid
	a.async(func() func() {
		state, newMid, err := cl.pollQR(mid)
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
				if mid == "" {
					a.toast(fmt.Sprintf("已添加账号 UID %s，正在运行第一轮", newMid))
				} else {
					a.toast(fmt.Sprintf("已授权，正在查询「%s」的主页属地", a.accountName(mid)))
				}
				a.refresh()
			case state == "none":
				a.qrState = "expired"
			default:
				a.qrState = state
			}
		}
	})
}
