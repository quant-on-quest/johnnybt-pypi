/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import ui from '@nuxt/ui/vite'

const backend = process.env.PYPI_DEV_BACKEND ?? 'http://127.0.0.1:8080'

export default defineConfig({
  plugins: [
    vue(),
    ui({
      // Bundle every icon we reference so the UI works offline / behind the GFW.
      icon: { clientBundle: { scan: true } },
    }),
  ],
  server: {
    port: 5173,
    // The Go server owns the API and the repository; proxy everything it serves.
    proxy: Object.fromEntries(['/api', '/simple', '/files', '/legacy'].map((p) => [p, { target: backend, changeOrigin: true }])),
  },
  build: {
    // Emitted straight into the Go embed directory; `go build` picks it up.
    outDir: '../internal/web/dist',
    emptyOutDir: true,
  },
  test: {
    environment: 'jsdom',
    globals: false,
    include: ['src/**/*.test.ts'],
    setupFiles: ['src/test/setup.ts'],
  },
})
