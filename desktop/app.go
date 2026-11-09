package main

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// version 要和 mygo.json 的 version 一致（version_test.go 检查）
const version = "v0.7.0"

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

	host      host
	onLocal   func(running bool)       // 本机运行开始、停止时调用，main 用它显示托盘图标
	pickFiles func() ([]picked, error) // 选择图片，在后台 goroutine 里调用

	phone       bool   // 在手机 App 里（WebAssembly）
	platform    string // 手机 App 的系统：android 或 ios
	resumeLocal bool   // 手机 App 重新打开界面时，本机的定时任务还在跑

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

	localOn   bool    // 正在本机运行
	cl        *client // nil 表示还没连上
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

	width       float32 // 窗口内容的宽度，每帧更新
	narrow      bool    // 窗口窄（手机）：侧边栏换成底部标签栏，卡片竖着排
	logScroll   ui.ScrollState
	logs        []logEntry // 从 /api/logs 拉到的，最多 maxLogs 条
	logSeq      int64      // 已经拉到的最后一条
	logsLoading bool
	logsTried   bool
	logMin      int    // 日志页的级别筛选，下标对应 logMins
	logAccount  string // 日志页的账号筛选，allAccounts 表示全部
	toasts      []string
	ticks       int
}

func newApp(s store, post func(func())) *app {
	p := s.load()
	a := &app{
		post:        post,
		store:       s,
		host:        newHost(s),
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

func (a *app) local() bool { return a.localOn }

// device 是本机运行时「这台电脑」还是「这台手机」
func (a *app) device() string {
	if a.phone {
		return "这台手机"
	}
	return "这台电脑"
}

// oldServer 表示连的是 v0.5 及更早的服务器，没有账号详情、多个视频等功能
func (a *app) oldServer() bool { return a.st != nil && a.st.Mode == "" }

// autoConnect 在启动时自动连接：上次是本机运行并勾选了自动开始，或者记住了服务器密码
func (a *app) autoConnect() {
	if a.modeIdx == 1 || a.resumeLocal {
		if (a.autoLocal || a.resumeLocal) && a.proxy != "" {
			a.modeIdx = 1
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
	server, err := normalizeServer(a.server)
	if err != nil {
		a.connectErr = err.Error()
		return
	}
	a.connecting, a.connectErr = true, ""
	password, remember, h := a.password, a.remember, a.host
	a.async(func() func() {
		var (
			cl *client
			st status
		)
		base, err := h.remote(server)
		if err == nil {
			cl, err = newClient(base, password)
		}
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
			a.server = server
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
	a.resetLogs()
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
	if !proxyRe.MatchString(proxy) {
		a.connectErr = "代理地址不对，应写成 http://127.0.0.1:7890 或 socks5://127.0.0.1:7891"
		return
	}
	a.connecting, a.connectErr = true, ""
	autoLocal, h := a.autoLocal, a.host
	a.async(func() func() {
		var (
			cl *client
			st status
		)
		run, err := h.startLocal(proxy)
		if err == nil {
			cl, err = newClient(run.Base, run.Password)
			if err == nil {
				err = cl.login()
			}
			if err == nil {
				st, err = cl.status()
			}
			if err != nil {
				h.stopLocal()
			}
		}
		return func() {
			a.connecting = false
			a.resumeLocal = false
			if err != nil {
				a.connectErr = err.Error()
				return
			}
			z := run.Zone
			a.probe = proxyProbe{proxy: proxy, zone: &z}
			a.localOn, a.connected = true, a.now()
			a.useClient(cl, st)
			a.savePrefs(func(p *prefs) { p.Mode, p.Proxy, p.AutoLocal = modeLocal, proxy, autoLocal })
			if run.Warning != "" {
				a.toast(run.Warning)
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
	if a.localOn {
		a.localOn = false
		h := a.host
		a.async(func() func() { h.stopLocal(); return nil })
		a.toast("已停止本机运行，账号和设置都还在")
		if a.onLocal != nil {
			a.onLocal(false)
		}
	}
	a.cl, a.st, a.loadErr = nil, nil, ""
	a.resetLogs()
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
	if a.page == pageLogs && a.ticks%2 == 0 {
		a.fetchLogs()
	}
}

const maxLogs = 1000

// levelLogs 表示服务器有分级日志（v0.7 起）
func (a *app) levelLogs() bool { return a.st != nil && a.st.LogLevel != "" }

func (a *app) resetLogs() {
	a.logs, a.logSeq, a.logsTried = nil, 0, false
}

// fetchLogs 拉新增的日志；已经在拉就跳过
func (a *app) fetchLogs() {
	cl := a.cl
	if cl == nil || a.logsLoading || !a.levelLogs() {
		return
	}
	a.logsLoading, a.logsTried = true, true
	after := a.logSeq
	a.async(func() func() {
		entries, err := cl.logs(after)
		return func() {
			a.logsLoading = false
			if a.cl != cl || a.logSeq != after || err != nil || len(entries) == 0 {
				return // 期间断开、换了服务器或者出错：下次再拉；连接错误由 refresh 显示
			}
			if entries[0].Seq <= after {
				a.logs = nil // 服务器重启过，序号从头开始，返回的是全部
			}
			a.logs = append(a.logs, entries...)
			if n := len(a.logs); n > maxLogs {
				a.logs = slices.Clone(a.logs[n-maxLogs:])
			}
			a.logSeq = entries[len(entries)-1].Seq
		}
	})
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
		where = a.device() + "上的"
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
	cl, h := a.cl, a.host
	a.async(func() func() {
		var z zone
		var err error
		if cl != nil {
			z, err = cl.testProxy(proxy)
		} else {
			z, err = h.probe(proxy)
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
		files, err := pick()
		var bad []string
		n := 0
		if err != nil {
			bad = append(bad, err.Error())
		}
		for _, f := range files {
			err := f.err
			if err == nil {
				err = cl.uploadImage(f.data)
			}
			if err != nil {
				bad = append(bad, fmt.Sprintf("%s：%v", f.name, err))
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
