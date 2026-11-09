// Package mobile 是手机 App 的 Go 核心，用 gomobile bind 编译成 Android 的 aar 和 iOS 的 xcframework。
//
// 界面和桌面客户端是同一份 MyGo 代码，编译成 WebAssembly（desktop/build/web），App 在 WebView 里打开。
// 这里在 127.0.0.1 上提供这些页面，以及和界面同源的接口：
//   - /host/：界面在浏览器里做不到的事（记住连接信息、在手机上运行定时任务、下载图片），见 desktop/host_web.go；
//   - /api/：本机运行时交给手机上的定时任务（core.Engine），连接服务器时转发给服务器。
//
// 手机上的其他 App 也能连 127.0.0.1，所以 /host/ 和 /api/ 只给打开过一次性入口地址（EnterURL）的 WebView 用。
// 只能用 gomobile 支持的类型：string、bool、error 和接口。
package mobile

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
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

// listenAddr 只在测试里改
var listenAddr = "127.0.0.1:0"

//go:embed touch.js
var touchJS string

// Listener 由 App 实现。本机运行开始、停止时在后台线程上调用，Android 用它开关前台服务。
type Listener interface {
	LocalChanged(running bool)
}

const (
	hostCookie   = "bia_host"
	tokenTTL     = 2 * time.Minute
	prefsFile    = "mobile-prefs.json" // 界面的连接信息（desktop/store.go 的 prefs）
	passwordFile = "mobile-password"   // 记住的服务器面板密码
	localFile    = "mobile-local"      // 本机运行时存在，内容是代理；用户停止时删掉
)

var (
	mu       sync.Mutex
	cur      *host
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

// Start 开始提供界面并返回 WebView 要打开的入口地址。webDir 是界面文件（desktop/build/web 的内容），
// dataDir 放账号、设置和记住的连接信息。已经开始过时返回一个新的入口地址。
func Start(webDir, dataDir string) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if cur == nil {
		h, err := newHost(webDir, dataDir)
		if err != nil {
			return "", err
		}
		cur = h
	}
	return cur.enterURL(), nil
}

// EnterURL 返回一个新的入口地址：只能打开一次，两分钟内有效。WebView 重新创建时用。
func EnterURL() (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if cur == nil {
		return "", errors.New("还没有开始")
	}
	return cur.enterURL(), nil
}

// LocalRunning 表示定时任务正在手机上运行
func LocalRunning() bool {
	mu.Lock()
	h := cur
	mu.Unlock()
	return h != nil && h.running()
}

// StopLocal 停下手机上的定时任务（例如通知里的「停止」），账号和设置都保留
func StopLocal() {
	mu.Lock()
	h := cur
	mu.Unlock()
	if h != nil {
		h.stopLocal(true)
	}
}

// ResumeLocal 在 Start 之后调用：上次本机运行没有被用户停止（App 被系统杀掉、手机重启），
// 就用同一个代理接着运行，不再检查代理。没有要接着运行的时什么也不做。
func ResumeLocal() error {
	mu.Lock()
	h := cur
	mu.Unlock()
	if h == nil {
		return errors.New("还没有开始")
	}
	proxy, err := os.ReadFile(filepath.Join(h.dataDir, localFile))
	if errors.Is(err, os.ErrNotExist) || strings.TrimSpace(string(proxy)) == "" {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = h.startLocal(strings.TrimSpace(string(proxy)))
	return err
}

// Shutdown 停下定时任务并关闭界面的服务。不算用户停止：下次 ResumeLocal 还会接着运行。
func Shutdown() {
	mu.Lock()
	h := cur
	cur = nil
	mu.Unlock()
	if h != nil {
		h.stopLocal(false)
		h.srv.Close()
	}
}

type host struct {
	webDir, dataDir string
	addr            string // 127.0.0.1:端口
	session         string // bia_host Cookie 的值
	index           []byte // 插入了触摸滚动脚本的 index.html
	srv             *http.Server

	mu     sync.Mutex
	tokens map[string]time.Time
	local  bool         // /api/ 交给 engine；否则转发给 remote
	engine *core.Engine // 本机运行时不为 nil
	api    http.Handler // engine 的接口
	remote http.Handler // 连接的服务器
}

func newHost(webDir, dataDir string) (*host, error) {
	page, err := os.ReadFile(filepath.Join(webDir, "index.html"))
	if err != nil {
		return nil, fmt.Errorf("没有界面文件：%w", err)
	}
	if dataDir == "" {
		return nil, errors.New("没有数据目录")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, err
	}
	h := &host{
		webDir:  webDir,
		dataDir: dataDir,
		addr:    ln.Addr().String(),
		session: randomHex(32),
		index:   renderIndex(page),
		tokens:  map[string]time.Time{},
	}
	h.srv = &http.Server{Handler: h.handler(), ReadHeaderTimeout: 10 * time.Second}
	go h.srv.Serve(ln)
	return h, nil
}

// renderIndex 在 MyGo 生成的页面里插入触摸滚动脚本，并禁止双指缩放（界面自己处理）
func renderIndex(page []byte) []byte {
	s := strings.Replace(string(page), `content="width=device-width, initial-scale=1"`,
		`content="width=device-width, initial-scale=1, maximum-scale=1, user-scalable=no, viewport-fit=cover"`, 1)
	shim := "<script>\n" + touchJS + "</script>\n"
	if i := strings.Index(s, "</head>"); i >= 0 {
		s = s[:i] + shim + s[i:]
	} else {
		s = shim + s
	}
	return []byte(s)
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *host) enterURL() string {
	token := randomHex(24)
	h.mu.Lock()
	for t, exp := range h.tokens {
		if time.Now().After(exp) {
			delete(h.tokens, t)
		}
	}
	h.tokens[token] = time.Now().Add(tokenTTL)
	h.mu.Unlock()
	return "http://" + h.addr + "/host/enter?token=" + token
}

func (h *host) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /host/enter", h.handleEnter)
	mux.HandleFunc("GET /host/state", h.auth(h.handleState))
	mux.HandleFunc("PUT /host/prefs", h.auth(h.handlePrefs))
	mux.HandleFunc("PUT /host/password", h.auth(h.handlePassword))
	mux.HandleFunc("DELETE /host/password", h.auth(h.handlePassword))
	mux.HandleFunc("POST /host/local", h.auth(h.handleLocal))
	mux.HandleFunc("POST /host/stop", h.auth(func(w http.ResponseWriter, r *http.Request) { h.stopLocal(true); writeOK(w) }))
	mux.HandleFunc("POST /host/remote", h.auth(h.handleRemote))
	mux.HandleFunc("POST /host/probe", h.auth(h.handleProbe))
	mux.HandleFunc("GET /host/image", h.auth(h.handleImage))
	mux.HandleFunc("/api/", h.auth(h.handleAPI))
	mux.HandleFunc("GET /{$}", h.handleIndex)
	mux.HandleFunc("GET /index.html", h.handleIndex)
	// 不写方法：写成 GET / 会和不限方法的 /api/ 冲突
	files := http.FileServer(http.Dir(h.webDir))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		files.ServeHTTP(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 只认 127.0.0.1:端口，防止别的网站把自己的域名解析到 127.0.0.1 来访问（DNS 重绑定）
		if r.Host != h.addr {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		mux.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func writeOK(w http.ResponseWriter) { writeJSON(w, http.StatusOK, map[string]bool{"ok": true}) }

func (h *host) authed(r *http.Request) bool {
	c, err := r.Cookie(hostCookie)
	return err == nil && c.Value == h.session
}

// auth 只放行打开过入口地址的 WebView；写操作还要带 X-Requested-With，跨站表单发不出它
func (h *host) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.authed(r) {
			writeErr(w, http.StatusForbidden, "请从 App 里打开")
			return
		}
		if r.Method != http.MethodGet && r.Header.Get("X-Requested-With") != "bia" {
			writeErr(w, http.StatusForbidden, "缺少 X-Requested-With 请求头")
			return
		}
		next(w, r)
	}
}

func (h *host) handleEnter(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	h.mu.Lock()
	exp, ok := h.tokens[token]
	delete(h.tokens, token)
	h.mu.Unlock()
	if !ok || time.Now().After(exp) {
		if h.authed(r) {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		http.Error(w, "入口地址已失效，请重新打开 App", http.StatusForbidden)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: hostCookie, Value: h.session, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *host) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(h.index)
}

func readBody(w http.ResponseWriter, r *http.Request, v any) error {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(v); err != nil {
		return errors.New("请求格式不对")
	}
	return nil
}

func (h *host) handleState(w http.ResponseWriter, r *http.Request) {
	prefs, _ := os.ReadFile(filepath.Join(h.dataDir, prefsFile))
	if !json.Valid(prefs) {
		prefs = []byte("{}")
	}
	pw, _ := os.ReadFile(filepath.Join(h.dataDir, passwordFile))
	writeJSON(w, http.StatusOK, map[string]any{
		"prefs":         json.RawMessage(prefs),
		"password":      string(pw),
		"local_running": h.running(),
		"platform":      runtime.GOOS,
	})
}

// writeFile 先写临时文件再改名，写到一半时 App 被杀掉也不会留下坏文件
func writeFile(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (h *host) handlePrefs(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil || !json.Valid(data) {
		writeErr(w, http.StatusBadRequest, "请求格式不对")
		return
	}
	if err := writeFile(filepath.Join(h.dataDir, prefsFile), data); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w)
}

func (h *host) handlePassword(w http.ResponseWriter, r *http.Request) {
	path := filepath.Join(h.dataDir, passwordFile)
	var body struct {
		Password string `json:"password"`
	}
	if r.Method == http.MethodPut {
		if err := readBody(w, r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	var err error
	if body.Password == "" {
		if err = os.Remove(path); errors.Is(err, os.ErrNotExist) {
			err = nil
		}
	} else {
		err = writeFile(path, []byte(body.Password))
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w)
}

func (h *host) running() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.engine != nil
}

// handleLocal 确认代理可用后在手机上开始运行定时任务；已经在运行时只更新代理
func (h *host) handleLocal(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Proxy string `json:"proxy"`
	}
	if err := readBody(w, r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	proxy := strings.TrimSpace(body.Proxy)
	if proxy == "" {
		writeErr(w, http.StatusBadRequest, "请填写代理的地址")
		return
	}
	if _, err := core.ParseProxy(proxy); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	via, err := core.ProbeProxy(proxy, transport)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	warning := ""
	if transport == nil {
		// 和直连比一下：一样说明代理没有接管这些请求
		if direct, derr := core.ProbeProxy("", nil); derr == nil && direct.Addr == via.Addr {
			warning = fmt.Sprintf("注意：经过代理和直连时 B 站看到的都是 %s，代理可能没有生效", via.Addr)
		}
	}

	e, err := h.startLocal(proxy)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"password": e.Password(), "zone": via, "warning": warning})
}

// startLocal 开始运行定时任务，/api/ 交给它；已经在运行时只更新代理
func (h *host) startLocal(proxy string) (*core.Engine, error) {
	h.mu.Lock()
	e, started := h.engine, false
	if e == nil {
		e = core.New(core.Config{
			DataDir:   h.dataDir,
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
			h.mu.Unlock()
			return nil, err
		}
	}
	if h.engine == nil {
		e.Start()
		h.engine, h.api, started = e, e.Handler(), true
	}
	h.local = true
	h.mu.Unlock()
	writeFile(filepath.Join(h.dataDir, localFile), []byte(proxy))
	if started {
		notify(true)
	}
	return e, nil
}

// stopLocal 停下定时任务；byUser 时下次不再自动接着运行
func (h *host) stopLocal(byUser bool) {
	h.mu.Lock()
	e := h.engine
	h.engine, h.api, h.local = nil, nil, false
	h.mu.Unlock()
	if byUser {
		os.Remove(filepath.Join(h.dataDir, localFile))
	}
	if e != nil {
		e.Stop()
		notify(false)
	}
}

// handleRemote 让之后的 /api/ 转发给 server
func (h *host) handleRemote(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Server string `json:"server"`
	}
	if err := readBody(w, r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	u, err := url.Parse(strings.TrimSpace(body.Server))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		writeErr(w, http.StatusBadRequest, "服务器地址不对："+body.Server)
		return
	}
	u.RawQuery, u.Fragment = "", ""
	rp := newRemoteProxy(u)
	h.mu.Lock()
	h.remote, h.local = rp, false
	h.mu.Unlock()
	writeOK(w)
}

func newRemoteProxy(u *url.URL) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(u)
			// 不把 App 自己的 Cookie 发给服务器
			pr.Out.Header.Del("Cookie")
			for _, c := range pr.In.Cookies() {
				if c.Name != hostCookie {
					pr.Out.AddCookie(c)
				}
			}
			pr.Out.Header.Del("Origin")
			pr.Out.Header.Del("Referer")
		},
		Transport:      transport,
		ModifyResponse: localCookies,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			writeErr(w, http.StatusBadGateway, fmt.Sprintf("连不上 %s：%v", u.Host, err))
		},
	}
}

// localCookies 让服务器的会话 Cookie 能存在 http://127.0.0.1 下：去掉 Secure 和 Domain
func localCookies(resp *http.Response) error {
	set := resp.Header.Values("Set-Cookie")
	if len(set) == 0 {
		return nil
	}
	resp.Header.Del("Set-Cookie")
	for _, s := range set {
		c, err := http.ParseSetCookie(s)
		if err != nil || c.Name == hostCookie {
			continue
		}
		c.Secure, c.Partitioned, c.Domain, c.Path = false, false, "", "/"
		resp.Header.Add("Set-Cookie", c.String())
	}
	return nil
}

func (h *host) handleAPI(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	next := h.remote
	if h.local {
		next = h.api
	}
	h.mu.Unlock()
	if next == nil {
		writeErr(w, http.StatusConflict, "本机运行已经停止。点「停止运行」回到首页，可以重新开始")
		return
	}
	next.ServeHTTP(w, r)
}

func (h *host) handleProbe(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Proxy string `json:"proxy"`
	}
	if err := readBody(w, r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	z, err := core.ProbeProxy(strings.TrimSpace(body.Proxy), transport)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, z)
}

// handleImage 下载 B 站图床的图片（头像、视频封面）给界面显示：WebView 里直接跨站请求会被拦下
func (h *host) handleImage(w http.ResponseWriter, r *http.Request) {
	u, err := url.Parse(r.URL.Query().Get("u"))
	if err != nil || u.Scheme != "https" || !strings.HasSuffix(u.Hostname(), ".hdslb.com") {
		writeErr(w, http.StatusBadRequest, "不是 B 站图床的图片")
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u.String(), nil)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second, Transport: transport}).Do(req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		writeErr(w, http.StatusBadGateway, fmt.Sprintf("HTTP %d", resp.StatusCode))
		return
	}
	if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "image/") {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "max-age=86400")
	io.Copy(w, io.LimitReader(resp.Body, 2<<20))
}
