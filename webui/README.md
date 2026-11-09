# WebUI

浏览器面板的源码。基于 [shadcn-admin](https://github.com/satnaing/shadcn-admin)（MIT，版权见 `LICENSE-shadcn-admin`），用 React、Vite、TypeScript、Tailwind、shadcn/ui 和 TanStack Router/Query。

构建结果写进 `../core/web`，由 Go 用 `go:embed` 打包。构建结果提交在仓库里，没有 bun 也能 `go build`；改了这里的源码要重新构建并一起提交，CI 会检查两者是否一致。

```sh
bun install
bun run build          # 类型检查并构建到 ../core/web
bun run dev            # 开发服务器，/api 转给 BIA_API（默认 http://127.0.0.1:18099）
```

本地演示数据：`DEMO_ADDR=127.0.0.1:18099 go test -run TestServeDemo -timeout 0 ./core`。
