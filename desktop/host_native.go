//go:build !js

package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/VOfficial404/bili-ip-anchor/core"
	"github.com/egoist/mygo"
)

// nativeHost 是桌面版：本机运行时在进程里跑 core.Engine，WebUI 只监听 127.0.0.1
type nativeHost struct {
	store     store
	transport http.RoundTripper // 不为 nil 时代替网络，给测试用

	mu     sync.Mutex
	engine *core.Engine
	base   string // engine 的 WebUI 地址
}

func newHost(s store) host { return &nativeHost{store: s} }

func (h *nativeHost) remote(server string) (string, error) { return server, nil }

func (h *nativeHost) startLocal(proxy string) (localRun, error) {
	if _, err := core.ParseProxy(proxy); err != nil {
		return localRun{}, err
	}
	rt := h.transport
	via, err := core.ProbeProxy(proxy, rt)
	if err != nil {
		return localRun{}, err
	}
	run := localRun{Zone: zoneOf(via)}
	if rt == nil {
		// 和直连比一下：一样说明代理没有接管这些请求
		if direct, derr := core.ProbeProxy("", nil); derr == nil && direct.Addr == via.Addr {
			run.Warning = fmt.Sprintf("注意：经过代理和直连时 B 站看到的都是 %s，代理可能没有生效", via.Addr)
		}
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	e := h.engine
	if e == nil {
		dir, err := h.store.dataDir()
		if err != nil {
			return localRun{}, err
		}
		e = core.New(core.Config{
			DataDir:   dir,
			Version:   version,
			Password:  core.RandomPassword(),
			Local:     true,
			Defaults:  core.DefaultSettings(),
			LogOutput: os.Stderr,
			Transport: rt,
		})
	}
	if s := e.Settings(); s.Proxy != proxy {
		s.Proxy = proxy
		if err := e.UpdateSettings(s); err != nil {
			return localRun{}, err
		}
	}
	if h.engine == nil {
		e.Start()
		addr, err := e.ServeLocal()
		if err != nil {
			e.Stop()
			return localRun{}, err
		}
		h.engine = e
		run.Base = addr
	} else {
		run.Base = h.base
	}
	h.base = run.Base
	run.Password = e.Password()
	return run, nil
}

func (h *nativeHost) stopLocal() {
	h.mu.Lock()
	e := h.engine
	h.engine = nil
	h.mu.Unlock()
	if e != nil {
		e.Stop()
	}
}

func (h *nativeHost) running() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.engine != nil
}

func (h *nativeHost) probe(proxy string) (zone, error) {
	z, err := core.ProbeProxy(proxy, h.transport)
	return zoneOf(z), err
}

func zoneOf(z core.Zone) zone {
	return zone{Addr: z.Addr, Country: z.Country, Province: z.Province, City: z.City, ISP: z.ISP}
}

// httpGetImage 直接从 B 站图床下载
func httpGetImage(u string) ([]byte, error) {
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

func pickImages() ([]picked, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Title:    "选择动态图片",
		Multiple: true,
		Filters:  []mygo.FileFilter{{Name: "图片", Extensions: []string{"jpg", "jpeg", "png", "gif", "webp"}}},
	})
	out := make([]picked, 0, len(paths))
	for _, p := range paths {
		data, err := readLimited(p, maxImageSize)
		out = append(out, picked{name: baseName(p), data: data, err: err})
	}
	return out, err
}

func readLimited(path string, max int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > max {
		return nil, fmt.Errorf("图片不能超过 %d MB", max>>20)
	}
	return os.ReadFile(path)
}
