/**
 * Whether the service worker must leave a request alone. public/sw.js carries
 * a copy of this rule (a worker cannot import app modules); this test is the
 * contract both follow. /api and /auth are never intercepted, so tailnet data
 * and sign-in always go to the network.
 */
export function swBypass(requestUrl: string, method: string, scopeUrl: string): boolean {
  const scope = new URL(scopeUrl)
  const url = new URL(requestUrl)

  if (method !== 'GET' || url.origin !== scope.origin || !url.pathname.startsWith(scope.pathname)) return true

  return /^(api|auth)(\/|$)/.test(url.pathname.slice(scope.pathname.length))
}
