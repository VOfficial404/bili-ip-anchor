package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"rsc.io/qr"
)

const ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36"

var (
	errNoCookies = errors.New("还没有登录会话，请先扫码登录")
	errLoggedOut = errors.New("登录已失效，请重新扫码登录")
)

type apiResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type biliClient struct {
	http    *http.Client
	cookies map[string]string
}

func newClient(cookies map[string]string) *biliClient {
	return &biliClient{http: &http.Client{Timeout: 15 * time.Second}, cookies: cookies}
}

// form 和 body 二选一：form 按表单提交，body 按 JSON 提交
func (c *biliClient) do(method, rawURL string, form url.Values, body any, out any) error {
	var rd io.Reader
	ctype := ""
	if form != nil {
		rd = strings.NewReader(form.Encode())
		ctype = "application/x-www-form-urlencoded"
	}
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
		ctype = "application/json"
	}
	req, err := http.NewRequest(method, rawURL, rd)
	if err != nil {
		return err
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	return c.send(req, out)
}

// send 补上通用请求头和 Cookie，发出请求并解析 JSON 响应
func (c *biliClient) send(req *http.Request, out any) error {
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", "https://www.bilibili.com/")
	for k, v := range c.cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s %s: HTTP %d", req.Method, req.URL, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type qrState string

const (
	qrNone    qrState = "none"
	qrWaiting qrState = "waiting"
	qrScanned qrState = "scanned"
	qrExpired qrState = "expired"
	qrSuccess qrState = "success"
)

type qrLogin struct {
	URL    string
	key    string
	client *biliClient
}

func qrStart() (*qrLogin, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	c := newClient(nil)
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
	return &qrLogin{URL: gen.Data.URL, key: gen.Data.Key, client: c}, nil
}

func (q *qrLogin) png() ([]byte, error) {
	code, err := qr.Encode(q.URL, qr.M)
	if err != nil {
		return nil, err
	}
	code.Scale = 8
	return code.PNG(), nil
}

// poll 查询一次扫码状态；成功时保存会话
func (q *qrLogin) poll() (qrState, error) {
	var poll struct {
		Data struct {
			Code int `json:"code"`
		} `json:"data"`
	}
	if err := q.client.do("GET", "https://passport.bilibili.com/x/passport-login/web/qrcode/poll?qrcode_key="+url.QueryEscape(q.key), nil, nil, &poll); err != nil {
		return qrNone, err
	}
	switch poll.Data.Code {
	case 0:
		return qrSuccess, saveCookies(q.client.http.Jar)
	case 86090:
		return qrScanned, nil
	case 86038:
		return qrExpired, nil
	default:
		return qrWaiting, nil
	}
}

func saveCookies(jar http.CookieJar) error {
	u, _ := url.Parse("https://api.bilibili.com/")
	saved := map[string]string{}
	for _, c := range jar.Cookies(u) {
		saved[c.Name] = c.Value
	}
	if saved["SESSDATA"] == "" || saved["bili_jct"] == "" {
		return errors.New("登录成功但没有拿到 SESSDATA/bili_jct，B 站接口可能已变化")
	}
	b, err := json.Marshal(saved)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return err
	}
	return os.WriteFile(cookieFile(), b, 0600)
}

func loadCookies() (map[string]string, error) {
	b, err := os.ReadFile(cookieFile())
	if errors.Is(err, os.ErrNotExist) {
		return nil, errNoCookies
	}
	if err != nil {
		return nil, err
	}
	var cookies map[string]string
	if err := json.Unmarshal(b, &cookies); err != nil {
		return nil, fmt.Errorf("解析 %s 失败：%w", cookieFile(), err)
	}
	return cookies, nil
}

// refresh 以账号身份发一轮请求，让 B 站记录到本机的出口 IP。
// 只要确认了登录状态就返回昵称，即使后续步骤失败。
func refresh(s settings) (string, error) {
	cookies, err := loadCookies()
	if err != nil {
		return "", err
	}
	c := newClient(cookies)
	csrf := cookies["bili_jct"]

	var nav struct {
		Data struct {
			IsLogin bool   `json:"isLogin"`
			Uname   string `json:"uname"`
		} `json:"data"`
	}
	if err := c.do("GET", "https://api.bilibili.com/x/web-interface/nav", nil, nil, &nav); err != nil {
		return "", err
	}
	if !nav.Data.IsLogin {
		return "", errLoggedOut
	}
	uname := nav.Data.Uname
	log.Printf("已登录：%s", uname)

	var hb apiResp
	if err := c.do("POST", "https://api.bilibili.com/x/click-interface/web/heartbeat", url.Values{
		"bvid":        {s.BVID},
		"played_time": {strconv.Itoa(30 + rand.Intn(271))},
		"csrf":        {csrf},
	}, nil, &hb); err != nil {
		return uname, err
	}
	log.Printf("心跳：%d %s", hb.Code, hb.Message)
	if hb.Code != 0 {
		return uname, fmt.Errorf("心跳被拒绝：%d %s", hb.Code, hb.Message)
	}

	if s.DoDynamic {
		return uname, postAndRemoveDynamic(c, csrf)
	}
	return uname, nil
}

//go:embed assets/dynamic.jpg
var defaultDynamicImage []byte

// dynamicImage 优先用数据目录里的 dynamic.jpg / dynamic.png，没有就用内置图片
func dynamicImage() (name string, data []byte, err error) {
	for _, n := range []string{"dynamic.jpg", "dynamic.png"} {
		data, err = os.ReadFile(filepath.Join(cfg.DataDir, n))
		if err == nil {
			return n, data, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", nil, err
		}
	}
	return "dynamic.jpg", defaultDynamicImage, nil
}

type uploadedPic struct {
	URL    string  `json:"image_url"`
	Width  int     `json:"image_width"`
	Height int     `json:"image_height"`
	Size   float64 `json:"img_size"` // KB
}

func (c *biliClient) uploadImage(csrf, name string, data []byte) (uploadedPic, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file_up", name)
	if err != nil {
		return uploadedPic{}, err
	}
	fw.Write(data)
	mw.WriteField("biz", "new_dyn")
	mw.WriteField("category", "daily")
	mw.WriteField("csrf", csrf)
	if err := mw.Close(); err != nil {
		return uploadedPic{}, err
	}
	req, err := http.NewRequest("POST", "https://api.bilibili.com/x/dynamic/feed/draw/upload_bfs", &buf)
	if err != nil {
		return uploadedPic{}, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	var r struct {
		apiResp
		Data uploadedPic `json:"data"`
	}
	if err := c.send(req, &r); err != nil {
		return uploadedPic{}, err
	}
	if r.Code != 0 || r.Data.URL == "" {
		return uploadedPic{}, fmt.Errorf("上传图片失败：%d %s", r.Code, r.Message)
	}
	return r.Data, nil
}

// removeDelay 发动态后等多久再删；测试里会改成 0
var removeDelay = func() time.Duration { return time.Duration(5+rand.Intn(16)) * time.Second }

func postAndRemoveDynamic(c *biliClient, csrf string) error {
	name, data, err := dynamicImage()
	if err != nil {
		return err
	}
	pic, err := c.uploadImage(csrf, name, data)
	if err != nil {
		return err
	}
	log.Printf("上传图片：%dx%d", pic.Width, pic.Height)

	var cr struct {
		apiResp
		Data struct {
			DynID string `json:"dyn_id_str"`
		} `json:"data"`
	}
	if err := c.do("POST", "https://api.bilibili.com/x/dynamic/feed/create/dyn?csrf="+url.QueryEscape(csrf), nil, map[string]any{
		"dyn_req": map[string]any{
			"content": map[string]any{"contents": []map[string]any{}},
			"pics": []map[string]any{{
				"img_src":    pic.URL,
				"img_width":  pic.Width,
				"img_height": pic.Height,
				"img_size":   pic.Size,
			}},
			"scene": 2, // 2 = 带图动态
			"meta":  map[string]any{"app_meta": map[string]any{"from": "create.dynamic.web", "mobi_app": "web"}},
		},
	}, &cr); err != nil {
		return err
	}
	log.Printf("发动态：%d %s", cr.Code, cr.Message)
	if cr.Data.DynID == "" {
		return fmt.Errorf("发动态失败：%d %s", cr.Code, cr.Message)
	}

	time.Sleep(removeDelay())
	var rr apiResp
	if err := c.do("POST", "https://api.bilibili.com/x/dynamic/feed/operate/remove?csrf="+url.QueryEscape(csrf), nil,
		map[string]any{"dyn_id_str": cr.Data.DynID}, &rr); err != nil {
		return err
	}
	log.Printf("删动态：%d %s", rr.Code, rr.Message)
	if rr.Code != 0 {
		return fmt.Errorf("删动态失败，请手动删除动态 %s：%d %s", cr.Data.DynID, rr.Code, rr.Message)
	}
	return nil
}

// logout 让 B 站注销这份会话，并删除本地文件。远端注销失败也会删除本地文件。
func logout() error {
	cookies, err := loadCookies()
	if errors.Is(err, errNoCookies) {
		return nil
	}
	if err == nil {
		var r apiResp
		err = newClient(cookies).do("POST", "https://passport.bilibili.com/login/exit/v2",
			url.Values{"biliCSRF": {cookies["bili_jct"]}}, nil, &r)
		if err == nil && r.Code != 0 {
			err = fmt.Errorf("%d %s", r.Code, r.Message)
		}
	}
	if err != nil {
		log.Printf("B 站注销会话失败（本地会话仍会删除）：%v", err)
	} else {
		log.Printf("已在 B 站注销这份会话")
	}
	if rmErr := os.Remove(cookieFile()); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
		return rmErr
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

// fetchZone 查询 B 站把本机出口 IP 识别为哪里
func fetchZone() (zoneInfo, error) {
	var r struct {
		apiResp
		Data zoneInfo `json:"data"`
	}
	if err := newClient(nil).do("GET", "https://api.bilibili.com/x/web-interface/zone", nil, nil, &r); err != nil {
		return zoneInfo{}, err
	}
	if r.Code != 0 {
		return zoneInfo{}, fmt.Errorf("%d %s", r.Code, r.Message)
	}
	return r.Data, nil
}
