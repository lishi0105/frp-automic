import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  base: './',
  server: {
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:28080',
        changeOrigin: true
      }
    }
  },
  build: {
    outDir: '../web',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks: { echarts: ['echarts'] }
      }
    }
  }
})
