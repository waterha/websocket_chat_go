import { fileURLToPath } from 'node:url'
import { defineConfig, loadEnv } from 'vite'

const projectRoot = fileURLToPath(new URL('..', import.meta.url))

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, projectRoot, '')
  const configuredBackend = process.env.BACKEND_HTTP_ADDR || env.HTTP_ADDR || '127.0.0.1:8080'
  const backend = /^https?:\/\//.test(configuredBackend) ? configuredBackend : 'http://' + configuredBackend
  const websocketTarget = backend.replace(/^http/, 'ws')

  return {
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
  }
})
