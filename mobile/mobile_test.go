package mobile

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeBili 模拟 B 站的出口 IP 查询；别的请求（测试里的假服务器）照常发出
type fakeBili struct{}

func (fakeBili) RoundTrip(r *http.Request) (*http.Response, error) {
	if !strings.HasSuffix(r.URL.Hostname(), "bilibili.com") {
		return http.DefaultTransport.RoundTrip(r)
	}
	body := `{"code":-404}`
	if r.URL.Path == "/x/web-interface/zone" {
		body = `{"code":0,"data":{"addr":"203.0.113.8","country":"中国","province":"广东","city":"深圳","isp":"电信"}}`
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
}

type recorder struct {
	mu  sync.Mutex
	got []bool
}

func (r *recorder) LocalChanged(on bool) {
	r.mu.Lock()
	r.got = append(r.got, on)
	r.mu.Unlock()
}

func (r *recorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, _ := json.Marshal(r.got)
	return string(b)
}

type tclient struct {
	t    *testing.T
	base string
	http *http.Client
}

func (c *tclient) do(method, path string, in any, out any) int {
	c.t.Helper()
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, body)
	req.Header.Set("X-Requested-With", "bia")
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func TestHost(t *testing.T) {
	transport = fakeBili{}
	rec := &recorder{}
	SetListener(rec)
	defer func() { transport = nil; SetListener(nil) }()
	SetTimeZone("Asia/Shanghai")

	web, data := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(web, "index.html"), []byte(`<html><head><meta name="viewport" content="width=device-width, initial-scale=1" /></head><body></body></html>`), 0o644)
	os.WriteFile(filepath.Join(web, "app.wasm"), []byte("wasm"), 0o644)

	if _, err := EnterURL(); err == nil {
		t.Fatal("没开始时不应有入口地址")
	}
	entry, err := Start(web, data)
	if err != nil {
		t.Fatal(err)
	}
	defer Shutdown()
	base := entry[:strings.Index(entry, "/host/")]

	// 没打开过入口地址的（例如手机上的其他 App）不能用接口
	if resp, _ := http.Get(base + "/host/state"); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("没有 Cookie 时 HTTP %d", resp.StatusCode)
	}
	// 别的域名解析到 127.0.0.1 也不行
	req, _ := http.NewRequest("GET", base+"/", nil)
	req.Host = "evil.example:80"
	if resp, _ := http.DefaultClient.Do(req); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("Host 不对时 HTTP %d", resp.StatusCode)
	}

	jar, _ := cookiejar.New(nil)
	c := &tclient{t: t, base: base, http: &http.Client{Jar: jar}}
	resp, err := c.http.Get(entry)
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(page), "pointercancel") || !strings.Contains(string(page), "user-scalable=no") {
		t.Fatalf("首页没有插入触摸脚本：%s", page)
	}
	if resp, _ := http.Get(entry); resp.StatusCode != http.StatusForbidden {
		t.Fatal("入口地址只能用一次")
	}
	if resp, _ := c.http.Get(base + "/app.wasm"); resp.StatusCode != 200 {
		t.Fatal("界面文件打不开")
	}

	// 写操作要带 X-Requested-With
	req, _ = http.NewRequest("PUT", base+"/host/prefs", strings.NewReader(`{}`))
	if resp, _ := c.http.Do(req); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("没有 X-Requested-With 时 HTTP %d", resp.StatusCode)
	}
	if code := c.do("PUT", "/host/prefs", map[string]any{"mode": "remote", "server": "https://example.com"}, nil); code != 200 {
		t.Fatalf("保存连接信息 HTTP %d", code)
	}
	c.do("PUT", "/host/password", map[string]string{"password": "secret"}, nil)
	var st struct {
		Prefs struct {
			Server string `json:"server"`
		} `json:"prefs"`
		Password     string `json:"password"`
		LocalRunning bool   `json:"local_running"`
		Platform     string `json:"platform"`
	}
	c.do("GET", "/host/state", nil, &st)
	if st.Prefs.Server != "https://example.com" || st.Password != "secret" || st.LocalRunning || st.Platform == "" {
		t.Fatalf("state %+v", st)
	}
	c.do("DELETE", "/host/password", nil, nil)
	c.do("GET", "/host/state", nil, &st)
	if st.Password != "" {
		t.Fatal("密码没有删掉")
	}

	// 本机运行
	if code := c.do("POST", "/host/local", map[string]string{"proxy": "127.0.0.1"}, nil); code != http.StatusBadRequest {
		t.Fatalf("没有端口的代理应该被拒绝，HTTP %d", code)
	}
	var run struct {
		Password string `json:"password"`
		Zone     struct {
			Addr string `json:"addr"`
		} `json:"zone"`
	}
	if code := c.do("POST", "/host/local", map[string]string{"proxy": "http://127.0.0.1:7890"}, &run); code != 200 || run.Password == "" || run.Zone.Addr != "203.0.113.8" {
		t.Fatalf("本机运行 HTTP %d %+v", code, run)
	}
	if !LocalRunning() || rec.String() != "[true]" {
		t.Fatalf("running=%v listener=%s", LocalRunning(), rec)
	}
	if code := c.do("POST", "/api/session", map[string]string{"password": run.Password}, nil); code != 200 {
		t.Fatalf("登录本机面板 HTTP %d", code)
	}
	var status struct {
		Mode     string `json:"mode"`
		Settings struct {
			Proxy string `json:"proxy"`
		} `json:"settings"`
	}
	if code := c.do("GET", "/api/status", nil, &status); code != 200 || status.Mode != "local" || status.Settings.Proxy != "http://127.0.0.1:7890" {
		t.Fatalf("本机状态 HTTP %d %+v", code, status)
	}
	// 再开始一次只换代理
	c.do("POST", "/host/local", map[string]string{"proxy": "socks5://127.0.0.1:7891"}, nil)
	c.do("GET", "/api/status", nil, &status)
	if status.Settings.Proxy != "socks5://127.0.0.1:7891" || rec.String() != "[true]" {
		t.Fatalf("换代理 %+v listener=%s", status, rec)
	}
	// App 被系统杀掉后重新开始：接着用同一个代理运行
	Shutdown()
	if _, err := Start(web, data); err != nil {
		t.Fatal(err)
	}
	if err := ResumeLocal(); err != nil || !LocalRunning() || rec.String() != "[true,false,true]" {
		t.Fatalf("没有接着运行：%v listener=%s", err, rec)
	}
	entry, _ = EnterURL()
	base = entry[:strings.Index(entry, "/host/")]
	jar, _ = cookiejar.New(nil)
	c = &tclient{t: t, base: base, http: &http.Client{Jar: jar}}
	c.http.Get(entry)
	StopLocal()
	if LocalRunning() || rec.String() != "[true,false,true,false]" {
		t.Fatalf("没有停下：listener=%s", rec)
	}
	if ResumeLocal(); LocalRunning() {
		t.Fatal("用户停止后不应再自动运行")
	}
	if code := c.do("GET", "/api/status", nil, nil); code != http.StatusConflict {
		t.Fatalf("停下后 HTTP %d", code)
	}

	// 连接服务器：/api/ 转发过去，服务器的 Secure Cookie 也能存下，App 的 Cookie 不发过去
	var leaked bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie(hostCookie); err == nil {
			leaked = true
		}
		switch r.URL.Path {
		case "/panel/api/session":
			http.SetCookie(w, &http.Cookie{Name: "bia_session", Value: "remote", Path: "/panel", Secure: true, Domain: "example.com"})
			w.Write([]byte(`{"ok":true}`))
		case "/panel/api/status":
			if c, err := r.Cookie("bia_session"); err != nil || c.Value != "remote" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Write([]byte(`{"mode":"server"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	if code := c.do("POST", "/host/remote", map[string]string{"server": "ftp://x"}, nil); code != http.StatusBadRequest {
		t.Fatalf("地址不对时 HTTP %d", code)
	}
	if code := c.do("POST", "/host/remote", map[string]string{"server": srv.URL + "/panel"}, nil); code != 200 {
		t.Fatalf("连接服务器 HTTP %d", code)
	}
	c.do("POST", "/api/session", map[string]string{"password": "x"}, nil)
	if code := c.do("GET", "/api/status", nil, &status); code != 200 || status.Mode != "server" {
		t.Fatalf("经过转发 HTTP %d %+v", code, status)
	}
	if leaked {
		t.Error("App 的 Cookie 发给了服务器")
	}

	if code := c.do("GET", "/host/image?u=https://evil.example/a.png", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("非 B 站图床 HTTP %d", code)
	}
	var z struct {
		Province string `json:"province"`
	}
	if code := c.do("POST", "/host/probe", map[string]string{"proxy": "http://127.0.0.1:7890"}, &z); code != 200 || z.Province != "广东" {
		t.Fatalf("probe HTTP %d %+v", code, z)
	}
}

// 在浏览器里试手机界面：先 cd desktop && go tool mygo build -platform web，再
// MOBILE_DEMO_ADDR=127.0.0.1:18077 go test -run TestServeHost -timeout 0 ./mobile，打开日志里的入口地址
func TestServeHost(t *testing.T) {
	addr := os.Getenv("MOBILE_DEMO_ADDR")
	if addr == "" {
		t.Skip("设置 MOBILE_DEMO_ADDR=127.0.0.1:18077 启动")
	}
	listenAddr = addr
	entry, err := Start(filepath.Join("..", "desktop", "build", "web"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Log("入口地址：", entry)
	select {}
}
