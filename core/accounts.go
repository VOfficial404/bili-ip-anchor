package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

// 每个账号一个目录：accounts/<UID>/ 下的
//   cookies.json  网页登录会话（Cookie）
//   session.json  refresh_token、Cookie 有效期、App 授权，和 cookies.json 一样要保密
//   profile.json  昵称、头像、硬币等公开资料和主页属地

var midRe = regexp.MustCompile(`^[0-9]{1,20}$`)

func (e *Engine) accountsDir() string          { return filepath.Join(e.cfg.DataDir, "accounts") }
func (e *Engine) accountDir(mid string) string { return filepath.Join(e.accountsDir(), mid) }
func (e *Engine) cookieFile(mid string) string {
	return filepath.Join(e.accountDir(mid), "cookies.json")
}
func (e *Engine) sessionFile(mid string) string {
	return filepath.Join(e.accountDir(mid), "session.json")
}
func (e *Engine) profileFile(mid string) string {
	return filepath.Join(e.accountDir(mid), "profile.json")
}
func (e *Engine) hasCookies(mid string) bool { _, err := os.Stat(e.cookieFile(mid)); return err == nil }
func removeAll(dir string) error             { return os.RemoveAll(dir) }
func writeJSONFile(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// saveAccountCookies 按 Cookie 里的 DedeUserID（即 UID）保存会话，返回 UID
func (e *Engine) saveAccountCookies(cookies map[string]string) (string, error) {
	mid := cookies["DedeUserID"]
	if cookies["SESSDATA"] == "" || cookies["bili_jct"] == "" || !midRe.MatchString(mid) {
		return "", errors.New("会话里缺少 SESSDATA/bili_jct/DedeUserID，B 站接口可能已变化")
	}
	return mid, writeJSONFile(e.cookieFile(mid), cookies)
}

func (e *Engine) loadAccountCookies(mid string) (map[string]string, error) {
	b, err := os.ReadFile(e.cookieFile(mid))
	if errors.Is(err, os.ErrNotExist) {
		return nil, errNoCookies
	}
	if err != nil {
		return nil, err
	}
	var cookies map[string]string
	if err := json.Unmarshal(b, &cookies); err != nil {
		return nil, fmt.Errorf("解析 %s 失败：%w", e.cookieFile(mid), err)
	}
	return cookies, nil
}

// listAccounts 返回有会话文件的账号 UID，按目录名排序
func (e *Engine) listAccounts() ([]string, error) {
	entries, err := os.ReadDir(e.accountsDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var mids []string
	for _, ent := range entries {
		if ent.IsDir() && midRe.MatchString(ent.Name()) && e.hasCookies(ent.Name()) {
			mids = append(mids, ent.Name())
		}
	}
	return mids, nil
}

// session 是账号的登录凭据以外的秘密信息
type session struct {
	RefreshToken  string    `json:"refresh_token,omitempty"`
	CookieExpires time.Time `json:"cookie_expires,omitempty"`
	AccessKey     string    `json:"access_key,omitempty"` // App 授权，只用来查询主页属地
	AccessExpires time.Time `json:"access_expires,omitempty"`
}

func (e *Engine) loadSession(mid string) session {
	var s session
	if b, err := os.ReadFile(e.sessionFile(mid)); err == nil {
		json.Unmarshal(b, &s)
	}
	return s
}

func (e *Engine) saveSession(mid string, s session) error {
	return writeJSONFile(e.sessionFile(mid), s)
}

// updateSession 读出、修改、写回 session.json
func (e *Engine) updateSession(mid string, fn func(s *session)) error {
	s := e.loadSession(mid)
	fn(&s)
	return e.saveSession(mid, s)
}

// Stats 来自 nav 接口，只用来显示
type Stats struct {
	Coins     float64   `json:"coins"`
	BCoins    float64   `json:"bcoins"`
	Level     int       `json:"level"`
	VIP       string    `json:"vip,omitempty"` // 大会员类型，空表示不是
	UpdatedAt time.Time `json:"updated_at"`
}

// HomeLocation 是 App 接口返回的个人主页 IP 属地
type HomeLocation struct {
	Text         string     `json:"text"` // 例如「广东」
	CheckedAt    time.Time  `json:"checked_at"`
	MatchedSince *time.Time `json:"matched_since,omitempty"` // 第一次看到它和服务器出口 IP 一致的时间
	Error        string     `json:"error,omitempty"`
}

// profile 是账号的公开资料，只用来在面板和日志里显示
type profile struct {
	Mid    string        `json:"mid"`
	Uname  string        `json:"uname"`
	Face   string        `json:"face"`
	Stats  *Stats        `json:"stats,omitempty"`
	Home   *HomeLocation `json:"home,omitempty"`
	LastOK *time.Time    `json:"last_ok,omitempty"` // 上次成功从出口 IP 发出请求的时间
}

func (e *Engine) loadProfile(mid string) profile {
	var p profile
	if b, err := os.ReadFile(e.profileFile(mid)); err == nil {
		json.Unmarshal(b, &p)
	}
	p.Mid = mid
	return p
}

func (e *Engine) saveProfile(p profile) error { return writeJSONFile(e.profileFile(p.Mid), p) }

// migrateLegacyCookies 把 v0.3 及更早版本的 data/cookies.json 搬进 accounts/<UID>/
func (e *Engine) migrateLegacyCookies() {
	legacy := filepath.Join(e.cfg.DataDir, "cookies.json")
	b, err := os.ReadFile(legacy)
	if err != nil {
		return
	}
	var cookies map[string]string
	if err := json.Unmarshal(b, &cookies); err != nil {
		e.Warnf("旧的 %s 无法解析，没有迁移：%v", legacy, err)
		return
	}
	mid, err := e.saveAccountCookies(cookies)
	if err != nil {
		e.Warnf("旧的 %s 没有迁移：%v", legacy, err)
		return
	}
	if err := os.Remove(legacy); err != nil {
		e.Warnf("删除旧的 %s 失败：%v", legacy, err)
	}
	e.Infof("已把旧的 cookies.json 迁移为账号 UID %s", mid)
}

// account 是一个正在排程的账号，每个账号有自己的循环和下次运行时间
type account struct {
	e   *Engine
	mid string

	mu       sync.Mutex
	prof     profile
	loggedIn bool
	running  bool
	busy     string // 正在做的单项操作，例如「检查」「刷新 Cookie」
	last     *runResult
	next     time.Time

	trigger chan struct{}
	resched chan struct{}
	stop    chan struct{}
}

type cookieStatus struct {
	Expires    *time.Time `json:"expires,omitempty"`
	CanRefresh bool       `json:"can_refresh"` // 有 refresh_token，可以刷新
}

type accountStatus struct {
	Mid      string        `json:"mid"`
	Uname    string        `json:"uname"`
	Face     string        `json:"face"`
	LoggedIn bool          `json:"logged_in"`
	Running  bool          `json:"running"`
	Busy     string        `json:"busy,omitempty"`
	Last     *runResult    `json:"last"`
	LastOK   *time.Time    `json:"last_ok"`
	Next     *time.Time    `json:"next"`
	Stats    *Stats        `json:"stats"`
	Cookie   cookieStatus  `json:"cookie"`
	AppAuth  bool          `json:"app_auth"`
	Home     *HomeLocation `json:"home"`
}

func (e *Engine) newAccount(mid string) *account {
	return &account{
		e:        e,
		mid:      mid,
		prof:     e.loadProfile(mid),
		loggedIn: true, // 第一轮跑完前的猜测
		trigger:  make(chan struct{}, 1),
		resched:  make(chan struct{}, 1),
		stop:     make(chan struct{}),
	}
}

func (acc *account) name() string {
	acc.mu.Lock()
	defer acc.mu.Unlock()
	if acc.prof.Uname != "" {
		return acc.prof.Uname
	}
	return "UID " + acc.mid
}

// updateProfile 修改资料并保存
func (acc *account) updateProfile(fn func(p *profile)) {
	acc.mu.Lock()
	fn(&acc.prof)
	p := acc.prof
	acc.mu.Unlock()
	if err := acc.e.saveProfile(p); err != nil {
		acc.e.Errorf("保存账号 %s 的资料失败：%v", acc.mid, err)
	}
}

// begin 标记一项操作开始；已经有操作在进行时返回 false
func (acc *account) begin(what string) bool {
	acc.mu.Lock()
	defer acc.mu.Unlock()
	if acc.running || acc.busy != "" {
		return false
	}
	if what == "" {
		acc.running = true
	} else {
		acc.busy = what
	}
	return true
}

func (acc *account) end() {
	acc.mu.Lock()
	acc.running, acc.busy = false, ""
	acc.mu.Unlock()
}

// markLogin 根据错误更新登录状态
func (acc *account) markLogin(err error) {
	acc.mu.Lock()
	defer acc.mu.Unlock()
	switch {
	case errors.Is(err, errNoCookies), errors.Is(err, errLoggedOut):
		acc.loggedIn = false
	case err == nil:
		acc.loggedIn = true
	}
}

// run 跑一轮定时任务
func (acc *account) run(s Settings) {
	for !acc.begin("") {
		// 手动操作（检查、刷新 Cookie）正在进行，等它结束
		select {
		case <-acc.stop:
			return
		case <-time.After(time.Second):
		}
	}
	err := acc.e.runAccount(acc, s)
	acc.end()

	acc.mu.Lock()
	acc.last = &runResult{At: time.Now(), OK: err == nil}
	if err != nil {
		acc.last.Error = err.Error()
	}
	acc.mu.Unlock()
	acc.markLogin(err)
	if err != nil {
		acc.errorf("本轮失败：%v", err)
	}
}

func (acc *account) scheduleFrom(base time.Time, s Settings) {
	next := base.Add(time.Duration(s.Interval) + jitter(time.Duration(s.Jitter)))
	acc.mu.Lock()
	acc.next = next
	acc.mu.Unlock()
	acc.infof("下一轮：%s", next.Format("2006-01-02 15:04:05"))
}

// loop 先等 first 再跑第一轮，之后按设置定时跑；失败只记日志，直到 stop 被关闭
func (acc *account) loop(first time.Duration) {
	acc.mu.Lock()
	acc.next = time.Now().Add(first)
	acc.mu.Unlock()
	for acc.wait() {
		acc.run(acc.e.Settings())
		acc.scheduleFrom(time.Now(), acc.e.Settings())
	}
}

// wait 等到下次运行时间或被手动触发；账号被移除时返回 false。
// 按墙上时间判断，最多一分钟看一次：电脑或手机休眠时单调时钟会停，只靠它会把这一轮推迟休眠那么久。
func (acc *account) wait() bool {
	for {
		acc.mu.Lock()
		d := time.Until(acc.next.Round(0)) // Round(0) 去掉单调时钟读数，按墙上时间算
		acc.mu.Unlock()
		if d <= 0 {
			return true
		}
		t := time.NewTimer(min(d, time.Minute))
		select {
		case <-t.C:
			continue
		case <-acc.trigger:
			t.Stop()
			acc.infof("手动触发运行")
			return true
		case <-acc.resched:
			t.Stop()
			acc.mu.Lock()
			last := acc.last
			acc.mu.Unlock()
			if last != nil { // 还没跑过第一轮的账号保持原来的启动时间
				acc.scheduleFrom(last.At, acc.e.Settings())
			}
		case <-acc.stop:
			t.Stop()
			return false
		}
	}
}

// runNow 请求立即跑一轮；已经在跑或已排队时返回 false
func (acc *account) runNow() bool {
	acc.mu.Lock()
	running := acc.running
	acc.mu.Unlock()
	if running {
		return false
	}
	select {
	case acc.trigger <- struct{}{}:
		return true
	default:
		return false
	}
}

func (acc *account) reschedule() {
	select {
	case acc.resched <- struct{}{}:
	default:
	}
}

func (acc *account) status() accountStatus {
	sess := acc.e.loadSession(acc.mid)
	acc.mu.Lock()
	defer acc.mu.Unlock()
	st := accountStatus{
		Mid:      acc.mid,
		Uname:    acc.prof.Uname,
		Face:     acc.prof.Face,
		LoggedIn: acc.loggedIn,
		Running:  acc.running,
		Busy:     acc.busy,
		Last:     acc.last,
		LastOK:   acc.prof.LastOK,
		Stats:    acc.prof.Stats,
		Home:     acc.prof.Home,
		AppAuth:  sess.AccessKey != "",
		Cookie:   cookieStatus{CanRefresh: sess.RefreshToken != ""},
	}
	if !sess.CookieExpires.IsZero() {
		exp := sess.CookieExpires
		st.Cookie.Expires = &exp
	}
	if !acc.running && !acc.next.IsZero() {
		next := acc.next
		st.Next = &next
	}
	return st
}
