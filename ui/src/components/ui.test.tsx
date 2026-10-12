import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { Laptop } from 'lucide-react'
import { Button, Drawer, Status } from './ui'

describe('Button', () => {
  it('outlines the danger variant instead of filling it', () => {
    const html = renderToStaticMarkup(<Button variant="danger">Revoke</Button>)
    expect(html).toContain('border-danger')
    expect(html).not.toContain('bg-danger ')
  })

  it('fills only danger-solid', () => {
    expect(renderToStaticMarkup(<Button variant="danger-solid">Delete</Button>)).toContain('bg-danger')
  })

  it('shares the control height: 40px on a phone with a 44px tap area, 36px on desktop', () => {
    const html = renderToStaticMarkup(<Button>Go</Button>)
    expect(html).toContain('h-10')
    expect(html).toContain('md:h-9')
    expect(html).toContain('after:-inset-0.5')
  })
})

describe('Status', () => {
  it('lets a long label wrap', () => {
    const html = renderToStaticMarkup(<Status ok={false} warn label="not linked to a Headscale user — an admin has to link it" />)
    expect(html).not.toContain('whitespace-nowrap')
  })
})

describe('Drawer', () => {
  it('leads the header with the icon of what the sheet is about', () => {
    const html = renderToStaticMarkup(<Drawer open onClose={() => {}} title="web-1" icon={Laptop}>body</Drawer>)
    expect(html).toMatch(/<header[^>]*>.*lucide-laptop.*<h2/s)
  })

  it('keeps a plain header when no icon is given', () => {
    const html = renderToStaticMarkup(<Drawer open onClose={() => {}} title="web-1">body</Drawer>)
    expect(html).not.toContain('lucide-laptop')
  })
})
