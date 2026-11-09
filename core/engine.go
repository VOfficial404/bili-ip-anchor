// Package core 是 bili-ip-anchor 的核心：账号会话、定时任务、B 站接口和 WebUI。
// 服务器版、桌面客户端的本机模式和手机 App 都用它。
package core

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	mrand "math/rand"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"
)

type Config struct {
	DataDir   string
	Version   string
	Password  string            // WebUI 密码
	Local     bool              // 在桌面或手机 App 里运行，而不是服务器上
	Defaults  Settings          // settings.json 不存在时用它
	LogOutput io.Writer         // 除了 WebUI 的日志缓冲以外再写到哪里，可以为 nil
	Transport http.RoundTripper // 不为 nil 时代替真实网络和代理，给测试和演示用
}

type runResult struct {
	At    time.Time `json:"at"`
	OK    bool      `json:"ok"`
	Error string    `json:"error,omitempty"`
}

var errNoAccount = errors.New("找不到这个账号")

// Engine 管理所有账号的排程和 WebUI。一个数据目录只能有一个 Engine 在运行。
type Engine struct {
	cfg  Config
	logs *logBook

	mu         sync.Mutex
	set        Settings
	accounts   map[string]*account
	zone       *Zone
	zoneErr    string
	zoneAt     time.Time
	qr         *QRLogin
	tvqr       map[string]*tvLogin // 正在授权的账号
	sessions   map[string]time.Time
	autoTokens map[string]time.Time
	videos     map[string]videoCache
	transports map[string]*http.Transport

	zoneKick chan struct{}
	stop     chan struct{}
	stopOnce sync.Once
	srv      *http.Server
}

func New(cfg Config) *Engine {
	e := &Engine{
		cfg:        cfg,
		logs:       newLogBook(1000, cfg.LogOutput, cfg.Defaults.LogLevel),
		accounts:   map[string]*account{},
		tvqr:       map[string]*tvLogin{},
		sessions:   map[string]time.Time{},
		autoTokens: map[string]time.Time{},
		videos:     map[string]videoCache{},
		transports: map[string]*http.Transport{},
		zoneKick:   make(chan struct{}, 1),
		stop:       make(chan struct{}),
	}
	e.set = e.loadSettings()
	e.logs.SetLevel(e.set.LogLevel)
	return e
}

func (e *Engine) Settings() Settings {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.set
}

func (e *Engine) DataDir() string { return e.cfg.DataDir }

// Start 迁移旧数据，接上所有账号并开始排程。WebUI 另外用 Handler、ServeLocal 或 ListenAndServe 提供。
func (e *Engine) Start() {
	e.migrate()
	s := e.Settings()
	where := "直连"
	if s.Proxy != "" {
		where = "经代理 " + redactProxy(s.Proxy)
	}
	e.Infof("bili-ip-anchor %s 启动：间隔 %s，随机抖动 %s，%s，日志级别 %s，数据目录 %s", e.cfg.Version, s.Interval, s.Jitter, where, s.LogLevel, e.cfg.DataDir)
	e.syncAccounts(true)
	if n := e.accountCount(); n == 0 {
		e.Infof("还没有账号：点「添加账号」扫码登录")
	} else {
		e.Infof("已加载 %d 个账号，各自独立排程", n)
	}
	go e.syncLoop()
	go e.zoneLoop()
}

// Stop 停下所有账号和后台循环，关闭 ServeLocal 开的 WebUI
func (e *Engine) Stop() {
	e.stopOnce.Do(func() {
		close(e.stop)
		e.mu.Lock()
		for mid, acc := range e.accounts {
			close(acc.stop)
			delete(e.accounts, mid)
		}
		srv := e.srv
		e.mu.Unlock()
		if srv != nil {
			srv.Close()
		}
	})
}

func (e *Engine) stopped() bool {
	select {
	case <-e.stop:
		return true
	default:
		return false
	}
}

// sleep 等 d，Engine 停止时提前返回 false
func (e *Engine) sleep(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-e.stop:
		return false
	}
}

func (e *Engine) migrate() {
	e.migrateLegacyCookies()
	e.migrateLegacyDynamicImage()
}

func (e *Engine) account(mid string) *account {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.accounts[mid]
}

func (e *Engine) accountCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.accounts)
}

func (e *Engine) startAccountLocked(mid string, first time.Duration) {
	acc := e.newAccount(mid)
	e.accounts[mid] = acc
	go acc.loop(first)
}

// syncAccounts 让运行中的账号和 accounts/ 目录保持一致：
// 命令行 login 新加的账号会被接上，会话文件被删掉的账号会停下。
// stagger 为 true 时（启动时）各账号的第一轮错开十几秒。
func (e *Engine) syncAccounts(stagger bool) {
	mids, err := e.listAccounts()
	if err != nil {
		e.Errorf("读取账号目录失败：%v", err)
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stopped() {
		return
	}
	listed := map[string]bool{}
	for i, mid := range mids {
		listed[mid] = true
		if _, ok := e.accounts[mid]; ok || !e.hasCookies(mid) {
			continue
		}
		var first time.Duration
		if stagger && i > 0 {
			first = time.Duration(i)*15*time.Second + jitter(10*time.Second)
		}
		e.startAccountLocked(mid, first)
		if !stagger {
			e.Infof("发现新账号 UID %s", mid)
		}
	}
	for mid, acc := range e.accounts {
		if !listed[mid] && !e.hasCookies(mid) {
			close(acc.stop)
			delete(e.accounts, mid)
			e.Warnf("账号 UID %s 的会话文件已不在，停止运行", mid)
		}
	}
}

func (e *Engine) syncLoop() {
	for e.sleep(time.Minute) {
		e.syncAccounts(false)
	}
}

// zoneLoop 定时查询 B 站把出口 IP 识别为哪里；换代理后立即重新查
func (e *Engine) zoneLoop() {
	for {
		zone, err := fetchZone(e.anonClient())
		e.mu.Lock()
		e.zoneAt = time.Now()
		if err != nil {
			e.zoneErr = err.Error()
		} else {
			e.zone, e.zoneErr = &zone, ""
		}
		e.mu.Unlock()
		wait := 30 * time.Minute
		if err == nil {
			e.Debugf("B 站把出口 IP %s 识别为 %s %s", zone.Addr, zone.region(), zone.ISP)
		}
		if err != nil {
			e.Warnf("查询出口 IP 归属失败：%v", err)
			wait = 5 * time.Minute
		}
		t := time.NewTimer(wait)
		select {
		case <-t.C:
		case <-e.zoneKick:
			t.Stop()
		case <-e.stop:
			t.Stop()
			return
		}
	}
}

func (e *Engine) currentZone() *Zone {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.zone
}

// accountLoggedIn 在扫码成功后调用：新账号开始排程，已有账号立即跑一轮
func (e *Engine) accountLoggedIn(mid string) {
	e.mu.Lock()
	acc, ok := e.accounts[mid]
	if !ok && !e.stopped() {
		e.startAccountLocked(mid, 0)
	}
	e.mu.Unlock()
	if ok {
		acc.mu.Lock()
		acc.loggedIn = true
		acc.mu.Unlock()
		acc.runNow()
	}
}

// removeAccount 让 B 站注销会话，删除本地目录并停止排程。远端注销失败也会删除本地目录。
func (e *Engine) removeAccount(mid string) error {
	acc := e.account(mid)
	if acc == nil {
		return errNoAccount
	}
	name := acc.name()
	if cookies, err := e.loadAccountCookies(mid); err == nil {
		if err := logout(e.client(cookies), cookies); err != nil {
			acc.warnf("B 站注销会话失败（本地会话仍会删除）：%v", err)
		} else {
			acc.infof("已在 B 站注销这份会话")
		}
	}
	if sess := e.loadSession(mid); sess.AccessKey != "" {
		if err := revokeAccessKey(e.anonClient(), sess.AccessKey); err != nil {
			acc.warnf("撤销 App 授权失败：%v", err)
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if err := removeAll(e.accountDir(mid)); err != nil {
		return err
	}
	if cur, ok := e.accounts[mid]; ok && cur == acc {
		close(acc.stop)
		delete(e.accounts, mid)
	}
	e.Infof("已移除账号 %s", name)
	return nil
}

// RunAll 让所有空闲账号立即跑一轮，返回实际触发的数量
func (e *Engine) RunAll() int {
	n := 0
	for _, acc := range e.accountList() {
		if acc.runNow() {
			n++
		}
	}
	return n
}

func (e *Engine) accountList() []*account {
	e.mu.Lock()
	accs := make([]*account, 0, len(e.accounts))
	for _, acc := range e.accounts {
		accs = append(accs, acc)
	}
	e.mu.Unlock()
	// UID 按数值排序
	sort.Slice(accs, func(i, j int) bool {
		x, y := accs[i].mid, accs[j].mid
		if len(x) != len(y) {
			return len(x) < len(y)
		}
		return x < y
	})
	return accs
}

func (e *Engine) UpdateSettings(s Settings) error {
	s.normalize()
	if err := s.Validate(); err != nil {
		return err
	}
	if err := e.saveSettings(s); err != nil {
		return err
	}
	e.mu.Lock()
	proxyChanged := e.set.Proxy != s.Proxy
	e.set = s
	e.logs.SetLevel(s.LogLevel)
	for _, acc := range e.accounts {
		acc.reschedule()
	}
	e.mu.Unlock()
	if proxyChanged {
		select {
		case e.zoneKick <- struct{}{}:
		default:
		}
	}
	where := "直连"
	if s.Proxy != "" {
		where = "代理 " + redactProxy(s.Proxy)
	}
	e.Infof("设置已更新：间隔 %s，随机抖动 %s，%d 个视频，发动态 %v，%s，日志级别 %s", s.Interval, s.Jitter, len(s.Videos), s.DoDynamic, where, s.LogLevel)
	return nil
}

// redactProxy 去掉代理地址里的密码，用于日志
func redactProxy(s string) string {
	u, err := ParseProxy(s)
	if err != nil || u == nil {
		return s
	}
	return u.Redacted()
}

type statusResp struct {
	Version       string          `json:"version"`
	Mode          string          `json:"mode"` // server 或 local
	Accounts      []accountStatus `json:"accounts"`
	Zone          *Zone           `json:"zone"`
	ZoneError     string          `json:"zone_error,omitempty"`
	ZoneCheckedAt *time.Time      `json:"zone_checked_at,omitempty"`
	Settings      Settings        `json:"settings"`
	Images        []imageInfo     `json:"images"`
	Logs          []string        `json:"logs"`      // v0.6 及更早的客户端用；新客户端用 /api/logs
	LogLevel      Level           `json:"log_level"` // 当前的日志级别
}

func (e *Engine) status() statusResp {
	e.mu.Lock()
	resp := statusResp{Version: e.cfg.Version, Mode: "server", Zone: e.zone, ZoneError: e.zoneErr, Settings: e.set}
	if !e.zoneAt.IsZero() {
		at := e.zoneAt
		resp.ZoneCheckedAt = &at
	}
	e.mu.Unlock()
	if e.cfg.Local {
		resp.Mode = "local"
	}
	accs := e.accountList()
	resp.Accounts = make([]accountStatus, 0, len(accs))
	for _, acc := range accs {
		resp.Accounts = append(resp.Accounts, acc.status())
	}
	resp.Images, _ = e.listImages()
	if resp.Images == nil {
		resp.Images = []imageInfo{}
	}
	resp.Logs = e.logs.lines(300)
	resp.LogLevel = e.logs.Level()
	return resp
}

// ServeLocal 在 127.0.0.1 的随机端口上提供 WebUI，返回地址；Stop 时关闭
func (e *Engine) ServeLocal() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	srv := &http.Server{Handler: e.Handler(), ReadHeaderTimeout: 10 * time.Second}
	e.mu.Lock()
	e.srv = srv
	e.mu.Unlock()
	go srv.Serve(ln)
	return "http://" + ln.Addr().String(), nil
}

// ListenAndServe 在 addr 上提供 WebUI，直到出错
func (e *Engine) ListenAndServe(addr string) error {
	if len(e.cfg.Password) < 8 {
		e.Warnf("WebUI 密码少于 8 位，建议换一个更长的密码")
	}
	srv := &http.Server{Addr: addr, Handler: e.Handler(), ReadHeaderTimeout: 10 * time.Second}
	e.Infof("WebUI 监听 %s", addr)
	return srv.ListenAndServe()
}

// RandomPassword 生成本机模式用的面板密码
func RandomPassword() string { return randomHex(24) }

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func jitter(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	return time.Duration(mrand.Int63n(int64(max)))
}

// Jitter 返回 0~max 之间的随机时长
func Jitter(max time.Duration) time.Duration { return jitter(max) }

// RunOnce 依次为每个账号跑一轮，不排程，给命令行 refresh 和 cron 用；有账号失败时返回错误
func (e *Engine) RunOnce() error {
	e.migrate()
	mids, err := e.listAccounts()
	if err != nil {
		return err
	}
	if len(mids) == 0 {
		return errNoCookies
	}
	s := e.Settings()
	failed := 0
	for _, mid := range mids {
		acc := e.newAccount(mid)
		acc.run(s)
		if last := acc.status().Last; last == nil || !last.OK {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d 个账号失败", failed)
	}
	return nil
}

// Password 是 WebUI 的面板密码；本机模式下客户端用它连自己开的 WebUI
func (e *Engine) Password() string { return e.cfg.Password }
