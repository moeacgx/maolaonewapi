/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { describe, expect, test } from 'vitest'

import {
  getCommonUptimeWindowLabel,
  getUptimeWindowLabel,
} from './uptime-window-labels'

describe('uptime availability window labels', () => {
  test('prefers backend label and falls back to hours or 24H', () => {
    expect(
      getUptimeWindowLabel({
        categoryName: 'api',
        monitors: [],
        timeWindowHours: 72,
        timeWindowLabel: '3 days',
      })
    ).toBe('3 days')
    expect(
      getUptimeWindowLabel({
        categoryName: 'api',
        monitors: [],
        timeWindowHours: 48,
      })
    ).toBe('48H')
    expect(getUptimeWindowLabel({ categoryName: 'api', monitors: [] })).toBe(
      '24H'
    )
  })

  test('does not mislabel heterogeneous groups as one common window', () => {
    expect(
      getCommonUptimeWindowLabel([
        { categoryName: 'api', monitors: [], timeWindowHours: 24 },
        { categoryName: 'web', monitors: [], timeWindowHours: 72 },
      ])
    ).toBe('')
  })
})
