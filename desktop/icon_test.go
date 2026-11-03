package main

import (
	"bytes"
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestGenerateIcon 用 MyGo 自己画出应用图标：GEN_ICON=1 go test -run TestGenerateIcon
func TestGenerateIcon(t *testing.T) {
	if os.Getenv("GEN_ICON") == "" {
		t.Skip("设置 GEN_ICON=1 才重新生成 resources/icon.png")
	}
	view := func(c *ui.Context) {
		th := *c.Theme()
		th.Background = ui.Transparent
		c.SetTheme(&th)
		ui.Box(c).Fill().Center().Children(func() {
			ui.Box(c).Size(824, 824).Center().Radius(184).
				LinearGradient(ui.LinearGradient{
					Angle: 160, Oklab: true,
					From: ui.Hex("#3b82f6"), To: ui.Hex("#1d4ed8"),
				}).
				Shadow(0, 16, 40, 0, ui.RGBA(0, 0, 0, 0.25)).
				Children(func() {
					ui.Icon(c, iconAnchor).Size(520, 520).TextColor(ui.Hex("#ffffff"))
				})
		})
	}
	img := ui.Render(view, 1024, 1024, 1)
	if _, _, _, a := img.At(4, 4).RGBA(); a != 0 {
		t.Fatalf("图标的角应该是透明的，alpha=%d", a)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("resources/icon.png", buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}

// 托盘图标：黑色线条锚、透明底，macOS 会按菜单栏的颜色着色
func TestGenerateTrayIcon(t *testing.T) {
	if os.Getenv("GEN_ICON") == "" {
		t.Skip("设置 GEN_ICON=1 才重新生成 resources/tray.png")
	}
	view := func(c *ui.Context) {
		th := *c.Theme()
		th.Background = ui.Transparent
		c.SetTheme(&th)
		ui.Box(c).Fill().Center().Children(func() {
			ui.Icon(c, iconAnchor).Size(30, 30).TextColor(ui.Hex("#000000"))
		})
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, ui.Render(view, 32, 32, 1)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("resources/tray.png", buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}
