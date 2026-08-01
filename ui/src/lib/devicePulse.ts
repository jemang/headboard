import type { Device } from './api'

export function devicePulse(devices: Device[]) {
  const online = devices.filter((device) => device.online).length

  return {
    total: devices.length,
    online,
    offline: devices.length - online,
    expired: devices.filter((device) => device.expired).length,
  }
}
