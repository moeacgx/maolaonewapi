/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import type { TFunction } from 'i18next'
import { describe, expect, test } from 'vitest'

import { parseQuotaFromDollars } from '@/lib/format'

import {
  getRedemptionFormSchema,
  transformFormDataToPayload,
  transformRedemptionToFormDefaults,
} from './redemption-form'

const t = ((key: string) => key) as unknown as TFunction

describe('redemption form contracts', () => {
  test('preserves exact quota units when an existing code is not changed', () => {
    const redemption = {
      id: 1,
      user_id: 2,
      name: 'existing',
      key: 'code',
      status: 1,
      quota: 500001,
      created_time: 1,
      redeemed_time: 0,
      expired_time: 0,
      used_user_id: 0,
      max_redeem_count: 9,
      redeemed_count: 3,
    }

    const values = transformRedemptionToFormDefaults(redemption)
    expect(values.max_redeem_count).toBe(9)
    expect(transformFormDataToPayload(values).quota).toBe(500001)

    const editedQuotaDollars = values.quota_dollars + 0.25
    expect(
      transformFormDataToPayload({
        ...values,
        quota_dollars: editedQuotaDollars,
      }).quota
    ).toBe(parseQuotaFromDollars(editedQuotaDollars))
  })

  test('accepts multi-use limits at bounds and rejects values outside them', () => {
    const schema = getRedemptionFormSchema(t)
    const base = { name: 'code', quota_dollars: 1, count: 1 }

    expect(schema.safeParse({ ...base, max_redeem_count: 1 }).success).toBe(
      true
    )
    expect(
      schema.safeParse({ ...base, max_redeem_count: 100000 }).success
    ).toBe(true)
    expect(schema.safeParse({ ...base, max_redeem_count: 0 }).success).toBe(
      false
    )
    expect(
      schema.safeParse({ ...base, max_redeem_count: 100001 }).success
    ).toBe(false)
  })
})
