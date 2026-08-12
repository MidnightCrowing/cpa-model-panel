import type { EntryRef } from '../api/types'

/**
 * Stable key for one (site, upstream) pair.
 *
 * Site ids are human provider names and model names come from arbitrary
 * upstreams, so no single separator character is safe. JSON encoding sidesteps
 * the question entirely and stays trivially reversible.
 */
export function refKey(site: string, upstream: string): string {
  return JSON.stringify([site, upstream])
}

export function keyOf(ref: EntryRef): string {
  return refKey(ref.site, ref.upstream)
}

export function parseKey(key: string): EntryRef {
  const [site, upstream] = JSON.parse(key) as [string, string]
  return { site, upstream }
}

/**
 * Stable key for one (site, channel) pair.
 *
 * Priorities are per channel — CPA ranks a site separately in each of its
 * three lists — so a priority override has to name both.
 */
export function siteChannelKey(site: string, channel: string): string {
  return JSON.stringify([site, channel])
}

export function parseSiteChannelKey(key: string): { site: string; channel: string } {
  const [site, channel] = JSON.parse(key) as [string, string]
  return { site, channel }
}
