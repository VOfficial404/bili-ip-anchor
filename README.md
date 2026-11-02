# bili-ip-anchor

[![Docker](https://github.com/VOfficial404/bili-ip-anchor/actions/workflows/docker.yml/badge.svg)](https://github.com/VOfficial404/bili-ip-anchor/actions/workflows/docker.yml)
[![CI](https://github.com/VOfficial404/bili-ip-anchor/actions/workflows/ci.yml/badge.svg)](https://github.com/VOfficial404/bili-ip-anchor/actions/workflows/ci.yml)

在你自己的云服务器上定时以 B 站账号身份发起少量请求，尝试把个人主页的「IP 属地」锚定到服务器所在地区，**本地不需要挂任何代理**，看视频的流量不经过服务器。支持多个账号同时运行，自带 WebUI，可以在网页上扫码添加账号、查看状态和修改设置。

> [!WARNING]
> - **效果不保证。** B 站没有公开主页 IP 属地依据哪类请求计算。本工具只是让服务器 IP 出现在账号的活动记录里，是否能改变主页显示，需要你按下文「验证是否生效」自行测试。
> - **本地发布的内容不受影响。** 每条评论、动态的属地按发布时的 IP 记录，你在本地发的内容仍显示本地属地，也可能把主页属地刷回去。
> - **有风控和违规风险。** B 站社区规则不鼓励刻意伪造属地，自动化请求也可能触发风控。是否使用、后果如何，请自行判断和承担。

## 工作原理

1. 在服务器上扫码登录一个或多个账号，每个账号得到一份**独立的登录会话**（相当于多了一台登录设备，不用从浏览器复制 Cookie）。
2. 之后每个账号**各自排程、同时进行**，每隔一段时间（默认 6 小时 + 0~30 分钟随机抖动）以自己的会话：
   - 请求 `nav` 接口确认登录状态；
   - 上报一次视频观看心跳；
   - （可选，默认关闭）发一条图片动态，几秒后删除。
3. 这些请求都从服务器发出，B 站记录到的是服务器的 IP。

每轮只有几 KB 流量。

## WebUI

设置了 `WEBUI_PASSWORD` 后，守护进程会同时提供一个网页面板。它的样子和 [MyGo](https://mygo.egoist.dev/) 原生 UI 的默认主题一致，跟桌面客户端看起来一样，并跟随系统的浅色/深色模式。左侧四个页面：

- **概览**：用 B 站自己的接口查询服务器出口 IP 的归属地和运营商，部署后一眼就能确认服务器会被识别成哪里；以及账号的整体情况。
- **账号**：扫码添加多个账号。每个账号显示头像、昵称、状态、上次和下次运行时间，可以单独立即运行、重新扫码或退出（退出会在 B 站注销这份会话）；也可以全部立即运行。
- **设置**：在线修改间隔、随机抖动、心跳视频、是否发动态，所有账号共用，保存后立即生效。
- **日志**：最近 300 行运行日志，每行带账号名。

### 桌面客户端

不想开浏览器，可以用 [bili-ip-anchor-desktop](https://github.com/VOfficial404/bili-ip-anchor-desktop)：用 MyGo 写的 Windows、macOS、Linux 桌面程序，功能和网页面板相同，连的也是这个 WebUI（地址和面板密码同下）。定时任务仍然在服务器上跑。

面板只用密码保护，并且是明文 HTTP，所以 Compose 默认**只监听服务器本机** `127.0.0.1:8080`。从自己电脑访问有两种方式：

**SSH 隧道（最简单）**：在自己电脑上执行下面的命令，然后打开 <http://localhost:8080>。

```bash
ssh -N -L 8080:127.0.0.1:8080 root@你的服务器IP
```

**HTTPS 反向代理（有域名时）**：例如 Caddy，会自动申请证书：

```
bili.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

## 快速开始（Docker Compose，推荐）

服务器需要已安装 Docker 和 Compose 插件。

```bash
mkdir -p /opt/bili-ip-anchor && cd /opt/bili-ip-anchor
curl -fsSLO https://raw.githubusercontent.com/VOfficial404/bili-ip-anchor/main/compose.yaml
curl -fsSL -o .env https://raw.githubusercontent.com/VOfficial404/bili-ip-anchor/main/.env.example
```

**1. 设置 WebUI 密码**（不需要 WebUI 可以跳过）：

```bash
sed -i "s|^WEBUI_PASSWORD=.*|WEBUI_PASSWORD=$(openssl rand -base64 18)|" .env && grep WEBUI_PASSWORD .env
```

记下输出的密码。

**2. 启动：**

```bash
docker compose up -d
```

**3. 扫码添加账号**，二选一，想加几个账号就重复几次：

- 用 WebUI：按上面的方法打开面板，点「添加账号」。
- 用终端（终端里会显示二维码）：

  ```bash
  docker compose run --rm bili-ip-anchor login
  ```

每个账号的会话保存在 `./data/accounts/<UID>/`。WebUI 添加的账号立即开始运行；终端添加的账号，守护进程会在 1 分钟内自动接上，不需要重启。同一个账号再扫一次，会更新它的会话。

> 从 v0.3 及更早版本升级：旧的 `./data/cookies.json` 会在启动时自动迁移到 `./data/accounts/<UID>/`，不需要重新扫码。

**查看日志：**

```bash
docker compose logs -f
```

正常的话会看到：

```
2026/11/01 12:00:00 bili-ip-anchor v0.5.0 守护模式启动：间隔 6h，随机抖动 30m，数据目录 /data
2026/11/01 12:00:00 已加载 2 个账号，各自独立排程
2026/11/01 12:00:00 WebUI 监听 :8080
2026/11/01 12:00:01 [你的昵称] 登录正常：你的昵称
2026/11/01 12:00:01 [你的昵称] 心跳：0 0
2026/11/01 12:00:01 [你的昵称] 下一轮：2026-11-01 18:12:37
2026/11/01 12:00:16 [另一个号] 登录正常：另一个号
```

启动时各账号的第一轮会错开十几秒，之后各自按间隔加随机抖动运行。日志出现「登录已失效」时，给那个账号重新扫码即可，不需要重启。

**更新镜像：**

```bash
docker compose pull && docker compose up -d
```

## 不用 Docker：直接运行二进制

从 [Releases](https://github.com/VOfficial404/bili-ip-anchor/releases) 下载对应架构的文件（服务器上执行 `uname -m`：`x86_64` 选 `linux_amd64`，`aarch64` 选 `linux_arm64`）。

```bash
mkdir -p /opt/bili-ip-anchor && cd /opt/bili-ip-anchor
curl -fsSL -o bili-ip-anchor https://github.com/VOfficial404/bili-ip-anchor/releases/latest/download/bili-ip-anchor_linux_amd64
chmod +x bili-ip-anchor
./bili-ip-anchor login
```

然后二选一：

- **常驻运行**（推荐配合 systemd，见下）：`WEBUI_PASSWORD=你的密码 WEBUI_ADDR=127.0.0.1:8080 ./bili-ip-anchor daemon`
- **交给 cron**（没有 WebUI）：`crontab -e` 加入

  ```
  0 */6 * * * /opt/bili-ip-anchor/bili-ip-anchor refresh >> /opt/bili-ip-anchor/refresh.log 2>&1
  ```

  `refresh` 不加 `--now` 时会先随机等待 0~`JITTER`。

不设 `DATA_DIR` 时，`accounts/` 和 `settings.json` 保存在二进制所在目录。再运行一次 `login` 可以添加更多账号；`refresh` 会为所有账号各跑一轮。

<details>
<summary>systemd 服务示例</summary>

`/etc/systemd/system/bili-ip-anchor.service`：

```ini
[Unit]
Description=bili-ip-anchor
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/opt/bili-ip-anchor/bili-ip-anchor daemon
Environment=DATA_DIR=/opt/bili-ip-anchor
Environment=WEBUI_ADDR=127.0.0.1:8080
Environment=WEBUI_PASSWORD=换成你的密码
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

```bash
chmod 600 /etc/systemd/system/bili-ip-anchor.service
systemctl daemon-reload && systemctl enable --now bili-ip-anchor
```

</details>

## 配置

全部通过环境变量配置。用 Compose 时修改 `.env`（模板见 [`.env.example`](.env.example)）。

| 变量 | 默认值 | 说明 |
|---|---|---|
| `WEBUI_PASSWORD` | 空 | 设置后启用 WebUI。建议用 `openssl rand -base64 18` 生成 |
| `WEBUI_ADDR` | `:8080` | WebUI 监听地址（容器内）。二进制直接运行时建议设为 `127.0.0.1:8080` |
| `WEBUI_PORT` | `8080` | 仅 Compose：映射到服务器本机的端口 |
| `INTERVAL` | `6h` | 每轮间隔，Go duration 格式，如 `90m`、`12h`，最少 `10m` |
| `JITTER` | `30m` | 每轮额外随机等待 0~JITTER，避免时间过于规律 |
| `BVID` | `BV15HFVz1EvM` | 心跳上报使用的视频，任意公开视频即可 |
| `DO_DYNAMIC` | `false` | 每轮发一条图片动态再删除。只发心跳不生效时再打开；动态会短暂出现在粉丝的动态流里。图片见下文「动态图片」 |
| `DATA_DIR` | 镜像内 `/data`；二进制为所在目录 | `accounts/`（各账号会话）、`settings.json` 存放位置 |
| `TZ` | `Asia/Shanghai` | 日志时区 |

`INTERVAL`、`JITTER`、`BVID`、`DO_DYNAMIC` 在 WebUI 里保存过之后，以 `data/settings.json` 为准，环境变量不再生效。想恢复用环境变量，删除这个文件并重启即可。

## 动态图片

打开 `DO_DYNAMIC` 后，每轮发的动态内容是一张图片，没有文字。默认使用仓库里的 [`assets/dynamic.jpg`](assets/dynamic.jpg)（已打包进程序）。

想换成别的图片，把 `dynamic.jpg` 或 `dynamic.png` 放进数据目录（Compose 下是 `./data/`），下一轮就会用它，不需要重启。

## 验证是否生效

1. 用浏览器**无痕窗口、不登录**打开你的个人空间，记下当前属地。不要用已登录的浏览器看，那本身就是一次从本地 IP 发出的请求。
2. 测试期间本地尽量不打开 B 站。
3. 在 WebUI 点「立即运行」，或执行 `docker compose run --rm bili-ip-anchor refresh --now`。
4. 隔几小时到一天，再用无痕窗口查看：
   - 变成服务器所在地：只发心跳就够了，保持默认配置。
   - 没变：在 WebUI 里勾选「发动态」（或在 `.env` 设 `DO_DYNAMIC=true`），再测一轮。
   - 仍没变：说明 B 站只按登录或发布内容时的 IP 计算，这个方案不适用。可以改用「只把 B 站接口域名分流到服务器代理、视频 CDN 直连」的方式。

## 安全

- `accounts/` 里的 `cookies.json` 等同于账号登录权限。保护好服务器，不要把 `data/` 目录提交到任何地方（仓库的 `.gitignore` 已排除）。
- WebUI 能扫码登录、退出你的账号。不要把它不加 HTTPS 直接暴露到公网，密码用足够长的随机字符串。
- 不想用了：在 WebUI 里对每个账号点「退出」（或删除 `data/` 目录后到 B 站「账号安全 → 登录设备」里把这台服务器下线），再删除容器。

## 从源码构建

```bash
git clone https://github.com/VOfficial404/bili-ip-anchor.git && cd bili-ip-anchor
go test ./... && go build -o bili-ip-anchor .
# 或者构建镜像
docker build -t bili-ip-anchor .
```

WebUI 的页面在 `web/` 目录，编译时通过 `go:embed` 打包进二进制，运行时不需要额外文件。

镜像由 GitHub Actions 自动构建并发布到 `ghcr.io/vofficial404/bili-ip-anchor`（`linux/amd64`、`linux/arm64`）：

- 推送到 `main`：更新 `latest` 标签
- 推送 `v*` 标签：发布版本号标签，并在 Releases 附上各平台二进制

## 致谢

接口参考 [SocialSisterYi/bilibili-API-collect](https://github.com/SocialSisterYi/bilibili-API-collect)。B 站接口随时可能变化，出问题欢迎提 Issue。

## 许可

[MIT](LICENSE)。`assets/dynamic.jpg` 不在 MIT 许可范围内，版权归原作者所有。本项目与哔哩哔哩官方无关。
