package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/egoist/mygo"
)

// prefs 是记住的连接信息。密码不放在这里，放在系统的凭据管理器里。
type prefs struct {
	Mode      string `json:"mode"` // remote 或 local
	Server    string `json:"server"`
	Remember  bool   `json:"remember"`
	Proxy     string `json:"proxy"`      // 本机模式用的代理
	AutoLocal bool   `json:"auto_local"` // 下次打开时自动开始本机运行
}

// store 保存连接信息和密码，并提供本机模式的数据目录；测试里换成内存实现
type store interface {
	load() prefs
	save(p prefs) error
	password() (string, error)
	setPassword(pw string) error
	forgetPassword() error
	dataDir() (string, error)
}

const passwordKey = "panel-password"

type systemStore struct{}

func prefsFile() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "prefs.json"), nil
}

func (systemStore) load() prefs {
	p := prefs{Mode: modeRemote, Server: "http://127.0.0.1:8080"}
	f, err := prefsFile()
	if err != nil {
		return p
	}
	if b, err := os.ReadFile(f); err == nil {
		json.Unmarshal(b, &p)
	}
	return p
}

func (systemStore) save(p prefs) error {
	f, err := prefsFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(f, b, 0600)
}

func (systemStore) password() (string, error) {
	b, err := mygo.Secrets.Get(passwordKey)
	return string(b), err
}

func (systemStore) setPassword(pw string) error {
	return mygo.Secrets.Set(passwordKey, []byte(pw))
}

func (systemStore) forgetPassword() error {
	err := mygo.Secrets.Delete(passwordKey)
	if errors.Is(err, mygo.ErrSecretNotFound) {
		return nil
	}
	return err
}

type memStore struct {
	mu  sync.Mutex
	p   prefs
	pw  string
	dir string
}

func (m *memStore) load() prefs {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.p
}

func (m *memStore) save(p prefs) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.p = p
	return nil
}

func (m *memStore) password() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pw == "" {
		return "", mygo.ErrSecretNotFound
	}
	return m.pw, nil
}

func (m *memStore) setPassword(pw string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pw = pw
	return nil
}

func (m *memStore) forgetPassword() error { return m.setPassword("") }

// dataDir 是本机模式下账号会话和设置的存放位置，和服务器的数据目录结构一样
func (systemStore) dataDir() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "data"), nil
}

func (m *memStore) dataDir() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dir == "" {
		return "", errors.New("测试里没有设置数据目录")
	}
	return m.dir, nil
}
