import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': 'http://192.168.100.152:8080',
      '/ws': { target: 'ws://192.168.100.152:8080', ws: true }
    }
  }
})
