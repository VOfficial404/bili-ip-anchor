package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Duration 在 JSON 里写成 "6h"、"90m" 这样的字符串
type Duration time.Duration

// String 去掉多余的零：6h0m0s 写成 6h，1h30m0s 写成 1h30m
func (d Duration) String() string {
	s := time.Duration(d).String()
	if strings.HasSuffix(s, "m0s") {
		s = s[:len(s)-2]
	}
	if strings.HasSuffix(s, "h0m") {
		s = s[:len(s)-2]
	}
	return s
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("时间格式不对：%q，应写成 90m、6h 这样", s)
	}
	*d = Duration(v)
	return nil
}

const (
	maxVideos      = 20
	maxDynamicText = 1000
)

// Settings 是所有账号共用的设置，WebUI 里可以修改
type Settings struct {
	Videos      []string `json:"videos"`       // 心跳视频，每轮随机选一个
	DoDynamic   bool     `json:"do_dynamic"`   // 每轮发一条动态再删掉
	DynamicText string   `json:"dynamic_text"` // 动态的文字，可以为空
	Interval    Duration `json:"interval"`
	Jitter      Duration `json:"jitter"`
	Proxy       string   `json:"proxy"` // 所有 B 站请求经过的代理，空表示直连
}

// UnmarshalJSON 兼容 v0.5 及更早版本只有一个 bvid 的设置文件
func (s *Settings) UnmarshalJSON(b []byte) error {
	type plain Settings
	var v struct {
		plain
		BVID string `json:"bvid"`
	}
	v.plain = plain(*s)
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*s = Settings(v.plain)
	if len(s.Videos) == 0 && v.BVID != "" {
		s.Videos = []string{v.BVID}
	}
	return nil
}

var bvidRe = regexp.MustCompile(`^BV[0-9A-Za-z]{10}$`)

// ParseVideos 从逗号、空白分隔的文字里取出 BV 号，也接受视频链接
func ParseVideos(s string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '，' || r == ' ' || r == '\n' || r == '\t' }) {
		if m := bvidFindRe.FindString(f); m != "" {
			out = append(out, m)
		} else {
			out = append(out, f)
		}
	}
	return out
}

var bvidFindRe = regexp.MustCompile(`BV[0-9A-Za-z]{10}`)

// normalize 去掉视频列表里的空项和重复项，整理代理地址
func (s *Settings) normalize() {
	seen := map[string]bool{}
	videos := []string{}
	for _, v := range s.Videos {
		v = strings.TrimSpace(v)
		if m := bvidFindRe.FindString(v); m != "" && !strings.HasPrefix(v, "BV") {
			v = m // 视频链接
		}
		if v != "" && !seen[v] {
			seen[v] = true
			videos = append(videos, v)
		}
	}
	s.Videos = videos
	s.Proxy = strings.TrimSpace(s.Proxy)
	s.DynamicText = strings.TrimSpace(s.DynamicText)
}

func (s Settings) Validate() error {
	if len(s.Videos) == 0 {
		return errors.New("至少要有一个心跳视频")
	}
	if len(s.Videos) > maxVideos {
		return fmt.Errorf("心跳视频最多 %d 个", maxVideos)
	}
	for _, v := range s.Videos {
		if !bvidRe.MatchString(v) {
			return fmt.Errorf("BV 号格式不对：%q", v)
		}
	}
	if time.Duration(s.Interval) < 10*time.Minute {
		return errors.New("间隔不能小于 10 分钟")
	}
	if s.Jitter < 0 {
		return errors.New("随机抖动不能是负数")
	}
	if n := utf8.RuneCountInString(s.DynamicText); n > maxDynamicText {
		return fmt.Errorf("动态文字最多 %d 字，现在 %d 字", maxDynamicText, n)
	}
	if _, err := ParseProxy(s.Proxy); err != nil {
		return err
	}
	return nil
}

// ParseProxy 检查代理地址；空字符串表示直连，返回 nil
func ParseProxy(s string) (*url.URL, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || u.Port() == "" {
		return nil, fmt.Errorf("代理地址不对：%q，应写成 http://127.0.0.1:7890 或 socks5://127.0.0.1:7891", s)
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, fmt.Errorf("不支持 %s 代理，只支持 http、https、socks5", u.Scheme)
	}
	return u, nil
}

func (e *Engine) settingsFile() string { return filepath.Join(e.cfg.DataDir, "settings.json") }

// loadSettings 以 Config.Defaults 为底，WebUI 保存过的 settings.json 优先
func (e *Engine) loadSettings() Settings {
	s := e.cfg.Defaults
	s.normalize()
	b, err := os.ReadFile(e.settingsFile())
	if errors.Is(err, os.ErrNotExist) {
		return s
	}
	if err == nil {
		err = json.Unmarshal(b, &s)
	}
	if err == nil {
		s.normalize()
		err = s.Validate()
	}
	if err != nil {
		e.Logf("忽略 %s，改用默认设置：%v", e.settingsFile(), err)
		s = e.cfg.Defaults
		s.normalize()
	}
	return s
}

func (e *Engine) saveSettings(s Settings) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(e.cfg.DataDir, 0700); err != nil {
		return err
	}
	return os.WriteFile(e.settingsFile(), b, 0600)
}

// DefaultSettings 是没有任何配置时的设置：每 6 小时一轮，只发心跳
func DefaultSettings() Settings {
	return Settings{
		Videos:   []string{"BV15HFVz1EvM"},
		Interval: Duration(6 * time.Hour),
		Jitter:   Duration(30 * time.Minute),
	}
}
