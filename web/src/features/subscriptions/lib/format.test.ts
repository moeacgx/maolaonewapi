import { describe, expect, test } from 'vitest'

import { formatPlanCurrencyAmount, normalizePlanCurrency } from './format'
import {
  formValuesToPlanPayload,
  planToFormValues,
  PLAN_FORM_DEFAULTS,
} from './plan-form'
import {
  buildSubscriptionPaymentRequest,
  calculateSubscriptionBalanceCost,
} from './purchase'

describe('subscription currency contracts', () => {
  test('normalizes unknown plan currencies to USD', () => {
    expect(normalizePlanCurrency('EUR')).toBe('USD')
    expect(normalizePlanCurrency('CNY')).toBe('CNY')
  })

  test('renders and submits the selected plan currency', () => {
    const values = {
      ...PLAN_FORM_DEFAULTS,
      price_amount: 12.5,
      currency: 'CNY' as const,
    }
    expect(formValuesToPlanPayload(values).plan.currency).toBe('CNY')
    expect(
      planToFormValues({
        ...values,
        id: 1,
        enabled: true,
        sort_order: 0,
        max_purchase_per_user: 0,
        total_amount: 0,
        duration_unit: 'month',
        duration_value: 1,
        quota_reset_period: 'never',
        allow_balance_pay: true,
        allow_wallet_overflow: true,
        title: 'CNY',
        subtitle: '',
      }).currency
    ).toBe('CNY')
    expect(formatPlanCurrencyAmount(12.5, 'CNY')).toMatch(/12\.50/)
  })

  test('builds preview payload with payment method, promo code, and invoice', () => {
    expect(
      buildSubscriptionPaymentRequest(42, 'bepusdt', ' SAVE10 ', {
        required: true,
        type: 'company',
        kind: 'normal',
        title: 'Example LLC',
        tax_no: 'TAX-1',
        email: 'billing@example.com',
        phone: '',
        remark: '',
      })
    ).toEqual({
      plan_id: 42,
      payment_method: 'bepusdt',
      promo_code: 'SAVE10',
      invoice: {
        required: true,
        type: 'company',
        kind: 'normal',
        title: 'Example LLC',
        tax_no: 'TAX-1',
        email: 'billing@example.com',
        phone: '',
        remark: '',
      },
    })
  })

  test('keeps a fully discounted preview at zero balance cost', () => {
    expect(calculateSubscriptionBalanceCost(0, 25, 500_000)).toBe(0)
  })
})
