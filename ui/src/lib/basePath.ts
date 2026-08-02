/**
 * Where this deployment is mounted, e.g. "" at a site root or "/manage" behind
 * a proxy that also serves Headscale on the same hostname.
 *
 * It comes from the <base href> the Go server writes into index.html, which is
 * the same value the browser already uses to resolve the build's relative asset
 * URLs. Reading it here rather than baking it in at build time keeps one image
 * usable at any path.
 */
export const basePath = readBasePath()

function readBasePath(): string {
  if (typeof document === 'undefined') return ''

  try {
    return new URL(document.baseURI).pathname.replace(/\/$/, '')
  } catch {
    return ''
  }
}

/** withBase turns an app-absolute path into a URL the server will route. */
export function withBase(path: string): string {
  return path.startsWith('/') ? basePath + path : path
}

/** stripBase turns a browser location back into an app-absolute path. */
export function stripBase(pathname: string): string {
  if (basePath === '' || !pathname.startsWith(basePath)) return pathname

  return pathname.slice(basePath.length) || '/'
}
