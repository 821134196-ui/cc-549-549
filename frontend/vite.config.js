import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 构建产物直接输出到 Go 的 embed 目录 backend/web/dist
export default defineConfig({
  plugins: [vue()],
  build: {
    outDir: '../backend/web/dist',
    emptyOutDir: true
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080'
    }
  }
})
