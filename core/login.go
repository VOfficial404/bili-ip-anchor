package core

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"rsc.io/qr"
)

type QRState string

const (
	QRNone    QRState = "none"
	QRWaiting QRState = "waiting"
	QRScanned QRState = "scanned"
	QRExpired QRState = "expired"
	QRSuccess QRState = "success"
)

func qrPNG(content string) ([]byte, error) {
	code, err := qr.Encode(content, qr.M)
	if err != nil {
		return nil, err
	}
	code.Scale = 8
	return code.PNG(), nil
}

/* 网页扫码登录：得到 Cookie 和 refresh_token */

// QRLogin 是一次网页扫码登录
type QRLogin struct {
	URL    string // 二维码内容
	e      *Engine
	key    string
	client *biliClient
}

// NewQRLogin 向 B 站申请一张登录二维码。请求按当前设置走代理，登录 IP 也是出口 IP。
func (e *Engine) NewQRLogin() (*QRLogin, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	c := e.anonClient()
	c.http.Jar = jar

	var gen struct {
		Data struct {
			URL string `json:"url"`
			Key string `json:"qrcode_key"`
		} `json:"data"`
	}
	if err := c.do("GET", "https://passport.bilibili.com/x/passport-login/web/qrcode/generate", nil, nil, &gen); err != nil {
		return nil, err
	}
	if gen.Data.Key == "" {
		return nil, errors.New("获取二维码失败，B 站接口可能已变化")
	}
	return &QRLogin{URL: gen.Data.URL, e: e, key: gen.Data.Key, client: c}, nil
}

func (q *QRLogin) PNG() ([]byte, error) { return qrPNG(q.URL) }

// Poll 查询一次扫码状态；成功时保存会话并返回账号 UID
func (q *QRLogin) Poll() (QRState, string, error) {
	var poll struct {
		Data struct {
			Code         int    `json:"code"`
			URL          string `json:"url"`
			RefreshToken string `json:"refresh_token"`
		} `json:"data"`
	}
	resp, err := q.client.doResp("GET", "https://passport.bilibili.com/x/passport-login/web/qrcode/poll?qrcode_key="+url.QueryEscape(q.key), nil, nil, &poll)
	if err != nil {
		return QRNone, "", err
	}
	switch poll.Data.Code {
	case 0:
	case 86090:
		return QRScanned, "", nil
	case 86038:
		return QRExpired, "", nil
	default:
		return QRWaiting, "", nil
	}

	u, _ := url.Parse("https://api.bilibili.com/")
	cookies := map[string]string{}
	for _, c := range q.client.http.Jar.Cookies(u) {
		cookies[c.Name] = c.Value
	}
	expires := cookieExpiry(resp.Cookies())
	if expires.IsZero() {
		expires = expiryFromURL(poll.Data.URL)
	}
	mid, err := q.e.saveAccountCookies(cookies)
	if err != nil {
		return QRSuccess, "", err
	}
	err = q.e.updateSession(mid, func(s *session) {
		s.RefreshToken, s.CookieExpires = poll.Data.RefreshToken, expires
	})
	return QRSuccess, mid, err
}

// cookieExpiry 取 SESSDATA 的过期时间
func cookieExpiry(cs []*http.Cookie) time.Time {
	for _, c := range cs {
		if c.Name != "SESSDATA" {
			continue
		}
		if c.MaxAge > 0 {
			return time.Now().Add(time.Duration(c.MaxAge) * time.Second)
		}
		return c.Expires
	}
	return time.Time{}
}

// expiryFromURL 从扫码成功返回的跨域登录链接里取 Expires（Unix 秒）
func expiryFromURL(raw string) time.Time {
	u, err := url.Parse(raw)
	if err != nil {
		return time.Time{}
	}
	sec, err := strconv.ParseInt(u.Query().Get("Expires"), 10, 64)
	if err != nil || sec <= 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}

func (e *Engine) startQR() ([]byte, error) {
	q, err := e.NewQRLogin()
	if err != nil {
		return nil, err
	}
	png, err := q.PNG()
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	e.qr = q
	e.mu.Unlock()
	return png, nil
}

func (e *Engine) pollQR() (QRState, string, error) {
	e.mu.Lock()
	q := e.qr
	e.mu.Unlock()
	if q == nil {
		return QRNone, "", nil
	}
	st, mid, err := q.Poll()
	if st == QRSuccess || st == QRExpired {
		e.mu.Lock()
		if e.qr == q {
			e.qr = nil
		}
		e.mu.Unlock()
	}
	if err != nil {
		return st, "", err
	}
	if st == QRSuccess {
		e.Logf("扫码登录成功：UID %s", mid)
		e.accountLoggedIn(mid)
	}
	return st, mid, nil
}

/* App（TV 端）授权：只用来查询个人主页的 IP 属地 */

const (
	tvAppKey = "4409e2ce8ffd12b8"
	tvAppSec = "59b43e04ad6965f34319062b478f83dd"
)

// appSign 按 App 接口的规则给参数签名，返回编码好的查询串
func appSign(v url.Values, key, sec string) string {
	v.Set("appkey", key)
	v.Set("ts", strconv.FormatInt(time.Now().Unix(), 10))
	q := v.Encode() // Encode 会按键排序
	sum := md5.Sum([]byte(q + sec))
	return q + "&sign=" + hex.EncodeToString(sum[:])
}

type tvLogin struct {
	code string
}

func (c *biliClient) postSigned(rawURL string, v url.Values, out any) error {
	req, err := http.NewRequest("POST", rawURL, strings.NewReader(appSign(v, tvAppKey, tvAppSec)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_, err = c.send(req, out)
	return err
}

// tvStart 为账号申请一张 App 授权二维码
func (e *Engine) tvStart(mid string) ([]byte, error) {
	if e.account(mid) == nil {
		return nil, errNoAccount
	}
	var r struct {
		apiResp
		Data struct {
			URL      string `json:"url"`
			AuthCode string `json:"auth_code"`
		} `json:"data"`
	}
	if err := e.anonClient().postSigned("https://passport.snm0516.aisee.tv/x/passport-tv-login/qrcode/auth_code", url.Values{"local_id": {"0"}}, &r); err != nil {
		return nil, err
	}
	if r.Code != 0 || r.Data.AuthCode == "" {
		return nil, fmt.Errorf("获取授权二维码失败：%d %s", r.Code, r.Message)
	}
	png, err := qrPNG(r.Data.URL)
	if err != nil {
		return nil, err
	}
	e.mu.Lock()
	e.tvqr[mid] = &tvLogin{code: r.Data.AuthCode}
	e.mu.Unlock()
	return png, nil
}

// tvPoll 查询一次授权状态；成功时保存 access_key 并立即查询主页属地
func (e *Engine) tvPoll(mid string) (QRState, error) {
	e.mu.Lock()
	t := e.tvqr[mid]
	e.mu.Unlock()
	acc := e.account(mid)
	if t == nil || acc == nil {
		return QRNone, nil
	}
	var r struct {
		apiResp
		Data struct {
			Mid         int64  `json:"mid"`
			AccessToken string `json:"access_token"`
			ExpiresIn   int64  `json:"expires_in"`
		} `json:"data"`
	}
	if err := e.anonClient().postSigned("https://passport.snm0516.aisee.tv/x/passport-tv-login/qrcode/poll", url.Values{"auth_code": {t.code}, "local_id": {"0"}}, &r); err != nil {
		return QRNone, err
	}
	var st QRState
	switch r.Code {
	case 0:
		st = QRSuccess
	case 86090:
		return QRScanned, nil
	case 86038:
		st = QRExpired
	case 86039:
		return QRWaiting, nil
	default:
		return QRNone, fmt.Errorf("授权失败：%d %s", r.Code, r.Message)
	}
	e.mu.Lock()
	if e.tvqr[mid] == t {
		delete(e.tvqr, mid)
	}
	e.mu.Unlock()
	if st == QRExpired {
		return st, nil
	}
	if got := strconv.FormatInt(r.Data.Mid, 10); got != mid {
		revokeAccessKey(e.anonClient(), r.Data.AccessToken)
		return QRNone, fmt.Errorf("扫码的是 UID %s，不是这个账号（UID %s）。请用这个账号的 App 扫码", got, mid)
	}
	if err := e.updateSession(mid, func(s *session) {
		s.AccessKey = r.Data.AccessToken
		s.AccessExpires = time.Now().Add(time.Duration(r.Data.ExpiresIn) * time.Second)
	}); err != nil {
		return QRNone, err
	}
	acc.logf("已授权查询主页属地")
	go e.checkHome(acc)
	return QRSuccess, nil
}

// removeAppAuth 撤销 App 授权并删除 access_key
func (e *Engine) removeAppAuth(mid string) error {
	acc := e.account(mid)
	if acc == nil {
		return errNoAccount
	}
	sess := e.loadSession(mid)
	if sess.AccessKey == "" {
		return nil
	}
	if err := revokeAccessKey(e.anonClient(), sess.AccessKey); err != nil {
		acc.logf("撤销 App 授权失败（本地仍会删除）：%v", err)
	}
	if err := e.updateSession(mid, func(s *session) { s.AccessKey, s.AccessExpires = "", time.Time{} }); err != nil {
		return err
	}
	acc.updateProfile(func(p *profile) { p.Home = nil })
	acc.logf("已取消主页属地授权")
	return nil
}

func revokeAccessKey(c *biliClient, key string) error {
	var r apiResp
	if err := c.postSigned("https://passport.bilibili.com/x/passport-login/revoke", url.Values{"access_key": {key}}, &r); err != nil {
		return err
	}
	if r.Code != 0 {
		return fmt.Errorf("%d %s", r.Code, r.Message)
	}
	return nil
}

// checkHome 用 App 接口查询账号个人主页显示的 IP 属地，结果写进资料
func (e *Engine) checkHome(acc *account) {
	sess := e.loadSession(acc.mid)
	if sess.AccessKey == "" {
		return
	}
	text, err := fetchHomeLocation(e.anonClient(), acc.mid, sess.AccessKey)
	if errors.Is(err, errAccessExpired) {
		e.updateSession(acc.mid, func(s *session) { s.AccessKey, s.AccessExpires = "", time.Time{} })
	}
	now := time.Now()
	target := ""
	if z := e.currentZone(); z != nil {
		target = z.region()
	}
	acc.updateProfile(func(p *profile) {
		h := &HomeLocation{CheckedAt: now}
		if p.Home != nil {
			*h = *p.Home
			h.CheckedAt = now
		}
		if err != nil {
			h.Error = err.Error()
			p.Home = h
			return
		}
		h.Text, h.Error = text, ""
		switch {
		case !sameRegion(text, target):
			h.MatchedSince = nil
		case h.MatchedSince == nil:
			h.MatchedSince = &now
		}
		p.Home = h
	})
	if err != nil {
		acc.logf("查询主页属地失败：%v", err)
		return
	}
	acc.logf("主页属地：%s", text)
}

func sameRegion(home, target string) bool {
	if home == "" || target == "" {
		return false
	}
	return strings.Contains(home, target) || strings.Contains(target, home)
}

var errAccessExpired = errors.New("App 授权已失效，请重新授权")

func fetchHomeLocation(c *biliClient, mid, accessKey string) (string, error) {
	v := url.Values{"vmid": {mid}, "access_key": {accessKey}, "build": {"7380300"}, "mobi_app": {"android"}, "platform": {"android"}}
	req, err := http.NewRequest("GET", "https://app.bilibili.com/x/v2/space?"+appSign(v, tvAppKey, tvAppSec), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 BiliDroid/7.38.0 (bbcallen@gmail.com) os/android mobi_app/android build/7380300")
	var r struct {
		apiResp
		Data struct {
			Card struct {
				SpaceTag []struct {
					Type  string `json:"type"`
					Title string `json:"title"`
				} `json:"space_tag"`
			} `json:"card"`
		} `json:"data"`
	}
	if _, err := c.send(req, &r); err != nil {
		return "", err
	}
	switch r.Code {
	case 0:
	case -101, -2, 61000:
		return "", errAccessExpired
	default:
		return "", fmt.Errorf("%d %s", r.Code, r.Message)
	}
	for _, t := range r.Data.Card.SpaceTag {
		if after, ok := strings.CutPrefix(t.Title, "IP属地"); ok {
			return strings.TrimSpace(strings.TrimLeft(after, "：:")), nil
		}
	}
	return "", errors.New("B 站没有返回 IP 属地")
}
