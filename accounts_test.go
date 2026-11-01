package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAccountStorage(t *testing.T) {
	cfg.DataDir = t.TempDir()

	// v0.3 的旧会话文件应迁移到 accounts/<UID>/
	legacy, _ := json.Marshal(map[string]string{"SESSDATA": "s1", "bili_jct": "j1", "DedeUserID": "42"})
	os.WriteFile(filepath.Join(cfg.DataDir, "cookies.json"), legacy, 0600)
	migrateLegacyCookies()
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "cookies.json")); !os.IsNotExist(err) {
		t.Error("旧的 cookies.json 应该被移走")
	}

	if _, err := saveAccountCookies(map[string]string{"SESSDATA": "s2", "bili_jct": "j2", "DedeUserID": "7"}); err != nil {
		t.Fatal(err)
	}
	if _, err := saveAccountCookies(map[string]string{"SESSDATA": "s", "bili_jct": "j", "DedeUserID": "../x"}); err == nil {
		t.Error("非法 UID 应该被拒绝")
	}
	if _, err := saveAccountCookies(map[string]string{"DedeUserID": "8"}); err == nil {
		t.Error("缺少 SESSDATA 应该被拒绝")
	}

	mids, err := listAccounts()
	if err != nil || strings.Join(mids, ",") != "42,7" {
		t.Fatalf("listAccounts = %v, %v", mids, err)
	}
	c, err := loadAccountCookies("42")
	if err != nil || c["SESSDATA"] != "s1" {
		t.Fatalf("迁移后的会话不对：%v %v", c, err)
	}
	if _, err := loadAccountCookies("99"); !errors.Is(err, errNoCookies) {
		t.Errorf("不存在的账号应返回 errNoCookies，得到 %v", err)
	}

	if err := saveProfile(profile{Mid: "7", Uname: "测试"}); err != nil {
		t.Fatal(err)
	}
	if p := loadProfile("7"); p.Uname != "测试" || p.Mid != "7" {
		t.Errorf("loadProfile = %+v", p)
	}
}
