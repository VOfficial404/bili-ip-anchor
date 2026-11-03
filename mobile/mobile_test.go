package mobile

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
)

type fakeBili struct{}

func (fakeBili) RoundTrip(r *http.Request) (*http.Response, error) {
	body := `{"code":-404}`
	if r.URL.Path == "/x/web-interface/zone" {
		body = `{"code":0,"data":{"addr":"203.0.113.8","country":"中国","province":"广东","city":"深圳","isp":"电信"}}`
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
}

func TestStartLoginStop(t *testing.T) {
	transport = fakeBili{}
	defer func() { transport = nil }()
	SetTimeZone("Asia/Shanghai")

	if _, err := LoginURL(); err == nil {
		t.Fatal("没开始运行时不应有登录地址")
	}
	if err := Start(t.TempDir(), "127.0.0.1"); err == nil {
		t.Fatal("没有端口的代理应该被拒绝")
	}
	if err := Start(t.TempDir(), "http://127.0.0.1:7890"); err != nil {
		t.Fatal(err)
	}
	defer Stop()
	if !Running() || Proxy() != "http://127.0.0.1:7890" {
		t.Fatalf("Running=%v Proxy=%q", Running(), Proxy())
	}
	if err := Start("", "socks5://127.0.0.1:7891"); err != nil || Proxy() != "socks5://127.0.0.1:7891" {
		t.Fatalf("运行中再调用 Start 应只换代理：%v %q", err, Proxy())
	}

	// 登录地址只能用一次，打开后就是面板
	u, err := LoginURL()
	if err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	resp, err := c.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(page), "Bili IP Anchor") {
		t.Fatalf("登录后应进入面板：HTTP %d", resp.StatusCode)
	}
	resp, err = c.Get(strings.Split(u, "/api/")[0] + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		Mode string `json:"mode"`
	}
	json.NewDecoder(resp.Body).Decode(&st)
	resp.Body.Close()
	if resp.StatusCode != 200 || st.Mode != "local" {
		t.Fatalf("面板状态不对：HTTP %d mode=%q", resp.StatusCode, st.Mode)
	}
	if resp, _ := (&http.Client{}).Get(u); resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Error("登录地址用过一次后应失效")
	}

	z, err := ProbeProxy("http://127.0.0.1:7890")
	if err != nil || !strings.Contains(z, `"region":"广东"`) || !strings.Contains(z, `"addr":"203.0.113.8"`) {
		t.Fatalf("ProbeProxy = %s, %v", z, err)
	}

	Stop()
	if Running() {
		t.Error("Stop 之后还在运行")
	}
}
