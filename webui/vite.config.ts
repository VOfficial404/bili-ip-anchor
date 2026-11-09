import { fileURLToPath } from 'node:url'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// 构建结果写进 core/web，由 Go 用 go:embed 打包；提交到仓库，没有 bun 也能 go build
export default defineConfig({
  base: './',
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    // 开发时把接口转给本机跑着的面板，例如 DEMO_ADDR=127.0.0.1:18099 的演示
    proxy: { '/api': process.env.BIA_API ?? 'http://127.0.0.1:18099' },
  },
  build: {
    outDir: '../core/web',
    emptyOutDir: true,
    assetsDir: 'assets',
    // 面板只在新浏览器和系统 WebView 里打开
    target: 'es2022',
    // Tailwind v4 本身要求的浏览器。更老的目标会让 Lightning CSS 把 oklch 换算成 lab，
    // 浮点数的最后一位在 Windows 和 Linux 上不同，CI 的构建结果就和提交的对不上
    cssTarget: ['chrome111', 'edge111', 'firefox128', 'safari16.4'],
    modulePreload: { polyfill: false },
    chunkSizeWarningLimit: 700,
  },
})
