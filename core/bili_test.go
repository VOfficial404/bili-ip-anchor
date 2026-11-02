package core

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func jsonResp(s string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(s)), Header: http.Header{}}
}

// fakeDynamic 记下上传的图片和发动态的请求体
type fakeDynamic struct {
	t        *testing.T
	uploaded [][]byte
	create   map[string]any
	removed  bool
}

func (f *fakeDynamic) RoundTrip(r *http.Request) (*http.Response, error) {
	switch r.URL.Path {
	case "/x/dynamic/feed/draw/upload_bfs":
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			f.t.Fatal(err)
		}
		if r.FormValue("biz") != "new_dyn" || r.FormValue("csrf") != "tok" {
			f.t.Errorf("上传表单字段不对：%v", r.MultipartForm.Value)
		}
		file, _, err := r.FormFile("file_up")
		if err != nil {
			f.t.Fatal(err)
		}
		data, _ := io.ReadAll(file)
		f.uploaded = append(f.uploaded, data)
		return jsonResp(fmt.Sprintf(`{"code":0,"data":{"image_url":"https://i0.hdslb.com/%d.jpg","image_width":687,"image_height":755,"img_size":68.0}}`, len(f.uploaded))), nil
	case "/x/dynamic/feed/create/dyn":
		json.NewDecoder(r.Body).Decode(&f.create)
		return jsonResp(`{"code":0,"data":{"dyn_id_str":"123"}}`), nil
	case "/x/dynamic/feed/operate/remove":
		f.removed = true
		return jsonResp(`{"code":0}`), nil
	}
	f.t.Fatalf("意外的请求：%s", r.URL)
	return nil, nil
}

func TestPostDynamic(t *testing.T) {
	removeDelay = func() time.Duration { return 0 }
	png := []byte("\x89PNG\r\n\x1a\nimage")

	cases := []struct {
		name       string
		text       string
		images     int
		wantScene  float64
		wantUpload int
		wantText   bool
	}{
		{"什么都没设置，用内置图片", "", 0, 2, 1, false},
		{"只有文字", "你好", 0, 1, 0, true},
		{"只有图片", "", 2, 2, 2, false},
		{"文字和图片", "你好", 3, 2, 3, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := testEngine(t)
			for i := 0; i < tc.images; i++ {
				if _, err := e.addImage(png); err != nil {
					t.Fatal(err)
				}
			}
			f := &fakeDynamic{t: t}
			c := newClient(f, map[string]string{"SESSDATA": "s", "bili_jct": "tok"})
			if err := e.postAndRemoveDynamic(c, "tok", tc.text, t.Logf); err != nil {
				t.Fatal(err)
			}
			if len(f.uploaded) != tc.wantUpload {
				t.Fatalf("上传了 %d 张图片，应为 %d", len(f.uploaded), tc.wantUpload)
			}
			if tc.images == 0 && tc.wantUpload == 1 && !bytes.Equal(f.uploaded[0], defaultDynamicImage) {
				t.Error("上传的不是内置图片")
			}
			req := f.create["dyn_req"].(map[string]any)
			if req["scene"].(float64) != tc.wantScene {
				t.Errorf("scene = %v，应为 %v", req["scene"], tc.wantScene)
			}
			pics, _ := req["pics"].([]any)
			if len(pics) != tc.wantUpload {
				t.Errorf("动态里有 %d 张图片，应为 %d", len(pics), tc.wantUpload)
			}
			contents := req["content"].(map[string]any)["contents"].([]any)
			if tc.wantText != (len(contents) == 1 && contents[0].(map[string]any)["raw_text"] == tc.text) {
				t.Errorf("动态文字不对：%v", contents)
			}
			if !f.removed {
				t.Error("没有删除动态")
			}
		})
	}
}

// 按 bilibili-API-collect 文档的流程，在假的 passport 上刷新一次 Cookie
func TestCookieRefresh(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	old := refreshPublicKey
	refreshPublicKey = &key.PublicKey
	defer func() { refreshPublicKey = old }()

	const ts = 1684466082562
	exp := time.Now().Add(180 * 24 * time.Hour).UTC().Truncate(time.Second)
	var confirmed bool
	fake := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		cookie := func(name string) string {
			c, _ := r.Cookie(name)
			if c == nil {
				return ""
			}
			return c.Value
		}
		switch {
		case r.URL.Path == "/x/passport-login/web/cookie/info":
			return jsonResp(fmt.Sprintf(`{"code":0,"data":{"refresh":true,"timestamp":%d}}`, ts)), nil
		case strings.HasPrefix(r.URL.Path, "/correspond/1/"):
			ct, _ := hex.DecodeString(strings.TrimPrefix(r.URL.Path, "/correspond/1/"))
			plain, err := rsa.DecryptOAEP(sha256.New(), nil, key, ct, nil)
			if err != nil || string(plain) != fmt.Sprintf("refresh_%d", ts) {
				return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`<html><div id="1-name">csrf123</div></html>`)), Header: http.Header{}}, nil
		case r.URL.Path == "/x/passport-login/web/cookie/refresh":
			r.ParseForm()
			if r.PostForm.Get("refresh_csrf") != "csrf123" || r.PostForm.Get("refresh_token") != "old-rt" || r.PostForm.Get("csrf") != "jct1" || cookie("SESSDATA") != "s1" {
				t.Errorf("刷新请求不对：%v", r.PostForm)
				return jsonResp(`{"code":86095}`), nil
			}
			resp := jsonResp(`{"code":0,"data":{"refresh_token":"new-rt"}}`)
			for _, c := range []*http.Cookie{
				{Name: "SESSDATA", Value: "s2", Expires: exp},
				{Name: "bili_jct", Value: "jct2", Expires: exp},
			} {
				resp.Header.Add("Set-Cookie", c.String())
			}
			return resp, nil
		case r.URL.Path == "/x/passport-login/web/confirm/refresh":
			r.ParseForm()
			// 确认时用新 Cookie 和旧 refresh_token
			if cookie("SESSDATA") != "s2" || r.PostForm.Get("csrf") != "jct2" || r.PostForm.Get("refresh_token") != "old-rt" {
				t.Errorf("确认请求不对：SESSDATA=%s %v", cookie("SESSDATA"), r.PostForm)
			}
			confirmed = true
			return jsonResp(`{"code":0}`), nil
		}
		t.Fatalf("意外的请求：%s", r.URL)
		return nil, nil
	})

	e := testEngine(t)
	cookies := map[string]string{"SESSDATA": "s1", "bili_jct": "jct1", "DedeUserID": "7"}
	if _, err := e.saveAccountCookies(cookies); err != nil {
		t.Fatal(err)
	}
	acc := e.newAccount("7")

	// 没有 refresh_token 时不刷新，手动刷新报错
	if done, err := e.checkCookie(acc, newClient(fake, cookies), false); done || err != nil {
		t.Fatalf("没有 refresh_token 时应跳过：%v %v", done, err)
	}
	if _, err := e.checkCookie(acc, newClient(fake, cookies), true); err != errNoRefreshToken {
		t.Fatalf("手动刷新应提示重新扫码，得到 %v", err)
	}

	e.updateSession("7", func(s *session) { s.RefreshToken = "old-rt" })
	done, err := e.checkCookie(acc, newClient(fake, cookies), false)
	if err != nil || !done {
		t.Fatalf("刷新失败：%v %v", done, err)
	}
	if !confirmed {
		t.Error("没有确认刷新")
	}
	saved, _ := e.loadAccountCookies("7")
	if saved["SESSDATA"] != "s2" || saved["bili_jct"] != "jct2" || saved["DedeUserID"] != "7" {
		t.Errorf("保存的新 Cookie 不对：%v", saved)
	}
	sess := e.loadSession("7")
	if sess.RefreshToken != "new-rt" || !sess.CookieExpires.Equal(exp) {
		t.Errorf("保存的 session 不对：%+v", sess)
	}
}

func TestAppSign(t *testing.T) {
	q := appSign(url.Values{"vmid": {"7"}, "access_key": {"k"}}, "key", "sec")
	i := strings.LastIndex(q, "&sign=")
	body, sign := q[:i], q[i+len("&sign="):]
	sum := md5.Sum([]byte(body + "sec"))
	if sign != hex.EncodeToString(sum[:]) {
		t.Errorf("签名不对：%s", q)
	}
	if !strings.HasPrefix(body, "access_key=k&appkey=key&ts=") || !strings.HasSuffix(body, "&vmid=7") {
		t.Errorf("参数应按键排序：%s", body)
	}
}

func TestSameRegion(t *testing.T) {
	for _, c := range []struct {
		home, target string
		want         bool
	}{
		{"广东", "广东", true},
		{"广东省", "广东", true},
		{"日本", "日本", true},
		{"北京", "广东", false},
		{"", "广东", false},
	} {
		if got := sameRegion(c.home, c.target); got != c.want {
			t.Errorf("sameRegion(%q, %q) = %v", c.home, c.target, got)
		}
	}
	if r := (zoneInfo{Country: "中国", Province: "广东", City: "深圳"}).region(); r != "广东" {
		t.Errorf("region = %s", r)
	}
	if r := (zoneInfo{Country: "日本", Province: "东京都"}).region(); r != "日本" {
		t.Errorf("region = %s", r)
	}
}

// B 站的请求要经过设置里的代理；http 和 socks5 两种都试
func TestProxyClient(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":0,"data":{"addr":"1.2.3.4","country":"中国","province":"广东"}}`))
	}))
	defer target.Close()

	httpHits := 0
	httpProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpHits++
		resp, err := http.Get(r.URL.String()) // 代理收到的是完整地址
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		io.Copy(w, resp.Body)
	}))
	defer httpProxy.Close()

	socksAddr, socksHits := startSOCKS5(t)

	for _, tc := range []struct {
		proxy string
		hits  *int
	}{
		{httpProxy.URL, &httpHits},
		{"socks5://" + socksAddr, socksHits},
	} {
		e := testEngine(t)
		s := e.Settings()
		s.Proxy = tc.proxy
		if err := e.UpdateSettings(s); err != nil {
			t.Fatal(err)
		}
		var r struct {
			Data zoneInfo `json:"data"`
		}
		if err := e.anonClient().do("GET", target.URL, nil, nil, &r); err != nil {
			t.Fatalf("%s：%v", tc.proxy, err)
		}
		if r.Data.Province != "广东" || *tc.hits != 1 {
			t.Errorf("%s：请求没有经过代理（%d 次），得到 %+v", tc.proxy, *tc.hits, r.Data)
		}
	}
}

// startSOCKS5 开一个只支持无认证 CONNECT 的最小 SOCKS5 代理
func startSOCKS5(t *testing.T) (string, *int) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	hits := new(int)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			*hits++
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 262)
				// 问候：VER NMETHODS METHODS
				if _, err := io.ReadFull(c, buf[:2]); err != nil {
					return
				}
				io.ReadFull(c, buf[:buf[1]])
				c.Write([]byte{5, 0})
				// 请求：VER CMD RSV ATYP ADDR PORT
				if _, err := io.ReadFull(c, buf[:4]); err != nil {
					return
				}
				var host string
				switch buf[3] {
				case 1:
					io.ReadFull(c, buf[:4])
					host = net.IP(buf[:4]).String()
				case 3:
					io.ReadFull(c, buf[:1])
					n := int(buf[0])
					io.ReadFull(c, buf[:n])
					host = string(buf[:n])
				default:
					return
				}
				io.ReadFull(c, buf[:2])
				port := binary.BigEndian.Uint16(buf[:2])
				up, err := net.Dial("tcp", net.JoinHostPort(host, fmt.Sprint(port)))
				if err != nil {
					c.Write([]byte{5, 1, 0, 1, 0, 0, 0, 0, 0, 0})
					return
				}
				defer up.Close()
				c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
				go io.Copy(up, c)
				io.Copy(c, up)
			}(c)
		}
	}()
	return ln.Addr().String(), hits
}
