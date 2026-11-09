# bili-ip-anchor

[![Docker](https://github.com/VOfficial404/bili-ip-anchor/actions/workflows/docker.yml/badge.svg)](https://github.com/VOfficial404/bili-ip-anchor/actions/workflows/docker.yml)
[![CI](https://github.com/VOfficial404/bili-ip-anchor/actions/workflows/ci.yml/badge.svg)](https://github.com/VOfficial404/bili-ip-anchor/actions/workflows/ci.yml)

定时以 B 站账号身份发起少量请求，尝试把个人主页的「IP 属地」锚定到另一个地区，**看视频的流量不经过它**。三种用法：

- **云服务器（推荐）**：用 Docker 跑在你的服务器上，属地锚定到服务器所在地区，本地不需要挂任何代理。
- **桌面客户端**：Windows、macOS、Linux。可以遥控服务器，也可以没有服务器、直接在电脑上运行，请求经过电脑上代理软件的本地端口出去。
- **手机 App**：Android 和 iOS，和桌面客户端一样两种用法。

支持多个账号同时运行。每个账号都能看到主页现在的属地、目标属地、Cookie 有效期和硬币数。

> [!WARNING]
> - **效果不保证。** B 站没有公开主页 IP 属地依据哪类请求计算。本工具只是让目标 IP 出现在账号的活动记录里，是否能改变主页显示，需要你按下文「验证是否生效」自行测试。
> - **本地发布的内容不受影响。** 每条评论、动态的属地按发布时的 IP 记录，你在本地发的内容仍显示本地属地，也可能把主页属地刷回去。
> - **有风控和违规风险。** B 站社区规则不鼓励刻意伪造属地，自动化请求也可能触发风控。是否使用、后果如何，请自行判断和承担。

## 工作原理

1. 扫码登录一个或多个账号，每个账号得到一份**独立的登录会话**（相当于多了一台登录设备，不用从浏览器复制 Cookie）。
2. 之后每个账号**各自排程、同时进行**，每隔一段时间（默认 6 小时 + 0~30 分钟随机抖动）以自己的会话：
   - 请求 `nav` 接口确认登录状态，顺便更新硬币、等级等资料；
   - 询问 B 站 Cookie 是否需要刷新，需要时自动刷新；
   - 从心跳视频里随机选一个，上报一次观看心跳；
   - （可选，默认关闭）发一条动态（文字和/或图片），几秒后删除。
3. 这些请求都从服务器（或代理）发出，B 站记录到的是那里的 IP。

每轮只有几 KB 流量。

## 面板

服务器上设置了 `WEBUI_PASSWORD` 后会提供一个网页面板，基于 [shadcn-admin](https://github.com/satnaing/shadcn-admin)：侧边栏可以折叠，手机浏览器上是抽屉；按 ⌘K / Ctrl+K 打开命令面板；浅色、深色或跟随系统。桌面客户端用 [MyGo](https://mygo.egoist.dev/) 画界面；手机 App 是原生界面，Android 用 Jetpack Compose（Material 3，Android 12 起跟随壁纸取色），iOS 用 SwiftUI。页面和功能四者一致：

- **概览**：B 站把出口 IP 识别为哪里（目标属地），有几个账号的主页已经是目标属地，下一次运行时间。
- **账号**：每个账号一张卡片：
  - 头像、昵称、等级、大会员、硬币和 B 币；
  - **主页属地**：主页现在显示的属地，是否已是目标属地、从什么时候起一致；还不是时显示预计什么时候变（B 站没有公开规则，只能说「下一轮之后几小时内」）。需要先「授权查询」，见下文；
  - **目标属地**：出口 IP、地区和运营商；
  - **运行**：下次运行时间、上次结果、上次成功从目标 IP 发出请求的时间；
  - **Cookie**：还有多少天过期，可以「刷新 Cookie」；
  - **收藏夹**：导出成文件，或者从文件导入，见下文；
  - 立即运行、刷新资料（不发心跳）、重新扫码、退出（会在 B 站注销这份会话）。
- **设置**：所有账号共用，保存后立即生效：
  - 间隔、随机抖动；
  - 代理，可以「测试」看 B 站经过它识别为哪里；
  - 心跳视频：可以添加多个，粘贴 BV 号或视频链接，会显示标题、封面、UP 主和时长；
  - 动态：开关、文字、最多 9 张图片，并预览每轮会发什么；
  - 日志级别：调试、信息、警告、错误，低于它的不记录。
- **日志**：最近 1000 条运行日志，每条带级别和账号名，可以按级别（全部、信息及以上、警告及以上、只看错误）和账号筛选，打开时自动拉取新的。「调试」级别会记下每一个发往 B 站的请求，排查问题时再打开。

侧边栏底部显示连的是哪里（面板地址或「本机运行」）以及 B 站看到的出口 IP。

### 主页属地查询（可选）

B 站的网页接口查不到账号主页显示的 IP 属地，只有 App 接口能查。在账号卡片上点「授权查询」，用**同一个账号**的 B 站 App 再扫一次码，就会得到一份 App 授权（相当于多一台「电视」登录设备），之后每轮顺便查一次主页属地。

这份授权只用来查询主页属地，保存在 `accounts/<UID>/session.json`。点「取消授权」或「退出」账号时会在 B 站撤销它。不授权也不影响定时任务，只是看不到主页属地。

### 收藏夹导出和导入

账号卡片上的「收藏夹」一栏：

- **导出**：把这个账号自己创建的全部收藏夹（名字、简介、是否私密，以及里面的视频）存成一个 JSON 文件，例如 `bili-favorites-<UID>-20261109.json`。只导出视频，音频和合集不导出；已失效的视频也记下来，但导入时跳过。收藏多时要一页页读，可能要一两分钟。
- **导入**：选一个导出的文件，导入到这个账号（可以是别的账号导出的）。
  - 默认收藏夹放进这个账号的默认收藏夹；其他收藏夹按名字放进同名的收藏夹，没有就新建，私密设置照原来的。
  - 已经收藏过的视频跳过，所以中途停下后再导入一次会接着补上。导入后的顺序和原来一样。
  - 为了不被 B 站限制，每收藏一个视频间隔两三秒，在后台进行，进度（已处理、新增、已有、失败和失败的视频）显示在账号卡片上。导入期间这个账号不运行定时任务。
  - 遇到登录失效、风控（-352、-412 等）或请求太频繁时会停下，不会一个个失败下去；过一会儿再导入一次即可。单个视频失败（例如视频已删除、收藏夹满了）只记下来，接着导入其他的。

### Cookie 有效期和刷新

扫码登录时会同时保存 Cookie 的过期时间和 `refresh_token`。每轮运行时询问 B 站 Cookie 是否需要刷新，需要就自动换一份新的（流程见 [bilibili-API-collect 的说明](https://socialsisteryi.github.io/bilibili-API-collect/docs/login/cookie_refresh.html)），也可以在账号卡片上手动「刷新 Cookie」。

v0.5 及更早版本扫码的账号没有 `refresh_token`，面板上显示「有效期未知」，重新扫码一次之后就能看到有效期并刷新。

## 云服务器：快速开始（Docker Compose）

服务器需要已安装 Docker 和 Compose 插件。

```bash
mkdir -p /opt/bili-ip-anchor && cd /opt/bili-ip-anchor
curl -fsSLO https://raw.githubusercontent.com/VOfficial404/bili-ip-anchor/main/compose.yaml
curl -fsSL -o .env https://raw.githubusercontent.com/VOfficial404/bili-ip-anchor/main/.env.example
```

**1. 设置面板密码**（不需要面板可以跳过）：

```bash
sed -i "s|^WEBUI_PASSWORD=.*|WEBUI_PASSWORD=$(openssl rand -base64 18)|" .env && grep WEBUI_PASSWORD .env
```

记下输出的密码。

**2. 启动：**

```bash
docker compose up -d
```

**3. 打开面板。** 面板只用密码保护，并且是明文 HTTP，所以 Compose 默认**只监听服务器本机** `127.0.0.1:8080`。从自己电脑访问有两种方式：

- **SSH 隧道（最简单）**：在自己电脑上执行下面的命令并保持打开，然后打开 <http://localhost:8080>，或在桌面客户端里填这个地址。

  ```bash
  ssh -N -L 8080:127.0.0.1:8080 root@你的服务器IP
  ```

- **HTTPS 反向代理（有域名时，手机 App 连服务器也用它）**：例如 Caddy，会自动申请证书：

  ```
  bili.example.com {
      reverse_proxy 127.0.0.1:8080
  }
  ```

**4. 扫码添加账号**，二选一，想加几个账号就重复几次：

- 在面板里点「添加账号」。
- 用终端（终端里会显示二维码）：

  ```bash
  docker compose run --rm bili-ip-anchor login
  ```

每个账号的会话保存在 `./data/accounts/<UID>/`。面板里添加的账号立即开始运行；终端添加的账号，守护进程会在 1 分钟内自动接上，不需要重启。同一个账号再扫一次，会更新它的会话。

**查看日志：**

```bash
docker compose logs -f
```

正常的话会看到：

```
2026/11/03 12:00:00 bili-ip-anchor v0.6.0 启动：间隔 6h，随机抖动 30m，直连，数据目录 /data
2026/11/03 12:00:00 已加载 2 个账号，各自独立排程
2026/11/03 12:00:00 WebUI 监听 :8080
2026/11/03 12:00:01 [你的昵称] 登录正常
2026/11/03 12:00:01 [你的昵称] 心跳 BV15HFVz1EvM：0 0
2026/11/03 12:00:01 [你的昵称] 主页属地：广东
2026/11/03 12:00:01 [你的昵称] 下一轮：2026-11-03 18:12:37
2026/11/03 12:00:16 [另一个号] 登录正常
```

启动时各账号的第一轮会错开十几秒，之后各自按间隔加随机抖动运行。日志出现「登录已失效」时，给那个账号重新扫码即可，不需要重启。

**更新：**

```bash
docker compose pull && docker compose up -d
```

> 从旧版本升级不需要重新扫码：v0.3 及更早的 `data/cookies.json` 会迁移到 `data/accounts/<UID>/`；v0.5 的 `settings.json` 里的 `bvid` 会变成视频列表；数据目录里的 `dynamic.jpg`、`dynamic.png` 会移到动态图片里。

### 不用 Docker：直接运行二进制

从 [Releases](https://github.com/VOfficial404/bili-ip-anchor/releases) 下载对应架构的 `bili-ip-anchor_linux_*`（服务器上执行 `uname -m`：`x86_64` 选 `linux_amd64`，`aarch64` 选 `linux_arm64`）。

```bash
mkdir -p /opt/bili-ip-anchor && cd /opt/bili-ip-anchor
curl -fsSL -o bili-ip-anchor https://github.com/VOfficial404/bili-ip-anchor/releases/latest/download/bili-ip-anchor_linux_amd64
chmod +x bili-ip-anchor
./bili-ip-anchor login
```

然后二选一：

- **常驻运行**（推荐配合 systemd，见下）：`WEBUI_PASSWORD=你的密码 WEBUI_ADDR=127.0.0.1:8080 ./bili-ip-anchor daemon`
- **交给 cron**（没有面板）：`crontab -e` 加入

  ```
  0 */6 * * * /opt/bili-ip-anchor/bili-ip-anchor refresh >> /opt/bili-ip-anchor/refresh.log 2>&1
  ```

  `refresh` 不加 `--now` 时会先随机等待 0~`JITTER`。

不设 `DATA_DIR` 时，`accounts/`、`dynamic/` 和 `settings.json` 保存在二进制所在目录。再运行一次 `login` 可以添加更多账号；`refresh` 会为所有账号各跑一轮。

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

## 桌面客户端

从 [Releases](https://github.com/VOfficial404/bili-ip-anchor/releases/latest) 下载：

| 系统 | 文件 |
|---|---|
| Windows | `bili-ip-anchor-desktop-setup-*-amd64.exe`（安装包），或 `*-windows-amd64-portable.zip`（免安装，解压即用） |
| macOS | `bili-ip-anchor-desktop-*.dmg`（Intel 和 Apple 芯片通用） |
| Linux | `.deb` 包，或 `.tar.gz`（解压后运行 `install.sh` 为当前用户安装） |

安装包还没有签名：Windows 可能弹出 SmartScreen 提示，点「更多信息 → 仍要运行」；macOS 第一次打开时在 Finder 里右键 → 打开。

打开后选择用法：

- **连接服务器**：填面板地址（SSH 隧道时是 `http://127.0.0.1:8080`）和面板密码。勾选「记住密码」后，密码保存在系统的凭据管理器里（Windows 凭据管理器、macOS 钥匙串、Linux 的 Secret Service），下次打开自动连接。服务器需要 v0.4.0 或更新版本；账号详情、多个视频、动态内容、代理需要服务器 v0.6.0。
- **在本机运行**：没有服务器时用。填电脑上代理软件的本地端口（例如 Clash 的 `http://127.0.0.1:7890`，也支持 `socks5://`），点「测试」可以看 B 站经过它把你识别为哪里，主页属地就会变成那里。只有本程序以账号身份发出的请求经过这个代理，你自己看视频不经过它。关闭窗口后程序在托盘里继续运行，托盘菜单可以重新打开或退出。账号和设置保存在系统的应用数据目录里。

账号头像和视频封面由客户端直接从 B 站图床（`*.hdslb.com`）下载，只用来显示，不带任何账号信息，也不影响 IP 属地。

## 手机 App

从 [Releases](https://github.com/VOfficial404/bili-ip-anchor/releases/latest) 下载：

- **Android**：`bili-ip-anchor-*.apk`，Android 7.0 及以上。
- **iOS**：`bili-ip-anchor-*-unsigned.ipa`，没有签名，需要用 [AltStore](https://altstore.io/)、[SideStore](https://sidestore.io/) 之类的工具用你自己的 Apple ID 签名安装。iOS 16 及以上。

界面是原生的：Android 是 Material 3 风格（底部导航栏，深色模式，Android 12 起跟随壁纸取色），iOS 是系统的列表和表单风格（底部标签栏，支持动态字体）。打开后和桌面客户端一样选择用法：

- **连接服务器**：填面板地址和面板密码，可以记住密码、下次自动连接。请求由 App 里的 Go 核心直接发给服务器，不经过手机浏览器。
- **在本机运行**：填手机上代理软件的本地端口（例如 Clash Meta for Android 的混合端口 `http://127.0.0.1:7890`）。
  - **Android** 会显示一条「正在本机运行」的常驻通知（点通知里的「停止」可以直接停下），App 退到后台后定时任务继续跑。App 被系统清理或手机重启后，再次打开会用同一个代理接着运行。手机深度休眠时可能推迟到下次唤醒再运行。
  - **iOS** 不允许 App 在后台一直运行：只有 App 在前台时才会运行，切回 App 时补上错过的那一轮。iPhone 更适合用来连服务器。

在「设置」最下面点「回到首页」可以换服务器或换用法。Android 上按返回键只是退到后台。导出的收藏夹用系统的「保存到」选位置，导入时从「文件」里选。

给账号扫码登录时，B 站 App 和本 App 在同一台手机上：把二维码截图，再在 B 站 App 的「扫一扫」里点相册选这张截图。

> GitHub 上构建的 APK 在没有配置签名密钥时用临时的调试密钥签名，新版本覆盖安装可能提示签名不一致，卸载旧版再装即可（本机模式的账号需要重新扫码）。

## 配置（服务器）

全部通过环境变量配置。用 Compose 时修改 `.env`（模板见 [`.env.example`](.env.example)）。

| 变量 | 默认值 | 说明 |
|---|---|---|
| `WEBUI_PASSWORD` | 空 | 设置后启用面板。建议用 `openssl rand -base64 18` 生成 |
| `WEBUI_ADDR` | `:8080` | 面板监听地址（容器内）。二进制直接运行时建议设为 `127.0.0.1:8080` |
| `WEBUI_PORT` | `8080` | 仅 Compose：映射到服务器本机的端口 |
| `INTERVAL` | `6h` | 每轮间隔，Go duration 格式，如 `90m`、`12h`，最少 `10m` |
| `JITTER` | `30m` | 每轮额外随机等待 0~JITTER，避免时间过于规律 |
| `BVIDS` | `BV15HFVz1EvM` | 心跳视频，每轮随机选一个，多个用逗号分隔，任意公开视频即可。旧的 `BVID` 仍然有效 |
| `DO_DYNAMIC` | `false` | 每轮发一条动态再删除。只发心跳不生效时再打开；动态会短暂出现在粉丝的动态流里 |
| `DYNAMIC_TEXT` | 空 | 动态的文字。图片在面板的设置页上传 |
| `PROXY` | 空 | 所有 B 站请求经过的代理（`http://`、`socks5://`），空表示直连 |
| `DATA_DIR` | 镜像内 `/data`；二进制为所在目录 | `accounts/`（各账号会话）、`dynamic/`（动态图片）、`settings.json` 存放位置 |
| `LOG_LEVEL` | `info` | 日志级别：`debug`、`info`、`warn`、`error`，低于它的不记录。`debug` 会记下每一个发往 B 站的请求 |
| `TZ` | `Asia/Shanghai` | 日志时区 |

`INTERVAL`、`JITTER`、`BVIDS`、`DO_DYNAMIC`、`DYNAMIC_TEXT`、`PROXY`、`LOG_LEVEL` 在面板里保存过之后，以 `data/settings.json` 为准，环境变量不再生效。想恢复用环境变量，删除这个文件并重启即可。

### 动态内容

打开「发动态」后，每轮发一条动态，几秒后删掉：

- 有文字和/或图片时，发你设置的内容。图片在面板的设置页上传，最多 9 张，每张不超过 10 MB，支持 JPG、PNG、GIF、WebP，保存在 `data/dynamic/`。
- 文字和图片都没有时，发一张内置图片 [`core/assets/dynamic.jpg`](core/assets/dynamic.jpg)。

## 验证是否生效

1. 最方便的是在账号卡片上「授权查询」，直接看主页属地。不授权的话，用浏览器**无痕窗口、不登录**打开你的个人空间，记下当前属地。不要用已登录的浏览器看，那本身就是一次从本地 IP 发出的请求。
2. 测试期间本地尽量不打开 B 站。
3. 在面板点「立即运行」，或执行 `docker compose run --rm bili-ip-anchor refresh --now`。
4. 隔几小时到一天，再看一次：
   - 变成目标属地：只发心跳就够了，保持默认配置。
   - 没变：在设置里打开「发动态」（或在 `.env` 设 `DO_DYNAMIC=true`），再测一轮。
   - 仍没变：说明 B 站只按登录或发布内容时的 IP 计算，这个方案不适用。可以改用「只把 B 站接口域名分流到代理、视频 CDN 直连」的方式。

## 安全

- `accounts/` 里的 `cookies.json` 和 `session.json` 等同于账号登录权限。保护好服务器，不要把 `data/` 目录提交到任何地方（仓库的 `.gitignore` 已排除）。
- 面板能扫码登录、退出你的账号。不要把它不加 HTTPS 直接暴露到公网，密码用足够长的随机字符串。
- 桌面客户端和手机 App 的本机模式只在 `127.0.0.1` 上开面板，密码每次启动随机生成。
- 不想用了：在面板里对每个账号点「退出」（会在 B 站注销会话、撤销 App 授权），再删除容器或 App。直接删除数据的话，到 B 站「账号安全 → 登录设备」里把对应设备下线。

## 从源码构建

```bash
git clone https://github.com/VOfficial404/bili-ip-anchor.git && cd bili-ip-anchor
go test ./...
go build -o bili-ip-anchor .   # 服务器
docker build -t bili-ip-anchor .
```

目录：

| 目录 | 内容 |
|---|---|
| `core/` | 核心：账号会话、排程、B 站接口和面板（构建好的 `core/web/`，编译时用 `go:embed` 打包） |
| `webui/` | 网页面板的源码：React、Vite、Tailwind、shadcn/ui，构建到 `core/web/` 并提交，所以只编译服务器时不需要它 |
| `.`（`main.go`） | 服务器命令行：`login`、`refresh`、`daemon` |
| `desktop/` | 桌面客户端，用 [MyGo](https://mygo.egoist.dev/) 写成，不需要 cgo，任何系统上都能交叉编译三个平台 |
| `mobile/` | 手机 App 的 Go 核心，用 gomobile 编译：本机运行时在 App 里跑定时任务，连接服务器时登录并转发；界面通过 `Call` 调用和网页面板同一套 `/api/` 接口 |
| `android/` | Android App：Kotlin、Jetpack Compose、Material 3；本机运行时有前台服务 |
| `ios/` | iOS App：SwiftUI，XcodeGen 生成工程 |

桌面客户端：

```bash
cd desktop
go tool mygo dev                                    # 开发：改代码自动重启
go tool mygo build -platform windows/amd64          # 打包到 desktop/build/windows-amd64/
```

- `go test ./desktop` 会在没有窗口的情况下跑界面测试（用模拟的服务器和 B 站接口），包括本机模式。设置 `SNAPSHOT_DIR=某个目录` 会把每个页面的截图存成 PNG。
- 连一台真的服务器测一遍：`BIA_SERVER=http://127.0.0.1:8080 BIA_PASSWORD=你的密码 go test ./desktop -run TestLiveServer`。
- 图标也是用 MyGo 画的：`GEN_ICON=1 go test ./desktop -run 'TestGenerate'` 重新生成桌面、托盘和手机图标。
- 想不登录账号看看面板：`DEMO_ADDR=127.0.0.1:8099 go test ./core -run TestServeDemo -timeout 0`，用模拟数据启动面板，密码见 `core/demo_test.go`。

网页面板（需要 [Bun](https://bun.sh/)）：

```bash
cd webui
bun install
bun run dev     # 开发：接口转发到 BIA_API（默认 http://127.0.0.1:18099，上面的模拟面板）
bun run build   # 构建到 core/web/，改了 webui/ 要把它一起提交，CI 会检查两者一致
```

手机 App（需要 Android SDK 和 NDK，或 Xcode；步骤见 [`.github/workflows/mobile.yml`](.github/workflows/mobile.yml)）。先用 gomobile 把 Go 核心编成库，再构建原生 App：

```bash
go install golang.org/x/mobile/cmd/gomobile@latest golang.org/x/mobile/cmd/gobind@latest && gomobile init
gomobile bind -target=android -androidapi 24 -o android/app/libs/bia.aar ./mobile && gradle -p android assembleRelease
gomobile bind -target=ios -o ios/Mobile.xcframework ./mobile && (cd ios && xcodegen generate)   # 然后用 Xcode 打开
```

镜像由 GitHub Actions 自动构建并发布到 `ghcr.io/vofficial404/bili-ip-anchor`（`linux/amd64`、`linux/arm64`）。推送到 `main` 更新 `latest` 标签；推送 `v*` 标签时发布版本号标签，并在同一个 Release 里附上服务器二进制、桌面安装包、APK 和 IPA（标签要和 `desktop/mygo.json` 的 `version` 一致）。推送到 `main` 时也会构建桌面安装包、APK 和 IPA，在那次运行（Actions → Desktop / Mobile）的 Artifacts 里下载，用来试还没发布的版本。

`go test ./mobile` 测试手机 App 的 Go 核心（本机运行、连接服务器、自动重新登录），界面的 Kotlin 和 Swift 代码只在 CI 里编译。

## 致谢

接口参考 [SocialSisterYi/bilibili-API-collect](https://github.com/SocialSisterYi/bilibili-API-collect)。B 站接口随时可能变化，出问题欢迎提 Issue。

## 许可

[MIT](LICENSE)。`core/assets/dynamic.jpg` 不在 MIT 许可范围内，版权归原作者所有。本项目与哔哩哔哩官方无关。
