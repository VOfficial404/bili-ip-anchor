package core

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

// 网页 Cookie 刷新，流程见 bilibili-API-collect 的 docs/login/cookie_refresh.md：
// cookie/info 判断是否需要刷新 → 用公钥加密 refresh_<毫秒时间戳> 得到 correspondPath →
// 从 correspond 页面取 refresh_csrf → cookie/refresh 换新 Cookie 和新 refresh_token →
// 用新 Cookie 和旧 refresh_token 调 confirm/refresh 让旧 Cookie 失效。

const refreshPublicKeyPEM = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDLgd2OAkcGVtoE3ThUREbio0Eg
Uc/prcajMKXvkCKFCWhJYJcLkcM2DKKcSeFpD/j6Boy538YXnR6VhcuUJOhH2x71
nzPjfdTcqMz7djHum0qSZA0AyCBDABUqCrfNgCiJ00Ra7GmRj+YCK1NJEuewlb40
JNrRuoEUXpabUzGB8QIDAQAB
-----END PUBLIC KEY-----`

// refreshPublicKey 测试里会换成自己生成的密钥
var refreshPublicKey = mustParsePublicKey(refreshPublicKeyPEM)

func mustParsePublicKey(s string) *rsa.PublicKey {
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		panic("公钥格式不对")
	}
	k, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		panic(err)
	}
	return k.(*rsa.PublicKey)
}

func correspondPath(ts int64) (string, error) {
	b, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, refreshPublicKey, []byte(fmt.Sprintf("refresh_%d", ts)), nil)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

var refreshCSRFRe = regexp.MustCompile(`<div id="1-name">([^<]+)</div>`)

var errNoRefreshToken = errors.New("这个账号是旧版本扫码登录的，没有 refresh_token，重新扫码一次之后才能刷新")

// checkCookie 询问 B 站 Cookie 是否需要刷新，需要（或 force）时刷新。返回是否刷新了。
func (e *Engine) checkCookie(acc *account, c *biliClient, force bool) (bool, error) {
	var info struct {
		apiResp
		Data struct {
			Refresh   bool  `json:"refresh"`
			Timestamp int64 `json:"timestamp"`
		} `json:"data"`
	}
	if err := c.do("GET", "https://passport.bilibili.com/x/passport-login/web/cookie/info?csrf="+url.QueryEscape(c.cookies["bili_jct"]), nil, nil, &info); err != nil {
		return false, err
	}
	switch info.Code {
	case 0:
	case -101:
		return false, errLoggedOut
	default:
		return false, fmt.Errorf("%d %s", info.Code, info.Message)
	}
	if !info.Data.Refresh && !force {
		return false, nil
	}
	sess := e.loadSession(acc.mid)
	if sess.RefreshToken == "" {
		if force {
			return false, errNoRefreshToken
		}
		acc.logf("B 站提示 Cookie 需要刷新，但这个账号没有 refresh_token，请重新扫码登录")
		return false, nil
	}
	ts := info.Data.Timestamp
	if ts == 0 {
		ts = time.Now().UnixMilli()
	}
	cookies, token, expires, err := refreshCookie(c, sess.RefreshToken, ts)
	if err != nil {
		return false, err
	}
	if _, err := e.saveAccountCookies(cookies); err != nil {
		return false, err
	}
	if err := e.updateSession(acc.mid, func(s *session) {
		s.RefreshToken = token
		if !expires.IsZero() {
			s.CookieExpires = expires
		}
	}); err != nil {
		return false, err
	}
	// 旧 Cookie 在确认之后才失效，新 Cookie 已经保存，确认失败也不影响使用
	if err := confirmRefresh(newClient(c.http.Transport, cookies), sess.RefreshToken); err != nil {
		acc.logf("确认 Cookie 刷新失败（新 Cookie 已经可用）：%v", err)
	}
	if expires.IsZero() {
		acc.logf("Cookie 已刷新")
	} else {
		acc.logf("Cookie 已刷新，有效期到 %s", expires.Format("2006-01-02"))
	}
	return true, nil
}

// refreshCookie 换一份新的 Cookie，返回新 Cookie、新 refresh_token 和新的过期时间
func refreshCookie(c *biliClient, refreshToken string, ts int64) (map[string]string, string, time.Time, error) {
	path, err := correspondPath(ts)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	req, err := http.NewRequest("GET", "https://www.bilibili.com/correspond/1/"+path, nil)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	resp, body, err := c.raw(req)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	m := refreshCSRFRe.FindSubmatch(body)
	if resp.StatusCode != http.StatusOK || m == nil {
		return nil, "", time.Time{}, fmt.Errorf("取 refresh_csrf 失败（HTTP %d）", resp.StatusCode)
	}

	var r struct {
		apiResp
		Data struct {
			RefreshToken string `json:"refresh_token"`
		} `json:"data"`
	}
	resp, err = c.doResp("POST", "https://passport.bilibili.com/x/passport-login/web/cookie/refresh", url.Values{
		"csrf":          {c.cookies["bili_jct"]},
		"refresh_csrf":  {string(m[1])},
		"source":        {"main_web"},
		"refresh_token": {refreshToken},
	}, nil, &r)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	if r.Code != 0 {
		if r.Code == 86095 {
			return nil, "", time.Time{}, errors.New("refresh_token 和 Cookie 对不上，请重新扫码登录")
		}
		return nil, "", time.Time{}, fmt.Errorf("刷新 Cookie 失败：%d %s", r.Code, r.Message)
	}
	cookies := maps.Clone(c.cookies)
	for _, ck := range resp.Cookies() {
		if ck.Value != "" {
			cookies[ck.Name] = ck.Value
		}
	}
	if cookies["SESSDATA"] == c.cookies["SESSDATA"] || r.Data.RefreshToken == "" {
		return nil, "", time.Time{}, errors.New("刷新 Cookie 的响应里没有新的会话，B 站接口可能已变化")
	}
	return cookies, r.Data.RefreshToken, cookieExpiry(resp.Cookies()), nil
}

func confirmRefresh(c *biliClient, oldToken string) error {
	var r apiResp
	if err := c.do("POST", "https://passport.bilibili.com/x/passport-login/web/confirm/refresh", url.Values{
		"csrf":          {c.cookies["bili_jct"]},
		"refresh_token": {oldToken},
	}, nil, &r); err != nil {
		return err
	}
	if r.Code != 0 {
		return fmt.Errorf("%d %s", r.Code, r.Message)
	}
	return nil
}

// refreshAccountCookie 手动刷新 Cookie
func (e *Engine) refreshAccountCookie(acc *account) error {
	if !acc.begin("刷新 Cookie") {
		return errBusy
	}
	defer acc.end()
	cookies, err := e.loadAccountCookies(acc.mid)
	if err != nil {
		return err
	}
	_, err = e.checkCookie(acc, e.client(cookies), true)
	acc.markLogin(err)
	return err
}
