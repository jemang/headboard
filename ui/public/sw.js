// Headboard service worker: the app shell and content-hashed assets only.
// /api and /auth are never intercepted, so tailnet data and sign-in always
// go to the network. The bypass rule mirrors src/lib/swBoundary.ts, whose
// test is the contract. BUILD and PRECACHE are stamped by the Vite build
// (vite.config.ts), so every deploy installs a new worker.
const BUILD = '__HB_BUILD__'
const PRECACHE = '__HB_PRECACHE__'.split(',').filter(Boolean)

const scope = new URL(self.registration.scope)
const PREFIX = `headboard-${scope.pathname}-`
const CACHE = PREFIX + BUILD
const SHELL = scope.href

function bypass(request) {
  const url = new URL(request.url)
  if (request.method !== 'GET' || url.origin !== scope.origin || !url.pathname.startsWith(scope.pathname)) return true

  return /^(api|auth)(\/|$)/.test(url.pathname.slice(scope.pathname.length))
}

// internal/web answers an unknown path with index.html and a 200, so a
// missing asset must not be cached as if it were one.
const isHtml = (res) => (res.headers.get('content-type') || '').includes('text/html')

self.addEventListener('install', (event) => {
  event.waitUntil((async () => {
    // All or nothing: a worker that activates without its shell or assets
    // would serve an offline page that cannot start.
    const cache = await caches.open(CACHE)
    const shell = await fetch(SHELL, { cache: 'reload' })
    if (!shell.ok || !isHtml(shell)) throw new Error('shell did not load')
    await cache.put(SHELL, shell)

    for (const path of PRECACHE) {
      const url = new URL(path, scope).href
      const res = await fetch(url, { cache: 'reload' })
      if (!res.ok || isHtml(res)) throw new Error(`${path} did not load`)
      await cache.put(url, res)
    }
  })())
})

self.addEventListener('activate', (event) => {
  event.waitUntil((async () => {
    for (const key of await caches.keys()) {
      if (key.startsWith(PREFIX) && key !== CACHE) await caches.delete(key)
    }
    await self.clients.claim()
  })())
})

self.addEventListener('message', (event) => {
  if (event.data && event.data.type === 'SKIP_WAITING') self.skipWaiting()
  // Lets the page tell whether it already runs this build (src/lib/pwa.ts).
  if (event.data && event.data.type === 'GET_BUILD') event.ports[0]?.postMessage({ build: BUILD, precache: PRECACHE })
})

self.addEventListener('fetch', (event) => {
  const { request } = event
  if (bypass(request)) return

  const path = new URL(request.url).pathname.slice(scope.pathname.length)

  if (request.mode === 'navigate') {
    // The cached shell is only the one this worker installed with: a newer page
    // from the network may reference assets this worker never cached.
    event.respondWith((async () => {
      try {
        return await fetch(request)
      } catch {
        return (await caches.match(SHELL, { cacheName: CACHE })) || Response.error()
      }
    })())

    return
  }

  if (path.startsWith('assets/')) {
    event.respondWith((async () => {
      const hit = await caches.match(request)
      if (hit) return hit

      const res = await fetch(request)
      if (res.ok && !isHtml(res)) (await caches.open(CACHE)).put(request, res.clone())

      return res
    })())
  }
})
