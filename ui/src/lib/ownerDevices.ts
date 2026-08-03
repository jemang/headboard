import type { Device } from './api'

export function devicesForOwner(devices: Device[], ownerID: number) {
  return devices.filter((device) => device.ownerId === ownerID)
}
