import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import { fileURLToPath, URL } from 'node:url'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  server: { port: 5173, proxy: { '/api': 'http://localhost:8080', '/healthz': 'http://localhost:8080' } },
  build: {
    chunkSizeWarningLimit: 700,
    rollupOptions: {
      output: { manualChunks: { vue: ['vue', 'vue-router', 'pinia'], naive: ['naive-ui'], icons: ['@lucide/vue'] } },
    },
  },
})
