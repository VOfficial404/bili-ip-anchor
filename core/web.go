package core

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"sync"
	"time"
)

//go:embed web
var webFiles embed.FS

const (
	sessionCookie = "bia_session"
	sessionTTL    = 7 * 24 * time.Hour
	autoTokenTTL  = 2 * time.Minute
)

var loginFailMu sync.Mutex // 密码错误时串行等待，拖慢暴力尝试

// Handler 返回 WebUI（页面和接口）
func (e *Engine) Handler() http.Handler {
	static, _ := fs.Sub(webFiles, "web")
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(static))
	mux.HandleFunc("POST /api/session", e.handleSessionCreate)
	mux.HandleFunc("GET /api/session/auto", e.handleSessionAuto)
	mux.HandleFunc("DELETE /api/session", e.auth(e.handleSessionDelete))
	mux.HandleFunc("GET /api/status", e.auth(e.handleStatus))
	mux.HandleFunc("POST /api/run", e.auth(e.handleRunAll))
	mux.HandleFunc("POST /api/accounts/{mid}/run", e.auth(e.handleAccountRun))
	mux.HandleFunc("POST /api/accounts/{mid}/check", e.auth(e.handleAccountCheck))
	mux.HandleFunc("POST /api/accounts/{mid}/cookie/refresh", e.auth(e.handleCookieRefresh))
	mux.HandleFunc("DELETE /api/accounts/{mid}", e.auth(e.handleAccountDelete))
	mux.HandleFunc("POST /api/accounts/{mid}/appauth", e.auth(e.handleTVStart))
	mux.HandleFunc("GET /api/accounts/{mid}/appauth", e.auth(e.handleTVPoll))
	mux.HandleFunc("DELETE /api/accounts/{mid}/appauth", e.auth(e.handleTVRemove))
	mux.HandleFunc("PUT /api/settings", e.auth(e.handleSettings))
	mux.HandleFunc("POST /api/qr", e.auth(e.handleQRStart))
	mux.HandleFunc("GET /api/qr", e.auth(e.handleQRPoll))
	mux.HandleFunc("GET /api/videos/{bvid}", e.auth(e.handleVideo))
	mux.HandleFunc("GET /api/dynamic/images", e.auth(e.handleImageList))
	mux.HandleFunc("POST /api/dynamic/images", e.auth(e.handleImageAdd))
	mux.HandleFunc("GET /api/dynamic/images/{name}", e.auth(e.handleImageGet))
	mux.HandleFunc("DELETE /api/dynamic/images/{name}", e.auth(e.handleImageDelete))
	mux.HandleFunc("POST /api/proxy/test", e.auth(e.handleProxyTest))
	return securityHeaders(mux)
}

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' blob: https://*.hdslb.com; style-src 'self'; script-src 'self'")
		h.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func writeOK(w http.ResponseWriter) { writeJSON(w, http.StatusOK, map[string]bool{"ok": true}) }

// 非 GET 请求必须带自定义请求头：跨站表单发不出它，用来防 CSRF
func sameOriginWrite(r *http.Request) bool {
	return r.Method == http.MethodGet || r.Header.Get("X-Requested-With") == "bia"
}

func (e *Engine) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !sameOriginWrite(r) {
			writeErr(w, http.StatusForbidden, "缺少 X-Requested-With 请求头")
			return
		}
		c, err := r.Cookie(sessionCookie)
		if err != nil || !e.validSession(c.Value) {
			writeErr(w, http.StatusUnauthorized, "请先输入密码")
			return
		}
		next(w, r)
	}
}

func (e *Engine) validSession(token string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	exp, ok := e.sessions[token]
	if ok && time.Now().After(exp) {
		delete(e.sessions, token)
		return false
	}
	return ok
}

func (e *Engine) checkPassword(given string) bool {
	if e.cfg.Password == "" {
		return false
	}
	g := sha256.Sum256([]byte(given))
	want := sha256.Sum256([]byte(e.cfg.Password))
	return subtle.ConstantTimeCompare(g[:], want[:]) == 1
}

// newSession 生成一个面板会话并写进 Cookie
func (e *Engine) newSession(w http.ResponseWriter, r *http.Request) {
	token := randomHex(32)
	e.mu.Lock()
	for t, exp := range e.sessions {
		if time.Now().After(exp) {
			delete(e.sessions, t)
		}
	}
	e.sessions[token] = time.Now().Add(sessionTTL)
	e.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteStrictMode,
	})
}

func (e *Engine) handleSessionCreate(w http.ResponseWriter, r *http.Request) {
	if !sameOriginWrite(r) {
		writeErr(w, http.StatusForbidden, "缺少 X-Requested-With 请求头")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body)
	if !e.checkPassword(body.Password) {
		loginFailMu.Lock()
		time.Sleep(time.Second)
		loginFailMu.Unlock()
		e.Logf("WebUI 密码错误（来自 %s）", r.RemoteAddr)
		writeErr(w, http.StatusUnauthorized, "密码不对")
		return
	}
	e.newSession(w, r)
	writeOK(w)
}

// AutoLoginToken 生成一个一次性的登录令牌，手机 App 用它让 WebView 免密码进入面板
func (e *Engine) AutoLoginToken() string {
	token := randomHex(24)
	e.mu.Lock()
	for t, exp := range e.autoTokens {
		if time.Now().After(exp) {
			delete(e.autoTokens, t)
		}
	}
	e.autoTokens[token] = time.Now().Add(autoTokenTTL)
	e.mu.Unlock()
	return token
}

// handleSessionAuto 用一次性令牌换面板会话，然后跳到首页
func (e *Engine) handleSessionAuto(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	e.mu.Lock()
	exp, ok := e.autoTokens[token]
	delete(e.autoTokens, token)
	e.mu.Unlock()
	if token == "" || !ok || time.Now().After(exp) {
		writeErr(w, http.StatusUnauthorized, "登录链接已失效")
		return
	}
	e.newSession(w, r)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (e *Engine) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		e.mu.Lock()
		delete(e.sessions, c.Value)
		e.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	writeOK(w)
}

func (e *Engine) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, e.status())
}

func (e *Engine) handleRunAll(w http.ResponseWriter, r *http.Request) {
	n := e.RunAll()
	if n == 0 {
		writeErr(w, http.StatusConflict, "没有可以立即运行的账号，可能都在运行中")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"triggered": n})
}

// withAccount 取出路径里的账号，找不到时返回 404
func (e *Engine) withAccount(w http.ResponseWriter, r *http.Request) *account {
	acc := e.account(r.PathValue("mid"))
	if acc == nil {
		writeErr(w, http.StatusNotFound, errNoAccount.Error())
	}
	return acc
}

// writeActionErr 把操作的错误写成合适的状态码
func writeActionErr(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		writeOK(w)
	case errors.Is(err, errBusy):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, errNoAccount):
		writeErr(w, http.StatusNotFound, err.Error())
	default:
		writeErr(w, http.StatusBadGateway, err.Error())
	}
}

func (e *Engine) handleAccountRun(w http.ResponseWriter, r *http.Request) {
	acc := e.withAccount(w, r)
	if acc == nil {
		return
	}
	if !acc.runNow() {
		writeErr(w, http.StatusConflict, errBusy.Error())
		return
	}
	writeOK(w)
}

func (e *Engine) handleAccountCheck(w http.ResponseWriter, r *http.Request) {
	if acc := e.withAccount(w, r); acc != nil {
		writeActionErr(w, e.checkAccount(acc))
	}
}

func (e *Engine) handleCookieRefresh(w http.ResponseWriter, r *http.Request) {
	if acc := e.withAccount(w, r); acc != nil {
		writeActionErr(w, e.refreshAccountCookie(acc))
	}
}

func (e *Engine) handleAccountDelete(w http.ResponseWriter, r *http.Request) {
	err := e.removeAccount(r.PathValue("mid"))
	if err != nil && !errors.Is(err, errNoAccount) {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeActionErr(w, err)
}

func (e *Engine) handleTVStart(w http.ResponseWriter, r *http.Request) {
	png, err := e.tvStart(r.PathValue("mid"))
	if err != nil {
		writeActionErr(w, err)
		return
	}
	writePNG(w, png)
}

func (e *Engine) handleTVPoll(w http.ResponseWriter, r *http.Request) {
	st, err := e.tvPoll(r.PathValue("mid"))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"state": st})
}

func (e *Engine) handleTVRemove(w http.ResponseWriter, r *http.Request) {
	writeActionErr(w, e.removeAppAuth(r.PathValue("mid")))
}

func (e *Engine) handleSettings(w http.ResponseWriter, r *http.Request) {
	var s Settings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&s); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := e.UpdateSettings(s); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, e.Settings())
}

func writePNG(w http.ResponseWriter, png []byte) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(png)
}

func (e *Engine) handleQRStart(w http.ResponseWriter, r *http.Request) {
	png, err := e.startQR()
	if err != nil {
		writeErr(w, http.StatusBadGateway, "获取二维码失败："+err.Error())
		return
	}
	writePNG(w, png)
}

func (e *Engine) handleQRPoll(w http.ResponseWriter, r *http.Request) {
	st, mid, err := e.pollQR()
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"state": st, "mid": mid})
}

func (e *Engine) handleVideo(w http.ResponseWriter, r *http.Request) {
	info, err := e.videoInfo(r.PathValue("bvid"))
	switch {
	case errors.Is(err, errVideoNotFound):
		writeErr(w, http.StatusNotFound, err.Error())
	case err != nil:
		writeErr(w, http.StatusBadGateway, err.Error())
	default:
		writeJSON(w, http.StatusOK, info)
	}
}

func (e *Engine) handleImageList(w http.ResponseWriter, r *http.Request) {
	list, err := e.listImages()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []imageInfo{}
	}
	writeJSON(w, http.StatusOK, list)
}

// handleImageAdd 接收请求体里的一张图片（不是表单）
func (e *Engine) handleImageAdd(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxImageSize+1))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "图片不能超过 10 MB")
		return
	}
	name, err := e.addImage(data)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	e.Logf("添加动态图片 %s", name)
	writeJSON(w, http.StatusOK, map[string]string{"name": name})
}

func (e *Engine) handleImageGet(w http.ResponseWriter, r *http.Request) {
	data, err := e.readImage(r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "没有这张图片")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=86400") // 文件名不会复用
	http.ServeContent(w, r, r.PathValue("name"), time.Time{}, bytes.NewReader(data))
}

func (e *Engine) handleImageDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := e.removeImage(name); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeErr(w, http.StatusNotFound, "没有这张图片")
		} else {
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	e.Logf("删除动态图片 %s", name)
	writeOK(w)
}

func (e *Engine) handleProxyTest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Proxy string `json:"proxy"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	z, err := e.TestProxy(body.Proxy)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, z)
}
