package main

import (
	"encoding/json"
	"os"
	"testing"
)

// 发布时标签要和 mygo.json 一致，程序里显示的版本也要一致
func TestVersionMatchesMygoJSON(t *testing.T) {
	b, err := os.ReadFile("mygo.json")
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	if "v"+cfg.Version != version {
		t.Errorf("mygo.json 的 version 是 %s，代码里是 %s", cfg.Version, version)
	}
}
