/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import type { LogOtherData } from '../types'

export interface RetainedBillingDetail {
  labelKey: string
  value?: string
  valueKey?: string
  amountUSD?: number
  suffixKey?: string
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

function formatNumber(value: number): string {
  return value
    .toFixed(6)
    .replace(/(\.\d*?)0+$/, '$1')
    .replace(/\.$/, '')
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
      rows.push({ labelKey: 'Input Images', value: formatNumber(inputImages) })
    }
    for (const [field, labelKey] of FORMULA_COMPONENTS) {
      const value = finiteNumber(other[`billing_formula_calc_${field}`])
      if (value != null) rows.push({ labelKey, value: formatNumber(value) })
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
    rows.push({ labelKey: 'Seconds', value: formatNumber(other.seconds) })
  }

  const groupRatio = finiteNumber(other.user_group_ratio ?? other.group_ratio)
  if (isRouteFormula && groupRatio != null) {
    rows.push({
      labelKey: 'Group Ratio',
      value: `${formatNumber(groupRatio)}x`,
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
