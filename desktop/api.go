package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// 这些类型与服务器 /api/status 的 JSON 对应，见 bili-ip-anchor 的 app.go 和 accounts.go

type runResult struct {
	At    time.Time `json:"at"`
	OK    bool      `json:"ok"`
	Error string    `json:"error"`
}

type account struct {
	Mid      string     `json:"mid"`
	Uname    string     `json:"uname"`
	Face     string     `json:"face"`
	LoggedIn bool       `json:"logged_in"`
	Running  bool       `json:"running"`
	Last     *runResult `json:"last"`
	Next     *time.Time `json:"next"`
}

type zone struct {
	Addr     string `json:"addr"`
	Country  string `json:"country"`
	Province string `json:"province"`
	City     string `json:"city"`
	ISP      string `json:"isp"`
}

type settings struct {
	BVID      string `json:"bvid"`
	DoDynamic bool   `json:"do_dynamic"`
	Interval  string `json:"interval"`
	Jitter    string `json:"jitter"`
}

type status struct {
	Version   string    `json:"version"`
	Accounts  []account `json:"accounts"`
	Zone      *zone     `json:"zone"`
	ZoneError string    `json:"zone_error"`
	Settings  settings  `json:"settings"`
	Logs      []string  `json:"logs"`
}

var errUnauthorized = errors.New("面板密码不对")

// client 用 WebUI 的接口操作服务器，和浏览器里的面板一样：
// 先用密码换一个会话 Cookie，之后的请求都带着它；会话过期时自动用密码重新登录一次。
type client struct {
	base     string
	password string
	http     *http.Client
}

func newClient(server, password string) (*client, error) {
	base, err := normalizeServer(server)
	if err != nil {
		return nil, err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &client{base: base, password: password, http: &http.Client{Timeout: 20 * time.Second, Jar: jar}}, nil
}

// normalizeServer 补上 http://，去掉末尾的斜杠
func normalizeServer(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("请填写服务器地址")
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("服务器地址不对：%s", s)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawQuery, u.Fragment = "", ""
	return u.String(), nil
}

func (c *client) send(method, path string, body []byte) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, c.base+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Requested-With", "bia") // 服务器要求写操作带它，用来防 CSRF
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连不上 %s：%w", c.base, err)
	}
	return resp, nil
}

// do 发出请求；会话失效时重新登录后再试一次。返回状态码为 200 的响应。
func (c *client) do(method, path string, in any) (*http.Response, error) {
	var body []byte
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}
		body = b
	}
	resp, err := c.send(method, path, body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && path != "/api/session" {
		resp.Body.Close()
		if err := c.login(); err != nil {
			return nil, err
		}
		if resp, err = c.send(method, path, body); err != nil {
			return nil, err
		}
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, responseError(resp)
	}
	return resp, nil
}

func responseError(resp *http.Response) error {
	if resp.StatusCode == http.StatusUnauthorized {
		return errUnauthorized
	}
	var e struct {
		Error string `json:"error"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&e) == nil && e.Error != "" {
		return errors.New(e.Error)
	}
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		return fmt.Errorf("这个地址不是 bili-ip-anchor 的 WebUI（HTTP %d）", resp.StatusCode)
	}
	return fmt.Errorf("请求失败（HTTP %d）", resp.StatusCode)
}

func (c *client) doJSON(method, path string, in, out any) error {
	resp, err := c.do(method, path, in)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("服务器的回应看不懂，可能不是 bili-ip-anchor：%w", err)
	}
	return nil
}

func (c *client) login() error {
	return c.doJSON("POST", "/api/session", map[string]string{"password": c.password}, nil)
}

// logout 让服务器忘掉这个面板会话，失败也无所谓
func (c *client) logout() {
	if resp, err := c.send("DELETE", "/api/session", nil); err == nil {
		resp.Body.Close()
	}
}

func (c *client) status() (status, error) {
	var s status
	err := c.doJSON("GET", "/api/status", nil, &s)
	return s, err
}

func (c *client) runAll() (int, error) {
	var r struct {
		Triggered int `json:"triggered"`
	}
	err := c.doJSON("POST", "/api/run", nil, &r)
	return r.Triggered, err
}

func (c *client) runAccount(mid string) error {
	return c.doJSON("POST", "/api/accounts/"+url.PathEscape(mid)+"/run", nil, nil)
}

func (c *client) removeAccount(mid string) error {
	return c.doJSON("DELETE", "/api/accounts/"+url.PathEscape(mid), nil, nil)
}

func (c *client) saveSettings(s settings) (settings, error) {
	var out settings
	err := c.doJSON("PUT", "/api/settings", s, &out)
	return out, err
}

// startQR 让服务器向 B 站申请一张登录二维码，返回 PNG
func (c *client) startQR() ([]byte, error) {
	resp, err := c.do("POST", "/api/qr", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

func (c *client) pollQR() (state, mid string, err error) {
	var r struct {
		State string `json:"state"`
		Mid   string `json:"mid"`
	}
	err = c.doJSON("GET", "/api/qr", nil, &r)
	return r.State, r.Mid, err
}

// fetchFace 下载头像缩略图。头像在 B 站的图床上，不经过服务器。
func fetchFace(face string) ([]byte, error) {
	u := faceURL(face)
	if u == "" {
		return nil, errors.New("不是 B 站图床的头像")
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 2<<20))
}

func faceURL(face string) string {
	u, err := url.Parse(face)
	if err != nil || !strings.HasSuffix(u.Hostname(), ".hdslb.com") {
		return ""
	}
	u.Scheme = "https"
	return u.String() + "@96w_96h_1c.png"
}
