package core

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// fakeBili 模拟 B 站接口，给演示和界面测试用：所有账号都登录正常，出口 IP 在广东
func fakeBili() http.RoundTripper {
	names := map[string]string{"10001": "小明同学", "10002": "测试小号", "10003": "很久以前的账号"}
	homes := map[string]string{"10001": "广东", "10002": "北京"}
	return roundTripFunc(func(r *http.Request) (*http.Response, error) {
		mid := ""
		if c, err := r.Cookie("DedeUserID"); err == nil {
			mid = c.Value
		}
		resp := func(s string) (*http.Response, error) { return jsonResp(s), nil }
		switch r.URL.Path {
		case "/x/web-interface/nav":
			vip := ""
			if mid == "10001" {
				vip = `"vipStatus":1,"vip_label":{"text":"年度大会员"},`
			}
			return resp(fmt.Sprintf(`{"code":0,"data":{"isLogin":true,"mid":%s,"uname":%q,"face":"",%s"money":%d.5,"level_info":{"current_level":%d},"wallet":{"bcoin_balance":%d}}}`,
				mid, names[mid], vip, len(names[mid])*7, 3+len(names[mid])%4, len(names[mid])%3*5))
		case "/x/web-interface/zone":
			return resp(`{"code":0,"data":{"addr":"203.0.113.8","country":"中国","province":"广东","city":"深圳","isp":"电信"}}`)
		case "/x/web-interface/view":
			bvid := r.URL.Query().Get("bvid")
			if bvid == "BV1xx411c7XX" {
				return resp(`{"code":-404,"message":"啥都木有"}`)
			}
			return resp(fmt.Sprintf(`{"code":0,"data":{"bvid":%q,"title":"演示视频 %s：一段很长很长的标题，用来看看界面里会不会被截断","pic":"","duration":%d,"owner":{"name":"某个 UP 主"},"stat":{"view":%d}}}`,
				bvid, bvid[2:6], 200+len(bvid)*7, 1234567))
		case "/x/click-interface/web/heartbeat":
			time.Sleep(2 * time.Second)
			return resp(`{"code":0,"message":"0"}`)
		case "/x/passport-login/web/cookie/info":
			return resp(`{"code":0,"data":{"refresh":false,"timestamp":0}}`)
		case "/x/v2/space":
			return resp(fmt.Sprintf(`{"code":0,"data":{"card":{"space_tag":[{"type":"location","title":"IP属地：%s"}]}}}`, homes[r.URL.Query().Get("vmid")]))
		case "/x/passport-login/web/qrcode/generate":
			return resp(`{"code":0,"data":{"url":"https://passport.bilibili.com/h5-app/passport/login/scan?qrcode_key=demo","qrcode_key":"demo"}}`)
		case "/x/passport-login/web/qrcode/poll":
			return resp(`{"code":0,"data":{"code":86101}}`)
		case "/x/passport-tv-login/qrcode/auth_code":
			return resp(`{"code":0,"data":{"url":"https://passport.bilibili.com/x/passport-tv-login/h5/qrcode/auth?auth_code=demo","auth_code":"demo"}}`)
		case "/x/passport-tv-login/qrcode/poll":
			return resp(`{"code":86039}`)
		case "/x/dynamic/feed/draw/upload_bfs":
			return resp(`{"code":0,"data":{"image_url":"https://i0.hdslb.com/x.jpg","image_width":100,"image_height":100,"img_size":1}}`)
		case "/x/dynamic/feed/create/dyn":
			return resp(`{"code":0,"data":{"dyn_id_str":"1"}}`)
		case "/x/dynamic/feed/operate/remove":
			return resp(`{"code":0}`)
		}
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
	})
}

// newDemoEngine 准备一个有三个账号的数据目录，所有 B 站请求都交给 fakeBili
func newDemoEngine(t *testing.T, dir string) *Engine {
	e := New(Config{
		DataDir:  dir,
		Version:  "v0.6.0-demo",
		Password: "demo1234",
		Defaults: Settings{Videos: []string{"BV15HFVz1EvM", "BV1GJ411x7h7"}, DoDynamic: true, DynamicText: "今天天气不错", Interval: Duration(6 * time.Hour), Jitter: Duration(30 * time.Minute)},
	})
	e.cfg.Transport = fakeBili()
	removeDelay = func() time.Duration { return time.Second }
	for i, mid := range []string{"10001", "10002", "10003"} {
		if _, err := e.saveAccountCookies(map[string]string{"SESSDATA": "s", "bili_jct": "j", "DedeUserID": mid}); err != nil {
			t.Fatal(err)
		}
		e.updateSession(mid, func(s *session) {
			if mid != "10003" { // 第三个是旧版本登录的账号：没有 refresh_token，也没有授权
				s.RefreshToken = "rt"
				s.CookieExpires = time.Now().Add(time.Duration(170-i*160) * 24 * time.Hour)
				s.AccessKey = "ak"
			}
		})
	}
	return e
}

// DEMO_ADDR=127.0.0.1:8099 go test -run TestServeDemo -timeout 0 ./core
func TestServeDemo(t *testing.T) {
	addr := os.Getenv("DEMO_ADDR")
	if addr == "" {
		t.Skip("设置 DEMO_ADDR=127.0.0.1:8099 启动演示面板（密码 demo1234）")
	}
	e := newDemoEngine(t, t.TempDir())
	e.Start()
	t.Fatal(e.ListenAndServe(addr))
}

// 演示数据跑完一轮后，状态里应该有硬币、Cookie 有效期和主页属地
func TestDemoStatus(t *testing.T) {
	e := newDemoEngine(t, t.TempDir())
	zone, err := fetchZone(e.anonClient())
	if err != nil {
		t.Fatal(err)
	}
	e.zone = &zone
	s := e.Settings()
	s.DoDynamic = false
	for _, mid := range []string{"10001", "10002"} {
		acc := e.newAccount(mid)
		acc.run(s)
		st := acc.status()
		if !st.LoggedIn || st.Last == nil || !st.Last.OK || st.Stats == nil || st.LastOK == nil || st.Home == nil {
			t.Fatalf("%s 的状态不全：%+v", mid, st)
		}
		if matched := st.Home.MatchedSince != nil; matched != (mid == "10001") {
			t.Errorf("%s 主页属地 %s，是否一致判断错了", mid, st.Home.Text)
		}
	}
	if v, err := e.videoInfo("BV1xx411c7XX"); err != errVideoNotFound {
		t.Errorf("不存在的视频应报错，得到 %+v %v", v, err)
	}
}
