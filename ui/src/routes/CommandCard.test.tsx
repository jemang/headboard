import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { CommandCard } from './Help'

describe('CommandCard copy label', () => {
  it('does not say "command" twice', () => {
    const markup = renderToStaticMarkup(<CommandCard title="Without the token in the command" description="d" command="echo" note="n" />)
    expect(markup).toContain('Copy without the token in the command')
    expect(markup).not.toContain('command command')
  })

  it('still names a plain title as a command', () => {
    const markup = renderToStaticMarkup(<CommandCard title="New machine" description="d" command="echo" note="n" />)
    expect(markup).toContain('Copy new machine command')
  })
})
