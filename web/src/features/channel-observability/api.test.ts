/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { createAnalyticsParams, getRangeTimestamps } from './api'
import type { AnalyticsFilters } from './types'

const baseFilters: AnalyticsFilters = {
  range: 'today',
  customStart: 0,
  customEnd: 0,
  granularity: 'auto',
  channelId: '',
  channelType: '',
  group: '',
  requestedModel: '',
  requestedModelHash: '',
  upstreamModel: '',
  upstreamModelHash: '',
  outcome: '',
  statusCode: '',
  stream: '',
  trafficSource: 'relay',
  dataOrigin: 'live,legacy',
}

describe('channel quality analytics queries', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-17T12:30:00.000Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('bounds custom ranges at the current time', () => {
    const now = Math.floor(Date.now() / 1000)
    expect(
      getRangeTimestamps({
        ...baseFilters,
        range: 'custom',
        customStart: now - 7200,
        customEnd: now + 3600,
      })
    ).toEqual([now - 7200, now])
  })

  it('prefers stable model hashes and selects the requested status scope', () => {
    const params = createAnalyticsParams(
      {
        ...baseFilters,
        range: '1h',
        channelId: '12',
        requestedModel: 'display-name',
        requestedModelHash: 'requested-hash',
        upstreamModel: 'upstream-display-name',
        upstreamModelHash: 'upstream-hash',
        outcome: 'failed',
        statusCode: '429',
        stream: 'true',
      },
      { page: 2 },
      { statusScope: 'client' }
    )

    expect(Object.fromEntries(params)).toMatchObject({
      channel_ids: '12',
      requested_model_hashes: 'requested-hash',
      upstream_model_hashes: 'upstream-hash',
      outcome: 'failed',
      client_status_codes: '429',
      stream: 'true',
      page: '2',
    })
    expect(params.has('requested_models')).toBe(false)
    expect(params.has('upstream_status_codes')).toBe(false)
  })

  it('omits unsupported dimensions from endpoint-specific requests', () => {
    const params = createAnalyticsParams(
      {
        ...baseFilters,
        range: '1h',
        outcome: 'failed',
        statusCode: '500',
        stream: 'false',
      },
      {},
      { includeOutcome: false, includeStatus: false, includeStream: false }
    )

    expect(params.has('outcome')).toBe(false)
    expect(params.has('upstream_status_codes')).toBe(false)
    expect(params.has('stream')).toBe(false)
  })
})
