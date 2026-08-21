/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { describe, expect, test } from 'vitest'

import {
  buildRetainedBillingDetails,
  buildTieredBillingDetails,
} from './billing-details'

describe('billing detail formatting', () => {
  test('uses structured formula fields and charged quota as authority', () => {
    const rows = buildRetainedBillingDetails(
      {
        billing_mode: 'route_formula',
        billing_formula_detail: 'base_price + output_cost',
        billing_formula_width: 1024,
        billing_formula_height: 768,
        billing_formula_calc_base_price: '0.04',
        billing_formula_calc_output_cost: '0.01',
        billing_formula_calc_subtotal: '0.05',
        model_price: 0.05,
        group_ratio: 2,
      },
      1200,
      20_000
    )

    expect(rows).toContainEqual({
      labelKey: 'Output specification',
      value: '1024×768',
    })
    expect(rows).toContainEqual({ labelKey: 'Formula Subtotal', value: '0.05' })
    expect(rows.at(-1)).toEqual({ labelKey: 'Final Charge', amountUSD: 0.06 })
  })

  test('formats per-second and specification status without heuristic prose', () => {
    expect(
      buildRetainedBillingDetails(
        {
          model_price: 0.01,
          model_price_unit: 'second',
          seconds: 12.5,
          billing_resolution: '1080p',
          billing_quality: 'high',
          billing_variant_price_status: 'fallback',
        },
        0,
        20_000
      )
    ).toEqual([
      { labelKey: 'Billing Mode', valueKey: 'Per-second' },
      {
        labelKey: 'Model Price',
        amountUSD: 0.01,
        suffixKey: 'second',
      },
      { labelKey: 'Billing resolution', value: '1080p' },
      { labelKey: 'Billing quality', valueKey: 'high' },
      {
        labelKey: 'Specification price status',
        valueKey: 'Fallback price',
      },
      { labelKey: 'Seconds', value: '12.5' },
    ])
  })

  test('uses actual tiered token params and settlement trace', () => {
    const rows = buildTieredBillingDetails(
      {
        billing_mode: 'tiered_expr',
        expr_b64: Buffer.from('tier("base", p * 2 + c * 10)').toString(
          'base64'
        ),
        matched_tier: 'base',
        estimated_tier: 'estimate',
        request_multiplier: 2,
        actual_quota_before_group: 200,
        actual_quota_after_group: 250,
        quota_per_unit: 500_000,
        tiered_token_params: { p: 100, c: 20 },
        crossed_tier: true,
      },
      { promptTokens: 999, completionTokens: 999 },
      20_000
    )

    expect(rows).toContainEqual({
      labelKey: 'Matched Tier',
      value: 'base',
    })
    expect(rows).toContainEqual({
      labelKey: 'Estimated Tier',
      value: 'estimate',
    })
    const input = rows.find((row) => row.labelKey === 'Input')
    expect(input).toMatchObject({ count: 100, pricePerMillionUSD: 2 })
    expect(input?.componentUSD).toBeCloseTo(0.0002)
    const output = rows.find((row) => row.labelKey === 'Output')
    expect(output).toMatchObject({ count: 20, pricePerMillionUSD: 10 })
    expect(output?.componentUSD).toBeCloseTo(0.0002)
    expect(rows).toContainEqual({
      labelKey: 'Tier subtotal',
      amountUSD: 0.0004,
    })
    expect(rows).toContainEqual({
      labelKey: 'Request Multiplier',
      value: '2x',
    })
    expect(rows).toContainEqual({
      labelKey: 'Actual Charge Before Group',
      amountUSD: 0.0004,
    })
    expect(rows).toContainEqual({
      labelKey: 'Actual Tier Charge',
      quotaAmount: 250,
    })
    expect(rows).toContainEqual({ labelKey: 'Tier crossed', valueKey: 'Yes' })
  })
})
