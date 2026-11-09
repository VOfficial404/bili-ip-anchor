//go:build !js

package main

import (
	"os"
	"testing"
	"time"
)

// TestLiveServer 连一台真的 bili-ip-anchor 服务器，确认接口对得上：
// BIA_SERVER=http://127.0.0.1:8080 BIA_PASSWORD=... go test -run TestLiveServer
func TestLiveServer(t *testing.T) {
	server, password := os.Getenv("BIA_SERVER"), os.Getenv("BIA_PASSWORD")
	if server == "" {
		t.Skip("设置 BIA_SERVER 和 BIA_PASSWORD 才连真的服务器")
	}
	h := newHarness(t, &memStore{})
	h.a.now = time.Now
	h.a.server, h.a.password = server, password
	h.a.connect()
	h.settle()
	if h.a.cl == nil {
		t.Fatalf("连接失败：%s", h.a.connectErr)
	}
	t.Logf("服务器 %s，%d 个账号，识别为 %v", h.a.st.Version, len(h.a.st.Accounts), h.a.st.Zone != nil)
	h.must("B 站把这台服务器识别为")
	h.snapshot("live-overview")
	h.click("账号")
	h.snapshot("live-accounts")
	h.click("设置")
	h.must(h.a.st.Settings.Interval)
	h.click("日志")
	h.snapshot("live-logs")
}
