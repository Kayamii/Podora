import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// In development the backend runs separately; proxy API calls to it so the
// browser only ever talks to one origin (same as in production behind nginx).
const backend = process.env.PODORA_BACKEND_URL ?? 'http://localhost:8080'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 3000,
    proxy: {
      '/api': { target: backend, changeOrigin: true, ws: true },
    },
  },
})
