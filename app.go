package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type duration time.Duration

// String 去掉多余的零：6h0m0s 写成 6h，1h30m0s 写成 1h30m
func (d duration) String() string {
	s := time.Duration(d).String()
	if strings.HasSuffix(s, "m0s") {
		s = s[:len(s)-2]
	}
	if strings.HasSuffix(s, "h0m") {
		s = s[:len(s)-2]
	}
	return s
}

func (d duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("时间格式不对：%q，应写成 90m、6h 这样", s)
	}
	*d = duration(v)
	return nil
}

type settings struct {
	BVID      string   `json:"bvid"`
	DoDynamic bool     `json:"do_dynamic"`
	Interval  duration `json:"interval"`
	Jitter    duration `json:"jitter"`
}

var bvidRe = regexp.MustCompile(`^BV[0-9A-Za-z]{10}$`)

func (s settings) validate() error {
	if !bvidRe.MatchString(s.BVID) {
		return fmt.Errorf("BV 号格式不对：%q", s.BVID)
	}
	if time.Duration(s.Interval) < 10*time.Minute {
		return errors.New("间隔不能小于 10 分钟")
	}
	if s.Jitter < 0 {
		return errors.New("随机抖动不能是负数")
	}
	return nil
}

// loadSettings 以环境变量为底，WebUI 保存过的 settings.json 优先
func loadSettings() settings {
	s := cfg.Defaults
	b, err := os.ReadFile(settingsFile())
	if errors.Is(err, os.ErrNotExist) {
		return s
	}
	if err == nil {
		err = json.Unmarshal(b, &s)
	}
	if err == nil {
		err = s.validate()
	}
	if err != nil {
		log.Printf("忽略 %s，改用环境变量：%v", settingsFile(), err)
		return cfg.Defaults
	}
	return s
}

func saveSettings(s settings) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return err
	}
	return os.WriteFile(settingsFile(), b, 0600)
}

// ringLog 保留最近若干行日志给 WebUI 看
type ringLog struct {
	mu    sync.Mutex
	lines []string
	max   int
}

func newRingLog(max int) *ringLog { return &ringLog{max: max} }

func (r *ringLog) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		r.lines = append(r.lines, line)
	}
	if over := len(r.lines) - r.max; over > 0 {
		r.lines = append([]string(nil), r.lines[over:]...)
	}
	return len(p), nil
}

func (r *ringLog) Lines() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.lines...)
}

type runResult struct {
	At    time.Time `json:"at"`
	OK    bool      `json:"ok"`
	Error string    `json:"error,omitempty"`
}

var errNoAccount = errors.New("找不到这个账号")

type app struct {
	mu       sync.Mutex
	set      settings
	accounts map[string]*account
	zone     *zoneInfo
	zoneErr  string
	qr       *qrLogin
	sessions map[string]time.Time

	logs *ringLog
}

func newApp(logs *ringLog) *app {
	return &app{
		set:      loadSettings(),
		accounts: map[string]*account{},
		sessions: map[string]time.Time{},
		logs:     logs,
	}
}

func (a *app) settings() settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.set
}

func (a *app) account(mid string) *account {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.accounts[mid]
}

func (a *app) accountCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.accounts)
}

func (a *app) startAccountLocked(mid string, first time.Duration) {
	acc := newAccount(mid)
	a.accounts[mid] = acc
	go acc.loop(a.settings, first)
}

// syncAccounts 让运行中的账号和 accounts/ 目录保持一致：
// 命令行 login 新加的账号会被接上，会话文件被删掉的账号会停下。
// stagger 为 true 时（启动时）各账号的第一轮错开十几秒。
func (a *app) syncAccounts(stagger bool) {
	mids, err := listAccounts()
	if err != nil {
		log.Printf("读取账号目录失败：%v", err)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	listed := map[string]bool{}
	for i, mid := range mids {
		listed[mid] = true
		if _, ok := a.accounts[mid]; ok || !hasCookies(mid) {
			continue
		}
		var first time.Duration
		if stagger && i > 0 {
			first = time.Duration(i)*15*time.Second + jitter(10*time.Second)
		}
		a.startAccountLocked(mid, first)
		if !stagger {
			log.Printf("发现新账号 UID %s", mid)
		}
	}
	for mid, acc := range a.accounts {
		if !listed[mid] && !hasCookies(mid) {
			close(acc.stop)
			delete(a.accounts, mid)
			log.Printf("账号 UID %s 的会话文件已不在，停止运行", mid)
		}
	}
}

func (a *app) syncLoop() {
	for {
		time.Sleep(time.Minute)
		a.syncAccounts(false)
	}
}

// zoneLoop 定时查询 B 站把服务器出口 IP 识别为哪里
func (a *app) zoneLoop() {
	for {
		zone, err := fetchZone()
		a.mu.Lock()
		if err != nil {
			a.zoneErr = err.Error()
		} else {
			a.zone, a.zoneErr = &zone, ""
		}
		a.mu.Unlock()
		if err != nil {
			log.Printf("查询出口 IP 归属失败：%v", err)
			time.Sleep(5 * time.Minute)
			continue
		}
		time.Sleep(30 * time.Minute)
	}
}

// accountLoggedIn 在扫码成功后调用：新账号开始排程，已有账号立即跑一轮
func (a *app) accountLoggedIn(mid string) {
	a.mu.Lock()
	acc, ok := a.accounts[mid]
	if !ok {
		a.startAccountLocked(mid, 0)
	}
	a.mu.Unlock()
	if ok {
		acc.mu.Lock()
		acc.loggedIn = true
		acc.mu.Unlock()
		acc.runNow()
	}
}

// removeAccount 让 B 站注销会话，删除本地目录并停止排程。远端注销失败也会删除本地目录。
func (a *app) removeAccount(mid string) error {
	acc := a.account(mid)
	if acc == nil {
		return errNoAccount
	}
	name := acc.name()
	if cookies, err := loadAccountCookies(mid); err == nil {
		if err := logout(cookies); err != nil {
			acc.logf("B 站注销会话失败（本地会话仍会删除）：%v", err)
		} else {
			acc.logf("已在 B 站注销这份会话")
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if err := os.RemoveAll(accountDir(mid)); err != nil {
		return err
	}
	if cur, ok := a.accounts[mid]; ok && cur == acc {
		close(acc.stop)
		delete(a.accounts, mid)
	}
	log.Printf("已移除账号 %s", name)
	return nil
}

// runAll 让所有空闲账号立即跑一轮，返回实际触发的数量
func (a *app) runAll() int {
	a.mu.Lock()
	accs := make([]*account, 0, len(a.accounts))
	for _, acc := range a.accounts {
		accs = append(accs, acc)
	}
	a.mu.Unlock()
	n := 0
	for _, acc := range accs {
		if acc.runNow() {
			n++
		}
	}
	return n
}

func (a *app) updateSettings(s settings) error {
	if err := s.validate(); err != nil {
		return err
	}
	if err := saveSettings(s); err != nil {
		return err
	}
	a.mu.Lock()
	a.set = s
	for _, acc := range a.accounts {
		acc.reschedule()
	}
	a.mu.Unlock()
	log.Printf("设置已更新：间隔 %s，随机抖动 %s，视频 %s，发动态 %v", s.Interval, s.Jitter, s.BVID, s.DoDynamic)
	return nil
}

func (a *app) startQR() ([]byte, error) {
	q, err := qrStart()
	if err != nil {
		return nil, err
	}
	png, err := q.png()
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.qr = q
	a.mu.Unlock()
	return png, nil
}

func (a *app) pollQR() (qrState, string, error) {
	a.mu.Lock()
	q := a.qr
	a.mu.Unlock()
	if q == nil {
		return qrNone, "", nil
	}
	st, mid, err := q.poll()
	if st == qrSuccess || st == qrExpired {
		a.mu.Lock()
		if a.qr == q {
			a.qr = nil
		}
		a.mu.Unlock()
	}
	if err != nil {
		return st, "", err
	}
	if st == qrSuccess {
		log.Printf("网页扫码登录成功：UID %s", mid)
		a.accountLoggedIn(mid)
	}
	return st, mid, nil
}

type statusResp struct {
	Version   string          `json:"version"`
	Accounts  []accountStatus `json:"accounts"`
	Zone      *zoneInfo       `json:"zone"`
	ZoneError string          `json:"zone_error,omitempty"`
	Settings  settings        `json:"settings"`
	Logs      []string        `json:"logs"`
}

func (a *app) status() statusResp {
	a.mu.Lock()
	resp := statusResp{Version: version, Zone: a.zone, ZoneError: a.zoneErr, Settings: a.set}
	accs := make([]*account, 0, len(a.accounts))
	for _, acc := range a.accounts {
		accs = append(accs, acc)
	}
	a.mu.Unlock()

	// UID 按数值排序
	sort.Slice(accs, func(i, j int) bool {
		x, y := accs[i].mid, accs[j].mid
		if len(x) != len(y) {
			return len(x) < len(y)
		}
		return x < y
	})
	resp.Accounts = make([]accountStatus, 0, len(accs))
	for _, acc := range accs {
		resp.Accounts = append(resp.Accounts, acc.status())
	}
	resp.Logs = a.logs.Lines()
	return resp
}
