package core

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Level 是日志级别。低于设置级别的日志直接丢弃，不进 WebUI 也不输出。
// 零值是 info：旧版客户端保存设置时不带日志级别，就是 info。
type Level int32

const (
	LevelDebug Level = iota - 1
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	}
	return "info"
}

func (l Level) MarshalJSON() ([]byte, error) { return json.Marshal(l.String()) }

func (l *Level) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := ParseLevel(s)
	*l = v
	return err
}

// ParseLevel 解析 debug、info、warn（warning）、error，不区分大小写；空字符串是 info
func ParseLevel(s string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug, nil
	case "", "info":
		return LevelInfo, nil
	case "warn", "warning":
		return LevelWarn, nil
	case "error":
		return LevelError, nil
	}
	return LevelInfo, fmt.Errorf("日志级别只能是 debug、info、warn 或 error，不能是 %q", s)
}

// LogEntry 是一条日志
type LogEntry struct {
	Seq     int64     `json:"seq"`
	Time    time.Time `json:"time"`
	Level   Level     `json:"level"`
	Mid     string    `json:"mid,omitempty"`     // 账号的 UID，和账号无关的日志为空
	Account string    `json:"account,omitempty"` // 账号名
	Msg     string    `json:"msg"`
}

// String 是输出到终端和旧版客户端看到的格式：2026/11/03 12:00:00 INFO  [昵称] 消息
func (en LogEntry) String() string {
	var b strings.Builder
	b.WriteString(en.Time.Format("2006/01/02 15:04:05 "))
	fmt.Fprintf(&b, "%-5s ", strings.ToUpper(en.Level.String()))
	if en.Account != "" {
		b.WriteString("[" + en.Account + "] ")
	}
	b.WriteString(en.Msg)
	return b.String()
}

// logBook 保留最近若干条日志给 WebUI 看，同时写到 out
type logBook struct {
	level atomic.Int32

	mu      sync.Mutex
	entries []LogEntry
	max     int
	seq     int64
	out     io.Writer
}

func newLogBook(max int, out io.Writer, level Level) *logBook {
	b := &logBook{max: max, out: out}
	b.level.Store(int32(level))
	return b
}

func (b *logBook) Level() Level         { return Level(b.level.Load()) }
func (b *logBook) SetLevel(l Level)     { b.level.Store(int32(l)) }
func (b *logBook) enabled(l Level) bool { return l >= b.Level() }

func (b *logBook) add(level Level, mid, account, format string, args ...any) {
	if !b.enabled(level) {
		return
	}
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	en := LogEntry{Seq: b.seq, Time: time.Now(), Level: level, Mid: mid, Account: account, Msg: msg}
	b.entries = append(b.entries, en)
	if over := len(b.entries) - b.max; over > 0 {
		b.entries = append([]LogEntry(nil), b.entries[over:]...)
	}
	if b.out != nil {
		io.WriteString(b.out, en.String()+"\n")
	}
}

// since 返回序号大于 after、级别不低于 min 的日志。after 比最新的序号还大时，
// 说明程序重启过、客户端拿的是上次的序号，返回全部。
func (b *logBook) since(after int64, min Level) []LogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	if n := len(b.entries); n > 0 && after > b.entries[n-1].Seq {
		after = 0
	}
	out := []LogEntry{}
	for _, en := range b.entries {
		if en.Seq > after && en.Level >= min {
			out = append(out, en)
		}
	}
	return out
}

// lines 是最近 n 条日志的文本，给 v0.6 及更早的客户端
func (b *logBook) lines(n int) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	start := max(0, len(b.entries)-n)
	out := make([]string, 0, len(b.entries)-start)
	for _, en := range b.entries[start:] {
		out = append(out, en.String())
	}
	return out
}

// 和账号无关的日志
func (e *Engine) Debugf(format string, args ...any) { e.logs.add(LevelDebug, "", "", format, args...) }
func (e *Engine) Infof(format string, args ...any)  { e.logs.add(LevelInfo, "", "", format, args...) }
func (e *Engine) Warnf(format string, args ...any)  { e.logs.add(LevelWarn, "", "", format, args...) }
func (e *Engine) Errorf(format string, args ...any) { e.logs.add(LevelError, "", "", format, args...) }

// Logf 等于 Infof
func (e *Engine) Logf(format string, args ...any) { e.Infof(format, args...) }

// LogLevel 是当前的日志级别
func (e *Engine) LogLevel() Level { return e.logs.Level() }

// Logs 返回序号大于 after、级别不低于 min 的日志
func (e *Engine) Logs(after int64, min Level) []LogEntry { return e.logs.since(after, min) }

// 账号的日志带上账号名；不能在持有 acc.mu 时调用
func (acc *account) debugf(format string, args ...any) { acc.logAt(LevelDebug, format, args...) }
func (acc *account) infof(format string, args ...any)  { acc.logAt(LevelInfo, format, args...) }
func (acc *account) warnf(format string, args ...any)  { acc.logAt(LevelWarn, format, args...) }
func (acc *account) errorf(format string, args ...any) { acc.logAt(LevelError, format, args...) }

func (acc *account) logAt(level Level, format string, args ...any) {
	if acc.e.logs.enabled(level) {
		acc.e.logs.add(level, acc.mid, acc.name(), format, args...)
	}
}
