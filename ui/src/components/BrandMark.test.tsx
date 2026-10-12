import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { BrandMark } from './BrandMark'

describe('BrandMark', () => {
  it('renders a decorative, scalable H whose crossbar is a heartbeat', () => {
    const markup = renderToStaticMarkup(<BrandMark className="size-8" />)

    expect(markup).toContain('aria-hidden="true"')
    expect(markup).toContain('viewBox="0 0 40 40"')
    expect(markup).toContain('class="size-8"')
    expect((markup.match(/<rect/g) ?? [])).toHaveLength(2)
    expect((markup.match(/<path/g) ?? [])).toHaveLength(1)
    expect(markup).not.toContain('<circle')
  })
})
