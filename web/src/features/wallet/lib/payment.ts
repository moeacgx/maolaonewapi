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
import {
  PAYMENT_TYPES,
  DEFAULT_PRESET_MULTIPLIERS,
  DEFAULT_PAYMENT_TYPE,
  DEFAULT_MIN_TOPUP,
} from '../constants'
import type { PaymentMethod, PresetAmount, TopupInfo } from '../types'

// ============================================================================
// Payment Processing Functions
// ============================================================================

export const TOPUP_QUOTA_LIMIT_ERROR = 'top-up quota limit exceeded'
export const TOPUP_QUOTA_LIMIT_MESSAGE =
  'Top-up would exceed the wallet quota limit. Please reduce the amount or contact an administrator.'

/**
 * Normalize payment errors while keeping provider-specific messages intact.
 * The quota-limit sentinel is translated here because legacy payment APIs
 * return it as a string in `data` instead of a typed error response.
 */
export function getTopupErrorMessage(
  message: string | undefined,
  data: unknown,
  translate: (key: string) => string
): string {
  const dataMessage = typeof data === 'string' ? data.trim() : ''
  const responseMessage = typeof message === 'string' ? message.trim() : ''
  const rawMessage = dataMessage || responseMessage

  if (rawMessage === TOPUP_QUOTA_LIMIT_ERROR) {
    return translate(TOPUP_QUOTA_LIMIT_MESSAGE)
  }

  if (!rawMessage || rawMessage === 'error') {
    return translate('Payment request failed')
  }

  return rawMessage
}

/**
 * Choose the EPay form target while preserving Safari compatibility.
 */
export function getPaymentFormTarget(userAgent: string): '_blank' | undefined {
  const isSafari = userAgent.includes('Safari') && !userAgent.includes('Chrome')
  return isSafari ? undefined : '_blank'
}

export function isSafeHttpPaymentUrl(value: string): boolean {
  const trimmed = value.trim()
  if (!trimmed) return false

  try {
    const url = new URL(trimmed)
    return url.protocol === 'http:' || url.protocol === 'https:'
  } catch {
    return false
  }
}

/**
 * Submit payment form (for non-Stripe payments)
 */
export function submitPaymentForm(
  url: string,
  params: Record<string, unknown>
): void {
  const form = document.createElement('form')
  form.action = url
  form.method = 'POST'

  const target = getPaymentFormTarget(navigator.userAgent)
  if (target) form.target = target

  // Add form parameters
  Object.entries(params).forEach(([key, value]) => {
    const input = document.createElement('input')
    input.type = 'hidden'
    input.name = key
    input.value = String(value)
    form.appendChild(input)
  })

  document.body.appendChild(form)
  form.submit()
  document.body.removeChild(form)
}

export function openPaymentResponse(response: {
  data?: unknown
  url?: string
}): boolean {
  const data = response.data
  if (data && typeof data === 'object') {
    for (const key of ['pay_link', 'payment_url', 'checkout_url'] as const) {
      const value = (data as Record<string, unknown>)[key]
      if (typeof value !== 'string' || !isSafeHttpPaymentUrl(value)) continue
      if (key === 'checkout_url') window.location.href = value
      else window.open(value, '_blank')
      return true
    }
  }
  if (
    response.url &&
    data &&
    typeof data === 'object' &&
    isSafeHttpPaymentUrl(response.url)
  ) {
    submitPaymentForm(response.url, data as Record<string, unknown>)
    return true
  }
  return false
}

/**
 * Check if payment method is Stripe
 */
export function isStripePayment(paymentType: string): boolean {
  return paymentType === PAYMENT_TYPES.STRIPE
}

/**
 * Check if payment method is Waffo
 */
export function isWaffoPayment(paymentType: string): boolean {
  return paymentType === PAYMENT_TYPES.WAFFO
}

/**
 * Check if payment method is Waffo Pancake
 *
 * Pancake is a metered-style payment that goes through a dedicated checkout
 * URL flow rather than the generic epay form submission, so it must be
 * special-cased in payment dispatch logic.
 */
export function isWaffoPancakePayment(paymentType: string): boolean {
  return paymentType === PAYMENT_TYPES.WAFFO_PANCAKE
}

export function isBepusdtPayment(paymentType: string): boolean {
  return paymentType === PAYMENT_TYPES.BEPUSDT
}

export function isOkpayPayment(paymentType: string): boolean {
  return paymentType === PAYMENT_TYPES.OKPAY
}

export interface PaymentProcessors {
  regular: (topupAmount: number, paymentType: string) => Promise<boolean>
  waffo: (topupAmount: number, payMethodIndex: number) => Promise<boolean>
  waffoPancake: (topupAmount: number) => Promise<boolean>
  bepusdt: (topupAmount: number, tradeType: string) => Promise<boolean>
  okpay: (topupAmount: number) => Promise<boolean>
}

export interface NativePaymentSelection {
  bepusdtTradeType?: string
}

export async function dispatchSelectedPayment(
  paymentMethod: PaymentMethod,
  topupAmount: number,
  waffoMethodIndex: number | null,
  processors: PaymentProcessors,
  nativeSelection: NativePaymentSelection = {}
): Promise<boolean> {
  if (isWaffoPayment(paymentMethod.type)) {
    if (waffoMethodIndex === null) {
      return false
    }
    return processors.waffo(topupAmount, waffoMethodIndex)
  }

  if (isWaffoPancakePayment(paymentMethod.type)) {
    return processors.waffoPancake(topupAmount)
  }

  if (isBepusdtPayment(paymentMethod.type)) {
    if (!nativeSelection.bepusdtTradeType) return false
    return processors.bepusdt(topupAmount, nativeSelection.bepusdtTradeType)
  }

  if (isOkpayPayment(paymentMethod.type)) {
    return processors.okpay(topupAmount)
  }

  return processors.regular(topupAmount, paymentMethod.type)
}

/**
 * Get default payment type from topup info
 */
export function getDefaultPaymentType(topupInfo: TopupInfo | null): string {
  if (!topupInfo) {
    return DEFAULT_PAYMENT_TYPE
  }

  // Return first available payment method or default
  if (topupInfo.pay_methods?.length > 0) {
    return topupInfo.pay_methods[0].type
  }

  if (topupInfo.enable_stripe_topup) {
    return PAYMENT_TYPES.STRIPE
  }

  if (topupInfo.enable_waffo_topup) {
    return PAYMENT_TYPES.WAFFO
  }

  if (topupInfo.enable_waffo_pancake_topup) {
    return PAYMENT_TYPES.WAFFO_PANCAKE
  }

  if (topupInfo.enable_bepusdt_topup) return PAYMENT_TYPES.BEPUSDT
  if (topupInfo.enable_okpay_topup) return PAYMENT_TYPES.OKPAY

  return DEFAULT_PAYMENT_TYPE
}

/**
 * Get minimum topup amount from topup info
 */
export function getMinTopupAmount(topupInfo: TopupInfo | null): number {
  if (!topupInfo) {
    return DEFAULT_MIN_TOPUP
  }

  if (topupInfo.enable_online_topup) {
    return topupInfo.min_topup
  }

  if (topupInfo.enable_stripe_topup) {
    return topupInfo.stripe_min_topup
  }

  if (topupInfo.enable_waffo_topup) {
    return topupInfo.waffo_min_topup || DEFAULT_MIN_TOPUP
  }

  if (topupInfo.enable_waffo_pancake_topup) {
    return topupInfo.waffo_pancake_min_topup || DEFAULT_MIN_TOPUP
  }

  if (topupInfo.enable_bepusdt_topup) {
    return topupInfo.bepusdt_min_topup || DEFAULT_MIN_TOPUP
  }
  if (topupInfo.enable_okpay_topup) {
    return topupInfo.okpay_min_topup || DEFAULT_MIN_TOPUP
  }

  return DEFAULT_MIN_TOPUP
}

/**
 * Generate preset amounts based on minimum topup
 */
export function generatePresetAmounts(minAmount: number): PresetAmount[] {
  return DEFAULT_PRESET_MULTIPLIERS.map((multiplier) => ({
    value: minAmount * multiplier,
  }))
}

/**
 * Merge custom preset amounts with discounts
 */
export function mergePresetAmounts(
  amountOptions: number[],
  discounts: Record<number, number>
): PresetAmount[] {
  if (!amountOptions || amountOptions.length === 0) {
    return []
  }

  return amountOptions.map((amount) => ({
    value: amount,
    discount: discounts[amount] || 1.0,
  }))
}
