/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import type { LogOtherData } from '../types'
import { getTieredBillingSummary } from './format'

export interface RetainedBillingDetail {
  labelKey: string
  value?: string
  valueKey?: string
  amountUSD?: number
  suffixKey?: string
}

export interface TieredBillingUsage {
  promptTokens?: number
  completionTokens?: number
}

export interface TieredBillingDetail {
  labelKey: string
  value?: string
  valueKey?: string
  amountUSD?: number
  quotaAmount?: number
  count?: number
  pricePerMillionUSD?: number
  componentUSD?: number
}

const VARIANT_STATUS_KEYS = {
  matched: 'Matched specification price',
  fallback: 'Fallback price',
  legacy: 'Legacy specification multiplier',
  disabled: 'Specification pricing disabled',
} as const

const FORMULA_COMPONENTS = [
  ['base_price', 'Base Price'],
  ['input_image_surcharge', 'Input Image Surcharge'],
  ['input_image_cost', 'Input Image Cost'],
  ['output_cost', 'Output Image Cost'],
  ['text_input_cost', 'Text Input Cost'],
  ['subtotal', 'Formula Subtotal'],
] as const

function finiteNumber(value: unknown): number | null {
  if (value === '' || value == null) return null
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : null
}

export function formatBillingDetailNumber(value: number): string {
  return value
    .toFixed(6)
    .replace(/(\.\d*?)0+$/, '$1')
    .replace(/\.$/, '')
}

const TIERED_TOKEN_PARAM_BY_FIELD: Record<
  string,
  keyof NonNullable<LogOtherData['tiered_token_params']>
> = {
  inputPrice: 'p',
  outputPrice: 'c',
  cacheReadPrice: 'cr',
  cacheCreatePrice: 'cc',
  cacheCreate1hPrice: 'cc1h',
  imagePrice: 'img',
  imageOutputPrice: 'img_o',
  audioInputPrice: 'ai',
  audioOutputPrice: 'ao',
}

export function buildRetainedBillingDetails(
  other: LogOtherData,
  chargedQuota: number,
  quotaPerUnit: number
): RetainedBillingDetail[] {
  const rows: RetainedBillingDetail[] = []
  const isRouteFormula =
    other.billing_mode === 'route_formula' ||
    other.billing_route_price_status === 'formula'
  const isPerSecond =
    other.model_price != null && other.model_price_unit === 'second'
  const hasVariantDetails = Boolean(
    other.billing_variant_price_status ||
    other.billing_resolution ||
    other.billing_quality
  )

  if (isRouteFormula) {
    rows.push({ labelKey: 'Billing Mode', valueKey: 'Route formula' })
    if (other.billing_formula_detail?.trim()) {
      rows.push({
        labelKey: 'Billing Formula',
        value: other.billing_formula_detail.trim(),
      })
    }
    if (
      other.billing_formula_width != null &&
      other.billing_formula_height != null
    ) {
      rows.push({
        labelKey: 'Output specification',
        value: `${other.billing_formula_width}×${other.billing_formula_height}`,
      })
    }
    if (other.billing_formula_quality) {
      rows.push({
        labelKey: 'Output quality',
        valueKey: other.billing_formula_quality,
      })
    }
    const inputImages = finiteNumber(other.billing_formula_input_images)
    if (inputImages != null) {
      rows.push({
        labelKey: 'Input Images',
        value: formatBillingDetailNumber(inputImages),
      })
    }
    for (const [field, labelKey] of FORMULA_COMPONENTS) {
      const value = finiteNumber(other[`billing_formula_calc_${field}`])
      if (value != null) {
        rows.push({ labelKey, value: formatBillingDetailNumber(value) })
      }
    }
  } else if (isPerSecond) {
    rows.push({ labelKey: 'Billing Mode', valueKey: 'Per-second' })
  } else if (hasVariantDetails && other.model_price != null) {
    rows.push({ labelKey: 'Billing Mode', valueKey: 'Per-call' })
  }

  if (
    (isRouteFormula || isPerSecond || hasVariantDetails) &&
    other.model_price != null
  ) {
    const amountUSD = finiteNumber(
      other.billing_formula_price ?? other.model_price
    )
    if (amountUSD != null) {
      rows.push({
        labelKey: 'Model Price',
        amountUSD,
        suffixKey: isPerSecond ? 'second' : undefined,
      })
    }
  }

  if (other.billing_resolution) {
    rows.push({
      labelKey:
        other.billing_variant_price_status === 'disabled'
          ? 'Requested resolution'
          : 'Billing resolution',
      value: other.billing_resolution,
    })
  }
  if (other.billing_quality) {
    rows.push({
      labelKey:
        other.billing_variant_price_status === 'disabled'
          ? 'Requested quality'
          : 'Billing quality',
      valueKey: other.billing_quality,
    })
  }
  if (other.billing_variant_price_status) {
    rows.push({
      labelKey: 'Specification price status',
      valueKey: VARIANT_STATUS_KEYS[other.billing_variant_price_status],
    })
  }
  if (isPerSecond && other.seconds != null && Number.isFinite(other.seconds)) {
    rows.push({
      labelKey: 'Seconds',
      value: formatBillingDetailNumber(other.seconds),
    })
  }

  const groupRatio = finiteNumber(other.user_group_ratio ?? other.group_ratio)
  if (isRouteFormula && groupRatio != null) {
    rows.push({
      labelKey: 'Group Ratio',
      value: `${formatBillingDetailNumber(groupRatio)}x`,
    })
  }
  if (
    isRouteFormula &&
    Number.isFinite(chargedQuota) &&
    chargedQuota >= 0 &&
    quotaPerUnit > 0
  ) {
    rows.push({
      labelKey: 'Final Charge',
      amountUSD: chargedQuota / quotaPerUnit,
    })
  }

  return rows
}

function getTieredTokenCount(
  other: LogOtherData,
  usage: TieredBillingUsage,
  field: string
): number | null {
  const paramKey = TIERED_TOKEN_PARAM_BY_FIELD[field]
  if (paramKey) {
    const actual = finiteNumber(other.tiered_token_params?.[paramKey])
    if (actual != null) return actual
  }
  switch (field) {
    case 'inputPrice':
      return finiteNumber(other.text_input) ?? usage.promptTokens ?? 0
    case 'outputPrice':
      return finiteNumber(other.text_output) ?? usage.completionTokens ?? 0
    case 'cacheReadPrice':
      return finiteNumber(other.cache_tokens) ?? 0
    case 'cacheCreatePrice':
      return (
        finiteNumber(other.cache_creation_tokens_5m) ??
        finiteNumber(other.cache_creation_tokens) ??
        0
      )
    case 'cacheCreate1hPrice':
      return finiteNumber(other.cache_creation_tokens_1h) ?? 0
    case 'imageOutputPrice':
      return finiteNumber(other.image_output) ?? 0
    case 'audioInputPrice':
      return finiteNumber(other.audio_input) ?? 0
    case 'audioOutputPrice':
      return finiteNumber(other.audio_output) ?? 0
    default:
      return null
  }
}

export function buildTieredBillingDetails(
  other: LogOtherData,
  usage: TieredBillingUsage,
  fallbackQuotaPerUnit: number
): TieredBillingDetail[] {
  const rows: TieredBillingDetail[] = [
    { labelKey: 'Billing Mode', valueKey: 'Dynamic Pricing' },
  ]
  const tieredSummary = getTieredBillingSummary(other)
  if (!tieredSummary) {
    rows.push({ labelKey: 'Matched Tier', valueKey: 'No matching results' })
    return rows
  }
  if (tieredSummary.tier.label) {
    rows.push({ labelKey: 'Matched Tier', value: tieredSummary.tier.label })
  }
  if (other.estimated_tier && other.estimated_tier !== other.matched_tier) {
    rows.push({ labelKey: 'Estimated Tier', value: other.estimated_tier })
  }

  let subtotalUSD = 0
  let hasComponent = false
  for (const entry of tieredSummary.priceEntries) {
    const count = getTieredTokenCount(other, usage, entry.field)
    if (count != null && count > 0) {
      const componentUSD = (count * entry.price) / 1_000_000
      subtotalUSD += componentUSD
      hasComponent = true
      rows.push({
        labelKey: entry.shortLabel,
        count,
        pricePerMillionUSD: entry.price,
        componentUSD,
      })
    } else {
      rows.push({
        labelKey: entry.shortLabel,
        pricePerMillionUSD: entry.price,
      })
    }
  }
  if (hasComponent)
    rows.push({ labelKey: 'Tier subtotal', amountUSD: subtotalUSD })

  const requestMultiplier = finiteNumber(other.request_multiplier)
  if (requestMultiplier != null && requestMultiplier !== 1) {
    rows.push({
      labelKey: 'Request Multiplier',
      value: `${formatBillingDetailNumber(requestMultiplier)}x`,
    })
  }
  const quotaPerUnit =
    finiteNumber(other.quota_per_unit) ?? fallbackQuotaPerUnit
  const actualQuotaBeforeGroup = finiteNumber(other.actual_quota_before_group)
  if (
    actualQuotaBeforeGroup != null &&
    Number.isFinite(quotaPerUnit) &&
    quotaPerUnit > 0
  ) {
    rows.push({
      labelKey: 'Actual Charge Before Group',
      amountUSD: actualQuotaBeforeGroup / quotaPerUnit,
    })
  }
  if (other.actual_quota_after_group != null) {
    rows.push({
      labelKey: 'Actual Tier Charge',
      quotaAmount: other.actual_quota_after_group,
    })
  }
  if (other.crossed_tier) {
    rows.push({ labelKey: 'Tier crossed', valueKey: 'Yes' })
  }

  return rows
}
