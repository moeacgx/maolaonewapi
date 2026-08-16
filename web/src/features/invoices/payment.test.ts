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

import { isSafeInvoicePaymentUrl } from './payment'

describe('invoice checkout URL safety', () => {
  test.each([
    'https://pay.example/checkout',
    'http://127.0.0.1:8080/pay',
    '  https://pay.example/order?id=1  ',
  ])('accepts an absolute HTTP(S) checkout URL: %s', (url) => {
    expect(isSafeInvoicePaymentUrl(url)).toBe(true)
  })

  test.each([
    '',
    '/api/payment',
    '//pay.example/checkout',
    'javascript:alert(1)',
    'data:text/html,payment',
    'ftp://pay.example/order',
  ])('rejects a non-contract checkout URL: %s', (url) => {
    expect(isSafeInvoicePaymentUrl(url)).toBe(false)
  })
})
