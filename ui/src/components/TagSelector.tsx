import { useMemo, useRef, useState } from 'react'

export function TagSelector({
  tags,
  value,
  onChange,
}: {
  tags: string[]
  value: string[]
  onChange: (tags: string[]) => void
}) {
  const [draft, setDraft] = useState('')
  const [open, setOpen] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)
  const suggestions = useMemo(() => {
    const query = draft.toLowerCase()

    return tags.filter((tag) => tag.toLowerCase().includes(query) && !value.includes(tag)).slice(0, 8)
  }, [draft, tags, value])

  const add = (tag: string | undefined) => {
    if (!tag || value.includes(tag)) return

    onChange([...value, tag])
    setDraft('')
    setOpen(false)
    inputRef.current?.focus()
  }

  if (tags.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">
        Define and save a tag owner in Access control before registering a tagged server.
      </p>
    )
  }

  return (
    <div className="relative mt-1 flex min-h-10 flex-1 flex-wrap items-center gap-1 rounded-lg border border-border bg-surface-1 px-1.5 py-1 shadow-sm transition-colors focus-within:border-accent-500">
      {value.map((tag) => (
        <span key={tag} className="inline-flex items-center gap-1 rounded-md bg-surface-2 px-1.5 py-0.5 font-mono text-xs text-foreground">
          {tag}
          <button
            type="button"
            aria-label={`Remove ${tag}`}
            className="opacity-60 hover:opacity-100"
            onClick={() => onChange(value.filter((selected) => selected !== tag))}
          >
            ×
          </button>
        </span>
      ))}
      <input
        ref={inputRef}
        value={draft}
        placeholder={value.length === 0 ? 'Find a saved tag…' : ''}
        aria-label="Find tags declared in tag owners"
        onChange={(event) => {
          setDraft(event.target.value)
          setOpen(true)
        }}
        onFocus={() => setOpen(true)}
        onBlur={() => setTimeout(() => setOpen(false), 120)}
        onKeyDown={(event) => {
          if (event.key === 'Enter' || event.key === ',') {
            event.preventDefault()
            const exact = tags.find((tag) => tag.toLowerCase() === draft.trim().toLowerCase())
            const selected = exact ?? (suggestions.length === 1 ? suggestions[0] : undefined)

            if (selected) {
              add(selected)
            } else {
              setDraft('')
              setOpen(false)
            }
          }

          if (event.key === 'Backspace' && draft === '' && value.length > 0) {
            onChange(value.slice(0, -1))
          }
        }}
        className="min-w-24 flex-1 bg-transparent px-1 py-0.5 font-mono text-xs outline-none placeholder:text-muted-foreground"
      />
      {open && suggestions.length > 0 && (
        <ul className="absolute left-0 top-full z-20 mt-1 max-h-56 w-56 overflow-y-auto rounded-lg border border-border bg-surface-0 py-1 shadow-raised">
          {suggestions.map((tag) => (
            <li key={tag}>
              <button
                type="button"
                className="block w-full px-2.5 py-1 text-left font-mono text-xs hover:bg-surface-2"
                onMouseDown={(event) => {
                  event.preventDefault()
                  add(tag)
                }}
              >
                {tag}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
