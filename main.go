package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // 让 TZ 环境变量在精简镜像里也能生效

	"github.com/mdp/qrterminal/v3"
)

const ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36"

var version = "dev"

type config struct {
	DataDir   string
	BVID      string
	DoDynamic bool
	Interval  time.Duration
	Jitter    time.Duration
}

var (
	cfg    config
	client = &http.Client{Timeout: 15 * time.Second}
)

type apiResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func loadConfig() config {
	c := config{
		DataDir:   os.Getenv("DATA_DIR"),
		BVID:      envOr("BVID", "BV1GJ411x7h7"),
		DoDynamic: envBool("DO_DYNAMIC", false),
		Interval:  envDuration("INTERVAL", 6*time.Hour),
		Jitter:    envDuration("JITTER", 30*time.Minute),
	}
	if c.DataDir == "" {
		c.DataDir = exeDir()
	}
	return c
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v, err := strconv.ParseBool(os.Getenv(key))
	if err != nil {
		return def
	}
	return v
}

func envDuration(key string, def time.Duration) time.Duration {
	v, err := time.ParseDuration(os.Getenv(key))
	if err != nil || v < 0 {
		return def
	}
	return v
}

func exeDir() string {
	p, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(p)
}

func cookieFile() string {
	return filepath.Join(cfg.DataDir, "cookies.json")
}

func jitter() time.Duration {
	if cfg.Jitter <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(cfg.Jitter)))
}

// form 和 body 二选一：form 按表单提交，body 按 JSON 提交
func call(method, rawURL string, cookies map[string]string, form url.Values, body any, out any) error {
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
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", "https://www.bilibili.com/")
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	for k, v := range cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s %s: HTTP %d", method, rawURL, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func login() error {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return err
	}
	client.Jar = jar
	defer func() { client.Jar = nil }()

	var gen struct {
		Data struct {
			URL string `json:"url"`
			Key string `json:"qrcode_key"`
		} `json:"data"`
	}
	if err := call("GET", "https://passport.bilibili.com/x/passport-login/web/qrcode/generate", nil, nil, nil, &gen); err != nil {
		return err
	}
	qrterminal.GenerateHalfBlock(gen.Data.URL, qrterminal.L, os.Stdout)
	fmt.Println("用 B 站 App 扫码并确认登录（二维码约 3 分钟内有效）...")

	for {
		time.Sleep(2 * time.Second)
		var poll struct {
			Data struct {
				Code int `json:"code"`
			} `json:"data"`
		}
		if err := call("GET", "https://passport.bilibili.com/x/passport-login/web/qrcode/poll?qrcode_key="+url.QueryEscape(gen.Data.Key), nil, nil, nil, &poll); err != nil {
			return err
		}
		if poll.Data.Code == 0 {
			break
		}
		if poll.Data.Code == 86038 {
			return errors.New("二维码已过期，请重新运行 login")
		}
	}

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
	if err := os.WriteFile(cookieFile(), b, 0600); err != nil {
		return err
	}
	fmt.Println("登录成功，已保存", cookieFile())
	return nil
}

func loadCookies() (map[string]string, error) {
	b, err := os.ReadFile(cookieFile())
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("找不到 %s，请先运行 login", cookieFile())
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

var errLoggedOut = errors.New("登录已失效，请重新运行 login")

// refresh 以账号身份发一轮请求，让 B 站记录到本机的出口 IP
func refresh() error {
	cookies, err := loadCookies()
	if err != nil {
		return err
	}
	csrf := cookies["bili_jct"]

	var nav struct {
		Data struct {
			IsLogin bool   `json:"isLogin"`
			Uname   string `json:"uname"`
		} `json:"data"`
	}
	if err := call("GET", "https://api.bilibili.com/x/web-interface/nav", cookies, nil, nil, &nav); err != nil {
		return err
	}
	if !nav.Data.IsLogin {
		return errLoggedOut
	}
	log.Printf("已登录：%s", nav.Data.Uname)

	var hb apiResp
	if err := call("POST", "https://api.bilibili.com/x/click-interface/web/heartbeat", cookies, url.Values{
		"bvid":        {cfg.BVID},
		"played_time": {strconv.Itoa(30 + rand.Intn(271))},
		"csrf":        {csrf},
	}, nil, &hb); err != nil {
		return err
	}
	log.Printf("心跳：%d %s", hb.Code, hb.Message)

	if cfg.DoDynamic {
		return postAndRemoveDynamic(cookies, csrf)
	}
	return nil
}

func postAndRemoveDynamic(cookies map[string]string, csrf string) error {
	texts := []string{"打卡", "路过", "1"}
	var cr struct {
		apiResp
		Data struct {
			DynID string `json:"dyn_id_str"`
		} `json:"data"`
	}
	if err := call("POST", "https://api.bilibili.com/x/dynamic/feed/create/dyn?csrf="+url.QueryEscape(csrf), cookies, nil, map[string]any{
		"dyn_req": map[string]any{
			"content": map[string]any{"contents": []map[string]any{
				{"raw_text": texts[rand.Intn(len(texts))], "type": 1, "biz_id": ""},
			}},
			"scene": 1,
		},
	}, &cr); err != nil {
		return err
	}
	log.Printf("发动态：%d %s", cr.Code, cr.Message)
	if cr.Data.DynID == "" {
		return nil
	}

	time.Sleep(time.Duration(5+rand.Intn(16)) * time.Second)
	var rr apiResp
	if err := call("POST", "https://api.bilibili.com/x/dynamic/feed/operate/remove?csrf="+url.QueryEscape(csrf), cookies, nil,
		map[string]any{"dyn_id_str": cr.Data.DynID}, &rr); err != nil {
		return err
	}
	log.Printf("删动态：%d %s", rr.Code, rr.Message)
	return nil
}

// daemon 启动时先跑一轮，之后每隔 INTERVAL + 随机 JITTER 再跑；失败只记日志不退出
func daemon() {
	log.Printf("bili-ip-anchor %s 守护模式启动：间隔 %s，随机抖动 %s，数据目录 %s", version, cfg.Interval, cfg.Jitter, cfg.DataDir)
	for {
		if err := refresh(); err != nil {
			log.Printf("本轮失败：%v", err)
		}
		next := cfg.Interval + jitter()
		log.Printf("下一轮：%s", time.Now().Add(next).Format("2006-01-02 15:04:05"))
		time.Sleep(next)
	}
}

const usage = `bili-ip-anchor %s

用法：
  bili-ip-anchor login            扫码登录，保存会话到数据目录
  bili-ip-anchor refresh [--now]  跑一轮（不加 --now 会先随机等待 0~JITTER）
  bili-ip-anchor daemon           常驻运行，按 INTERVAL 定时跑
  bili-ip-anchor version          显示版本

环境变量：DATA_DIR  BVID  DO_DYNAMIC  INTERVAL  JITTER  TZ（详见 README）
`

func main() {
	log.SetFlags(log.LstdFlags)
	cfg = loadConfig()

	if len(os.Args) < 2 {
		fmt.Printf(usage, version)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "login":
		if err := login(); err != nil {
			log.Fatal(err)
		}
	case "refresh":
		if len(os.Args) < 3 || os.Args[2] != "--now" {
			time.Sleep(jitter())
		}
		if err := refresh(); err != nil {
			log.Fatal(err)
		}
	case "daemon":
		daemon()
	case "version":
		fmt.Println(version)
	default:
		fmt.Printf(usage, version)
		os.Exit(2)
	}
}
