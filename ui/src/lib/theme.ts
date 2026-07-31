import { useEffect, useState } from 'react'

export type Theme = 'dark' | 'light'

const KEY = 'headboard:theme'

/**
 * Dark is the default, not an inversion of a light theme: this is an operations
 * console people keep open all day. `index.html` ships with `class="dark"`
 * already on <html>, so the first paint is dark and there is no flash before
 * this hook runs — only a stored preference for light moves it.
 */
export function useTheme(): [Theme, (t: Theme) => void] {
  const [theme, setTheme] = useState<Theme>(stored)

  useEffect(() => {
    document.documentElement.classList.toggle('dark', theme === 'dark')
    document.documentElement.style.colorScheme = theme

    try {
      localStorage.setItem(KEY, theme)
    } catch {
      // Private browsing, or storage disabled. The theme still applies for
      // this session; it just will not be remembered.
    }
  }, [theme])

  return [theme, setTheme]
}

function stored(): Theme {
  try {
    return localStorage.getItem(KEY) === 'light' ? 'light' : 'dark'
  } catch {
    return 'dark'
  }
}
