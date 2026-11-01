package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
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

type app struct {
	mu       sync.Mutex
	set      settings
	running  bool
	last     *runResult
	next     time.Time
	loggedIn bool
	uname    string
	zone     *zoneInfo
	zoneErr  string
	qr       *qrLogin
	sessions map[string]time.Time

	logs    *ringLog
	trigger chan struct{}
	resched chan struct{}
}

func newApp(logs *ringLog) *app {
	_, err := loadCookies()
	return &app{
		set:      loadSettings(),
		loggedIn: err == nil, // 第一轮跑完前的猜测
		sessions: map[string]time.Time{},
		logs:     logs,
		trigger:  make(chan struct{}, 1),
		resched:  make(chan struct{}, 1),
	}
}

func (a *app) settings() settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.set
}

func (a *app) runOnce() {
	a.mu.Lock()
	a.running = true
	s := a.set
	a.mu.Unlock()

	uname, err := refresh(s)
	zone, zoneErr := fetchZone()

	a.mu.Lock()
	defer a.mu.Unlock()
	a.running = false
	a.last = &runResult{At: time.Now(), OK: err == nil}
	switch {
	case errors.Is(err, errNoCookies), errors.Is(err, errLoggedOut):
		a.loggedIn, a.uname = false, ""
	case uname != "":
		a.loggedIn, a.uname = true, uname
	}
	if err != nil {
		a.last.Error = err.Error()
		log.Printf("本轮失败：%v", err)
	}
	if zoneErr != nil {
		a.zoneErr = zoneErr.Error()
		log.Printf("查询出口 IP 归属失败：%v", zoneErr)
	} else {
		a.zone, a.zoneErr = &zone, ""
	}
}

func (a *app) scheduleFrom(base time.Time) {
	a.mu.Lock()
	s := a.set
	a.next = base.Add(time.Duration(s.Interval) + jitter(time.Duration(s.Jitter)))
	next := a.next
	a.mu.Unlock()
	log.Printf("下一轮：%s", next.Format("2006-01-02 15:04:05"))
}

// loop 启动时先跑一轮，之后按 next 定时跑；失败只记日志不退出
func (a *app) loop() {
	for {
		a.runOnce()
		a.scheduleFrom(time.Now())
	wait:
		for {
			a.mu.Lock()
			t := time.NewTimer(time.Until(a.next))
			a.mu.Unlock()
			select {
			case <-t.C:
				break wait
			case <-a.trigger:
				t.Stop()
				log.Printf("手动触发运行")
				break wait
			case <-a.resched:
				t.Stop()
				a.mu.Lock()
				base := time.Now()
				if a.last != nil {
					base = a.last.At
				}
				a.mu.Unlock()
				a.scheduleFrom(base)
			}
		}
	}
}

// runNow 请求立即跑一轮；已经在跑或已排队时返回 false
func (a *app) runNow() bool {
	a.mu.Lock()
	running := a.running
	a.mu.Unlock()
	if running {
		return false
	}
	select {
	case a.trigger <- struct{}{}:
		return true
	default:
		return false
	}
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
	a.mu.Unlock()
	log.Printf("设置已更新：间隔 %s，随机抖动 %s，视频 %s，发动态 %v",
		s.Interval, s.Jitter, s.BVID, s.DoDynamic)
	select {
	case a.resched <- struct{}{}:
	default:
	}
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

func (a *app) pollQR() (qrState, error) {
	a.mu.Lock()
	q := a.qr
	a.mu.Unlock()
	if q == nil {
		return qrNone, nil
	}
	st, err := q.poll()
	if err != nil {
		return st, err
	}
	if st == qrSuccess || st == qrExpired {
		a.mu.Lock()
		if a.qr == q {
			a.qr = nil
		}
		a.mu.Unlock()
	}
	if st == qrSuccess {
		log.Printf("网页扫码登录成功")
		a.mu.Lock()
		a.loggedIn = true
		a.mu.Unlock()
		a.runNow()
	}
	return st, nil
}

func (a *app) logoutBili() error {
	err := logout()
	a.mu.Lock()
	a.loggedIn, a.uname = false, ""
	a.mu.Unlock()
	return err
}
