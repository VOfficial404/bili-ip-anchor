package mobile

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/VOfficial404/bili-ip-anchor/core"
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

func setup(t *testing.T) (string, *recorder) {
	t.Helper()
	transport = fakeBili{}
	rec := &recorder{}
	SetListener(rec)
	t.Cleanup(func() { Shutdown(); transport = nil; SetListener(nil) })
	dir := t.TempDir()
	if err := Open(dir); err != nil {
		t.Fatal(err)
	}
	return dir, rec
}

func state(t *testing.T) map[string]any {
	t.Helper()
	var st map[string]any
	if err := json.Unmarshal([]byte(State()), &st); err != nil {
		t.Fatal(err)
	}
	return st
}

func call(t *testing.T, method, path, body string) *Response {
	t.Helper()
	var b []byte
	if body != "" {
		b = []byte(body)
	}
	resp, err := Call(method, path, b)
	if err != nil {
		t.Fatalf("%s %s：%v", method, path, err)
	}
	return resp
}

func TestLocal(t *testing.T) {
	dir, rec := setup(t)
	if st := state(t); st["mode"] != ModeNone || st["local_running"] != false {
		t.Fatalf("一开始的状态不对：%v", st)
	}
	if r := call(t, "GET", "/api/status", ""); r.Status != 409 {
		t.Errorf("没选用法时应该返回 409，得到 %d", r.Status)
	}
	if _, err := StartLocal(""); err == nil {
		t.Error("没填代理应该报错")
	}
	if _, err := StartLocal("ftp://x"); err == nil {
		t.Error("不支持的代理应该报错")
	}
	out, err := StartLocal("http://127.0.0.1:7890")
	if err != nil || !strings.Contains(out, "广东") {
		t.Fatalf("开始本机运行：%s %v", out, err)
	}
	if !LocalRunning() || rec.String() != "[true]" {
		t.Fatalf("应该在运行并通知一次：%s", rec)
	}
	if st := state(t); st["mode"] != ModeLocal || st["proxy"] != "http://127.0.0.1:7890" {
		t.Errorf("状态不对：%v", st)
	}

	// 会话由这里管：第一次请求自动登录
	r := call(t, "GET", "/api/status", "")
	var status struct {
		Settings struct{ Proxy string } `json:"settings"`
	}
	if !r.OK() || json.Unmarshal(r.Body, &status) != nil || status.Settings.Proxy != "http://127.0.0.1:7890" {
		t.Fatalf("读状态：%d %s", r.Status, r.Body)
	}
	if r := call(t, "POST", "/api/accounts/404/favorites", `{"format":"bili-ip-anchor/favorites","version":1}`); r.Status != 404 || r.Message() == "" {
		t.Errorf("不存在的账号：%d %s", r.Status, r.Message())
	}
	if _, err := Call("GET", "/host/state", nil); err == nil {
		t.Error("只能调用 /api/")
	}

	// 被系统结束不算用户停止：下次打开接着运行
	Shutdown()
	if err := Open(dir); err != nil {
		t.Fatal(err)
	}
	if err := ResumeLocal(); err != nil || !LocalRunning() {
		t.Fatalf("应该接着运行：%v", err)
	}
	if r := call(t, "GET", "/api/status", ""); !r.OK() {
		t.Errorf("接着运行后读状态：%d", r.Status)
	}

	// 用户停止：用法还记着，但不再接着运行
	StopLocal()
	if LocalRunning() || rec.String() != "[true,false,true,false]" {
		t.Errorf("停止后的通知不对：%s", rec)
	}
	if r := call(t, "GET", "/api/status", ""); r.Status != 409 {
		t.Errorf("停止后应该返回 409，得到 %d", r.Status)
	}
	if err := ResumeLocal(); err != nil || LocalRunning() {
		t.Errorf("用户停止后不应该接着运行：%v", err)
	}
	if st := state(t); st["mode"] != ModeLocal {
		t.Errorf("停止后用法应该还在：%v", st)
	}
	Disconnect()
	if st := state(t); st["mode"] != ModeNone {
		t.Errorf("回到首页后用法应该清掉：%v", st)
	}
}

func TestRemote(t *testing.T) {
	dir, _ := setup(t)
	e := core.New(core.Config{DataDir: t.TempDir(), Version: "v-test", Password: "pw", Defaults: core.DefaultSettings(), Transport: fakeBili{}})
	srv := httptest.NewServer(e.Handler())
	defer srv.Close()

	if err := ConnectServer("", "pw", false); err == nil {
		t.Error("没填地址应该报错")
	}
	if err := ConnectServer(srv.URL, "wrong", true); err == nil {
		t.Error("密码不对应该报错")
	}
	if st := state(t); st["mode"] != ModeNone {
		t.Fatalf("登录失败不应该记住：%v", st)
	}
	if err := ConnectServer(strings.TrimPrefix(srv.URL, "http://")+"/", "pw", true); err != nil {
		t.Fatal(err)
	}
	if st := state(t); st["mode"] != ModeRemote || st["server"] != srv.URL || st["remember"] != true {
		t.Fatalf("状态不对：%v", st)
	}
	r := call(t, "GET", "/api/status", "")
	if !r.OK() || !strings.Contains(string(r.Body), "v-test") {
		t.Fatalf("读服务器状态：%d %s", r.Status, r.Body)
	}
	if r := call(t, "POST", "/api/qr", ""); r.Status == 401 || r.Status == 403 {
		t.Errorf("写操作要带 X-Requested-With：%d %s", r.Status, r.Body)
	}

	// 重新打开 App：会话没有了，用记住的密码重新登录
	Shutdown()
	if err := Open(dir); err != nil {
		t.Fatal(err)
	}
	if r := call(t, "GET", "/api/status", ""); !r.OK() {
		t.Fatalf("应该自动重新登录：%d %s", r.Status, r.Body)
	}

	// 不记住密码：会话没有了要重新输入
	if err := ConnectServer(srv.URL, "pw", false); err != nil {
		t.Fatal(err)
	}
	Shutdown()
	Open(dir)
	if r := call(t, "GET", "/api/status", ""); r.Status != 401 {
		t.Errorf("没记住密码时应该返回 401，得到 %d", r.Status)
	}
	Disconnect()
	if st := state(t); st["mode"] != ModeNone || st["remember"] != false {
		t.Errorf("回到首页后应该清掉：%v", st)
	}
}

func TestImage(t *testing.T) {
	for _, u := range []string{"https://example.com/a.jpg", "file:///etc/passwd", "https://hdslb.com.evil.cn/a.jpg"} {
		if _, err := Image(u); err == nil {
			t.Errorf("%s 应该被拒绝", u)
		}
	}
}
