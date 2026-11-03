// Package mobile 是手机 App 用的接口，用 gomobile bind 编译成 Android 的 aar 和 iOS 的 xcframework。
// App 用它在手机上运行定时任务（本机模式），界面就是 WebUI，在 WebView 里打开 LoginURL。
// 只能用 gomobile 支持的类型：string、bool、error。
package mobile

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // 给 SetTimeZone 用

	"github.com/VOfficial404/bili-ip-anchor/core"
)

// version 发布时用 -ldflags 写入
var version = "dev"

// transport 不为 nil 时代替网络，给测试用
var transport http.RoundTripper

var (
	mu     sync.Mutex
	engine *core.Engine
	base   string
)

func Version() string { return version }

// SetTimeZone 设置日志用的时区，传系统的时区名，例如 Asia/Shanghai。手机上 Go 自己取不到本地时区。
func SetTimeZone(name string) {
	if loc, err := time.LoadLocation(name); err == nil {
		time.Local = loc
	}
}

// Start 开始在这台手机上运行：账号会话和设置放在 dataDir，所有 B 站请求经过 proxy。
// 已经在运行时只更新代理。
func Start(dataDir, proxy string) error {
	proxy = strings.TrimSpace(proxy)
	if _, err := core.ParseProxy(proxy); err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	if engine != nil {
		return setProxy(engine, proxy)
	}
	if dataDir == "" {
		return errors.New("没有数据目录")
	}
	e := core.New(core.Config{
		DataDir:   dataDir,
		Version:   version,
		Password:  core.RandomPassword(),
		Local:     true,
		Defaults:  core.DefaultSettings(),
		LogOutput: os.Stderr,
		Transport: transport,
	})
	if err := setProxy(e, proxy); err != nil {
		return err
	}
	e.Start()
	addr, err := e.ServeLocal()
	if err != nil {
		e.Stop()
		return err
	}
	engine, base = e, addr
	return nil
}

func setProxy(e *core.Engine, proxy string) error {
	s := e.Settings()
	if s.Proxy == proxy {
		return nil
	}
	s.Proxy = proxy
	return e.UpdateSettings(s)
}

func Running() bool {
	mu.Lock()
	defer mu.Unlock()
	return engine != nil
}

// Proxy 是正在使用的代理；面板里也能改，App 下次启动时以这里为准
func Proxy() string {
	mu.Lock()
	defer mu.Unlock()
	if engine == nil {
		return ""
	}
	return engine.Settings().Proxy
}

// LoginURL 返回一个打开后直接进入面板的地址，只能用一次，两分钟内有效
func LoginURL() (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if engine == nil {
		return "", errors.New("还没有开始运行")
	}
	return base + "/api/session/auto?token=" + engine.AutoLoginToken(), nil
}

// Stop 停下定时任务和面板；账号和设置留在数据目录里
func Stop() {
	mu.Lock()
	e := engine
	engine, base = nil, ""
	mu.Unlock()
	if e != nil {
		e.Stop()
	}
}

// ProbeProxy 经过 proxy 查询 B 站把出口 IP 识别为哪里，返回 JSON：addr、country、province、city、isp，
// 以及主页属地会显示的 region
func ProbeProxy(proxy string) (string, error) {
	z, err := core.ProbeProxy(proxy, transport)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(struct {
		core.Zone
		Region string `json:"region"`
	}{z, z.Region()})
	return string(b), err
}
