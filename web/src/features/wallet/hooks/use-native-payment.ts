/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import i18next from 'i18next'
import { useCallback, useState } from 'react'
import { toast } from 'sonner'

import {
  getInvoicePayload,
  type InvoiceRequest,
} from '@/features/invoices/types'

import {
  isApiSuccess,
  requestBepusdtPayment,
  requestOkpayPayment,
} from '../api'
import { openPaymentResponse } from '../lib'

export function useNativePayment() {
  const [processing, setProcessing] = useState(false)

  const processBepusdtPayment = useCallback(
    async (
      topupAmount: number,
      tradeType: string,
      promoCode?: string,
      invoiceRequest?: InvoiceRequest
    ) => {
      setProcessing(true)
      try {
        const response = await requestBepusdtPayment({
          amount: Math.floor(topupAmount),
          trade_type: tradeType,
          promo_code: promoCode,
          ...getInvoicePayload(invoiceRequest),
        })
        if (!isApiSuccess(response)) {
          toast.error(response.message || i18next.t('Payment request failed'))
          return false
        }
        if (response.data?.completed === true) {
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

  const processOkpayPayment = useCallback(
    async (
      topupAmount: number,
      promoCode?: string,
      invoiceRequest?: InvoiceRequest
    ) => {
      setProcessing(true)
      try {
        const response = await requestOkpayPayment({
          amount: Math.floor(topupAmount),
          promo_code: promoCode,
          ...getInvoicePayload(invoiceRequest),
        })
        if (!isApiSuccess(response)) {
          toast.error(response.message || i18next.t('Payment request failed'))
          return false
        }
        if (response.data?.completed === true) {
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

  return { processing, processBepusdtPayment, processOkpayPayment }
}
