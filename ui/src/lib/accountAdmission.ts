import type { AdmissionState } from './api'

export function admissionPresentation(state: AdmissionState | undefined) {
  switch (state) {
    case 'active':
      return { canUseApp: true, title: '', detail: '' }
    case 'pending':
      return {
        canUseApp: false,
        title: 'Awaiting approval',
        detail: 'An owner or administrator must approve your account before you can use Headboard.',
      }
    default:
      return {
        canUseApp: false,
        title: 'Access not approved',
        detail: 'Your account is not approved to use Headboard. Contact an owner or administrator if you need access.',
      }
  }
}
