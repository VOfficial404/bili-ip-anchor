package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

// 每个账号一个目录：accounts/<UID>/cookies.json 和 profile.json

var midRe = regexp.MustCompile(`^[0-9]{1,20}$`)

func accountsDir() string                  { return filepath.Join(cfg.DataDir, "accounts") }
func accountDir(mid string) string         { return filepath.Join(accountsDir(), mid) }
func accountCookieFile(mid string) string  { return filepath.Join(accountDir(mid), "cookies.json") }
func accountProfileFile(mid string) string { return filepath.Join(accountDir(mid), "profile.json") }

func hasCookies(mid string) bool {
	_, err := os.Stat(accountCookieFile(mid))
	return err == nil
}

// saveAccountCookies 按 Cookie 里的 DedeUserID（即 UID）保存会话，返回 UID
func saveAccountCookies(cookies map[string]string) (string, error) {
	mid := cookies["DedeUserID"]
	if cookies["SESSDATA"] == "" || cookies["bili_jct"] == "" || !midRe.MatchString(mid) {
		return "", errors.New("会话里缺少 SESSDATA/bili_jct/DedeUserID，B 站接口可能已变化")
	}
	b, err := json.Marshal(cookies)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(accountDir(mid), 0700); err != nil {
		return "", err
	}
	return mid, os.WriteFile(accountCookieFile(mid), b, 0600)
}

func loadAccountCookies(mid string) (map[string]string, error) {
	b, err := os.ReadFile(accountCookieFile(mid))
	if errors.Is(err, os.ErrNotExist) {
		return nil, errNoCookies
	}
	if err != nil {
		return nil, err
	}
	var cookies map[string]string
	if err := json.Unmarshal(b, &cookies); err != nil {
		return nil, fmt.Errorf("解析 %s 失败：%w", accountCookieFile(mid), err)
	}
	return cookies, nil
}

// listAccounts 返回有会话文件的账号 UID，按目录名排序
func listAccounts() ([]string, error) {
	entries, err := os.ReadDir(accountsDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var mids []string
	for _, e := range entries {
		if e.IsDir() && midRe.MatchString(e.Name()) && hasCookies(e.Name()) {
			mids = append(mids, e.Name())
		}
	}
	return mids, nil
}

func loadProfile(mid string) profile {
	var p profile
	if b, err := os.ReadFile(accountProfileFile(mid)); err == nil {
		json.Unmarshal(b, &p)
	}
	p.Mid = mid
	return p
}

func saveProfile(p profile) error {
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(accountProfileFile(p.Mid), b, 0600)
}

// migrateLegacyCookies 把 v0.3 及更早版本的 data/cookies.json 搬进 accounts/<UID>/
func migrateLegacyCookies() {
	legacy := filepath.Join(cfg.DataDir, "cookies.json")
	b, err := os.ReadFile(legacy)
	if err != nil {
		return
	}
	var cookies map[string]string
	if err := json.Unmarshal(b, &cookies); err != nil {
		log.Printf("旧的 %s 无法解析，没有迁移：%v", legacy, err)
		return
	}
	mid, err := saveAccountCookies(cookies)
	if err != nil {
		log.Printf("旧的 %s 没有迁移：%v", legacy, err)
		return
	}
	if err := os.Remove(legacy); err != nil {
		log.Printf("删除旧的 %s 失败：%v", legacy, err)
	}
	log.Printf("已把旧的 cookies.json 迁移为账号 UID %s", mid)
}

// account 是一个正在排程的账号，每个账号有自己的循环和下次运行时间
type account struct {
	mid string

	mu       sync.Mutex
	prof     profile
	loggedIn bool
	running  bool
	last     *runResult
	next     time.Time

	trigger chan struct{}
	resched chan struct{}
	stop    chan struct{}
}

type accountStatus struct {
	Mid      string     `json:"mid"`
	Uname    string     `json:"uname"`
	Face     string     `json:"face"`
	LoggedIn bool       `json:"logged_in"`
	Running  bool       `json:"running"`
	Last     *runResult `json:"last"`
	Next     *time.Time `json:"next"`
}

func newAccount(mid string) *account {
	return &account{
		mid:      mid,
		prof:     loadProfile(mid),
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

// logf 给日志加上账号名前缀；不能在持有 acc.mu 时调用
func (acc *account) logf(format string, args ...any) {
	log.Printf("[%s] "+format, append([]any{acc.name()}, args...)...)
}

func (acc *account) run(s settings) {
	acc.mu.Lock()
	acc.running = true
	acc.mu.Unlock()

	cookies, err := loadAccountCookies(acc.mid)
	var p profile
	if err == nil {
		p, err = refresh(cookies, s, acc.logf)
	}
	if p.Uname != "" {
		p.Mid = acc.mid
		if e := saveProfile(p); e != nil {
			log.Printf("保存账号 %s 的资料失败：%v", acc.mid, e)
		}
	}

	acc.mu.Lock()
	acc.running = false
	acc.last = &runResult{At: time.Now(), OK: err == nil}
	switch {
	case errors.Is(err, errNoCookies), errors.Is(err, errLoggedOut):
		acc.loggedIn = false
	case p.Uname != "":
		acc.loggedIn, acc.prof = true, p
	}
	if err != nil {
		acc.last.Error = err.Error()
	}
	acc.mu.Unlock()

	if err != nil {
		acc.logf("本轮失败：%v", err)
	}
}

func (acc *account) scheduleFrom(base time.Time, s settings) {
	next := base.Add(time.Duration(s.Interval) + jitter(time.Duration(s.Jitter)))
	acc.mu.Lock()
	acc.next = next
	acc.mu.Unlock()
	acc.logf("下一轮：%s", next.Format("2006-01-02 15:04:05"))
}

// loop 先等 first 再跑第一轮，之后按设置定时跑；失败只记日志，直到 stop 被关闭
func (acc *account) loop(settings func() settings, first time.Duration) {
	acc.mu.Lock()
	acc.next = time.Now().Add(first)
	acc.mu.Unlock()
	for acc.wait(settings) {
		acc.run(settings())
		acc.scheduleFrom(time.Now(), settings())
	}
}

// wait 等到下次运行时间或被手动触发；账号被移除时返回 false
func (acc *account) wait(settings func() settings) bool {
	for {
		acc.mu.Lock()
		t := time.NewTimer(time.Until(acc.next))
		acc.mu.Unlock()
		select {
		case <-t.C:
			return true
		case <-acc.trigger:
			t.Stop()
			acc.logf("手动触发运行")
			return true
		case <-acc.resched:
			t.Stop()
			acc.mu.Lock()
			last := acc.last
			acc.mu.Unlock()
			if last != nil { // 还没跑过第一轮的账号保持原来的启动时间
				acc.scheduleFrom(last.At, settings())
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
	acc.mu.Lock()
	defer acc.mu.Unlock()
	st := accountStatus{
		Mid:      acc.mid,
		Uname:    acc.prof.Uname,
		Face:     acc.prof.Face,
		LoggedIn: acc.loggedIn,
		Running:  acc.running,
		Last:     acc.last,
	}
	if !acc.running && !acc.next.IsZero() {
		next := acc.next
		st.Next = &next
	}
	return st
}
