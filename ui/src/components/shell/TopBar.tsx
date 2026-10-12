import { Search } from 'lucide-react'
import { SudoChip } from '../Sudo'

/** Desktop bar above the page: search. Headscale's state lives in the sidebar card. */
export function TopBar({ onSearch, sudoUntil }: { onSearch: () => void; sudoUntil?: string }) {
  return (
    <div className="border-b border-border bg-surface-1/60 px-8 py-2.5">
      <div className="mx-auto flex max-w-7xl items-center gap-3">
        <SudoChip until={sudoUntil} />
        <button
          type="button"
          onClick={onSearch}
          className="ml-auto inline-flex h-9 w-72 items-center gap-2 rounded-lg border border-border bg-surface-1 px-3 text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          <Search aria-hidden className="size-4" strokeWidth={1.5} />
          Search
          <kbd className="ml-auto font-mono text-[0.6875rem]">⌘K</kbd>
        </button>
      </div>
    </div>
  )
}
