import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src')
    }
  },
  server: {
    port: 3000,
    proxy: {
      // API请求代理到API专用端口
      '/api/v1': {
        target: 'http://localhost:8080',  // API端口
        changeOrigin: true,
        secure: false
      },
      // WebSocket连接代理到WebSocket专用端口
      '/ws': {
        target: 'ws://localhost:8081',   // WebSocket端口
        ws: true,
        changeOrigin: true
      },
      // 代理请求代理到代理专用端口
      '/proxy': {
        target: 'http://localhost:8082', // 代理端口
        changeOrigin: true,
        secure: false
      }
    }
  },
  build: {
    outDir: '../internal/web/dist',
    assetsDir: 'assets',
    sourcemap: false,
    rollupOptions: {
      output: {
        manualChunks: {
          vendor: ['vue', 'vue-router', 'pinia'],
          elementPlus: ['element-plus', '@element-plus/icons-vue'],
          charts: ['echarts', 'vue-echarts']
        }
      }
    }
  }
})