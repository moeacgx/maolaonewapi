/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { describe, expect, test } from 'vitest'

import { PAYMENT_TYPES } from '../constants'
import {
  dispatchSelectedPayment,
  getTopupErrorMessage,
  getPaymentFormTarget,
  isStripePayment,
  isWaffoPayment,
  isWaffoPancakePayment,
} from './payment'

describe('topup error messages', () => {
  test('translates the wallet quota limit sentinel from response data', () => {
    const translate = (key: string) => `translated:${key}`

    expect(
      getTopupErrorMessage('error', 'top-up quota limit exceeded', translate)
    ).toBe(
      'translated:Top-up would exceed the wallet quota limit. Please reduce the amount or contact an administrator.'
    )
  })

  test('keeps unknown provider messages and falls back for generic errors', () => {
    const translate = (key: string) => `translated:${key}`

    expect(
      getTopupErrorMessage(
        'top-up quota limit exceeded',
        undefined,
        translate
      )
    ).toBe(
      'translated:Top-up would exceed the wallet quota limit. Please reduce the amount or contact an administrator.'
    )
    expect(getTopupErrorMessage('error', 'provider unavailable', translate)).toBe(
      'provider unavailable'
    )
    expect(getTopupErrorMessage('error', undefined, translate)).toBe(
      'translated:Payment request failed'
    )
  })
})

describe('payment type classification', () => {
  test('keeps Waffo and Waffo Pancake on their dedicated flows', () => {
    expect(isWaffoPayment(PAYMENT_TYPES.WAFFO)).toBe(true)
    expect(isWaffoPayment(PAYMENT_TYPES.WAFFO_PANCAKE)).toBe(false)
    expect(isWaffoPancakePayment(PAYMENT_TYPES.WAFFO_PANCAKE)).toBe(true)
    expect(isWaffoPancakePayment(PAYMENT_TYPES.WAFFO)).toBe(false)
    expect(isStripePayment(PAYMENT_TYPES.STRIPE)).toBe(true)
  })
})

describe('EPay form target', () => {
  test('keeps Safari in the current tab and opens other browsers separately', () => {
    const safariUserAgent =
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Version/17.4 Safari/605.1.15'
    const chromeUserAgent =
      'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/123.0.0.0 Safari/537.36'

    expect(getPaymentFormTarget(safariUserAgent)).toBeUndefined()
    expect(getPaymentFormTarget(chromeUserAgent)).toBe('_blank')
  })
})

describe('payment dispatch', () => {
  test('keeps the selected Waffo method index through confirmation', async () => {
    const calls: string[] = []
    const success = await dispatchSelectedPayment(
      { name: 'Waffo Card', type: PAYMENT_TYPES.WAFFO },
      120,
      3,
      {
        regular: async () => {
          calls.push('regular')
          return false
        },
        waffo: async (amount, index) => {
          calls.push(`waffo:${amount}:${index}`)
          return true
        },
        waffoPancake: async () => false,
        bepusdt: async () => false,
        okpay: async () => false,
      }
    )

    expect(success).toBe(true)
    expect(calls).toEqual(['waffo:120:3'])
  })

  test('does not create a Waffo order without a selected method index', async () => {
    let called = false
    const success = await dispatchSelectedPayment(
      { name: 'Waffo Card', type: PAYMENT_TYPES.WAFFO },
      120,
      null,
      {
        regular: async () => false,
        waffo: async () => {
          called = true
          return true
        },
        waffoPancake: async () => false,
        bepusdt: async () => false,
        okpay: async () => false,
      }
    )
    expect(success).toBe(false)
    expect(called).toBe(false)
  })

  test('requires a selected network for native USDT payment', async () => {
    const called: string[] = []
    const success = await dispatchSelectedPayment(
      { name: 'USDT', type: PAYMENT_TYPES.BEPUSDT },
      100,
      null,
      {
        regular: async () => false,
        waffo: async () => false,
        waffoPancake: async () => false,
        bepusdt: async (_amount, tradeType) => {
          called.push(tradeType)
          return true
        },
        okpay: async () => false,
      },
      { bepusdtTradeType: 'usdt.trc20' }
    )

    expect(success).toBe(true)
    expect(called).toEqual(['usdt.trc20'])
  })
})
