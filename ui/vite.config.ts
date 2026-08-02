import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  // Relative asset URLs, resolved by the <base href> the Go server writes into
  // index.html. An absolute base would have to be known at build time, which
  // would mean a separate image per deployment path.
  base: './',
  plugins: [react(), tailwindcss()],
  server: {
    // The Go server is the single origin in production, so the dev server
    // proxies the API rather than the app knowing two base URLs.
    proxy: {
      '/api': 'http://127.0.0.1:3000',
      '/auth': 'http://127.0.0.1:3000',
    },
  },
})
