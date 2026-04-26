import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Curator frontend dev server runs on :5175 to avoid colliding with admin (:5174).
// Proxies /curator/* to the backend on :8082 so cookies stay same-origin.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5175,
    proxy: {
      '/curator': {
        target: 'http://localhost:8082',
        changeOrigin: true,
      },
    },
  },
})
