package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36"

var (
	errNoCookies = errors.New("还没有登录会话，请先扫码登录")
	errLoggedOut = errors.New("登录已失效，请重新扫码登录")
)

type logFunc func(format string, args ...any)

type apiResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type biliClient struct {
	http    *http.Client
	cookies map[string]string
}

// transport 按代理地址缓存，换代理时用新的连接池
func (e *Engine) transport(proxy string) *http.Transport {
	e.mu.Lock()
	defer e.mu.Unlock()
	if t, ok := e.transports[proxy]; ok {
		return t
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	if u, err := ParseProxy(proxy); err == nil && u != nil {
		t.Proxy = http.ProxyURL(u)
	}
	e.transports[proxy] = t
	return t
}

// client 返回以账号身份请求 B 站的客户端，按当前设置走代理
func (e *Engine) client(cookies map[string]string) *biliClient {
	return newClient(e.roundTripper(e.Settings().Proxy), cookies)
}

// anonClient 不带账号，用于查询出口 IP、视频信息等
func (e *Engine) anonClient() *biliClient { return e.client(nil) }

func newClient(t http.RoundTripper, cookies map[string]string) *biliClient {
	return &biliClient{http: &http.Client{Timeout: 20 * time.Second, Transport: t}, cookies: cookies}
}

// form 和 body 二选一：form 按表单提交，body 按 JSON 提交
func (c *biliClient) do(method, rawURL string, form url.Values, body any, out any) error {
	_, err := c.doResp(method, rawURL, form, body, out)
	return err
}

// doResp 和 do 一样，另外返回响应（Body 已读完），用来取 Set-Cookie
func (c *biliClient) doResp(method, rawURL string, form url.Values, body any, out any) (*http.Response, error) {
	var rd io.Reader
	ctype := ""
	if form != nil {
		rd = strings.NewReader(form.Encode())
		ctype = "application/x-www-form-urlencoded"
	}
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
		ctype = "application/json"
	}
	req, err := http.NewRequest(method, rawURL, rd)
	if err != nil {
		return nil, err
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	return c.send(req, out)
}

// send 补上通用请求头和 Cookie，发出请求并解析 JSON 响应
func (c *biliClient) send(req *http.Request, out any) (*http.Response, error) {
	resp, body, err := c.raw(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return resp, fmt.Errorf("%s %s: HTTP %d", req.Method, req.URL.Host+req.URL.Path, resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return resp, fmt.Errorf("%s %s: 返回的不是 JSON", req.Method, req.URL.Host+req.URL.Path)
	}
	return resp, nil
}

// raw 发出请求并读完响应
func (c *biliClient) raw(req *http.Request) (*http.Response, []byte, error) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", ua)
	}
	if req.Header.Get("Referer") == "" {
		req.Header.Set("Referer", "https://www.bilibili.com/")
	}
	for k, v := range c.cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return resp, body, err
}

type navData struct {
	IsLogin   bool    `json:"isLogin"`
	Mid       int64   `json:"mid"`
	Uname     string  `json:"uname"`
	Face      string  `json:"face"`
	Money     float64 `json:"money"`
	VipStatus int     `json:"vipStatus"`
	VipLabel  struct {
		Text string `json:"text"`
	} `json:"vip_label"`
	LevelInfo struct {
		Level int `json:"current_level"`
	} `json:"level_info"`
	Wallet struct {
		BCoin float64 `json:"bcoin_balance"`
	} `json:"wallet"`
}

// nav 确认登录状态，顺便拿到资料和硬币等数据
func nav(c *biliClient) (navData, error) {
	var r struct {
		Data navData `json:"data"`
	}
	if err := c.do("GET", "https://api.bilibili.com/x/web-interface/nav", nil, nil, &r); err != nil {
		return navData{}, err
	}
	if !r.Data.IsLogin {
		return navData{}, errLoggedOut
	}
	return r.Data, nil
}

func (n navData) stats() *Stats {
	s := &Stats{Coins: n.Money, BCoins: n.Wallet.BCoin, Level: n.LevelInfo.Level, UpdatedAt: time.Now()}
	if n.VipStatus == 1 {
		s.VIP = n.VipLabel.Text
		if s.VIP == "" {
			s.VIP = "大会员"
		}
	}
	return s
}

// navAndSave 请求 nav 并更新账号资料
func (acc *account) navAndSave(c *biliClient) error {
	n, err := nav(c)
	if err != nil {
		return err
	}
	acc.updateProfile(func(p *profile) {
		p.Uname, p.Face, p.Stats = n.Uname, n.Face, n.stats()
	})
	return nil
}

// runAccount 以账号身份发一轮请求，让 B 站记录到本机（或代理）的出口 IP
func (e *Engine) runAccount(acc *account, s Settings) error {
	cookies, err := e.loadAccountCookies(acc.mid)
	if err != nil {
		return err
	}
	c := e.client(cookies)
	if err := acc.navAndSave(c); err != nil {
		return err
	}
	acc.logf("登录正常")

	if refreshed, err := e.checkCookie(acc, c, false); err != nil {
		acc.logf("检查 Cookie 失败：%v", err)
	} else if refreshed {
		if cookies, err = e.loadAccountCookies(acc.mid); err != nil {
			return err
		}
		c = e.client(cookies)
	}

	bvid := s.Videos[rand.Intn(len(s.Videos))]
	var hb apiResp
	if err := c.do("POST", "https://api.bilibili.com/x/click-interface/web/heartbeat", url.Values{
		"bvid":        {bvid},
		"played_time": {strconv.Itoa(30 + rand.Intn(271))},
		"csrf":        {cookies["bili_jct"]},
	}, nil, &hb); err != nil {
		return err
	}
	acc.logf("心跳 %s：%d %s", bvid, hb.Code, hb.Message)
	if hb.Code != 0 {
		return fmt.Errorf("心跳被拒绝：%d %s", hb.Code, hb.Message)
	}
	now := time.Now()
	acc.updateProfile(func(p *profile) { p.LastOK = &now })

	if s.DoDynamic {
		if err := e.postAndRemoveDynamic(c, cookies["bili_jct"], s.DynamicText, acc.logf); err != nil {
			return err
		}
	}
	if e.loadSession(acc.mid).AccessKey != "" {
		e.checkHome(acc)
	}
	return nil
}

// checkAccount 不发心跳，只刷新资料、检查 Cookie 和主页属地
func (e *Engine) checkAccount(acc *account) error {
	if !acc.begin("检查") {
		return errBusy
	}
	defer acc.end()
	cookies, err := e.loadAccountCookies(acc.mid)
	if err == nil {
		c := e.client(cookies)
		if err = acc.navAndSave(c); err == nil {
			if _, cerr := e.checkCookie(acc, c, false); cerr != nil {
				acc.logf("检查 Cookie 失败：%v", cerr)
			}
		}
	}
	acc.markLogin(err)
	if err != nil {
		return err
	}
	if e.loadSession(acc.mid).AccessKey != "" {
		e.checkHome(acc)
	}
	acc.logf("已刷新资料")
	return nil
}

var errBusy = errors.New("这个账号正在运行，等这一轮结束再试")

// logout 让 B 站注销这份会话
func logout(c *biliClient, cookies map[string]string) error {
	var r apiResp
	if err := c.do("POST", "https://passport.bilibili.com/login/exit/v2",
		url.Values{"biliCSRF": {cookies["bili_jct"]}}, nil, &r); err != nil {
		return err
	}
	if r.Code != 0 {
		return fmt.Errorf("%d %s", r.Code, r.Message)
	}
	return nil
}

type zoneInfo struct {
	Addr     string `json:"addr"`
	Country  string `json:"country"`
	Province string `json:"province"`
	City     string `json:"city"`
	ISP      string `json:"isp"`
}

// region 是主页属地会显示的那一级：国内是省份，国外是国家
func (z zoneInfo) region() string {
	if z.Country == "中国" && z.Province != "" {
		return z.Province
	}
	return z.Country
}

// fetchZone 查询 B 站把出口 IP 识别为哪里
func fetchZone(c *biliClient) (zoneInfo, error) {
	var r struct {
		apiResp
		Data zoneInfo `json:"data"`
	}
	if err := c.do("GET", "https://api.bilibili.com/x/web-interface/zone", nil, nil, &r); err != nil {
		return zoneInfo{}, err
	}
	if r.Code != 0 {
		return zoneInfo{}, fmt.Errorf("%d %s", r.Code, r.Message)
	}
	return r.Data, nil
}

// TestProxy 经过 proxy 查询出口 IP 归属，用来确认代理可用
func (e *Engine) TestProxy(proxy string) (zoneInfo, error) {
	if _, err := ParseProxy(proxy); err != nil {
		return zoneInfo{}, err
	}
	z, err := fetchZone(newClient(e.roundTripper(strings.TrimSpace(proxy)), nil))
	if err != nil {
		return z, fmt.Errorf("经过代理访问 B 站失败：%w", err)
	}
	return z, nil
}

func (e *Engine) roundTripper(proxy string) http.RoundTripper {
	if e.testTransport != nil {
		return e.testTransport
	}
	return e.transport(proxy)
}
