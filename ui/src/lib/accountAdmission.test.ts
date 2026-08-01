import { describe, expect, it } from 'vitest'
import { admissionPresentation } from './accountAdmission'

describe('admissionPresentation', () => {
  it('keeps pending and rejected users outside the application shell', () => {
    expect(admissionPresentation('pending')).toMatchObject({ canUseApp: false, title: 'Awaiting approval' })
    expect(admissionPresentation('rejected')).toMatchObject({ canUseApp: false, title: 'Access not approved' })
    expect(admissionPresentation('active')).toMatchObject({ canUseApp: true })
  })
})
