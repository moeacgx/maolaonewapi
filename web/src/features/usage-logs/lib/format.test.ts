/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { describe, expect, test } from 'vitest'

import {
  formatLogUseTime,
  getLogUseTimeSeconds,
  getReasoningEffortVariant,
} from './format'

describe('usage log timing and effort aliases', () => {
  test('prefers precise milliseconds and keeps legacy fallbacks', () => {
    expect(getLogUseTimeSeconds(9, { use_time_ms: 375 })).toBe(0.375)
    expect(formatLogUseTime(9, { use_time_ms: 375 })).toBe('375ms')
    expect(formatLogUseTime(1.5, null)).toBe('1.5s')
    expect(formatLogUseTime(0, null)).toBe('<1s')
    expect(formatLogUseTime(Number.NaN, null)).toBe('-')
  })

  test('normalizes thinking effort aliases in the shared formatter', () => {
    expect(getReasoningEffortVariant('thinking')).toBe('yellow')
    expect(getReasoningEffortVariant('thinking:high')).toBe('orange')
    expect(getReasoningEffortVariant('thinking:low')).toBe('green')
  })
})
