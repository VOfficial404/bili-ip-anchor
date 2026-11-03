package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSettingsJSON(t *testing.T) {
	var s Settings
	if err := json.Unmarshal([]byte(`{"videos":["BV1GJ411x7h7"],"do_dynamic":true,"interval":"90m","jitter":"0s"}`), &s); err != nil {
		t.Fatal(err)
	}
	if time.Duration(s.Interval) != 90*time.Minute || !s.DoDynamic {
		t.Fatalf("解析结果不对：%+v", s)
	}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(s)
	if !strings.Contains(string(b), `"interval":"1h30m"`) {
		t.Fatalf("序列化结果不对：%s", b)
	}
}

// v0.5 的 settings.json 只有一个 bvid
func TestSettingsMigrateBVID(t *testing.T) {
	var s Settings
	if err := json.Unmarshal([]byte(`{"bvid":"BV1GJ411x7h7","do_dynamic":false,"interval":"6h","jitter":"30m"}`), &s); err != nil {
		t.Fatal(err)
	}
	if len(s.Videos) != 1 || s.Videos[0] != "BV1GJ411x7h7" {
		t.Fatalf("bvid 没有变成 videos：%+v", s)
	}
	b, _ := json.Marshal(s)
	if strings.Contains(string(b), `"bvid"`) {
		t.Errorf("新格式不应再写 bvid：%s", b)
	}
}

func TestSettingsNormalize(t *testing.T) {
	s := Settings{
		Videos:   []string{" BV1GJ411x7h7 ", "https://www.bilibili.com/video/BV15HFVz1EvM/?p=1", "BV1GJ411x7h7", ""},
		Proxy:    " http://127.0.0.1:7890 ",
		Interval: Duration(time.Hour),
	}
	s.normalize()
	if strings.Join(s.Videos, ",") != "BV1GJ411x7h7,BV15HFVz1EvM" || s.Proxy != "http://127.0.0.1:7890" {
		t.Fatalf("normalize = %+v", s)
	}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := ParseVideos("BV1GJ411x7h7，BV15HFVz1EvM https://b23.tv/x"); len(got) != 3 || got[1] != "BV15HFVz1EvM" {
		t.Errorf("ParseVideos = %v", got)
	}
}

func TestSettingsValidate(t *testing.T) {
	ok := Settings{Videos: []string{"BV1GJ411x7h7"}, Interval: Duration(time.Hour)}
	cases := map[string]Settings{
		"间隔太短":   {Videos: ok.Videos, Interval: Duration(5 * time.Minute)},
		"没有视频":   {Interval: ok.Interval},
		"BV 号不对": {Videos: []string{"av170001"}, Interval: ok.Interval},
		"抖动为负":   {Videos: ok.Videos, Interval: ok.Interval, Jitter: Duration(-time.Second)},
		"代理不对":   {Videos: ok.Videos, Interval: ok.Interval, Proxy: "ftp://127.0.0.1:21"},
		"代理没端口":  {Videos: ok.Videos, Interval: ok.Interval, Proxy: "127.0.0.1"},
		"文字太长":   {Videos: ok.Videos, Interval: ok.Interval, DynamicText: strings.Repeat("字", maxDynamicText+1)},
	}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, s := range cases {
		if s.Validate() == nil {
			t.Errorf("%s：应该校验失败", name)
		}
	}
	for _, p := range []string{"", "127.0.0.1:7890", "socks5://user:pw@127.0.0.1:7891"} {
		if _, err := ParseProxy(p); err != nil {
			t.Errorf("%q 应该是合法代理：%v", p, err)
		}
	}
	if got := redactProxy("socks5://user:secret@127.0.0.1:7891"); strings.Contains(got, "secret") {
		t.Errorf("日志里不应出现代理密码：%s", got)
	}
}

func TestRingLog(t *testing.T) {
	r := newRingLog(3)
	r.Write([]byte("a\nb\n"))
	r.Write([]byte("c\n"))
	r.Write([]byte("d\n"))
	if got := strings.Join(r.Lines(), ","); got != "b,c,d" {
		t.Fatalf("got %q", got)
	}
}

func TestAuth(t *testing.T) {
	e := testEngine(t)
	e.sessions = map[string]time.Time{"good": time.Now().Add(time.Hour), "old": time.Now().Add(-time.Hour)}
	h := e.auth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })

	do := func(method, token string, header bool) int {
		req := httptest.NewRequest(method, "/api/x", nil)
		if token != "" {
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		}
		if header {
			req.Header.Set("X-Requested-With", "bia")
		}
		w := httptest.NewRecorder()
		h(w, req)
		return w.Code
	}

	if c := do("GET", "", false); c != http.StatusUnauthorized {
		t.Errorf("无会话应 401，得到 %d", c)
	}
	if c := do("GET", "old", false); c != http.StatusUnauthorized {
		t.Errorf("过期会话应 401，得到 %d", c)
	}
	if c := do("GET", "good", false); c != http.StatusTeapot {
		t.Errorf("有效会话 GET 应放行，得到 %d", c)
	}
	if c := do("POST", "good", false); c != http.StatusForbidden {
		t.Errorf("POST 缺请求头应 403，得到 %d", c)
	}
	if c := do("POST", "good", true); c != http.StatusTeapot {
		t.Errorf("POST 带请求头应放行，得到 %d", c)
	}
	if !e.checkPassword("correct horse") || e.checkPassword("wrong") {
		t.Error("checkPassword 结果不对")
	}
	e.cfg.Password = ""
	if e.checkPassword("") {
		t.Error("没设密码时不能用空密码登录")
	}
}

// 一次性登录令牌只能用一次
func TestAutoLoginToken(t *testing.T) {
	e := testEngine(t)
	h := e.Handler()
	token := e.AutoLoginToken()
	get := func(tok string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/api/session/auto?token="+tok, nil))
		return w
	}
	w := get(token)
	if w.Code != http.StatusFound || len(w.Result().Cookies()) == 0 {
		t.Fatalf("第一次应登录并跳转，得到 %d", w.Code)
	}
	if c := w.Result().Cookies()[0]; !e.validSession(c.Value) {
		t.Error("拿到的会话无效")
	}
	if w := get(token); w.Code != http.StatusUnauthorized {
		t.Errorf("第二次应 401，得到 %d", w.Code)
	}
	if w := get(""); w.Code != http.StatusUnauthorized {
		t.Errorf("空令牌应 401，得到 %d", w.Code)
	}
}

func TestDurationString(t *testing.T) {
	for in, want := range map[time.Duration]string{
		0:                       "0s",
		30 * time.Minute:        "30m",
		6 * time.Hour:           "6h",
		90 * time.Minute:        "1h30m",
		90 * time.Second:        "1m30s",
		time.Hour + time.Second: "1h0m1s",
	} {
		if got := Duration(in).String(); got != want {
			t.Errorf("%v: got %q, want %q", in, got, want)
		}
	}
}
