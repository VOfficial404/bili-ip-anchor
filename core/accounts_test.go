package core

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testEngine 返回一个用临时数据目录的 Engine
func testEngine(t *testing.T) *Engine {
	t.Helper()
	return New(Config{
		DataDir:  t.TempDir(),
		Version:  "test",
		Password: "correct horse",
		Defaults: Settings{Videos: []string{"BV1GJ411x7h7"}, Interval: Duration(6 * time.Hour), Jitter: Duration(30 * time.Minute)},
	})
}

func TestAccountStorage(t *testing.T) {
	e := testEngine(t)

	// v0.3 的旧会话文件应迁移到 accounts/<UID>/
	legacy, _ := json.Marshal(map[string]string{"SESSDATA": "s1", "bili_jct": "j1", "DedeUserID": "42"})
	os.WriteFile(filepath.Join(e.cfg.DataDir, "cookies.json"), legacy, 0600)
	e.migrateLegacyCookies()
	if _, err := os.Stat(filepath.Join(e.cfg.DataDir, "cookies.json")); !os.IsNotExist(err) {
		t.Error("旧的 cookies.json 应该被移走")
	}

	if _, err := e.saveAccountCookies(map[string]string{"SESSDATA": "s2", "bili_jct": "j2", "DedeUserID": "7"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.saveAccountCookies(map[string]string{"SESSDATA": "s", "bili_jct": "j", "DedeUserID": "../x"}); err == nil {
		t.Error("非法 UID 应该被拒绝")
	}
	if _, err := e.saveAccountCookies(map[string]string{"DedeUserID": "8"}); err == nil {
		t.Error("缺少 SESSDATA 应该被拒绝")
	}

	mids, err := e.listAccounts()
	if err != nil || strings.Join(mids, ",") != "42,7" {
		t.Fatalf("listAccounts = %v, %v", mids, err)
	}
	c, err := e.loadAccountCookies("42")
	if err != nil || c["SESSDATA"] != "s1" {
		t.Fatalf("迁移后的会话不对：%v %v", c, err)
	}
	if _, err := e.loadAccountCookies("99"); !errors.Is(err, errNoCookies) {
		t.Errorf("不存在的账号应返回 errNoCookies，得到 %v", err)
	}

	if err := e.saveProfile(profile{Mid: "7", Uname: "测试", Stats: &Stats{Coins: 12.5}}); err != nil {
		t.Fatal(err)
	}
	if p := e.loadProfile("7"); p.Uname != "测试" || p.Mid != "7" || p.Stats == nil || p.Stats.Coins != 12.5 {
		t.Errorf("loadProfile = %+v", p)
	}

	exp := time.Now().Add(180 * 24 * time.Hour).Truncate(time.Second)
	if err := e.updateSession("7", func(s *session) { s.RefreshToken, s.CookieExpires = "rt", exp }); err != nil {
		t.Fatal(err)
	}
	st := e.newAccount("7").status()
	if !st.Cookie.CanRefresh || st.Cookie.Expires == nil || !st.Cookie.Expires.Equal(exp) || st.AppAuth {
		t.Errorf("账号状态里的 Cookie 信息不对：%+v", st.Cookie)
	}
}

func TestLegacyDynamicImageMigration(t *testing.T) {
	e := testEngine(t)
	os.WriteFile(filepath.Join(e.cfg.DataDir, "dynamic.png"), []byte("\x89PNG\r\n\x1a\nxxxx"), 0600)
	e.migrateLegacyDynamicImage()
	list, err := e.listImages()
	if err != nil || len(list) != 1 || !strings.HasSuffix(list[0].Name, ".png") {
		t.Fatalf("迁移后的图片不对：%v %v", list, err)
	}
	if _, err := os.Stat(filepath.Join(e.cfg.DataDir, "dynamic.png")); !os.IsNotExist(err) {
		t.Error("旧的 dynamic.png 应该被移走")
	}
}

func TestImages(t *testing.T) {
	e := testEngine(t)
	if _, err := e.addImage([]byte("not an image")); err == nil {
		t.Error("不是图片应该被拒绝")
	}
	jpg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 1, 2, 3}
	var names []string
	for i := 0; i < maxImages; i++ {
		n, err := e.addImage(jpg)
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	if _, err := e.addImage(jpg); !errors.Is(err, errTooManyImages) {
		t.Errorf("超过 %d 张应该被拒绝，得到 %v", maxImages, err)
	}
	list, _ := e.listImages()
	for i, img := range list {
		if img.Name != names[i] {
			t.Fatalf("图片顺序不对：%v", list)
		}
	}
	if _, err := e.readImage("../settings.json"); err == nil {
		t.Error("非法文件名应该被拒绝")
	}
	if err := e.removeImage(names[0]); err != nil {
		t.Fatal(err)
	}
	if list, _ := e.listImages(); len(list) != maxImages-1 {
		t.Errorf("删除后还有 %d 张", len(list))
	}
}
