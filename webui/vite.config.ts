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
    modulePreload: { polyfill: false },
    chunkSizeWarningLimit: 700,
  },
})
