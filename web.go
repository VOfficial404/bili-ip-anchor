package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"
)

//go:embed web
var webFiles embed.FS

const (
	sessionCookie = "bia_session"
	sessionTTL    = 7 * 24 * time.Hour
)

var loginFailMu sync.Mutex // 密码错误时串行等待，拖慢暴力尝试

func (a *app) serveWeb() {
	static, _ := fs.Sub(webFiles, "web")
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(static))
	mux.HandleFunc("POST /api/session", a.handleSessionCreate)
	mux.HandleFunc("DELETE /api/session", a.auth(a.handleSessionDelete))
	mux.HandleFunc("GET /api/status", a.auth(a.handleStatus))
	mux.HandleFunc("POST /api/run", a.auth(a.handleRun))
	mux.HandleFunc("PUT /api/settings", a.auth(a.handleSettings))
	mux.HandleFunc("POST /api/qr", a.auth(a.handleQRStart))
	mux.HandleFunc("GET /api/qr", a.auth(a.handleQRPoll))
	mux.HandleFunc("POST /api/bili-logout", a.auth(a.handleBiliLogout))

	if len(cfg.WebPassword) < 8 {
		log.Printf("警告：WEBUI_PASSWORD 少于 8 位，建议换一个更长的密码")
	}
	srv := &http.Server{
		Addr:              cfg.WebAddr,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("WebUI 监听 %s", cfg.WebAddr)
	log.Fatal(srv.ListenAndServe())
}

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' blob:; style-src 'self'; script-src 'self'")
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

// 非 GET 请求必须带自定义请求头：跨站表单发不出它，用来防 CSRF
func sameOriginWrite(r *http.Request) bool {
	return r.Method == http.MethodGet || r.Header.Get("X-Requested-With") == "bia"
}

func (a *app) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !sameOriginWrite(r) {
			writeErr(w, http.StatusForbidden, "缺少 X-Requested-With 请求头")
			return
		}
		c, err := r.Cookie(sessionCookie)
		if err != nil || !a.validSession(c.Value) {
			writeErr(w, http.StatusUnauthorized, "请先输入密码")
			return
		}
		next(w, r)
	}
}

func (a *app) validSession(token string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	exp, ok := a.sessions[token]
	if ok && time.Now().After(exp) {
		delete(a.sessions, token)
		return false
	}
	return ok
}

func checkPassword(given string) bool {
	g := sha256.Sum256([]byte(given))
	want := sha256.Sum256([]byte(cfg.WebPassword))
	return subtle.ConstantTimeCompare(g[:], want[:]) == 1
}

func (a *app) handleSessionCreate(w http.ResponseWriter, r *http.Request) {
	if !sameOriginWrite(r) {
		writeErr(w, http.StatusForbidden, "缺少 X-Requested-With 请求头")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body)
	if !checkPassword(body.Password) {
		loginFailMu.Lock()
		time.Sleep(time.Second)
		loginFailMu.Unlock()
		log.Printf("WebUI 密码错误（来自 %s）", r.RemoteAddr)
		writeErr(w, http.StatusUnauthorized, "密码不对")
		return
	}

	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)
	a.mu.Lock()
	for t, exp := range a.sessions {
		if time.Now().After(exp) {
			delete(a.sessions, t)
		}
	}
	a.sessions[token] = time.Now().Add(sessionTTL)
	a.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *app) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		a.mu.Lock()
		delete(a.sessions, c.Value)
		a.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type statusResp struct {
	Version   string     `json:"version"`
	LoggedIn  bool       `json:"logged_in"`
	Uname     string     `json:"uname"`
	Running   bool       `json:"running"`
	Last      *runResult `json:"last"`
	Next      *time.Time `json:"next"`
	Zone      *zoneInfo  `json:"zone"`
	ZoneError string     `json:"zone_error,omitempty"`
	Settings  settings   `json:"settings"`
	Logs      []string   `json:"logs"`
}

func (a *app) handleStatus(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	resp := statusResp{
		Version:   version,
		LoggedIn:  a.loggedIn,
		Uname:     a.uname,
		Running:   a.running,
		Last:      a.last,
		Zone:      a.zone,
		ZoneError: a.zoneErr,
		Settings:  a.set,
	}
	if !a.next.IsZero() {
		next := a.next
		resp.Next = &next
	}
	a.mu.Unlock()
	resp.Logs = a.logs.Lines()
	writeJSON(w, http.StatusOK, resp)
}

func (a *app) handleRun(w http.ResponseWriter, r *http.Request) {
	if !a.runNow() {
		writeErr(w, http.StatusConflict, "正在运行，等这一轮结束再试")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *app) handleSettings(w http.ResponseWriter, r *http.Request) {
	var s settings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&s); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.updateSettings(s); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.settings())
}

func (a *app) handleQRStart(w http.ResponseWriter, r *http.Request) {
	png, err := a.startQR()
	if err != nil {
		writeErr(w, http.StatusBadGateway, "获取二维码失败："+err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(png)
}

func (a *app) handleQRPoll(w http.ResponseWriter, r *http.Request) {
	st, err := a.pollQR()
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]qrState{"state": st})
}

func (a *app) handleBiliLogout(w http.ResponseWriter, r *http.Request) {
	if err := a.logoutBili(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
