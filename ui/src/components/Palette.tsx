import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type Me } from '../lib/api'
import { useRouter } from '../lib/router'
import type { Theme } from '../lib/theme'

interface Command {
  id: string
  label: string
  hint?: string
  group: string
  run: () => void
}

/**
 * ⌘K, over pages and devices.
 *
 * The device list is the part that earns it: an operator who knows a machine's
 * name should not have to go to the right page and filter a table to reach it.
 * Commands are built from the same capabilities the nav uses, so the palette
 * can never offer a page the person is not allowed to open.
 */
export function Palette({
  me,
  theme,
  setTheme,
  onDevice,
}: {
  me: Me
  theme: Theme
  setTheme: (t: Theme) => void
  onDevice: (id: number) => void
}) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const { navigate } = useRouter()

  // Only fetched once the palette has been opened: a member who never presses
  // ⌘K should not pay for the list.
  const devices = useQuery({ queryKey: ['devices'], queryFn: api.devices, enabled: open })

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'k' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault()
        setOpen((v) => !v)
        setQuery('')
        setActive(0)
      }

      if (e.key === 'Escape') setOpen(false)
    }

    window.addEventListener('keydown', onKey)

    return () => window.removeEventListener('keydown', onKey)
  }, [])

  useEffect(() => {
    if (open) inputRef.current?.focus()
  }, [open])

  const can = (c: string) => me.capabilities.includes(c)

  const commands = useMemo<Command[]>(() => {
    const go = (to: string) => () => navigate(to)

    const pages: Command[] = [
      {
        id: 'devices',
        label: can('view:all') ? 'Devices' : 'My devices',
        group: 'Go to',
        run: go('/devices'),
      },
      { id: 'keys', label: 'Keys', group: 'Go to', run: go('/keys') },
    ]

    if (can('manage:policy'))
      pages.splice(1, 0, { id: 'acl', label: 'Access control', group: 'Go to', run: go('/acl') })

    if (can('manage:users'))
      pages.push({ id: 'people', label: 'People', group: 'Go to', run: go('/people') })

    const machines: Command[] = (devices.data?.devices ?? []).map((d) => ({
      id: `device-${d.id}`,
      label: d.name,
      hint: d.owner ?? d.tags?.join(', '),
      group: 'Devices',
      run: () => onDevice(d.id),
    }))

    return [
      ...pages,
      ...machines,
      {
        id: 'theme',
        label: theme === 'dark' ? 'Switch to the light theme' : 'Switch to the dark theme',
        group: 'Preferences',
        run: () => setTheme(theme === 'dark' ? 'light' : 'dark'),
      },
    ]
  }, [devices.data, me.capabilities, navigate, onDevice, setTheme, theme])

  const matches = useMemo(() => {
    const q = query.trim().toLowerCase()
    const hit = q === '' ? commands : commands.filter((c) => searchable(c).includes(q))

    return hit.slice(0, 40)
  }, [commands, query])

  if (!open) return null

  const choose = (c: Command | undefined) => {
    if (!c) return

    c.run()
    setOpen(false)
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-black/50 p-4 pt-[12vh]"
      onClick={() => setOpen(false)}
    >
      <div
        role="dialog"
        aria-label="Command palette"
        className="w-full max-w-lg overflow-hidden rounded-xl border border-border bg-surface-1 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <input
          ref={inputRef}
          value={query}
          placeholder="Search pages and devices…"
          onChange={(e) => {
            setQuery(e.target.value)
            setActive(0)
          }}
          onKeyDown={(e) => {
            if (e.key === 'ArrowDown') {
              e.preventDefault()
              setActive((i) => Math.min(i + 1, matches.length - 1))
            }

            if (e.key === 'ArrowUp') {
              e.preventDefault()
              setActive((i) => Math.max(i - 1, 0))
            }

            if (e.key === 'Enter') {
              e.preventDefault()
              choose(matches[active])
            }
          }}
          className="w-full border-b border-border bg-transparent px-4 py-3 text-sm outline-none"
        />

        <ul className="max-h-80 overflow-y-auto py-1">
          {matches.length === 0 && (
            <li className="px-4 py-6 text-center text-sm text-muted-foreground">
              Nothing matches “{query}”.
            </li>
          )}

          {matches.map((c, i) => (
            <li key={c.id}>
              <button
                type="button"
                onMouseEnter={() => setActive(i)}
                onClick={() => choose(c)}
                className={
                  i === active
                    ? 'flex w-full items-baseline gap-2 bg-surface-2 px-4 py-2 text-left text-sm'
                    : 'flex w-full items-baseline gap-2 px-4 py-2 text-left text-sm'
                }
              >
                <span>{c.label}</span>
                {c.hint && <span className="text-xs text-muted-foreground">{c.hint}</span>}
                <span className="ml-auto text-xs text-muted-foreground">{c.group}</span>
              </button>
            </li>
          ))}
        </ul>
      </div>
    </div>
  )
}

function searchable(c: Command) {
  return `${c.label} ${c.hint ?? ''} ${c.group}`.toLowerCase()
}
