import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The console gateway serves the SPA and proxies /api/<service>/* to the
// backends. In dev, forward /api to the local gateway so the browser keeps a
// single origin.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:8090',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: '../console/web/console',
    emptyOutDir: true,
  },
})