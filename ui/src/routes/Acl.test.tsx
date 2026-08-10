import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { PolicyDeletionConfirm } from './Acl'

describe('PolicyDeletionConfirm', () => {
  it('names the target and affected policy entries before staging a cascade', () => {
    const markup = renderToStaticMarkup(
      <PolicyDeletionConfirm
        deletion={{ name: 'group:ops', affected: 4 }}
        onConfirm={() => {}}
        onCancel={() => {}}
      />,
    )

    expect(markup).toContain('Delete group:ops and clean dependencies?')
    expect(markup).toContain('updates 4 policy entries')
    expect(markup).toContain('Stage deletion')
    expect(markup).not.toContain('Save policy')
  })
})

