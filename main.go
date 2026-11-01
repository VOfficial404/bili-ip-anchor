package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"time"
	_ "time/tzdata" // 让 TZ 环境变量在精简镜像里也能生效

	"github.com/mdp/qrterminal/v3"
)

var version = "dev"

type config struct {
	DataDir     string
	WebAddr     string
	WebPassword string
	Defaults    settings // 来自环境变量，WebUI 保存的 settings.json 会覆盖它
}

var cfg config

func loadConfig() config {
	c := config{
		DataDir:     os.Getenv("DATA_DIR"),
		WebAddr:     envOr("WEBUI_ADDR", ":8080"),
		WebPassword: os.Getenv("WEBUI_PASSWORD"),
		Defaults: settings{
			BVID:      envOr("BVID", "BV15HFVz1EvM"),
			DoDynamic: envBool("DO_DYNAMIC", false),
			Interval:  duration(envDuration("INTERVAL", 6*time.Hour)),
			Jitter:    duration(envDuration("JITTER", 30*time.Minute)),
		},
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

func settingsFile() string { return filepath.Join(cfg.DataDir, "settings.json") }

func jitter(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(max)))
}

func cliLogin() error {
	q, err := qrStart()
	if err != nil {
		return err
	}
	qrterminal.GenerateHalfBlock(q.URL, qrterminal.L, os.Stdout)
	fmt.Println("用 B 站 App 扫码并确认登录（二维码约 3 分钟内有效）...")
	for {
		time.Sleep(2 * time.Second)
		st, mid, err := q.poll()
		if err != nil {
			return err
		}
		switch st {
		case qrSuccess:
			fmt.Printf("登录成功，账号 UID %s 已保存到 %s\n", mid, accountDir(mid))
			fmt.Println("守护进程在运行的话，1 分钟内会自动接上这个账号。想再加一个账号，再运行一次 login。")
			return nil
		case qrExpired:
			return errors.New("二维码已过期，请重新运行 login")
		}
	}
}

// cliRefresh 依次为每个账号跑一轮；有账号失败时返回错误
func cliRefresh() error {
	migrateLegacyCookies()
	mids, err := listAccounts()
	if err != nil {
		return err
	}
	if len(mids) == 0 {
		return errNoCookies
	}
	s := loadSettings()
	failed := 0
	for _, mid := range mids {
		acc := newAccount(mid)
		acc.run(s)
		if !acc.status().Last.OK {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d 个账号失败", failed)
	}
	return nil
}

func daemon() {
	migrateLegacyCookies()
	a := newApp(logs)
	s := a.settings()
	log.Printf("bili-ip-anchor %s 守护模式启动：间隔 %s，随机抖动 %s，数据目录 %s", version, s.Interval, s.Jitter, cfg.DataDir)
	a.syncAccounts(true)
	if n := a.accountCount(); n == 0 {
		log.Printf("还没有账号：在 WebUI 里点「添加账号」，或运行 login 扫码")
	} else {
		log.Printf("已加载 %d 个账号，各自独立排程", n)
	}
	go a.syncLoop()
	go a.zoneLoop()
	if cfg.WebPassword != "" {
		go a.serveWeb()
	} else {
		log.Printf("未设置 WEBUI_PASSWORD，WebUI 未启用")
	}
	select {}
}

const usage = `bili-ip-anchor %s

用法：
  bili-ip-anchor login            扫码登录一个账号（再运行一次可以添加更多账号）
  bili-ip-anchor refresh [--now]  为所有账号各跑一轮（不加 --now 会先随机等待 0~JITTER）
  bili-ip-anchor daemon           常驻运行，每个账号按 INTERVAL 独立定时；设置了 WEBUI_PASSWORD 时同时提供 WebUI
  bili-ip-anchor version          显示版本

环境变量：DATA_DIR  BVID  DO_DYNAMIC  INTERVAL  JITTER  WEBUI_ADDR  WEBUI_PASSWORD  TZ（详见 README）
`

var logs = newRingLog(300)

func main() {
	log.SetFlags(log.LstdFlags)
	log.SetOutput(io.MultiWriter(os.Stderr, logs))
	cfg = loadConfig()

	if len(os.Args) < 2 {
		fmt.Printf(usage, version)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "login":
		if err := cliLogin(); err != nil {
			log.Fatal(err)
		}
	case "refresh":
		if len(os.Args) < 3 || os.Args[2] != "--now" {
			time.Sleep(jitter(time.Duration(loadSettings().Jitter)))
		}
		if err := cliRefresh(); err != nil {
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
