/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { getCurrencyDisplay } from '@/lib/currency'
import { computeTimeRange } from '@/lib/time'

import type { RevenueDataPoint } from '../api'

export type RevenueGranularity = 'hour' | 'day'

export interface RevenueRequestParams {
  start_timestamp: number
  end_timestamp: number
  granularity: RevenueGranularity
  /** Seconds east of UTC, matching the revenue API contract. */
  timezone_offset: number
}

export function buildRevenueRequest(
  days: number,
  timezoneOffsetMinutes: number,
  now?: Date
): RevenueRequestParams {
  const normalizedDays = Math.max(1, days)
  const range = now
    ? computeTimeRange(normalizedDays - 1, undefined, now, true)
    : computeTimeRange(normalizedDays - 1, undefined, undefined, true)

  return {
    ...range,
    granularity: normalizedDays <= 1 ? 'hour' : 'day',
    timezone_offset: -timezoneOffsetMinutes * 60,
  }
}

export function quotaToLocalCurrency(quota: number): number {
  const { quotaPerUnit, usdExchangeRate } = getCurrencyDisplay().config
  if (!Number.isFinite(quota) || quotaPerUnit <= 0) return 0
  return (quota / quotaPerUnit) * usdExchangeRate
}

export function buildRevenueChartData(
  points: RevenueDataPoint[],
  granularity: RevenueGranularity,
  labels: { online: string; redemption: string }
): Array<{ time: string; value: number; type: string }> {
  const result: Array<{ time: string; value: number; type: string }> = []
  for (const point of points) {
    const date = new Date(point.timestamp * 1000)
    const time =
      granularity === 'hour'
        ? `${String(date.getHours()).padStart(2, '0')}:00`
        : `${date.getMonth() + 1}/${date.getDate()}`
    result.push({ time, value: point.online_money, type: labels.online })
    result.push({
      time,
      value: quotaToLocalCurrency(point.redemption_quota),
      type: labels.redemption,
    })
  }
  return result
}
