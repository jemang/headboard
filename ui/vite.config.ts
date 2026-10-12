import { createHash } from 'node:crypto'
import { readFileSync, writeFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { defineConfig, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

/** Stamps dist/sw.js with this build's id and hashed assets, so every deploy ships a new worker. */
function serviceWorkerStamp(): Plugin {
  let assets: string[] = []
  let outDir = 'dist'

  return {
    name: 'headboard-sw-stamp',
    apply: 'build',
    configResolved(config) {
      outDir = resolve(config.root, config.build.outDir)
    },
    generateBundle(_, bundle) {
      assets = Object.keys(bundle).filter((f) => f.startsWith('assets/') && /\.(js|css)$/.test(f))
    },
    // closeBundle, not writeBundle: public/ is copied after writeBundle runs.
    closeBundle() {
      const file = resolve(outDir, 'sw.js')
      // A hash of every asset name, so any changed chunk yields a new cache name.
      const build = createHash('sha256').update([...assets].sort().join(',')).digest('hex').slice(0, 16)
      writeFileSync(file, readFileSync(file, 'utf8').replace("'__HB_BUILD__'", `'${build}'`).replace("'__HB_PRECACHE__'", `'${assets.join(',')}'`))
    },
  }
}

export default defineConfig({
  // Relative asset URLs, resolved by the <base href> the Go server writes into
  // index.html. An absolute base would have to be known at build time, which
  // would mean a separate image per deployment path.
  base: './',
  plugins: [react(), tailwindcss(), serviceWorkerStamp()],
  server: {
    // The Go server is the single origin in production, so the dev server
    // proxies the API rather than the app knowing two base URLs.
    proxy: {
      '/api': 'http://127.0.0.1:3000',
      '/auth': 'http://127.0.0.1:3000',
    },
  },
})
