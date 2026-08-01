type HeadscaleHealth = {
  headscaleState?: string
  headscaleLastSynced?: string
}

export type ConnectionPresentation = {
  tone: 'connected' | 'stale' | 'unavailable' | 'checking'
  label: string
  title: string
}

// connectionPresentation keeps the visual states explicit: unknown server
// values fail closed to unavailable instead of presenting a false green state.
export function connectionPresentation(health?: HeadscaleHealth): ConnectionPresentation {
  if (!health) {
    return { tone: 'checking', label: 'Checking Headscale', title: 'Checking Headscale connection' }
  }

  if (health.headscaleState === 'connected') {
    return { tone: 'connected', label: 'Headscale connected', title: 'Headscale is connected' }
  }

  if (health.headscaleState === 'stale') {
    const title = health.headscaleLastSynced
      ? `Last successful sync: ${new Date(health.headscaleLastSynced).toLocaleString()}`
      : 'Headscale is reconnecting'

    return { tone: 'stale', label: 'Headscale reconnecting', title }
  }

  return { tone: 'unavailable', label: 'Headscale unavailable', title: 'Headscale cannot be reached' }
}
