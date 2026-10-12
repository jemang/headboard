/**
 * Two posts joined by a heartbeat: the H of Headboard, drawn as the live
 * status it watches. Posts take `currentColor`; the pulse is always the
 * accent, so the mark stands bare on ink and on the light surface alike.
 */
export function BrandMark({ className }: { className?: string }) {
  return (
    <svg aria-hidden="true" viewBox="0 0 40 40" className={className} fill="none">
      <rect x="6" y="6" width="6" height="28" rx="3" fill="currentColor" />
      <rect x="28" y="6" width="6" height="28" rx="3" fill="currentColor" />
      <path d="M11 21h4.5l2.5-7 4 13 2.5-6H29" stroke="var(--color-accent-500)" strokeWidth="3.2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}
