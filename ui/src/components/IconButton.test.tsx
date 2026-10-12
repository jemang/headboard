import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { Trash2 } from 'lucide-react'
import { IconButton } from './ui'

describe('IconButton', () => {
  it('names the action for screen readers and as a tooltip', () => {
    const html = renderToStaticMarkup(<IconButton icon={Trash2} label="Remove device" variant="danger" />)
    expect(html).toContain('aria-label="Remove device"')
    expect(html).toContain('title="Remove device"')
    expect(html).not.toContain('>Remove device<')
  })

  it('is the same size as a text button, square', () => {
    const html = renderToStaticMarkup(<IconButton icon={Trash2} label="Delete" />)
    expect(html).toContain('size-10')
    expect(html).toContain('md:size-9')
    expect(html).toContain('after:-inset-0.5')
  })
})
