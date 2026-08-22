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
import i18next from 'i18next'
import { useState, useCallback } from 'react'
import { toast } from 'sonner'

import {
  getInvoicePayload,
  type InvoiceRequest,
} from '@/features/invoices/types'

import {
  calculateAmount,
  calculateBepusdtAmount,
  calculateOkpayAmount,
  calculateStripeAmount,
  calculateWaffoAmount,
  calculateWaffoPancakeAmount,
  requestPayment,
  requestStripePayment,
  isApiSuccess,
} from '../api'
import {
  isStripePayment,
  isWaffoPayment,
  isBepusdtPayment,
  isOkpayPayment,
  openPaymentResponse,
  isWaffoPancakePayment,
  getTopupErrorMessage,
} from '../lib'
import type { AmountRequest, AmountResponse } from '../types'

// ============================================================================
// Payment Hook
// ============================================================================

type AmountCalculator = (request: AmountRequest) => Promise<AmountResponse>

export interface PaymentAmountCalculators {
  regular: AmountCalculator
  stripe: AmountCalculator
  waffo: AmountCalculator
  waffoPancake: AmountCalculator
  bepusdt: AmountCalculator
  okpay: AmountCalculator
}

const defaultPaymentAmountCalculators: PaymentAmountCalculators = {
  regular: calculateAmount,
  stripe: calculateStripeAmount,
  waffo: calculateWaffoAmount,
  waffoPancake: calculateWaffoPancakeAmount,
  bepusdt: calculateBepusdtAmount,
  okpay: calculateOkpayAmount,
}

export interface PaymentAmountResult {
  amount: number
  amountText: string
  invoiceFee: number
}

export async function requestPaymentAmount(
  request: AmountRequest,
  paymentType: string,
  calculators: PaymentAmountCalculators = defaultPaymentAmountCalculators
): Promise<PaymentAmountResult> {
  let calculator = calculators.regular
  if (isStripePayment(paymentType)) calculator = calculators.stripe
  else if (isWaffoPayment(paymentType)) calculator = calculators.waffo
  else if (isWaffoPancakePayment(paymentType)) {
    calculator = calculators.waffoPancake
  } else if (isBepusdtPayment(paymentType)) calculator = calculators.bepusdt
  else if (isOkpayPayment(paymentType)) calculator = calculators.okpay

  const response = await calculator(request)
  return {
    amount:
      isApiSuccess(response) && response.data
        ? Number.parseFloat(response.data)
        : 0,
    amountText: response.amount_text || '',
    invoiceFee: Number(response.invoice_fee || 0),
  }
}

export function usePayment() {
  const [amount, setAmount] = useState<number>(0)
  const [amountText, setAmountText] = useState('')
  const [invoiceFee, setInvoiceFee] = useState(0)
  const [calculating, setCalculating] = useState(false)
  const [processing, setProcessing] = useState(false)

  // Calculate payment amount
  const calculatePaymentAmount = useCallback(
    async (
      topupAmount: number,
      paymentType: string,
      promoCode?: string,
      invoiceRequest?: InvoiceRequest
    ) => {
      try {
        setCalculating(true)
        const result = await requestPaymentAmount(
          {
            amount: topupAmount,
            promo_code: promoCode,
            ...getInvoicePayload(invoiceRequest),
          },
          paymentType
        )
        setAmount(result.amount)
        setAmountText(result.amountText)
        setInvoiceFee(result.invoiceFee)
        return result.amount
      } catch {
        setAmount(0)
        setAmountText('')
        setInvoiceFee(0)
        return 0
      } finally {
        setCalculating(false)
      }
    },
    []
  )

  // Process payment
  const processPayment = useCallback(
    async (
      topupAmount: number,
      paymentType: string,
      promoCode?: string,
      invoiceRequest?: InvoiceRequest
    ) => {
      try {
        setProcessing(true)

        const isStripe = isStripePayment(paymentType)
        const amount = Math.floor(topupAmount)
        const requestDetails = {
          promo_code: promoCode,
          ...getInvoicePayload(invoiceRequest),
        }

        const response = isStripe
          ? await requestStripePayment({
              amount,
              payment_method: 'stripe',
              ...requestDetails,
            })
          : await requestPayment({
              amount,
              payment_method: paymentType,
              ...requestDetails,
            })

        if (!isApiSuccess(response)) {
          toast.error(
            getTopupErrorMessage(
              response.message,
              response.data,
              (key) => i18next.t(key)
            )
          )
          return false
        }

        if (
          response.data &&
          typeof response.data === 'object' &&
          'completed' in response.data &&
          response.data.completed === true
        ) {
          toast.success(i18next.t('Order completed successfully'))
          return true
        }

        if (openPaymentResponse(response)) {
          toast.success(i18next.t('Redirecting to payment page...'))
          return true
        }

        return false
      } catch {
        toast.error(i18next.t('Payment request failed'))
        return false
      } finally {
        setProcessing(false)
      }
    },
    []
  )

  return {
    amount,
    amountText,
    invoiceFee,
    calculating,
    processing,
    calculatePaymentAmount,
    processPayment,
    setAmount,
  }
}
