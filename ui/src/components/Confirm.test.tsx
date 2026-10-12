import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { Confirm } from './ui'

describe('Confirm', () => {
  // A second click while the first request runs would send it twice.
  it('disables the confirm button while busy', () => {
    const markup = renderToStaticMarkup(
      <Confirm open busy title="Rotate token" body="x" confirmLabel="Rotate" onConfirm={() => {}} onCancel={() => {}} />,
    )
    const confirm = markup.slice(markup.lastIndexOf('<button'))
    expect(confirm).toContain('Rotate')
    expect(confirm).toMatch(/<button[^>]* disabled=""/)
  })
})
