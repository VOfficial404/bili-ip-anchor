// Package mobile 是手机 App 的 Go 核心，用 gomobile bind 编译成 Android 的 aar 和 iOS 的 xcframework。
//
// 界面是原生的（Android 用 Jetpack Compose，iOS 用 SwiftUI），这里只提供数据：
//   - 两种用法：在手机上运行定时任务（StartLocal，core.Engine 跑在 App 里），或者连接服务器（ConnectServer）；
//   - Call 把面板接口（/api/...，和网页面板、桌面客户端用的是同一套）发给手机上的定时任务或者服务器，
//     登录会话由这里管，界面不用处理 Cookie；
//   - Image 下载 B 站图床的头像和封面。
//
// 只能用 gomobile 支持的类型：string、bool、int、[]byte、error、导出的结构体和接口。
// 函数都可以在后台线程上同时调用；会访问网络的不要在主线程上调用。
package mobile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // 给 SetTimeZone 用

	"github.com/VOfficial404/bili-ip-anchor/core"
)

// version 发布时用 -ldflags 写入
var version = "dev"

// transport 不为 nil 时代替网络，给测试用
var transport http.RoundTripper

// Listener 由 App 实现。本机运行开始、停止时在后台线程上调用，Android 用它开关前台服务。
type Listener interface {
	LocalChanged(running bool)
}

const (
	prefsFile    = "mobile-prefs.json" // 选的用法和服务器地址
	passwordFile = "mobile-password"   // 记住的服务器面板密码
	localFile    = "mobile-local"      // 本机运行时存在，内容是代理；用户停止时删掉
	localBase    = "http://127.0.0.1"  // 本机运行时 Call 用的地址，请求不出 App
)

// 用法
const (
	ModeNone   = ""       // 还没选：显示首页
	ModeLocal  = "local"  // 在手机上运行
	ModeRemote = "remote" // 连接服务器
)

var (
	mu       sync.Mutex
	cur      *app
	listener Listener
)

func Version() string { return version }

// SetTimeZone 设置日志用的时区，传系统的时区名，例如 Asia/Shanghai。手机上 Go 自己取不到本地时区。
func SetTimeZone(name string) {
	if loc, err := time.LoadLocation(name); err == nil {
		time.Local = loc
	}
}

func SetListener(l Listener) {
	mu.Lock()
	listener = l
	mu.Unlock()
}

func notify(running bool) {
	mu.Lock()
	l := listener
	mu.Unlock()
	if l != nil {
		l.LocalChanged(running)
	}
}

// Open 读出 dataDir 里记住的用法，之后才能调用其他函数。已经打开过时什么也不做。
// dataDir 放账号、设置和连接信息。
func Open(dataDir string) error {
	mu.Lock()
	defer mu.Unlock()
	if cur != nil {
		return nil
	}
	if dataDir == "" {
		return errors.New("没有数据目录")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	a := &app{dataDir: dataDir}
	b, _ := os.ReadFile(filepath.Join(dataDir, prefsFile))
	json.Unmarshal(b, &a.prefs)
	if a.prefs.Mode == ModeRemote {
		if u, err := parseServer(a.prefs.Server); err == nil {
			a.remote = newRemote(u)
		} else {
			a.prefs.Mode = ModeNone
		}
	}
	cur = a
	return nil
}

func current() (*app, error) {
	mu.Lock()
	defer mu.Unlock()
	if cur == nil {
		return nil, errors.New("还没有调用 Open")
	}
	return cur, nil
}

// State 返回现在的状态（JSON）：
//
//	{"mode":"local|remote|", "server":"服务器地址", "proxy":"本机运行的代理",
//	 "remember":是否记住了服务器密码, "local_running":定时任务是否在手机上运行, "version":"v0.8.0"}
func State() string {
	st := map[string]any{"mode": ModeNone, "server": "", "proxy": "", "remember": false, "local_running": false, "version": version}
	if a, err := current(); err == nil {
		a.mu.Lock()
		st["mode"], st["server"], st["proxy"] = a.prefs.Mode, a.prefs.Server, a.prefs.Proxy
		st["local_running"] = a.engine != nil
		a.mu.Unlock()
		_, err := os.Stat(filepath.Join(a.dataDir, passwordFile))
		st["remember"] = err == nil
	}
	b, _ := json.Marshal(st)
	return string(b)
}

// LocalRunning 表示定时任务正在手机上运行
func LocalRunning() bool {
	a, err := current()
	return err == nil && a.running()
}

// ProbeProxy 经过代理查询 B 站看到的出口 IP，返回 JSON：{"addr","country","province","city","isp"}
func ProbeProxy(proxy string) (string, error) {
	z, err := core.ProbeProxy(strings.TrimSpace(proxy), transport)
	if err != nil {
		return "", err
	}
	b, _ := json.Marshal(z)
	return string(b), nil
}

// StartLocal 确认代理可用后在手机上开始运行定时任务，记住这个用法；已经在运行时只更新代理。
// 返回 JSON：{"zone":{出口 IP 和属地}, "warning":"代理可能没有生效时的提示"}
func StartLocal(proxy string) (string, error) {
	a, err := current()
	if err != nil {
		return "", err
	}
	proxy = strings.TrimSpace(proxy)
	if proxy == "" {
		return "", errors.New("请填写代理的地址")
	}
	if _, err := core.ParseProxy(proxy); err != nil {
		return "", err
	}
	via, err := core.ProbeProxy(proxy, transport)
	if err != nil {
		return "", err
	}
	warning := ""
	if transport == nil {
		// 和直连比一下：一样说明代理没有接管这些请求
		if direct, derr := core.ProbeProxy("", nil); derr == nil && direct.Addr == via.Addr {
			warning = fmt.Sprintf("注意：经过代理和直连时 B 站看到的都是 %s，代理可能没有生效", via.Addr)
		}
	}
	if err := a.startLocal(proxy); err != nil {
		return "", err
	}
	b, _ := json.Marshal(map[string]any{"zone": via, "warning": warning})
	return string(b), nil
}

// ResumeLocal 在 Open 之后调用：上次本机运行没有被用户停止（App 被系统杀掉、手机重启），
// 就用同一个代理接着运行，不再检查代理。没有要接着运行的时什么也不做。
func ResumeLocal() error {
	a, err := current()
	if err != nil {
		return err
	}
	proxy, err := os.ReadFile(filepath.Join(a.dataDir, localFile))
	if errors.Is(err, os.ErrNotExist) || strings.TrimSpace(string(proxy)) == "" {
		return nil
	}
	if err != nil {
		return err
	}
	return a.startLocal(strings.TrimSpace(string(proxy)))
}

// StopLocal 停下手机上的定时任务（例如通知里的「停止」），账号、设置和选的用法都保留
func StopLocal() {
	if a, err := current(); err == nil {
		a.stopLocal(true)
	}
}

// ConnectServer 用面板密码登录服务器，记住这个用法。remember 时记住密码，会话过期后自动重新登录。
func ConnectServer(server, password string, remember bool) error {
	a, err := current()
	if err != nil {
		return err
	}
	u, err := parseServer(server)
	if err != nil {
		return err
	}
	rc := newRemote(u)
	resp, err := rc.login(password)
	if err != nil {
		return err
	}
	if !resp.OK() {
		return errors.New(resp.Message())
	}
	pw := filepath.Join(a.dataDir, passwordFile)
	if remember {
		err = writeFile(pw, []byte(password))
	} else if err = os.Remove(pw); errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	if err != nil {
		return err
	}
	a.stopLocal(true)
	a.mu.Lock()
	a.remote = rc
	a.prefs.Mode, a.prefs.Server = ModeRemote, u.String()
	err = a.savePrefs()
	a.mu.Unlock()
	return err
}

// Disconnect 回到首页：停下本机运行，或者退出服务器的登录并忘掉密码。账号和设置都保留。
func Disconnect() {
	a, err := current()
	if err != nil {
		return
	}
	a.stopLocal(true)
	a.mu.Lock()
	rc := a.remote
	a.remote = nil
	a.prefs.Mode = ModeNone
	a.savePrefs()
	a.mu.Unlock()
	os.Remove(filepath.Join(a.dataDir, passwordFile))
	if rc != nil {
		rc.do("DELETE", "/api/session", nil)
	}
}

// Shutdown 停下定时任务。不算用户停止：下次 ResumeLocal 还会接着运行。
func Shutdown() {
	mu.Lock()
	a := cur
	cur = nil
	mu.Unlock()
	if a != nil {
		a.stopLocal(false)
	}
}

// Response 是 Call 的结果
type Response struct {
	Status      int    // HTTP 状态码；401 表示要重新输入服务器密码
	Body        []byte // 一般是 JSON；二维码是 PNG，动态图片是图片
	ContentType string
	Filename    string // 下载的文件名（导出收藏夹时有）
}

// OK 表示状态码是 2xx
func (r *Response) OK() bool { return r.Status >= 200 && r.Status < 300 }

// Message 是出错时界面上显示的说明（接口返回的 {"error": "..."}）
func (r *Response) Message() string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(r.Body, &e) == nil && e.Error != "" {
		return e.Error
	}
	if r.OK() {
		return ""
	}
	return fmt.Sprintf("HTTP %d", r.Status)
}

// Call 发一个面板接口请求，例如 Call("GET", "/api/status", nil)、Call("PUT", "/api/settings", json)。
// 本机运行时交给手机上的定时任务，连接服务器时发给服务器。body 是 JSON 或者原始数据（上传图片、导入收藏）。
// 只有连不上时才返回 error；接口的错误在 Response 的状态码和 Message 里。
func Call(method, path string, body []byte) (*Response, error) {
	a, err := current()
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(path, "/api/") {
		return nil, errors.New("只能调用 /api/ 下的接口")
	}
	a.mu.Lock()
	mode, rc, lc := a.prefs.Mode, a.remote, a.local
	a.mu.Unlock()
	switch {
	case mode == ModeRemote && rc != nil:
		resp, err := rc.do(method, path, body)
		if err == nil && resp.Status == http.StatusUnauthorized {
			// 会话过期（或者服务器重启了）：用记住的密码重新登录一次
			if pw, rerr := os.ReadFile(filepath.Join(a.dataDir, passwordFile)); rerr == nil {
				if lr, lerr := rc.login(string(pw)); lerr == nil && lr.OK() {
					return rc.do(method, path, body)
				}
			}
		}
		return resp, err
	case mode == ModeLocal && lc != nil:
		return lc.do(method, path, body)
	case mode == ModeLocal:
		return &Response{Status: http.StatusConflict, ContentType: "application/json",
			Body: []byte(`{"error":"本机运行已经停止"}`)}, nil
	}
	return &Response{Status: http.StatusConflict, ContentType: "application/json",
		Body: []byte(`{"error":"还没有选择在手机上运行或者连接服务器"}`)}, nil
}

// Image 下载 B 站图床（*.hdslb.com）的图片，例如头像和视频封面
func Image(u string) ([]byte, error) {
	p, err := url.Parse(u)
	if err != nil || (p.Scheme != "https" && p.Scheme != "http") || !strings.HasSuffix(p.Hostname(), ".hdslb.com") {
		return nil, errors.New("不是 B 站图床的图片")
	}
	p.Scheme = "https"
	resp, err := (&http.Client{Timeout: 15 * time.Second, Transport: transport}).Get(p.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

type prefs struct {
	Mode   string `json:"mode"`
	Server string `json:"server,omitempty"`
	Proxy  string `json:"proxy,omitempty"`
}

type app struct {
	dataDir string

	mu     sync.Mutex
	prefs  prefs
	engine *core.Engine // 本机运行时不为 nil
	local  *client      // engine 的接口
	remote *client      // 连接的服务器
}

// savePrefs 要在 a.mu 里调用
func (a *app) savePrefs() error {
	b, _ := json.Marshal(a.prefs)
	return writeFile(filepath.Join(a.dataDir, prefsFile), b)
}

func (a *app) running() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.engine != nil
}

// startLocal 开始运行定时任务；已经在运行时只更新代理
func (a *app) startLocal(proxy string) error {
	a.mu.Lock()
	e, started := a.engine, false
	if e == nil {
		e = core.New(core.Config{
			DataDir:   a.dataDir,
			Version:   version,
			Password:  core.RandomPassword(),
			Local:     true,
			Defaults:  core.DefaultSettings(),
			LogOutput: os.Stderr,
			Transport: transport,
		})
	}
	if s := e.Settings(); s.Proxy != proxy {
		s.Proxy = proxy
		if err := e.UpdateSettings(s); err != nil {
			a.mu.Unlock()
			return err
		}
	}
	if a.engine == nil {
		e.Start()
		a.engine, started = e, true
		a.local = newLocal(e)
	}
	a.remote = nil
	a.prefs.Mode, a.prefs.Proxy = ModeLocal, proxy
	a.savePrefs()
	a.mu.Unlock()
	writeFile(filepath.Join(a.dataDir, localFile), []byte(proxy))
	if started {
		notify(true)
	}
	return nil
}

// stopLocal 停下定时任务；byUser 时下次不再自动接着运行
func (a *app) stopLocal(byUser bool) {
	a.mu.Lock()
	e := a.engine
	a.engine, a.local = nil, nil
	a.mu.Unlock()
	if byUser {
		os.Remove(filepath.Join(a.dataDir, localFile))
	}
	if e != nil {
		e.Stop()
		notify(false)
	}
}

// client 发面板接口请求，自己保存会话 Cookie
type client struct {
	base     *url.URL
	http     *http.Client
	password func() string // 本机运行时自动登录用；服务器为 nil
}

func newRemote(u *url.URL) *client {
	jar, _ := cookiejar.New(nil)
	// 导出收藏夹要一页页读完，可能要一两分钟
	return &client{base: u, http: &http.Client{Jar: jar, Timeout: 5 * time.Minute, Transport: transport}}
}

func newLocal(e *core.Engine) *client {
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse(localBase)
	return &client{base: u, http: &http.Client{Jar: jar, Transport: handlerTransport{e.Handler()}}, password: e.Password}
}

func (c *client) login(password string) (*Response, error) {
	b, _ := json.Marshal(map[string]string{"password": password})
	return c.send("POST", "/api/session", b)
}

func (c *client) do(method, path string, body []byte) (*Response, error) {
	resp, err := c.send(method, path, body)
	if err == nil && resp.Status == http.StatusUnauthorized && c.password != nil {
		if lr, lerr := c.login(c.password()); lerr == nil && lr.OK() {
			return c.send(method, path, body)
		}
	}
	return resp, err
}

func (c *client) send(method, path string, body []byte) (*Response, error) {
	u := *c.base
	ref, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + ref.Path
	u.RawQuery = ref.RawQuery
	var r io.Reader = http.NoBody
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, u.String(), r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Requested-With", "bia")
	if body != nil && json.Valid(body) {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if c.password == nil {
			return nil, fmt.Errorf("连不上 %s：%w", c.base.Host, unwrapURLErr(err))
		}
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	out := &Response{Status: resp.StatusCode, Body: data, ContentType: resp.Header.Get("Content-Type")}
	if _, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition")); err == nil {
		out.Filename = filepath.Base(params["filename"])
	}
	return out, nil
}

func unwrapURLErr(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// handlerTransport 把请求直接交给手机上的定时任务的接口，不经过网络
type handlerTransport struct{ h http.Handler }

func (t handlerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	r.RemoteAddr = "127.0.0.1:1"
	t.h.ServeHTTP(rec, r)
	resp := rec.Result()
	resp.Request = r
	return resp, nil
}

func parseServer(s string) (*url.URL, error) {
	s = strings.TrimSpace(s)
	if s != "" && !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("服务器地址不对：" + s)
	}
	u.RawQuery, u.Fragment = "", ""
	u.Path = strings.TrimSuffix(u.Path, "/")
	return u, nil
}

// writeFile 先写临时文件再改名，写到一半时 App 被杀掉也不会留下坏文件
func writeFile(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
