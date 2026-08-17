import {
  getInvoicePayload,
  type InvoiceRequest,
} from '@/features/invoices/types'

import type { SubscriptionPayRequest } from '../types'

export function buildSubscriptionPaymentRequest(
  planId: number,
  paymentMethod: string,
  promoCode: string,
  invoiceRequest?: InvoiceRequest
): SubscriptionPayRequest {
  return {
    plan_id: planId,
    payment_method: paymentMethod || undefined,
    promo_code: promoCode.trim() || undefined,
    ...getInvoicePayload(invoiceRequest),
  }
}

export function calculateSubscriptionBalanceCost(
  previewAmountUsd: number | null | undefined,
  fallbackAmountUsd: number,
  quotaPerUnit: number
): number {
  return Math.ceil((previewAmountUsd ?? fallbackAmountUsd) * quotaPerUnit)
}
