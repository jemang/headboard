import { useEffect, useState } from 'react'

export type Theme = 'dark' | 'light'

const KEY = 'headboard:theme'

/**
 * Light is the default. `index.html` ships without `class="dark"`, so the first
 * paint is light and there is no flash before this hook runs; only a stored
 * preference for dark moves it.
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

/** A stored value, or nothing stored, as a theme. Anything but "dark" is light. */
export function parseTheme(value: string | null): Theme {
  return value === 'dark' ? 'dark' : 'light'
}

function stored(): Theme {
  try {
    return parseTheme(localStorage.getItem(KEY))
  } catch {
    return 'light'
  }
}
