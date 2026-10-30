import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig, loadEnv } from 'vite'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  return {
    plugins: [react(), tailwindcss()],
    server: {
      host: '127.0.0.1',
      port: 5173,
      strictPort: true,
      proxy: {
        '/api': {
          target: env.DEV_API_URL || 'http://127.0.0.1:8080',
          // Local dev-server configuration; this secret never enters browser code.
          headers: env.DEV_ADMIN_PROXY_SECRET
            ? { 'X-Invites-Admin-Secret': env.DEV_ADMIN_PROXY_SECRET }
            : {},
        },
      },
    },
  }
})
