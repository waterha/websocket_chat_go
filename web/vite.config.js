import { defineConfig } from 'vite'

// The device proxy target follows BACKEND_HTTP_ADDR so the dev server can reach a
// backend that had to move off the default port. The default stays 8080, so
// nothing changes for a normal checkout.
const backend = process.env.BACKEND_HTTP_ADDR || 'http://127.0.0.1:8080'
const websocketTarget = backend.replace(/^http/, 'ws')

export default defineConfig({
  define: {
    __VUE_OPTIONS_API__: true,
    __VUE_PROD_DEVTOOLS__: false,
    __VUE_PROD_HYDRATION_MISMATCH_DETAILS__: false,
  },
  server: {
    proxy: {
      '/api': backend,
      '/ws': { target: websocketTarget, ws: true },
    },
  },
})
