type ClipboardWriter = Pick<Clipboard, 'writeText'>

type CopyOptions = {
  clipboard?: ClipboardWriter
  fallback?: (value: string) => boolean
}

// copyText works on HTTPS through the modern Clipboard API and on the local
// HTTP development address through the selection-based browser fallback.
export async function copyText(value: string, options?: CopyOptions): Promise<boolean> {
  const clipboard = options
    ? options.clipboard
    : typeof navigator === 'undefined'
      ? undefined
      : navigator.clipboard
  const fallback = options?.fallback ?? copyWithSelection

  try {
    if (clipboard) {
      await clipboard.writeText(value)
      return true
    }
  } catch {
    // Browsers deny navigator.clipboard on insecure origins such as a LAN HTTP
    // development address. The legacy fallback below covers that case.
  }

  try {
    return fallback(value)
  } catch {
    return false
  }
}

function copyWithSelection(value: string): boolean {
  if (typeof document === 'undefined' || !document.body) return false

  const input = document.createElement('textarea')
  input.value = value
  input.setAttribute('readonly', '')
  input.style.cssText = 'position:fixed;opacity:0;pointer-events:none'
  document.body.appendChild(input)
  input.select()
  input.setSelectionRange(0, value.length)

  try {
    return document.execCommand('copy')
  } finally {
    input.remove()
  }
}
