/**
 * True when the page already runs the bundle a waiting worker would install:
 * navigations go to the network, so after a deploy the old worker can serve
 * the new page, and offering "update" for it would only reload the same build.
 */
export function isRunningBuild(workerPrecache: string[], pageScripts: string[]): boolean {
  const scripts = workerPrecache.filter((p) => p.endsWith('.js'))
  if (scripts.length === 0) return false

  return scripts.every((p) => pageScripts.some((src) => new URL(src).pathname.endsWith('/' + p)))
}

/** Asks a worker which assets its build precaches. */
function workerPrecache(worker: ServiceWorker): Promise<string[]> {
  return new Promise((resolve) => {
    const channel = new MessageChannel()
    const timer = setTimeout(() => resolve([]), 2000)
    channel.port1.onmessage = (e) => {
      clearTimeout(timer)
      resolve(Array.isArray(e.data?.precache) ? e.data.precache : [])
    }
    worker.postMessage({ type: 'GET_BUILD' }, [channel.port2])
  })
}

/**
 * Registers the service worker in production builds and reports a waiting
 * update. Paths are relative to <base href>, so a /manage deployment gets a
 * worker scoped to /manage/ and nothing else on the host. An installed app can
 * stay open for days without navigating, so it also checks for a new worker
 * when it comes back into view, comes back online, and every 30 minutes.
 */
export function registerServiceWorker(onUpdate: (apply: () => void) => void) {
  if (!import.meta.env.PROD || !('serviceWorker' in navigator)) return

  const url = new URL('sw.js', document.baseURI)
  const scope = new URL('./', document.baseURI).pathname
  const pageScripts = [...document.querySelectorAll<HTMLScriptElement>('script[src]')].map((s) => s.src)

  void navigator.serviceWorker.register(url, { scope, updateViaCache: 'none' }).then((reg) => {
    const offer = async (worker: ServiceWorker) => {
      if (isRunningBuild(await workerPrecache(worker), pageScripts)) return

      onUpdate(() => {
        navigator.serviceWorker.addEventListener('controllerchange', () => window.location.reload(), { once: true })
        worker.postMessage({ type: 'SKIP_WAITING' })
      })
    }

    const watch = (worker: ServiceWorker | null) => {
      worker?.addEventListener('statechange', () => {
        if (worker.state === 'installed' && navigator.serviceWorker.controller) void offer(worker)
      })
    }

    if (reg.waiting && navigator.serviceWorker.controller) void offer(reg.waiting)
    watch(reg.installing)
    reg.addEventListener('updatefound', () => watch(reg.installing))

    const check = () => {
      if (document.visibilityState === 'visible') void reg.update().catch(() => {})
    }
    document.addEventListener('visibilitychange', check)
    window.addEventListener('online', check)
    setInterval(check, 30 * 60_000)
  })
}
