import type { AclSchema, Grant, PatchOp, SimulationRequest } from './api'

export type PolicyTab = 'grants' | 'groups' | 'tags' | 'hosts' | 'auto' | 'ssh' | 'tests' | 'raw'

export type PolicyNotice = {
  tone: 'info' | 'warn'
  message: string
  rawPointer?: string
}

export function policyStateNotice(sections: string[], schema: AclSchema): PolicyNotice {
  if (sections.includes('acls')) {
    return {
      tone: 'warn',
      message: 'Legacy ACLs are enforced. Edit them only in Raw HuJSON.',
      rawPointer: '/acls',
    }
  }

  if (sections.includes('grants') && (schema.grants?.length ?? 0) === 0) {
    return { tone: 'warn', message: 'Deny all: no traffic is allowed by policy.' }
  }

  if (schema.grants && schema.grants.length > 0) {
    return { tone: 'info', message: 'Grant-based policy is active.' }
  }

  return { tone: 'info', message: 'Headscale default: all devices may communicate.' }
}

export function grantStageOp(index: number | 'new', grant: Grant, hasGrantsSection: boolean): PatchOp {
  if (index === 'new') {
    return hasGrantsSection
      ? { op: 'add', path: '/grants/-', value: grant }
      : { op: 'add', path: '/grants', value: [grant] }
  }

  return { op: 'replace', path: `/grants/${index}`, value: grant }
}

export function jumpTarget(pointer: string): PolicyTab {
  if (pointer.startsWith('/grants/')) return 'grants'
  if (pointer.startsWith('/ssh/')) return 'ssh'

  return 'raw'
}

/**
 * testDestinationIssue reports why Headscale would refuse a tests-block
 * destination, before it can be staged.
 *
 * A failed assertion turns one row red; a destination naming more than one
 * machine is worse than that. It is rejected while the document is parsed, so
 * the whole policy stops loading and the workspace falls back to its parse
 * error — for a line the operator was told to write here.
 *
 * Grants made this easy to hit: their destinations are subnets, so `hosts`
 * now tends to hold CIDR aliases like backend-lan = 11.0.0.0/24, and
 * `backend-lan:22` reads like a perfectly good test. Measured against the
 * engine: a literal prefix is refused even at /32, while a host alias whose
 * value is a single address — bare or /32 — is accepted.
 */
export function testDestinationIssue(
  destination: string,
  hosts: Record<string, string> | undefined,
): string | undefined {
  const trimmed = destination.trim()
  if (trimmed === '') return undefined

  const cut = trimmed.lastIndexOf(':')
  const base = cut > 0 ? trimmed.slice(0, cut) : trimmed

  if (base.includes('/')) {
    return isPrefix(base)
      ? `${base} covers a range of addresses — a test needs one machine, e.g. ${exampleHost(base)}:22`
      : `${base} is written as a prefix — a test destination is a plain address, e.g. ${exampleHost(base)}:22`
  }

  const host = hosts?.[base]

  if (host !== undefined && isPrefix(host)) {
    return `${base} is ${host} — a test needs one machine, e.g. ${exampleHost(host)}:22`
  }

  return undefined
}

/** isPrefix is true for a CIDR covering more than one address. */
function isPrefix(value: string): boolean {
  if (!value.includes('/')) return false

  const bits = value.slice(value.lastIndexOf('/') + 1)

  return value.includes(':') ? bits !== '128' : bits !== '32'
}

/** exampleHost turns a prefix into a plausible address inside it. */
function exampleHost(value: string): string {
  const addr = value.split('/')[0]
  if (!isPrefix(value) || addr.includes(':')) return addr

  const octets = addr.split('.')

  return octets.length === 4 ? [...octets.slice(0, 3), '5'].join('.') : addr
}

export function simulationRequest(
  mode: 'device' | 'ip',
  src: number,
  dst: number,
  ip: string,
  port: string,
): SimulationRequest | { error: string } {
  if (src === 0) return { error: 'Choose a source device.' }

  const parsedPort = Number(port)

  if (!Number.isInteger(parsedPort) || parsedPort < 1 || parsedPort > 65535) {
    return { error: 'Enter a port from 1 to 65535.' }
  }

  if (mode === 'device') {
    if (dst === 0) return { error: 'Choose a destination device.' }

    return { src, dst, port: parsedPort }
  }

  const destinationIP = ip.trim()

  if (destinationIP === '') return { error: 'Enter an IP address.' }
  if (destinationIP.includes('/')) return { error: 'Enter one IPv4 or IPv6 address, not a CIDR.' }

  return { src, destinationIP, port: parsedPort }
}
