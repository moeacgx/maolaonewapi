/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { describe, expect, test } from 'vitest'

import { buildRevenueRequest } from './revenue'

describe('revenue request math', () => {
  test('uses hourly buckets for one day and sends seconds east of UTC', () => {
    const request = buildRevenueRequest(
      1,
      -480,
      new Date('2026-08-17T12:00:00+08:00')
    )

    expect(request.granularity).toBe('hour')
    expect(request.timezone_offset).toBe(8 * 60 * 60)
    expect(request.end_timestamp - request.start_timestamp).toBe(
      24 * 60 * 60 - 1
    )
  })

  test('uses daily buckets and preserves west-of-UTC sign', () => {
    const request = buildRevenueRequest(
      7,
      300,
      new Date('2026-08-17T12:00:00-05:00')
    )

    expect(request.granularity).toBe('day')
    expect(request.timezone_offset).toBe(-5 * 60 * 60)
  })
})
