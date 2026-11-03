package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"
	_ "time/tzdata" // 让 TZ 环境变量在精简镜像里也能生效

	"github.com/VOfficial404/bili-ip-anchor/core"
	"github.com/mdp/qrterminal/v3"
)

var version = "dev"

func loadConfig() core.Config {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = exeDir()
	}
	videos := core.ParseVideos(os.Getenv("BVIDS"))
	if len(videos) == 0 {
		videos = []string{envOr("BVID", "BV15HFVz1EvM")}
	}
	return core.Config{
		DataDir:   dataDir,
		Version:   version,
		Password:  os.Getenv("WEBUI_PASSWORD"),
		LogOutput: os.Stderr,
		Defaults: core.Settings{
			Videos:      videos,
			DoDynamic:   envBool("DO_DYNAMIC", false),
			DynamicText: os.Getenv("DYNAMIC_TEXT"),
			Interval:    core.Duration(envDuration("INTERVAL", 6*time.Hour)),
			Jitter:      core.Duration(envDuration("JITTER", 30*time.Minute)),
			Proxy:       os.Getenv("PROXY"),
		},
	}
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

func cliLogin(e *core.Engine) error {
	q, err := e.NewQRLogin()
	if err != nil {
		return err
	}
	qrterminal.GenerateHalfBlock(q.URL, qrterminal.L, os.Stdout)
	fmt.Println("用 B 站 App 扫码并确认登录（二维码约 3 分钟内有效）...")
	for {
		time.Sleep(2 * time.Second)
		st, mid, err := q.Poll()
		if err != nil {
			return err
		}
		switch st {
		case core.QRSuccess:
			fmt.Printf("登录成功，账号 UID %s 已保存到 %s\n", mid, filepath.Join(e.DataDir(), "accounts", mid))
			fmt.Println("守护进程在运行的话，1 分钟内会自动接上这个账号。想再加一个账号，再运行一次 login。")
			return nil
		case core.QRExpired:
			return errors.New("二维码已过期，请重新运行 login")
		}
	}
}

func daemon(e *core.Engine, password string) {
	e.Start()
	if password == "" {
		e.Logf("未设置 WEBUI_PASSWORD，WebUI 未启用")
		select {}
	}
	log.Fatal(e.ListenAndServe(envOr("WEBUI_ADDR", ":8080")))
}

const usage = `bili-ip-anchor %s

用法：
  bili-ip-anchor login            扫码登录一个账号（再运行一次可以添加更多账号）
  bili-ip-anchor refresh [--now]  为所有账号各跑一轮（不加 --now 会先随机等待 0~JITTER）
  bili-ip-anchor daemon           常驻运行，每个账号按 INTERVAL 独立定时；设置了 WEBUI_PASSWORD 时同时提供 WebUI
  bili-ip-anchor version          显示版本

环境变量：DATA_DIR  BVIDS  DO_DYNAMIC  DYNAMIC_TEXT  INTERVAL  JITTER  PROXY  WEBUI_ADDR  WEBUI_PASSWORD  TZ（详见 README）
`

func main() {
	log.SetFlags(log.LstdFlags)
	if len(os.Args) < 2 {
		fmt.Printf(usage, version)
		os.Exit(2)
	}
	cfg := loadConfig()
	switch os.Args[1] {
	case "login":
		if err := cliLogin(core.New(cfg)); err != nil {
			log.Fatal(err)
		}
	case "refresh":
		e := core.New(cfg)
		if len(os.Args) < 3 || os.Args[2] != "--now" {
			time.Sleep(core.Jitter(time.Duration(e.Settings().Jitter)))
		}
		if err := e.RunOnce(); err != nil {
			log.Fatal(err)
		}
	case "daemon":
		daemon(core.New(cfg), cfg.Password)
	case "version":
		fmt.Println(version)
	default:
		fmt.Printf(usage, version)
		os.Exit(2)
	}
}
