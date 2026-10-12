import { RefreshCw } from 'lucide-react'
import { Button } from './ui'

/** A new build is installed and waiting; reloading switches to it. */
export function UpdateBar({ onReload }: { onReload: () => void }) {
  return (
    <div role="status" className="fixed inset-x-0 top-0 z-[60] flex items-center justify-center gap-3 border-b border-accent-500/40 bg-surface-1 px-4 pb-2 pt-[max(0.5rem,env(safe-area-inset-top))] text-sm shadow-raised">
      A new version of Headboard is ready.
      <Button variant="primary" icon={RefreshCw} onClick={onReload}>Reload</Button>
    </div>
  )
}
