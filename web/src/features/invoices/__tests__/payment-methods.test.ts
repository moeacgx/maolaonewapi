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
*/
import { describe, expect, it } from 'vitest'

import { buildInvoicePaymentRequest } from '../payment'
import { normalizeInvoiceConfig } from '../types'

describe('invoice payment methods', () => {
  it('keeps configured Epay Alipay and WeChat methods for the selector', () => {
    const config = normalizeInvoiceConfig({
      enabled: true,
      pay_methods: [
        {
          name: '支付宝',
          type: 'alipay',
          provider: 'epay',
          color: '#1677ff',
        },
        {
          name: '微信',
          type: 'wxpay',
          provider: 'epay',
          color: '#07c160',
        },
      ],
    })

    expect(config.pay_methods).toEqual([
      {
        name: '支付宝',
        type: 'alipay',
        provider: 'epay',
        color: '#1677ff',
      },
      {
        name: '微信',
        type: 'wxpay',
        provider: 'epay',
        color: '#07c160',
      },
    ])
  })

  it('builds an external invoice payment request with the selected Epay method', () => {
    expect(
      buildInvoicePaymentRequest(
        [{ source_type: 'topup', source_id: 'TOP-1' }],
        {
          required: true,
          type: 'personal',
          kind: 'normal',
          title: '测试',
          tax_no: '',
          email: '',
          phone: '',
          remark: '',
        },
        'wxpay',
      ),
    ).toEqual({
      orders: [{ source_type: 'topup', source_id: 'TOP-1' }],
      invoice: {
        required: true,
        type: 'personal',
        kind: 'normal',
        title: '测试',
        tax_no: '',
        email: '',
        phone: '',
        remark: '',
      },
      payment_method: 'wxpay',
    })
  })
})
