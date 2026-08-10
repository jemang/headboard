import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { TagSelector } from './TagSelector'

describe('TagSelector', () => {
  it('renders selected saved tags as removable chips with the policy-style picker', () => {
    const markup = renderToStaticMarkup(
      <TagSelector tags={['tag:ci', 'tag:prod']} value={['tag:prod']} onChange={() => {}} />,
    )

    expect(markup).toContain('Remove tag:prod')
    expect(markup).toContain('<input')
    expect(markup).toContain('Find tags declared in tag owners')
    expect(markup).not.toContain('<select')
  })

  it('explains how to make a tag available when no tag owners are saved', () => {
    const markup = renderToStaticMarkup(<TagSelector tags={[]} value={[]} onChange={() => {}} />)

    expect(markup).toContain('Define and save a tag owner in Access control before registering a tagged server.')
  })
})
