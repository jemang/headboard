import { cn } from '../lib/cn'
import type { FlipField } from '../lib/statusFlip'

function Flip({ text, play }: { text: string; play: boolean }) {
  if (!play) return <>{text}</>

  return (
    <>
      <span className="sr-only">{text}</span>
      <span aria-hidden className="[perspective:200px]">
        {[...text].map((ch, i) => (
          <span key={i} className="inline-block animate-flip-char" style={{ animationDelay: `${Math.min(i * 18, 240)}ms` }}>
            {ch}
          </span>
        ))}
      </span>
    </>
  )
}

/**
 * Online/offline plus expiry. When a live update changes one of them, only
 * that label's letters flip in and its dot cross-fades; the accessible text is
 * right immediately. `flipKey` changes per update so a repeat flip replays.
 */
export function FlipStatus({ online, expired, flip, flipKey }: { online: boolean; expired: boolean; flip: FlipField | null; flipKey?: number }) {
  const label = online ? 'online' : 'offline'

  return (
    <span className="flex flex-col gap-1">
      <span className="inline-flex items-start gap-1.5">
        <span aria-hidden className={cn('mt-1.5 size-2 shrink-0 rounded-full transition-colors duration-300', online ? 'bg-ok' : 'bg-muted-foreground/40')} />
        <span key={flip === 'online' ? flipKey : 'still'} className={cn('text-sm', !online && 'text-muted-foreground')}>
          <Flip text={label} play={flip === 'online'} />
        </span>
      </span>
      {expired && (
        <span className="inline-flex items-start gap-1.5">
          <span aria-hidden className="mt-1.5 size-2 shrink-0 rounded-full bg-warn" />
          <span key={flip === 'expired' ? flipKey : 'still'} className="text-sm">
            <Flip text="key expired" play={flip === 'expired'} />
          </span>
        </span>
      )}
    </span>
  )
}
