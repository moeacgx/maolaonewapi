/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import type { TFunction } from 'i18next'

import type { StatusCodeItem } from './types'

export const PAGE_SIZE = 30
export const STABILITY_WINDOWS = [900, 3600, 21600, 86400, 604800]

export function formatInteger(value: number | null | undefined) {
  return Number(value ?? 0).toLocaleString()
}

export function formatCompact(value: number | null | undefined) {
  const number = Number(value ?? 0)
  return new Intl.NumberFormat(undefined, {
    notation: Math.abs(number) >= 1000 ? 'compact' : 'standard',
    maximumFractionDigits: 1,
  }).format(number)
}

export function formatPercent(value: number | null | undefined) {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return '-'
  }
  const percent = Math.abs(value) <= 1 ? value * 100 : value
  return `${percent.toFixed(1)}%`
}

export function percentValue(value: number | null | undefined) {
  if (value === null || value === undefined || !Number.isFinite(value)) return 0
  return Math.max(0, Math.min(100, Math.abs(value) <= 1 ? value * 100 : value))
}

export function formatDuration(value: number | null | undefined) {
  if (value === null || value === undefined || !Number.isFinite(value)) {
    return '-'
  }
  if (value < 1000) return `${Math.round(value)} ms`
  return `${(value / 1000).toFixed(value < 10_000 ? 2 : 1)} s`
}

export function formatDateTime(timestamp: number | null | undefined) {
  if (!timestamp) return '-'
  return new Intl.DateTimeFormat(undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(new Date(timestamp * 1000))
}

export function formatMoney(microUsd: number | null | undefined) {
  return new Intl.NumberFormat(undefined, {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 2,
    maximumFractionDigits: 4,
  }).format(Number(microUsd ?? 0) / 1_000_000)
}

export function getStatusLabel(item: StatusCodeItem) {
  if (!item.status_present) return item.label || '-'
  if (item.status_code === 0) return item.label || 'No response'
  return item.label || String(item.status_code)
}

export function statusBadgeVariant(
  code: number,
  present = true
): 'default' | 'secondary' | 'destructive' | 'outline' {
  if (!present || code === 0 || code >= 500) return 'destructive'
  if (code >= 400) return 'secondary'
  if (code >= 200 && code < 300) return 'default'
  return 'outline'
}

export function windowLabel(seconds: number, t: TFunction) {
  const labels: Record<number, string> = {
    900: t('15 minutes'),
    3600: t('1 hour'),
    21600: t('6 hours'),
    86400: t('24 hours'),
    604800: t('7 days'),
  }
  return labels[seconds] ?? `${formatCompact(seconds)} ${t('seconds')}`
}
