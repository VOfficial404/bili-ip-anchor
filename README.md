# bili-ip-anchor

[![Docker](https://github.com/VOfficial404/bili-ip-anchor/actions/workflows/docker.yml/badge.svg)](https://github.com/VOfficial404/bili-ip-anchor/actions/workflows/docker.yml)
[![CI](https://github.com/VOfficial404/bili-ip-anchor/actions/workflows/ci.yml/badge.svg)](https://github.com/VOfficial404/bili-ip-anchor/actions/workflows/ci.yml)

在你自己的云服务器上定时以 B 站账号身份发起少量请求，尝试把个人主页的「IP 属地」锚定到服务器所在地区，**本地不需要挂任何代理**，看视频的流量不经过服务器。自带 WebUI，可以在网页上扫码登录、查看状态和修改设置。

> [!WARNING]
> - **效果不保证。** B 站没有公开主页 IP 属地依据哪类请求计算。本工具只是让服务器 IP 出现在账号的活动记录里，是否能改变主页显示，需要你按下文「验证是否生效」自行测试。
> - **本地发布的内容不受影响。** 每条评论、动态的属地按发布时的 IP 记录，你在本地发的内容仍显示本地属地，也可能把主页属地刷回去。
> - **有风控和违规风险。** B 站社区规则不鼓励刻意伪造属地，自动化请求也可能触发风控。是否使用、后果如何，请自行判断和承担。

## 工作原理

1. 在服务器上扫码登录，得到一份**独立的登录会话**（相当于多了一台登录设备，不用从浏览器复制 Cookie）。
2. 之后每隔一段时间（默认 6 小时 + 0~30 分钟随机抖动），程序以这个会话：
   - 请求 `nav` 接口确认登录状态；
   - 上报一次视频观看心跳；
   - （可选，默认关闭）发一条动态，几秒后删除。
3. 这些请求都从服务器发出，B 站记录到的是服务器的 IP。

每轮只有几 KB 流量。

## WebUI

设置了 `WEBUI_PASSWORD` 后，守护进程会同时提供一个网页面板：

- **B 站识别到的服务器位置**：用 B 站自己的接口查询服务器出口 IP 的归属地和运营商，部署前就能确认服务器会被识别成哪里。
- **账号**：网页上扫码登录、退出账号（会在 B 站注销这份会话）。
- **运行**：上次运行时间和结果、下次运行时间，可以立即运行一轮。
- **设置**：在线修改间隔、随机抖动、心跳视频、是否发动态，保存后立即生效。
- **日志**：最近 300 行运行日志。

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

**3. 扫码登录**，二选一：

- 用 WebUI：按上面的方法打开面板，点「扫码登录」。
- 用终端（终端里会显示二维码）：

  ```bash
  docker compose run --rm bili-ip-anchor login
  ```

会话保存在 `./data/cookies.json`。登录后守护进程会在下一轮（WebUI 登录则是立即）开始工作。

**查看日志：**

```bash
docker compose logs -f
```

正常的话会看到：

```
2026/11/01 12:00:00 bili-ip-anchor v0.2.0 守护模式启动：间隔 6h，随机抖动 30m，数据目录 /data
2026/11/01 12:00:00 WebUI 监听 :8080
2026/11/01 12:00:01 已登录：你的昵称
2026/11/01 12:00:01 心跳：0 0
2026/11/01 12:00:01 下一轮：2026-11-01 18:12:37
```

日志出现「登录已失效」时重新扫码登录即可，不需要重启。

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

不设 `DATA_DIR` 时，`cookies.json` 和 `settings.json` 保存在二进制所在目录。

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
| `BVID` | `BV1GJ411x7h7` | 心跳上报使用的视频，任意公开视频即可 |
| `DO_DYNAMIC` | `false` | 每轮发一条动态再删除。只发心跳不生效时再打开；动态会短暂出现在粉丝的动态流里 |
| `DATA_DIR` | 镜像内 `/data`；二进制为所在目录 | `cookies.json`、`settings.json` 存放位置 |
| `TZ` | `Asia/Shanghai` | 日志时区 |

`INTERVAL`、`JITTER`、`BVID`、`DO_DYNAMIC` 在 WebUI 里保存过之后，以 `data/settings.json` 为准，环境变量不再生效。想恢复用环境变量，删除这个文件并重启即可。

## 验证是否生效

1. 用浏览器**无痕窗口、不登录**打开你的个人空间，记下当前属地。不要用已登录的浏览器看，那本身就是一次从本地 IP 发出的请求。
2. 测试期间本地尽量不打开 B 站。
3. 在 WebUI 点「立即运行」，或执行 `docker compose run --rm bili-ip-anchor refresh --now`。
4. 隔几小时到一天，再用无痕窗口查看：
   - 变成服务器所在地：只发心跳就够了，保持默认配置。
   - 没变：在 WebUI 里勾选「发动态」（或在 `.env` 设 `DO_DYNAMIC=true`），再测一轮。
   - 仍没变：说明 B 站只按登录或发布内容时的 IP 计算，这个方案不适用。可以改用「只把 B 站接口域名分流到服务器代理、视频 CDN 直连」的方式。

## 安全

- `cookies.json` 等同于账号登录权限。保护好服务器，不要把 `data/` 目录提交到任何地方（仓库的 `.gitignore` 已排除）。
- WebUI 能扫码登录、退出你的账号。不要把它不加 HTTPS 直接暴露到公网，密码用足够长的随机字符串。
- 不想用了：在 WebUI 点「退出账号」（或删除 `data/` 目录后到 B 站「账号安全 → 登录设备」里把这台服务器下线），再删除容器。

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

[MIT](LICENSE)。本项目与哔哩哔哩官方无关。
