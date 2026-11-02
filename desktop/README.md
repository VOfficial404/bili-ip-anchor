# bili-ip-anchor-desktop

[![CI](https://github.com/VOfficial404/bili-ip-anchor-desktop/actions/workflows/ci.yml/badge.svg)](https://github.com/VOfficial404/bili-ip-anchor-desktop/actions/workflows/ci.yml)

[bili-ip-anchor](https://github.com/VOfficial404/bili-ip-anchor) 的桌面客户端，用 [MyGo](https://mygo.egoist.dev/) 的原生 UI 写成，支持 Windows、macOS 和 Linux。

**定时任务仍然在你的云服务器上跑，这个程序只是遥控器。** 它通过服务器 WebUI 的接口查看状态、扫码添加账号、修改设置和看日志，功能和网页面板一样，只是换成了独立的窗口。B 站看到的始终是服务器的 IP，桌面客户端本身不会以你的账号访问 B 站（头像图片除外，见下文）。

## 准备服务器

需要 bili-ip-anchor **v0.4.0 或更新版本**，并且设置了 `WEBUI_PASSWORD`，启用了 WebUI。部署方法见 [bili-ip-anchor 的 README](https://github.com/VOfficial404/bili-ip-anchor#快速开始docker-compose推荐)。

WebUI 默认只监听服务器本机，桌面客户端可以用两种方式连过去：

- **SSH 隧道**：在自己电脑上运行下面这条命令并保持打开，然后在客户端里填 `http://127.0.0.1:8080`。

  ```bash
  ssh -N -L 8080:127.0.0.1:8080 root@你的服务器IP
  ```

- **HTTPS 域名**：服务器上用 Caddy 等反向代理配好 HTTPS 后，填 `https://你的域名`。

## 安装

从 [Releases](https://github.com/VOfficial404/bili-ip-anchor-desktop/releases/latest) 下载：

| 系统 | 文件 |
|---|---|
| Windows | `bili-ip-anchor-desktop-setup-*-amd64.exe`（安装包），或 `*-windows-amd64-portable.zip`（免安装，解压即用） |
| macOS | `bili-ip-anchor-desktop-*.dmg`（Intel 和 Apple 芯片通用） |
| Linux | `.deb` 包，或 `.tar.gz`（解压后运行 `install.sh` 为当前用户安装） |

安装包还没有签名：Windows 可能弹出 SmartScreen 提示，点「更多信息 → 仍要运行」；macOS 第一次打开时在 Finder 里右键 → 打开。

## 使用

1. 打开后填服务器地址和面板密码，点「连接」。勾选「记住密码」后，密码保存在系统的凭据管理器里（Windows 凭据管理器、macOS 钥匙串、Linux 的 Secret Service），下次打开自动连接。
2. 左侧四个页面：
   - **概览**：B 站把服务器识别为哪里，以及账号的整体情况。
   - **账号**：每个账号的状态、上次和下次运行时间；可以立即运行、重新扫码、退出。「添加账号」会显示一张 B 站登录二维码，用手机 App 扫码即可。二维码由服务器向 B 站申请，扫码登录的会话也保存在服务器上。
   - **设置**：间隔、随机抖动、心跳视频、图片动态，所有账号共用。
   - **日志**：服务器最近 300 行日志。
3. 界面每 5 秒自动刷新，跟随系统的浅色/深色模式。

账号头像由客户端直接从 B 站图床（`*.hdslb.com`）下载，只用来显示，不带任何账号信息，也不影响 IP 属地。

## 从源码构建

需要 Go 1.27 或更新版本，不需要 cgo，在任何系统上都能交叉编译三个平台：

```bash
git clone https://github.com/VOfficial404/bili-ip-anchor-desktop.git && cd bili-ip-anchor-desktop
go test ./...
go tool mygo dev                                    # 开发：改代码自动重启
go tool mygo build -platform windows/amd64          # 打包到 build/windows-amd64/
```

- `go test ./...` 会在没有窗口的情况下跑界面测试（用一个模拟的服务器）。设置 `SNAPSHOT_DIR=某个目录` 会把每个页面的截图存成 PNG。
- 连一台真的服务器测一遍：`BIA_SERVER=http://127.0.0.1:8080 BIA_PASSWORD=你的密码 go test -run TestLiveServer`。
- 应用图标也是用 MyGo 画的：`GEN_ICON=1 go test -run TestGenerateIcon` 重新生成 `resources/icon.png`。

推送 `v*` 标签时，GitHub Actions 会构建三个平台的安装包并发布到 Releases（标签要和 `mygo.json` 的 `version` 一致）。

## 许可

[MIT](LICENSE)。本项目与哔哩哔哩官方无关。
