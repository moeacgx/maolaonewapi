/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { describe, expect, test } from 'vitest'

import { buildRetainedBillingDetails } from './billing-details'

describe('retained billing detail formatting', () => {
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
})
