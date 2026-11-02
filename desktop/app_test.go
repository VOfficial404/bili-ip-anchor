package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// fakeServer 模拟 bili-ip-anchor 的 WebUI 接口
type fakeServer struct {
	mu       sync.Mutex
	password string
	st       status
	runs     []string
	removed  []string
	polls    int
	*httptest.Server
}

var testNow = time.Date(2026, 11, 2, 14, 0, 0, 0, time.Local)

func at(min int) *time.Time {
	t := testNow.Add(time.Duration(min) * time.Minute)
	return &t
}

func newFakeServer(t *testing.T) *fakeServer {
	f := &fakeServer{password: "secret", st: status{
		Version:  "v0.5.0",
		Zone:     &zone{Addr: "203.0.113.24", Country: "中国", Province: "广东", City: "深圳", ISP: "电信"},
		Settings: settings{BVID: "BV15HFVz1EvM", Interval: "6h", Jitter: "30m"},
		Accounts: []account{
			{Mid: "1001", Uname: "锚点测试甲", LoggedIn: true, Last: &runResult{At: *at(-12), OK: true}, Next: at(352)},
			{Mid: "1002", Uname: "一个名字特别特别长的测试账号昵称", LoggedIn: true, Running: true, Last: &runResult{At: *at(-300), OK: true}},
			{Mid: "1003", LoggedIn: true, Last: &runResult{At: *at(-40), OK: false, Error: "心跳被拒绝：-101 账号未登录"}, Next: at(320)},
			{Mid: "1004", Uname: "失效的号", LoggedIn: false, Last: &runResult{At: *at(-5), OK: false, Error: "登录已失效"}, Next: at(355)},
		},
		Logs: []string{
			"2026/11/02 13:48:01 bili-ip-anchor v0.5.0 守护模式启动：间隔 6h，随机抖动 30m，数据目录 /data",
			"2026/11/02 13:48:01 已加载 4 个账号，各自独立排程",
			"2026/11/02 13:48:02 [锚点测试甲] 登录正常：锚点测试甲",
			"2026/11/02 13:48:02 [锚点测试甲] 心跳：0 0",
			"2026/11/02 13:48:17 [UID 1003] 本轮失败：心跳被拒绝：-101 账号未登录",
			"2026/11/02 13:55:12 [失效的号] 本轮失败：登录已失效，请重新扫码登录",
		},
	}}
	mux := http.NewServeMux()
	authed := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" && r.Header.Get("X-Requested-With") != "bia" {
				http.Error(w, `{"error":"缺少请求头"}`, http.StatusForbidden)
				return
			}
			if c, err := r.Cookie("bia_session"); err != nil || c.Value != "ok" {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"请先输入密码"}`))
				return
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			h(w, r)
		}
	}
	reply := func(w http.ResponseWriter, v any) { json.NewEncoder(w).Encode(v) }
	mux.HandleFunc("POST /api/session", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Password string }
		json.NewDecoder(r.Body).Decode(&body)
		if body.Password != f.password {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"密码不对"}`))
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "bia_session", Value: "ok", Path: "/"})
		reply(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/status", authed(func(w http.ResponseWriter, r *http.Request) { reply(w, f.st) }))
	mux.HandleFunc("POST /api/run", authed(func(w http.ResponseWriter, r *http.Request) {
		f.runs = append(f.runs, "all")
		reply(w, map[string]int{"triggered": 2})
	}))
	mux.HandleFunc("POST /api/accounts/{mid}/run", authed(func(w http.ResponseWriter, r *http.Request) {
		f.runs = append(f.runs, r.PathValue("mid"))
		reply(w, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("DELETE /api/accounts/{mid}", authed(func(w http.ResponseWriter, r *http.Request) {
		mid := r.PathValue("mid")
		f.removed = append(f.removed, mid)
		var kept []account
		for _, acc := range f.st.Accounts {
			if acc.Mid != mid {
				kept = append(kept, acc)
			}
		}
		f.st.Accounts = kept
		reply(w, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("PUT /api/settings", authed(func(w http.ResponseWriter, r *http.Request) {
		var s settings
		json.NewDecoder(r.Body).Decode(&s)
		if s.Interval == "5m" {
			w.WriteHeader(http.StatusBadRequest)
			reply(w, map[string]string{"error": "间隔不能小于 10 分钟"})
			return
		}
		f.st.Settings = s
		reply(w, s)
	}))
	mux.HandleFunc("POST /api/qr", authed(func(w http.ResponseWriter, r *http.Request) {
		img := image.NewGray(image.Rect(0, 0, 29, 29))
		for y := 0; y < 29; y++ {
			for x := 0; x < 29; x++ {
				if (x*7+y*13)%5 < 2 || (x < 7 && y < 7) {
					img.SetGray(x, y, color.Gray{})
				} else {
					img.SetGray(x, y, color.Gray{Y: 255})
				}
			}
		}
		png.Encode(w, img)
	}))
	mux.HandleFunc("GET /api/qr", authed(func(w http.ResponseWriter, r *http.Request) {
		f.polls++
		if f.polls < 2 {
			reply(w, map[string]string{"state": "scanned"})
			return
		}
		f.st.Accounts = append(f.st.Accounts, account{Mid: "1005", LoggedIn: true, Next: at(1)})
		reply(w, map[string]string{"state": "success", "mid": "1005"})
	}))
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

// harness 让后台请求的结果排队，测试里在 settle 时交给视图，代替 Window.Update
type harness struct {
	a     *app
	tt    *ui.Tester
	mu    sync.Mutex
	queue []func()
	t     *testing.T
}

func newHarness(t *testing.T, s store) *harness {
	h := &harness{t: t}
	h.a = newApp(s, func(fn func()) {
		h.mu.Lock()
		h.queue = append(h.queue, fn)
		h.mu.Unlock()
	})
	h.a.now = func() time.Time { return testNow }
	h.tt = ui.NewTester(h.a.view, 1040, 700)
	return h
}

func (h *harness) settle() {
	for i := 0; i < 20; i++ {
		h.a.wg.Wait()
		h.mu.Lock()
		q := h.queue
		h.queue = nil
		h.mu.Unlock()
		if len(q) == 0 {
			break
		}
		for _, fn := range q {
			fn()
		}
	}
	h.tt.Frame()
}

func (h *harness) click(s string) {
	h.t.Helper()
	if err := h.tt.Click(s); err != nil {
		h.t.Fatalf("点击 %q：%v，屏幕上有 %q", s, err, h.tt.Texts())
	}
	h.settle()
}

func (h *harness) must(s string) {
	h.t.Helper()
	if !h.tt.HasText(s) {
		h.t.Fatalf("应该显示 %q，屏幕上有 %q", s, h.tt.Texts())
	}
}

// snapshot 在设置了 SNAPSHOT_DIR 时把当前画面存成 PNG，方便人看布局
func (h *harness) snapshot(name string) {
	dir := os.Getenv("SNAPSHOT_DIR")
	if dir == "" {
		return
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, h.tt.Image()); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".png"), buf.Bytes(), 0644); err != nil {
		h.t.Fatal(err)
	}
}

func TestConnectAndPages(t *testing.T) {
	f := newFakeServer(t)
	st := &memStore{}
	h := newHarness(t, st)
	h.a.server = f.URL
	h.tt.Frame()
	h.must("连接到服务器")

	h.a.password = "wrong"
	h.click("连接")
	h.must("面板密码不对")
	h.snapshot("1-connect-error")

	h.a.password, h.a.remember = "secret", true
	h.click("连接")
	h.must("广东 深圳")
	h.must("需要重新扫码")
	h.snapshot("2-overview")
	if p := st.load(); !p.Remember || p.Server != f.URL {
		t.Errorf("连接信息没有保存：%+v", p)
	}
	if pw, _ := st.password(); pw != "secret" {
		t.Errorf("勾选了记住密码，保存的是 %q", pw)
	}

	h.click("账号")
	h.must("锚点测试甲")
	h.must("登录失效")
	h.must("心跳被拒绝：-101 账号未登录")
	h.snapshot("3-accounts")

	h.click("全部立即运行")
	h.must("已让 2 个账号立即运行")
	h.a.runAccount(h.a.st.Accounts[0])
	h.settle()
	if strings.Join(f.runs, ",") != "all,1001" {
		t.Errorf("运行记录 %v", f.runs)
	}

	h.click("设置")
	h.must("每轮间隔")
	h.snapshot("4-settings")
	h.a.form.Interval, h.a.formEdited = "5m", true
	h.tt.Frame()
	h.must("最少 10m")
	h.a.form.Interval = "2h"
	h.tt.Frame()
	h.click("保存设置")
	h.must("设置已保存")
	if f.st.Settings.Interval != "2h" {
		t.Errorf("服务器上的间隔是 %q", f.st.Settings.Interval)
	}

	h.click("日志")
	h.must("[锚点测试甲]")
	h.snapshot("5-logs")

	h.tt.SetDark(true)
	h.click("账号")
	h.snapshot("6-accounts-dark")
	h.tt.SetDark(false)
}

func TestAddAccountByQR(t *testing.T) {
	f := newFakeServer(t)
	h := newHarness(t, &memStore{})
	h.a.server, h.a.password = f.URL, "secret"
	h.a.connect()
	h.settle()

	h.click("添加账号")
	h.must("用 B 站 App 扫一扫")
	h.snapshot("7-qr")
	for i := 0; i < 2; i++ {
		h.a.tick()
	}
	h.settle()
	h.must("已扫码，请在手机上确认登录")
	for i := 0; i < 2; i++ {
		h.a.tick()
	}
	h.settle()
	if h.a.qrOpen || h.a.page != pageAccounts {
		t.Fatalf("扫码成功后应关闭弹窗并显示账号页：open=%v page=%s", h.a.qrOpen, h.a.page)
	}
	h.must("UID 1005")
}

func TestRemoveAccount(t *testing.T) {
	f := newFakeServer(t)
	h := newHarness(t, &memStore{})
	h.a.server, h.a.password = f.URL, "secret"
	h.a.connect()
	h.settle()
	h.click("账号")

	h.a.askRemove("1004")
	h.tt.Frame()
	h.must("退出「失效的号」？")
	h.tt.Key(0, ui.KeyEnter) // 默认按钮是「退出」
	h.settle()
	if strings.Join(f.removed, ",") != "1004" {
		t.Fatalf("移除记录 %v", f.removed)
	}
	h.must("已退出 失效的号")
	if h.tt.HasText("登录失效") {
		t.Error("移除后列表里不应再有失效的账号")
	}
}

func TestSessionExpiryAndPasswordChange(t *testing.T) {
	f := newFakeServer(t)
	h := newHarness(t, &memStore{})
	h.a.server, h.a.password = f.URL, "secret"
	h.a.connect()
	h.settle()

	// 服务器重启后面板会话丢了：客户端应自动用密码重新登录
	u := h.a.cl.base
	h.a.cl.http.Jar.SetCookies(mustURL(t, u), []*http.Cookie{{Name: "bia_session", Value: "stale", Path: "/"}})
	h.a.refresh()
	h.settle()
	if h.a.cl == nil || h.a.loadErr != "" {
		t.Fatalf("会话过期后应自动重新登录：cl=%v err=%q", h.a.cl != nil, h.a.loadErr)
	}

	// 密码改了：回到连接页并提示
	f.mu.Lock()
	f.password = "changed"
	f.mu.Unlock()
	h.a.cl.http.Jar.SetCookies(mustURL(t, u), []*http.Cookie{{Name: "bia_session", Value: "stale", Path: "/"}})
	h.a.refresh()
	h.settle()
	h.must("面板密码已经改了，请重新输入")
}

func mustURL(t *testing.T, s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestNormalizeServer(t *testing.T) {
	for in, want := range map[string]string{
		"127.0.0.1:8080":             "http://127.0.0.1:8080",
		" https://bili.example.com/": "https://bili.example.com",
		"http://h:1/panel/":          "http://h:1/panel",
	} {
		if got, err := normalizeServer(in); err != nil || got != want {
			t.Errorf("normalizeServer(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "ftp://x", "http://"} {
		if _, err := normalizeServer(bad); err == nil {
			t.Errorf("normalizeServer(%q) 应该报错", bad)
		}
	}
	if faceURL("//i0.hdslb.com/bfs/face/a.jpg") != "https://i0.hdslb.com/bfs/face/a.jpg@96w_96h_1c.png" {
		t.Error("faceURL 结果不对")
	}
	if faceURL("https://evil.example.com/a.jpg") != "" {
		t.Error("非 B 站图床的头像应被拒绝")
	}
}
