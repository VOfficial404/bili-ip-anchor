package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path"
	"strings"
	"time"
)

// 这些类型与 WebUI 的 /api/status 对应，见 core/engine.go 和 core/accounts.go

type runResult struct {
	At    time.Time `json:"at"`
	OK    bool      `json:"ok"`
	Error string    `json:"error"`
}

type stats struct {
	Coins  float64 `json:"coins"`
	BCoins float64 `json:"bcoins"`
	Level  int     `json:"level"`
	VIP    string  `json:"vip"`
}

type homeLocation struct {
	Text         string     `json:"text"`
	CheckedAt    time.Time  `json:"checked_at"`
	MatchedSince *time.Time `json:"matched_since"`
	Error        string     `json:"error"`
}

type cookieStatus struct {
	Expires    *time.Time `json:"expires"`
	CanRefresh bool       `json:"can_refresh"`
}

type account struct {
	Mid      string        `json:"mid"`
	Uname    string        `json:"uname"`
	Face     string        `json:"face"`
	LoggedIn bool          `json:"logged_in"`
	Running  bool          `json:"running"`
	Busy     string        `json:"busy"`
	Last     *runResult    `json:"last"`
	LastOK   *time.Time    `json:"last_ok"`
	Next     *time.Time    `json:"next"`
	Stats    *stats        `json:"stats"`
	Cookie   cookieStatus  `json:"cookie"`
	AppAuth  bool          `json:"app_auth"`
	Home     *homeLocation `json:"home"`
	Fav      *favImport    `json:"fav_import"` // v0.8 起有
}

// favImport 是导入收藏的进度，见 core/favorites.go 的 FavImport
type favImport struct {
	Running  bool   `json:"running"`
	Folder   string `json:"folder"`
	Total    int    `json:"total"`
	Done     int    `json:"done"`
	Added    int    `json:"added"`
	Skipped  int    `json:"skipped"`
	Failed   int    `json:"failed"`
	Failures []struct {
		BVID   string `json:"bvid"`
		Title  string `json:"title"`
		Reason string `json:"reason"`
	} `json:"failures"`
	Created  []string   `json:"created"`
	Error    string     `json:"error"`
	Finished *time.Time `json:"finished"`
}

// favFile 是导出的收藏夹文件里导入前要显示的部分
type favFile struct {
	Format  string `json:"format"`
	Mid     string `json:"mid"`
	Uname   string `json:"uname"`
	Folders []struct {
		Title  string `json:"title"`
		Videos []struct {
			Invalid bool `json:"invalid"`
		} `json:"videos"`
	} `json:"folders"`
}

const favFormat = "bili-ip-anchor/favorites"

// parseFavFile 检查是不是导出的收藏夹文件，返回来源账号、收藏夹数和要导入的视频数
func parseFavFile(data []byte) (from string, folders, videos int, err error) {
	var f favFile
	if json.Unmarshal(data, &f) != nil || f.Format != favFormat {
		return "", 0, 0, errors.New("不是 bili-ip-anchor 导出的收藏夹文件")
	}
	for _, d := range f.Folders {
		for _, v := range d.Videos {
			if !v.Invalid {
				videos++
			}
		}
	}
	from = f.Uname
	if from == "" {
		from = "UID " + f.Mid
	}
	return from, len(f.Folders), videos, nil
}

type zone struct {
	Addr     string `json:"addr"`
	Country  string `json:"country"`
	Province string `json:"province"`
	City     string `json:"city"`
	ISP      string `json:"isp"`
}

type settings struct {
	Videos      []string `json:"videos"`
	DoDynamic   bool     `json:"do_dynamic"`
	DynamicText string   `json:"dynamic_text"`
	Interval    string   `json:"interval"`
	Jitter      string   `json:"jitter"`
	Proxy       string   `json:"proxy"`
	LogLevel    string   `json:"log_level,omitempty"` // v0.7 起有：debug、info、warn、error
}

type imageInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type videoInfo struct {
	BVID     string `json:"bvid"`
	Title    string `json:"title"`
	Cover    string `json:"cover"`
	Owner    string `json:"owner"`
	Duration int    `json:"duration"`
	Views    int64  `json:"views"`
}

type status struct {
	Version       string      `json:"version"`
	Mode          string      `json:"mode"` // server 或 local；v0.5 及更早的服务器没有这个字段
	Accounts      []account   `json:"accounts"`
	Zone          *zone       `json:"zone"`
	ZoneError     string      `json:"zone_error"`
	ZoneCheckedAt *time.Time  `json:"zone_checked_at"`
	Settings      settings    `json:"settings"`
	Images        []imageInfo `json:"images"`
	Logs          []string    `json:"logs"`      // v0.6 及更早的服务器只有这个
	LogLevel      string      `json:"log_level"` // v0.7 起有，同时有 /api/logs
}

// logEntry 是 /api/logs 返回的一条日志
type logEntry struct {
	Seq     int64     `json:"seq"`
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Mid     string    `json:"mid"`
	Account string    `json:"account"`
	Msg     string    `json:"msg"`
}

var errUnauthorized = errors.New("面板密码不对")

// client 用 WebUI 的接口操作服务器，和浏览器里的面板一样：
// 先用密码换一个会话 Cookie，之后的请求都带着它；会话过期时自动用密码重新登录一次。
// 本机模式下连的是程序自己开在 127.0.0.1 上的 WebUI。
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
	return &client{base: base, password: password, http: &http.Client{Timeout: 30 * time.Second, Jar: jar}}, nil
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

func (c *client) send(method, path string, body []byte, ctype string) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, c.base+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Requested-With", "bia") // 服务器要求写操作带它，用来防 CSRF
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连不上 %s：%w", c.base, err)
	}
	return resp, nil
}

// do 发出请求；会话失效时重新登录后再试一次。返回状态码为 200 的响应。
func (c *client) do(method, path string, body []byte, ctype string) (*http.Response, error) {
	resp, err := c.send(method, path, body, ctype)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && path != "/api/session" {
		resp.Body.Close()
		if err := c.login(); err != nil {
			return nil, err
		}
		if resp, err = c.send(method, path, body, ctype); err != nil {
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
		return fmt.Errorf("服务器不支持这个操作（HTTP %d），可能是旧版本，请更新服务器", resp.StatusCode)
	}
	return fmt.Errorf("请求失败（HTTP %d）", resp.StatusCode)
}

func (c *client) doJSON(method, path string, in, out any) error {
	var body []byte
	ctype := ""
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body, ctype = b, "application/json"
	}
	resp, err := c.do(method, path, body, ctype)
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

// bytes 发出请求并读出响应体，用于二维码和图片
func (c *client) bytes(method, path string) ([]byte, error) {
	resp, err := c.do(method, path, nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 16<<20))
}

func (c *client) login() error {
	return c.doJSON("POST", "/api/session", map[string]string{"password": c.password}, nil)
}

// logout 让服务器忘掉这个面板会话，失败也无所谓
func (c *client) logout() {
	if resp, err := c.send("DELETE", "/api/session", nil, ""); err == nil {
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

func accountPath(mid, action string) string {
	p := "/api/accounts/" + url.PathEscape(mid)
	if action != "" {
		p += "/" + action
	}
	return p
}

// accountAction 对一个账号做一件事，例如 POST run、POST check、DELETE appauth
func (c *client) accountAction(method, mid, action string) error {
	return c.doJSON(method, accountPath(mid, action), nil, nil)
}

func (c *client) saveSettings(s settings) (settings, error) {
	var out settings
	err := c.doJSON("PUT", "/api/settings", s, &out)
	return out, err
}

// startQR 让服务器向 B 站申请一张二维码，返回 PNG。mid 为空时是登录新账号，否则是给这个账号授权查询主页属地。
func (c *client) startQR(mid string) ([]byte, error) {
	if mid == "" {
		return c.bytes("POST", "/api/qr")
	}
	return c.bytes("POST", accountPath(mid, "appauth"))
}

func (c *client) pollQR(mid string) (state, newMid string, err error) {
	var r struct {
		State string `json:"state"`
		Mid   string `json:"mid"`
	}
	path := "/api/qr"
	if mid != "" {
		path = accountPath(mid, "appauth")
	}
	err = c.doJSON("GET", path, nil, &r)
	return r.State, r.Mid, err
}

func (c *client) video(bvid string) (videoInfo, error) {
	var v videoInfo
	err := c.doJSON("GET", "/api/videos/"+url.PathEscape(bvid), nil, &v)
	return v, err
}

func (c *client) image(name string) ([]byte, error) {
	return c.bytes("GET", "/api/dynamic/images/"+url.PathEscape(name))
}

func (c *client) uploadImage(data []byte) error {
	resp, err := c.do("POST", "/api/dynamic/images", data, http.DetectContentType(data))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *client) removeImage(name string) error {
	return c.doJSON("DELETE", "/api/dynamic/images/"+url.PathEscape(name), nil, nil)
}

// logs 取 seq 在 after 之后的日志，按时间先后
func (c *client) logs(after int64) ([]logEntry, error) {
	var r struct {
		Entries []logEntry `json:"entries"`
	}
	err := c.doJSON("GET", fmt.Sprintf("/api/logs?after=%d&level=debug", after), nil, &r)
	return r.Entries, err
}

// exportFavorites 读出账号的全部收藏夹，返回服务器给的文件名和文件内容
func (c *client) exportFavorites(mid string) (string, []byte, error) {
	// 服务器要一页页读完所有收藏夹，可能超过平常的 30 秒
	slow, hc := *c, *c.http
	hc.Timeout = 5 * time.Minute
	slow.http = &hc
	resp, err := slow.do("GET", accountPath(mid, "favorites"), nil, "")
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return "", nil, err
	}
	name := "bili-favorites-" + mid + ".json"
	if _, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition")); err == nil && params["filename"] != "" {
		name = path.Base(params["filename"])
	}
	return name, data, nil
}

// importFavorites 把导出的文件交给服务器在后台导入，返回要处理的视频数；进度在账号的 fav_import 里
func (c *client) importFavorites(mid string, data []byte) (int, error) {
	resp, err := c.send("POST", accountPath(mid, "favorites"), data, "application/json")
	if err != nil {
		return 0, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		if err := c.login(); err != nil {
			return 0, err
		}
		if resp, err = c.send("POST", accountPath(mid, "favorites"), data, "application/json"); err != nil {
			return 0, err
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return 0, responseError(resp)
	}
	var r struct {
		Total int `json:"total"`
	}
	json.NewDecoder(resp.Body).Decode(&r)
	return r.Total, nil
}

func (c *client) testProxy(proxy string) (zone, error) {
	var z zone
	err := c.doJSON("POST", "/api/proxy/test", map[string]string{"proxy": proxy}, &z)
	return z, err
}

// fetchHdslb 下载 B 站图床上的图片（头像、视频封面）。只用来显示，不经过服务器，也不带账号信息。
func fetchHdslb(src, size string) ([]byte, error) {
	u := hdslbURL(src, size)
	if u == "" {
		return nil, errors.New("不是 B 站图床的图片")
	}
	return httpGetImage(u)
}

const (
	faceSize  = "@96w_96h_1c.png"
	coverSize = "@160w_100h_1c.png"
)

// hdslbURL 只接受 *.hdslb.com 的地址，加上缩略图参数
func hdslbURL(src, size string) string {
	u, err := url.Parse(src)
	if err != nil || !strings.HasSuffix(u.Hostname(), ".hdslb.com") {
		return ""
	}
	u.Scheme = "https"
	return u.String() + size
}
