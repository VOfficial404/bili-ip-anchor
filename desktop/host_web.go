//go:build js

package main

// 手机 App 里，界面编译成 WebAssembly，在 App 外壳的 WebView 里运行。页面由外壳里的 Go 核心
// （mobile/host.go）在 127.0.0.1 上提供，同源的 /host/ 接口替界面做浏览器里做不到的事：
// 记住连接信息、在手机上跑定时任务、把 /api/ 转发给服务器、下载 B 站图床的图片。

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"syscall/js"
	"time"

	"github.com/egoist/mygo"
)

var origin = js.Global().Get("location").Get("origin").String()

var hostHTTP = &http.Client{Timeout: 90 * time.Second}

// hostCall 调用外壳的 /host/ 接口。不能在主线程上调用：要等浏览器的 fetch，会卡死界面。
func hostCall(method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, origin+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("X-Requested-With", "bia")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := hostHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&e) == nil && e.Error != "" {
			return errors.New(e.Error)
		}
		return fmt.Errorf("App 内部错误（HTTP %d）", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type webHost struct{}

func newHost(store) host { return webHost{} }

func (webHost) remote(server string) (string, error) {
	if err := hostCall("POST", "/host/remote", map[string]string{"server": server}, nil); err != nil {
		return "", err
	}
	return origin, nil
}

func (webHost) startLocal(proxy string) (localRun, error) {
	var r localRun
	err := hostCall("POST", "/host/local", map[string]string{"proxy": proxy}, &r)
	r.Base = origin
	return r, err
}

func (webHost) stopLocal() { hostCall("POST", "/host/stop", nil, nil) }

func (webHost) probe(proxy string) (zone, error) {
	var z zone
	err := hostCall("POST", "/host/probe", map[string]string{"proxy": proxy}, &z)
	return z, err
}

// hostState 是外壳记住的东西，启动时读一次
type hostState struct {
	Prefs        prefs  `json:"prefs"`
	Password     string `json:"password"`      // 记住的面板密码，没有时为空
	LocalRunning bool   `json:"local_running"` // 界面重新打开时，本机的定时任务可能还在跑
	Platform     string `json:"platform"`      // android 或 ios
}

// webStore 把连接信息存在外壳那里。读的是启动时的缓存，写的时候在后台发给外壳：
// 界面在主线程上调用它，主线程不能等浏览器。
type webStore struct {
	mu sync.Mutex
	p  prefs
	pw string
}

func loadHostState() (hostState, error) {
	var s hostState
	err := hostCall("GET", "/host/state", nil, &s)
	return s, err
}

func (s *webStore) load() prefs {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.p
}

func (s *webStore) save(p prefs) error {
	s.mu.Lock()
	s.p = p
	s.mu.Unlock()
	go hostCall("PUT", "/host/prefs", p, nil)
	return nil
}

func (s *webStore) password() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pw == "" {
		return "", mygo.ErrSecretNotFound
	}
	return s.pw, nil
}

func (s *webStore) setPassword(pw string) error {
	s.mu.Lock()
	s.pw = pw
	s.mu.Unlock()
	go hostCall("PUT", "/host/password", map[string]string{"password": pw}, nil)
	return nil
}

func (s *webStore) forgetPassword() error {
	s.mu.Lock()
	had := s.pw != ""
	s.pw = ""
	s.mu.Unlock()
	if had {
		go hostCall("DELETE", "/host/password", nil, nil)
	}
	return nil
}

func (s *webStore) dataDir() (string, error) {
	return "", errors.New("手机上数据目录由 App 管理")
}

// httpGetImage 经过外壳下载 B 站图床的图片：WebView 里直接跨站请求会被浏览器拦下
func httpGetImage(u string) ([]byte, error) {
	resp, err := hostHTTP.Get(origin + "/host/image?u=" + url.QueryEscape(u))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 2<<20))
}

// await 等一个 JavaScript 的 Promise，不能在主线程上调用
func await(p js.Value) (js.Value, error) {
	ok, bad := make(chan js.Value, 1), make(chan js.Value, 1)
	then := js.FuncOf(func(_ js.Value, args []js.Value) any { ok <- args[0]; return nil })
	catch := js.FuncOf(func(_ js.Value, args []js.Value) any { bad <- args[0]; return nil })
	defer then.Release()
	defer catch.Release()
	p.Call("then", then, catch)
	select {
	case v := <-ok:
		return v, nil
	case e := <-bad:
		return js.Undefined(), errors.New(e.Call("toString").String())
	}
}

// pickImages 用网页的文件选择框选图片，手机上会打开相册
func pickImages() ([]picked, error) {
	doc := js.Global().Get("document")
	input := doc.Call("createElement", "input")
	input.Set("type", "file")
	input.Set("accept", "image/jpeg,image/png,image/gif,image/webp")
	input.Set("multiple", true)

	done := make(chan bool, 1) // true 表示选了文件
	finish := func(chose bool) {
		select {
		case done <- chose:
		default:
		}
	}
	onChange := js.FuncOf(func(js.Value, []js.Value) any { finish(true); return nil })
	onCancel := js.FuncOf(func(js.Value, []js.Value) any { finish(false); return nil })
	// 不支持 cancel 事件的 WebView：选择框关掉、页面重新拿到焦点一会儿还没有 change，就当取消
	onFocus := js.FuncOf(func(js.Value, []js.Value) any {
		go func() {
			time.Sleep(time.Second)
			finish(false)
		}()
		return nil
	})
	defer onChange.Release()
	defer onCancel.Release()
	defer onFocus.Release()
	input.Call("addEventListener", "change", onChange)
	input.Call("addEventListener", "cancel", onCancel)
	win := js.Global().Get("window")
	input.Call("click")
	go func() {
		time.Sleep(500 * time.Millisecond) // 选择框打开时页面会失去焦点，之后再等它回来
		win.Call("addEventListener", "focus", onFocus, map[string]any{"once": true})
	}()
	defer win.Call("removeEventListener", "focus", onFocus)

	if !<-done {
		return nil, nil
	}
	files := input.Get("files")
	out := make([]picked, 0, files.Length())
	for i := 0; i < files.Length(); i++ {
		f := files.Index(i)
		p := picked{name: f.Get("name").String()}
		if size := int64(f.Get("size").Float()); size > maxImageSize {
			p.err = fmt.Errorf("图片不能超过 %d MB", maxImageSize>>20)
		} else if buf, err := await(f.Call("arrayBuffer")); err != nil {
			p.err = err
		} else {
			u8 := js.Global().Get("Uint8Array").New(buf)
			p.data = make([]byte, u8.Length())
			js.CopyBytesToGo(p.data, u8)
		}
		out = append(out, p)
	}
	return out, nil
}
