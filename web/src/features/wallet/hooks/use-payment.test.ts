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
import { requestPaymentAmount } from './use-payment'

describe('payment amount routing', () => {
  test('uses the dedicated Waffo amount calculator and forwards promo invoice payload', async () => {
    const calls: unknown[] = []
    const result = await requestPaymentAmount(
      {
        amount: 120,
        promo_code: 'SAVE10',
        invoice: {
          required: true,
          type: 'personal',
          kind: 'normal',
          title: 'Buyer',
          tax_no: '',
          email: 'buyer@example.com',
          phone: '',
          remark: '',
        },
      },
      PAYMENT_TYPES.WAFFO,
      {
        regular: async () => ({ success: true, data: '1' }),
        stripe: async () => ({ success: true, data: '2' }),
        waffo: async (request) => {
          calls.push(request)
          return {
            success: true,
            data: '18.75',
            amount_text: '¥18.75',
            invoice_fee: 2,
          }
        },
        waffoPancake: async () => ({ success: true, data: '4' }),
        bepusdt: async () => ({ success: true, data: '5' }),
        okpay: async () => ({ success: true, data: '6' }),
      }
    )

    expect(result).toEqual({
      amount: 18.75,
      amountText: '¥18.75',
      invoiceFee: 2,
    })
    expect(calls).toEqual([
      {
        amount: 120,
        promo_code: 'SAVE10',
        invoice: {
          required: true,
          type: 'personal',
          kind: 'normal',
          title: 'Buyer',
          tax_no: '',
          email: 'buyer@example.com',
          phone: '',
          remark: '',
        },
      },
    ])
  })
})
