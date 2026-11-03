package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
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
	actions  []string // 「方法 UID 操作」，例如 POST 1001 run
	polls    int
	images   map[string][]byte
	*httptest.Server
}

var testNow = time.Date(2026, 11, 2, 14, 0, 0, 0, time.Local)

func at(min int) *time.Time {
	t := testNow.Add(time.Duration(min) * time.Minute)
	return &t
}

func qrPNG() []byte {
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
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func newFakeServer(t *testing.T) *fakeServer {
	f := &fakeServer{password: "secret", images: map[string][]byte{}, st: status{
		Version:       "v0.6.0",
		Mode:          "server",
		Zone:          &zone{Addr: "203.0.113.24", Country: "中国", Province: "广东", City: "深圳", ISP: "电信"},
		ZoneCheckedAt: at(-3),
		Settings:      settings{Videos: []string{"BV15HFVz1EvM"}, Interval: "6h", Jitter: "30m", DoDynamic: true, DynamicText: "今天天气不错"},
		Images:        []imageInfo{},
		Accounts: []account{
			{Mid: "1001", Uname: "锚点测试甲", LoggedIn: true, Last: &runResult{At: *at(-12), OK: true}, LastOK: at(-12), Next: at(352),
				Stats: &stats{Coins: 84.5, Level: 6, VIP: "年度大会员"}, Cookie: cookieStatus{Expires: at(170 * 24 * 60), CanRefresh: true},
				AppAuth: true, Home: &homeLocation{Text: "广东", CheckedAt: *at(-12), MatchedSince: at(-600)}},
			{Mid: "1002", Uname: "一个名字特别特别长的测试账号昵称", LoggedIn: true, Running: true, Last: &runResult{At: *at(-300), OK: true},
				Stats: &stats{Coins: 3, Level: 3}, Cookie: cookieStatus{Expires: at(10 * 24 * 60), CanRefresh: true},
				AppAuth: true, Home: &homeLocation{Text: "北京", CheckedAt: *at(-300)}},
			{Mid: "1003", LoggedIn: true, Last: &runResult{At: *at(-40), OK: false, Error: "心跳被拒绝：-101 账号未登录"}, Next: at(320)},
			{Mid: "1004", Uname: "失效的号", LoggedIn: false, Last: &runResult{At: *at(-5), OK: false, Error: "登录已失效"}, Next: at(355)},
		},
		Logs: []string{
			"2026/11/02 13:48:01 bili-ip-anchor v0.6.0 启动：间隔 6h，随机抖动 30m，直连，数据目录 /data",
			"2026/11/02 13:48:01 已加载 4 个账号，各自独立排程",
			"2026/11/02 13:48:02 [锚点测试甲] 登录正常",
			"2026/11/02 13:48:02 [锚点测试甲] 心跳 BV15HFVz1EvM：0 0",
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
	ok := func(w http.ResponseWriter) { reply(w, map[string]bool{"ok": true}) }
	mux.HandleFunc("POST /api/session", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Password string }
		json.NewDecoder(r.Body).Decode(&body)
		if body.Password != f.password {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"密码不对"}`))
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "bia_session", Value: "ok", Path: "/"})
		ok(w)
	})
	mux.HandleFunc("GET /api/status", authed(func(w http.ResponseWriter, r *http.Request) { reply(w, f.st) }))
	mux.HandleFunc("POST /api/run", authed(func(w http.ResponseWriter, r *http.Request) {
		f.actions = append(f.actions, "POST all run")
		reply(w, map[string]int{"triggered": 2})
	}))
	for _, action := range []string{"run", "check", "cookie/refresh"} {
		mux.HandleFunc("POST /api/accounts/{mid}/"+action, authed(func(w http.ResponseWriter, r *http.Request) {
			f.actions = append(f.actions, "POST "+r.PathValue("mid")+" "+action)
			ok(w)
		}))
	}
	mux.HandleFunc("DELETE /api/accounts/{mid}/appauth", authed(func(w http.ResponseWriter, r *http.Request) {
		f.actions = append(f.actions, "DELETE "+r.PathValue("mid")+" appauth")
		ok(w)
	}))
	mux.HandleFunc("POST /api/accounts/{mid}/appauth", authed(func(w http.ResponseWriter, r *http.Request) { w.Write(qrPNG()) }))
	mux.HandleFunc("GET /api/accounts/{mid}/appauth", authed(func(w http.ResponseWriter, r *http.Request) {
		f.actions = append(f.actions, "GET "+r.PathValue("mid")+" appauth")
		reply(w, map[string]string{"state": "success"})
	}))
	mux.HandleFunc("DELETE /api/accounts/{mid}", authed(func(w http.ResponseWriter, r *http.Request) {
		mid := r.PathValue("mid")
		f.actions = append(f.actions, "DELETE "+mid+" ")
		f.st.Accounts = slices.DeleteFunc(f.st.Accounts, func(a account) bool { return a.Mid == mid })
		ok(w)
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
	mux.HandleFunc("POST /api/qr", authed(func(w http.ResponseWriter, r *http.Request) { w.Write(qrPNG()) }))
	mux.HandleFunc("GET /api/qr", authed(func(w http.ResponseWriter, r *http.Request) {
		f.polls++
		if f.polls < 2 {
			reply(w, map[string]string{"state": "scanned"})
			return
		}
		f.st.Accounts = append(f.st.Accounts, account{Mid: "1005", LoggedIn: true, Next: at(1)})
		reply(w, map[string]string{"state": "success", "mid": "1005"})
	}))
	mux.HandleFunc("GET /api/videos/{bvid}", authed(func(w http.ResponseWriter, r *http.Request) {
		bvid := r.PathValue("bvid")
		if bvid == "BV1xx411c7XX" {
			w.WriteHeader(http.StatusNotFound)
			reply(w, map[string]string{"error": "找不到这个视频，可能不存在或不公开"})
			return
		}
		reply(w, videoInfo{BVID: bvid, Title: "演示视频 " + bvid, Owner: "某个 UP 主", Duration: 284, Views: 1234567})
	}))
	mux.HandleFunc("POST /api/dynamic/images", authed(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		name := fmt.Sprintf("%d-0000000%d.png", len(f.images)+1, len(f.images)+1)
		f.images[name] = data
		f.st.Images = append(f.st.Images, imageInfo{Name: name, Size: int64(len(data))})
		reply(w, map[string]string{"name": name})
	}))
	mux.HandleFunc("GET /api/dynamic/images/{name}", authed(func(w http.ResponseWriter, r *http.Request) {
		w.Write(f.images[r.PathValue("name")])
	}))
	mux.HandleFunc("DELETE /api/dynamic/images/{name}", authed(func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		delete(f.images, name)
		f.st.Images = slices.DeleteFunc(f.st.Images, func(i imageInfo) bool { return i.Name == name })
		ok(w)
	}))
	mux.HandleFunc("POST /api/proxy/test", authed(func(w http.ResponseWriter, r *http.Request) {
		reply(w, zone{Addr: "198.51.100.7", Country: "中国", Province: "上海", City: "上海", ISP: "联通"})
	}))
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func (f *fakeServer) did(action string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.actions, action)
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
	h.a.pickFiles = func() ([]string, error) { return nil, nil }
	h.tt = ui.NewTester(h.a.view, 1080, 1500) // 足够高，设置页的按钮不用滚动就能点到
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

// connected 连上假服务器
func (h *harness) connected(f *fakeServer) {
	h.t.Helper()
	h.a.server, h.a.password = f.URL, "secret"
	h.a.connect()
	h.settle()
	if h.a.cl == nil {
		h.t.Fatalf("连接失败：%s", h.a.connectErr)
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
	h.must("连接服务器")
	h.snapshot("1-connect")

	h.a.password = "wrong"
	h.click("连接")
	h.must("面板密码不对")

	h.a.password, h.a.remember = "secret", true
	h.click("连接")
	h.must("广东 深圳")
	h.must("需要处理")
	h.must("1 / 2") // 两个授权了的账号里一个已是目标属地
	h.must(serverHost(f.URL))
	h.must("203.0.113.24 · 广东")
	h.snapshot("2-overview")
	if p := st.load(); !p.Remember || p.Server != f.URL || p.Mode != modeRemote {
		t.Errorf("连接信息没有保存：%+v", p)
	}
	if pw, _ := st.password(); pw != "secret" {
		t.Errorf("勾选了记住密码，保存的是 %q", pw)
	}

	h.click("账号")
	for _, s := range []string{"锚点测试甲", "年度大会员", "硬币 84.5", "已是目标属地", "还不是广东", "还有 170 天", "还有 10 天", "未授权查询", "登录失效", "心跳被拒绝：-101 账号未登录"} {
		h.must(s)
	}
	h.snapshot("3-accounts")

	h.click("全部立即运行")
	h.must("已让 2 个账号立即运行")
	h.click("刷新 Cookie")
	h.must("Cookie 已刷新")
	if !f.did("POST all run") || !f.did("POST 1001 cookie/refresh") {
		t.Errorf("操作记录 %v", f.actions)
	}

	h.click("设置")
	h.must("每轮间隔")
	h.must("演示视频 BV15HFVz1EvM")
	h.must("每轮会发：文字「今天天气不错」，几秒后删掉。")
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
	h.connected(f)

	h.click("添加账号")
	h.must("用 B 站 App 扫一扫")
	h.snapshot("7-qr")
	for i := 0; i < 2; i++ {
		h.a.tick()
	}
	h.settle()
	h.must("已扫码，请在手机上确认")
	for i := 0; i < 2; i++ {
		h.a.tick()
	}
	h.settle()
	if h.a.qrOpen || h.a.page != pageAccounts {
		t.Fatalf("扫码成功后应关闭弹窗并显示账号页：open=%v page=%s", h.a.qrOpen, h.a.page)
	}
	h.must("UID 1005")
}

func TestAppAuth(t *testing.T) {
	f := newFakeServer(t)
	h := newHarness(t, &memStore{})
	h.connected(f)
	h.click("账号")

	h.click("授权查询") // 只有 UID 1003 没有授权
	h.must("授权查询主页属地")
	h.must("用「UID 1003」的 B 站 App 扫码并确认。这份授权只用来查询这个账号主页显示的 IP 属地，随时可以取消。")
	for i := 0; i < 2; i++ {
		h.a.tick()
	}
	h.settle()
	if h.a.qrOpen || !f.did("GET 1003 appauth") {
		t.Fatalf("授权成功后应关闭弹窗：open=%v 操作 %v", h.a.qrOpen, f.actions)
	}
	h.must("已授权，正在查询「UID 1003」的主页属地")

	h.a.askUnauth("1001")
	h.tt.Frame()
	h.must("取消「锚点测试甲」的主页属地授权？")
	h.tt.Key(0, ui.KeyEnter)
	h.settle()
	if !f.did("DELETE 1001 appauth") {
		t.Fatalf("没有取消授权：%v", f.actions)
	}
}

func TestRemoveAccount(t *testing.T) {
	f := newFakeServer(t)
	h := newHarness(t, &memStore{})
	h.connected(f)
	h.click("账号")

	h.a.askRemove("1004")
	h.tt.Frame()
	h.must("退出「失效的号」？")
	h.tt.Key(0, ui.KeyEnter) // 默认按钮是「退出」
	h.settle()
	if !f.did("DELETE 1004 ") {
		t.Fatalf("操作记录 %v", f.actions)
	}
	h.must("已退出 失效的号")
	if h.tt.HasText("登录失效") {
		t.Error("移除后列表里不应再有失效的账号")
	}
}

func TestSettingsVideosAndImages(t *testing.T) {
	f := newFakeServer(t)
	h := newHarness(t, &memStore{})
	h.connected(f)
	h.click("设置")

	h.a.videoAdd = "https://www.bilibili.com/video/BV1ab411c7mD/?p=1 和 BV1xx411c7XX 还有 BV15HFVz1EvM"
	h.tt.Frame()
	h.click("添加")
	if strings.Join(h.a.form.Videos, ",") != "BV15HFVz1EvM,BV1ab411c7mD" {
		t.Fatalf("视频列表 %v", h.a.form.Videos)
	}
	h.must("BV1xx411c7XX：找不到这个视频，可能不存在或不公开")
	h.must("演示视频 BV1ab411c7mD")

	pic := filepath.Join(t.TempDir(), "a.png")
	os.WriteFile(pic, qrPNG(), 0600)
	h.a.pickFiles = func() ([]string, error) { return []string{pic}, nil }
	h.click("添加图片")
	h.must("已添加 1 张图片")
	h.must("每轮会发：文字「今天天气不错」 + 1 张图片，几秒后删掉。")
	if len(h.a.images) != 1 {
		t.Errorf("缩略图没有加载：%v", h.a.images)
	}

	h.a.form.Proxy = "socks5://127.0.0.1:7891"
	h.click("测试")
	h.must("可用。B 站看到的是 上海 · 198.51.100.7，主页属地会变成上海。")
	h.snapshot("8-settings-edited")

	h.click("保存设置")
	if s := f.st.Settings; strings.Join(s.Videos, ",") != "BV15HFVz1EvM,BV1ab411c7mD" || s.Proxy != "socks5://127.0.0.1:7891" {
		t.Errorf("服务器上的设置 %+v", s)
	}

	h.a.removeImage(f.st.Images[0].Name)
	h.settle()
	if len(f.st.Images) != 0 {
		t.Error("图片没有删除")
	}
}

// v0.5 的服务器没有 mode 字段，也不认识新接口
func TestOldServer(t *testing.T) {
	f := newFakeServer(t)
	f.st.Mode, f.st.Version = "", "v0.5.0"
	h := newHarness(t, &memStore{})
	h.connected(f)
	h.must("服务器是 v0.5.0，账号详情、多个视频等功能需要服务器更新到 v0.6")
	h.click("账号")
	if h.tt.HasText("未授权查询") || h.tt.HasText("刷新资料") {
		t.Error("旧服务器不应显示账号详情")
	}
}

func TestSessionExpiryAndPasswordChange(t *testing.T) {
	f := newFakeServer(t)
	h := newHarness(t, &memStore{})
	h.connected(f)

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

// fakeBili 模拟 B 站接口，给本机模式用：出口 IP 在广东，账号 10001 登录正常
type fakeBili struct {
	mu         sync.Mutex
	heartbeats int
}

func (b *fakeBili) RoundTrip(r *http.Request) (*http.Response, error) {
	body := `{"code":-404,"message":"没有这个接口"}`
	switch r.URL.Path {
	case "/x/web-interface/zone":
		body = `{"code":0,"data":{"addr":"203.0.113.8","country":"中国","province":"广东","city":"深圳","isp":"电信"}}`
	case "/x/web-interface/nav":
		body = `{"code":0,"data":{"isLogin":true,"mid":10001,"uname":"本机账号","money":12,"level_info":{"current_level":4}}}`
	case "/x/passport-login/web/cookie/info":
		body = `{"code":0,"data":{"refresh":false}}`
	case "/x/click-interface/web/heartbeat":
		b.mu.Lock()
		b.heartbeats++
		b.mu.Unlock()
		body = `{"code":0,"message":"0"}`
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
}

func TestLocalMode(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "accounts", "10001"), 0700)
	os.WriteFile(filepath.Join(dir, "accounts", "10001", "cookies.json"), []byte(`{"SESSDATA":"s","bili_jct":"j","DedeUserID":"10001"}`), 0600)

	st := &memStore{dir: dir}
	h := newHarness(t, st)
	b := &fakeBili{}
	h.a.transport = b
	var trayOn []bool
	h.a.onLocal = func(on bool) { trayOn = append(trayOn, on) }
	t.Cleanup(func() {
		if h.a.engine != nil {
			h.a.engine.Stop()
		}
	})

	h.a.modeIdx = 1
	h.tt.Frame()
	h.must("下次打开时自动开始运行")
	h.a.proxy = "127.0.0.1"
	h.click("开始运行")
	h.must("代理地址不对")
	h.snapshot("9-local-connect")

	h.a.proxy, h.a.autoLocal = "http://127.0.0.1:7890", true
	h.click("开始运行")
	if h.a.engine == nil {
		t.Fatalf("没有开始本机运行：%s", h.a.connectErr)
	}
	if p := st.load(); p.Mode != modeLocal || p.Proxy != "http://127.0.0.1:7890" || !p.AutoLocal {
		t.Errorf("本机运行的设置没有保存：%+v", p)
	}
	if fmt.Sprint(trayOn) != "[true]" {
		t.Errorf("托盘图标 %v", trayOn)
	}

	// 等第一轮跑完、出口 IP 查到
	deadline := time.Now().Add(10 * time.Second)
	for {
		h.a.refresh()
		h.settle()
		s := h.a.st
		if s.Zone != nil && len(s.Accounts) == 1 && s.Accounts[0].Last != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("本机运行没有跑完第一轮：%+v", s)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if s := h.a.st; s.Mode != "local" || s.Settings.Proxy != "http://127.0.0.1:7890" || !s.Accounts[0].Last.OK {
		t.Fatalf("状态不对：%+v %+v", s, s.Accounts[0].Last)
	}
	h.must("本机运行 · " + version)
	h.must("B 站把本机（经代理）识别为")
	h.must("经代理 203.0.113.8 · 广东")
	h.snapshot("10-local-overview")
	b.mu.Lock()
	if b.heartbeats != 1 {
		t.Errorf("心跳 %d 次", b.heartbeats)
	}
	b.mu.Unlock()

	h.click("停止运行")
	if h.a.engine != nil || h.a.cl != nil || fmt.Sprint(trayOn) != "[true false]" {
		t.Fatalf("没有停下：engine=%v tray=%v", h.a.engine != nil, trayOn)
	}
	h.must("在本机运行")
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
	if hdslbURL("//i0.hdslb.com/bfs/face/a.jpg", faceSize) != "https://i0.hdslb.com/bfs/face/a.jpg@96w_96h_1c.png" {
		t.Error("hdslbURL 结果不对")
	}
	if hdslbURL("https://evil.example.com/a.jpg", faceSize) != "" {
		t.Error("非 B 站图床的图片应被拒绝")
	}
	if fmtNum(84.5) != "84.5" || fmtNum(12) != "12" {
		t.Error("fmtNum 结果不对")
	}
}
