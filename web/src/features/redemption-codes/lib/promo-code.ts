/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { parseQuotaFromDollars, quotaUnitsToEditableAmount } from '@/lib/format'

import type { PromoCode, PromoCodeFormData } from '../types'

export interface PromoCodeFormState {
  id?: number
  name: string
  code: string
  discount_type: 'percent' | 'fixed'
  discount_value: number
  applies_to_topup: boolean
  applies_to_all_subscription: boolean
  subscription_plan_ids: number[]
  max_redeem_count: number
  expired_time_text: string
}

export const EMPTY_PROMO_CODE_FORM: PromoCodeFormState = {
  name: '',
  code: '',
  discount_type: 'percent',
  discount_value: 10,
  applies_to_topup: true,
  applies_to_all_subscription: false,
  subscription_plan_ids: [],
  max_redeem_count: 0,
  expired_time_text: '',
}

export function promoCodeToForm(row: PromoCode): PromoCodeFormState {
  return {
    id: row.id,
    name: row.name,
    code: row.code,
    discount_type: row.discount_type,
    discount_value:
      row.discount_type === 'fixed'
        ? quotaUnitsToEditableAmount(row.discount_value)
        : row.discount_value,
    applies_to_topup: row.applies_to_topup,
    applies_to_all_subscription: row.applies_to_all_subscription,
    subscription_plan_ids: (row.subscription_plan_ids || '')
      .split(',')
      .map((value) => Number(value.trim()))
      .filter((value) => Number.isInteger(value) && value > 0),
    max_redeem_count: row.max_redeem_count || 0,
    expired_time_text:
      row.expired_time > 0
        ? new Date(row.expired_time * 1000).toISOString().slice(0, 16)
        : '',
  }
}

export function buildPromoCodePayload(
  form: PromoCodeFormState
): PromoCodeFormData {
  return {
    name: form.name.trim(),
    code: form.code.trim().toUpperCase(),
    discount_type: form.discount_type,
    discount_value:
      form.discount_type === 'fixed'
        ? parseQuotaFromDollars(form.discount_value)
        : Math.round(form.discount_value),
    applies_to_topup: form.applies_to_topup,
    applies_to_all_subscription: form.applies_to_all_subscription,
    subscription_plan_ids: form.subscription_plan_ids.join(','),
    max_redeem_count: Math.max(0, Math.floor(form.max_redeem_count)),
    expired_time: form.expired_time_text
      ? Math.floor(new Date(form.expired_time_text).getTime() / 1000)
      : 0,
  }
}
