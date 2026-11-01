package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSettingsJSON(t *testing.T) {
	var s settings
	if err := json.Unmarshal([]byte(`{"bvid":"BV1GJ411x7h7","do_dynamic":true,"interval":"90m","jitter":"0s"}`), &s); err != nil {
		t.Fatal(err)
	}
	if time.Duration(s.Interval) != 90*time.Minute || !s.DoDynamic {
		t.Fatalf("解析结果不对：%+v", s)
	}
	if err := s.validate(); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(s)
	if !strings.Contains(string(b), `"interval":"1h30m"`) {
		t.Fatalf("序列化结果不对：%s", b)
	}
}

func TestSettingsValidate(t *testing.T) {
	ok := settings{BVID: "BV1GJ411x7h7", Interval: duration(time.Hour)}
	cases := map[string]settings{
		"间隔太短":   {BVID: ok.BVID, Interval: duration(5 * time.Minute)},
		"BV 号不对": {BVID: "av170001", Interval: ok.Interval},
		"抖动为负":   {BVID: ok.BVID, Interval: ok.Interval, Jitter: duration(-time.Second)},
	}
	if err := ok.validate(); err != nil {
		t.Fatal(err)
	}
	for name, s := range cases {
		if s.validate() == nil {
			t.Errorf("%s：应该校验失败", name)
		}
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
	cfg.WebPassword = "correct horse"
	a := &app{sessions: map[string]time.Time{"good": time.Now().Add(time.Hour), "old": time.Now().Add(-time.Hour)}}
	h := a.auth(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })

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
	if !checkPassword("correct horse") || checkPassword("wrong") {
		t.Error("checkPassword 结果不对")
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
		if got := duration(in).String(); got != want {
			t.Errorf("%v: got %q, want %q", in, got, want)
		}
	}
}
