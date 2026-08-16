/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { CalendarClock, Crown, Loader2, Package } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { GroupBadge } from '@/components/group-badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import { InvoiceRequestForm } from '@/features/invoices/components/invoice-request-form'
import {
  createEmptyInvoiceRequest,
  isInvoicePreviewRequestEnabled,
  isInvoiceRequestValid,
  normalizeInvoiceConfig,
  type InvoiceConfig,
  type InvoiceRequest,
} from '@/features/invoices/types'
import { submitPaymentForm } from '@/features/wallet/lib'
import type { BepusdtChain } from '@/features/wallet/types'
import { useSystemConfig } from '@/hooks/use-system-config'
import { formatQuota } from '@/lib/format'
import { DEFAULT_CURRENCY_CONFIG } from '@/stores/system-config-store'

import {
  paySubscriptionBepusdt,
  paySubscriptionBalance,
  paySubscriptionCreem,
  paySubscriptionEpay,
  paySubscriptionOkpay,
  paySubscriptionStripe,
  paySubscriptionWaffoPancake,
  previewSubscriptionAmount,
} from '../../api'
import {
  buildSubscriptionPaymentRequest,
  calculateSubscriptionBalanceCost,
  formatDuration,
  formatPlanCurrencyAmount,
} from '../../lib'
import type { PlanRecord, SubscriptionAmountResponse } from '../../types'

interface PaymentMethod {
  type: string
  name?: string
  icon?: string
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  plan: PlanRecord | null
  enableStripe?: boolean
  enableCreem?: boolean
  enableWaffoPancake?: boolean
  enableBepusdt?: boolean
  enableOkpay?: boolean
  bepusdtChains?: BepusdtChain[]
  enableOnlineTopUp?: boolean
  enableBalance?: boolean
  enableBalancePromo?: boolean
  epayMethods?: PaymentMethod[]
  invoiceConfig?: InvoiceConfig | null
  purchaseLimit?: number
  purchaseCount?: number
  userQuota?: number
  onPurchaseSuccess?: () => void | Promise<void>
}
function getInitialPaymentKind(
  hasEpay: boolean,
  hasBepusdt: boolean,
  hasStripe: boolean,
  hasOkpay: boolean,
  hasBalance: boolean
): string {
  if (hasEpay) return 'epay'
  if (hasBepusdt) return 'bepusdt'
  if (hasStripe) return 'stripe'
  if (hasOkpay) return 'okpay'
  if (hasBalance) return 'balance'
  return ''
}

export function SubscriptionPurchaseDialog(props: Props) {
  const { t } = useTranslation()
  const { currency } = useSystemConfig()
  const plan = props.plan?.plan
  const invoiceConfig = useMemo(
    () => normalizeInvoiceConfig(props.invoiceConfig),
    [props.invoiceConfig]
  )
  const [paying, setPaying] = useState(false)
  const [amountLoading, setAmountLoading] = useState(false)
  const [preview, setPreview] = useState<SubscriptionAmountResponse | null>(
    null
  )
  const [promoCode, setPromoCode] = useState('')
  const [selectedEpayMethod, setSelectedEpayMethod] = useState('')
  const [selectedPaymentKind, setSelectedPaymentKind] = useState('')
  const [selectedBepusdtTradeType, setSelectedBepusdtTradeType] = useState('')
  const [invoiceRequest, setInvoiceRequest] = useState<InvoiceRequest>(
    createEmptyInvoiceRequest()
  )

  const hasBalance =
    props.enableBalance !== false && plan?.allow_balance_pay !== false
  const hasStripe = !!props.enableStripe && !!plan?.stripe_price_id
  const hasCreem = !!props.enableCreem && !!plan?.creem_product_id
  const hasPancake =
    !!props.enableWaffoPancake && !!plan?.waffo_pancake_product_id
  const hasEpay =
    !!props.enableOnlineTopUp && (props.epayMethods?.length || 0) > 0
  const hasBepusdt =
    !!props.enableBepusdt && (props.bepusdtChains?.length || 0) > 0
  const hasOkpay = !!props.enableOkpay
  const limitReached =
    !!props.purchaseLimit && (props.purchaseCount || 0) >= props.purchaseLimit
  const invoiceValid = isInvoiceRequestValid(invoiceConfig, invoiceRequest)
  const selectedPaymentMethod =
    selectedPaymentKind === 'epay'
      ? selectedEpayMethod
      : selectedPaymentKind || (hasBalance ? 'balance' : '')

  useEffect(() => {
    if (props.open) {
      setSelectedEpayMethod(props.epayMethods?.[0]?.type || '')
      setSelectedPaymentKind(
        getInitialPaymentKind(
          hasEpay,
          hasBepusdt,
          hasStripe,
          hasOkpay,
          hasBalance
        )
      )
      setSelectedBepusdtTradeType(props.bepusdtChains?.[0]?.trade_type || '')
      setPromoCode('')
      setPreview(null)
      setInvoiceRequest(
        createEmptyInvoiceRequest(
          invoiceConfig.types[0],
          invoiceConfig.kinds[0]
        )
      )
    }
  }, [
    props.open,
    props.epayMethods,
    props.bepusdtChains,
    invoiceConfig.types,
    invoiceConfig.kinds,
    hasEpay,
    hasBepusdt,
    hasStripe,
    hasOkpay,
    hasBalance,
  ])

  const loadPreview = async (
    paymentMethod = selectedPaymentMethod,
    request = invoiceRequest,
    code = promoCode
  ) => {
    if (!plan) return
    setAmountLoading(true)
    try {
      const previewRequest = isInvoicePreviewRequestEnabled(
        invoiceConfig,
        request
      )
        ? request
        : createEmptyInvoiceRequest(request.type, request.kind)
      const response = await previewSubscriptionAmount(
        buildSubscriptionPaymentRequest(
          plan.id,
          paymentMethod,
          code,
          previewRequest
        )
      )
      if (response.message === 'success' || response.success === true) {
        setPreview(response)
      } else setPreview(null)
    } catch {
      setPreview(null)
    } finally {
      setAmountLoading(false)
    }
  }

  useEffect(() => {
    if (props.open && plan) void loadPreview()
    // Preview is deliberately refreshed when the dialog/payment selection opens.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [props.open, plan?.id, selectedEpayMethod, selectedPaymentKind])

  if (!plan) return null

  const originalPrice = Number(plan.price_amount || 0)
  const amount =
    typeof preview?.amount === 'number'
      ? preview.amount
      : Number(preview?.data || originalPrice)
  const amountCurrency = preview?.currency || plan.currency || 'USD'
  const amountText = formatPlanCurrencyAmount(amount, amountCurrency)
  const discount = preview?.discount
  const invoiceFee = Number(preview?.invoice_fee || 0)
  const quotaPerUnit =
    currency?.quotaPerUnit && currency.quotaPerUnit > 0
      ? currency.quotaPerUnit
      : DEFAULT_CURRENCY_CONFIG.quotaPerUnit
  const balanceCost = calculateSubscriptionBalanceCost(
    preview?.amount_usd,
    amountCurrency === 'USD' ? amount : originalPrice,
    quotaPerUnit
  )
  const insufficientBalance = Number(props.userQuota || 0) < balanceCost
  const balancePromoBlocked =
    props.enableBalancePromo === false && promoCode.trim() !== ''
  const quotaText =
    Number(plan.total_amount || 0) > 0
      ? formatQuota(Number(plan.total_amount))
      : t('Unlimited')

  const complete = () => {
    toast.success(t('Subscription purchased successfully'))
    void props.onPurchaseSuccess?.()
    props.onOpenChange(false)
  }

  const pay = async (kind: string) => {
    if (!invoiceValid || limitReached) return
    setPaying(true)
    try {
      if (kind === 'balance' && balancePromoBlocked) {
        toast.error(
          t('Promo codes are disabled for balance subscription purchases.')
        )
        return
      }
      const payload = buildSubscriptionPaymentRequest(
        plan.id,
        '',
        promoCode,
        invoiceRequest
      )
      if (kind === 'balance') {
        const response = await paySubscriptionBalance(payload)
        if (response.success) complete()
        else toast.error(response.message || t('Payment request failed'))
      } else if (kind === 'stripe') {
        const response = await paySubscriptionStripe(payload)
        if (response.data?.completed) complete()
        else if (response.data?.pay_link) {
          window.open(response.data.pay_link, '_blank')
          props.onOpenChange(false)
        } else toast.error(response.message || t('Payment request failed'))
      } else if (kind === 'creem') {
        const response = await paySubscriptionCreem(payload)
        if (response.data?.checkout_url) {
          window.open(response.data.checkout_url, '_blank')
          props.onOpenChange(false)
        } else toast.error(response.message || t('Payment request failed'))
      } else if (kind === 'pancake') {
        const response = await paySubscriptionWaffoPancake(payload)
        if (response.data?.completed) complete()
        else if (response.data?.checkout_url) {
          window.location.href = response.data.checkout_url
        } else toast.error(response.message || t('Payment request failed'))
      } else if (kind === 'bepusdt') {
        if (!selectedBepusdtTradeType) return
        const response = await paySubscriptionBepusdt({
          ...payload,
          trade_type: selectedBepusdtTradeType,
        })
        if (response.data?.completed) complete()
        else if (response.data?.payment_url) {
          window.open(response.data.payment_url, '_blank')
          props.onOpenChange(false)
        } else toast.error(response.message || t('Payment request failed'))
      } else if (kind === 'okpay') {
        const response = await paySubscriptionOkpay(payload)
        if (response.data?.completed) complete()
        else if (response.data?.payment_url) {
          window.open(response.data.payment_url, '_blank')
          props.onOpenChange(false)
        } else toast.error(response.message || t('Payment request failed'))
      } else if (kind === 'epay' && selectedEpayMethod) {
        const response = await paySubscriptionEpay({
          ...payload,
          payment_method: selectedEpayMethod,
        })
        if (response.url) {
          submitPaymentForm(response.url, response.data || {})
          props.onOpenChange(false)
        } else toast.error(response.message || t('Payment request failed'))
      }
    } catch {
      toast.error(t('Payment request failed'))
    } finally {
      setPaying(false)
    }
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={
        <>
          <Crown className='h-5 w-5' />
          {t('Purchase Subscription')}
        </>
      }
      contentClassName='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-md'
      titleClassName='flex items-center gap-2'
      contentHeight='auto'
      bodyClassName='space-y-4'
    >
      <div className='bg-muted/50 space-y-2.5 rounded-lg border p-3 sm:p-4'>
        <div className='flex justify-between'>
          <span className='text-muted-foreground text-sm'>
            {t('Plan Name')}
          </span>
          <span className='font-medium'>{plan.title}</span>
        </div>
        <div className='flex justify-between'>
          <span className='text-muted-foreground text-sm'>
            {t('Validity Period')}
          </span>
          <span className='flex items-center gap-1 text-sm'>
            <CalendarClock className='h-3.5 w-3.5' />
            {formatDuration(plan, t)}
          </span>
        </div>
        <div className='flex justify-between'>
          <span className='text-muted-foreground text-sm'>
            {t('Plan Quota')}
          </span>
          <span className='flex items-center gap-1 text-sm'>
            <Package className='h-3.5 w-3.5' />
            {quotaText}
          </span>
        </div>
        {plan.upgrade_group && (
          <div className='flex justify-between'>
            <span className='text-muted-foreground text-sm'>
              {t('Upgrade Group')}
            </span>
            <GroupBadge group={plan.upgrade_group} />
          </div>
        )}
        <Separator />
        <div className='flex justify-between'>
          <span className='text-muted-foreground text-sm'>
            {t('Amount Due')}
          </span>
          <span className='text-primary text-lg font-bold'>
            {amountLoading ? '...' : amountText}
          </span>
        </div>
        {discount && Number(discount.discount_amount || 0) > 0 && (
          <div className='text-right text-xs text-green-600'>
            -
            {formatPlanCurrencyAmount(
              Number(discount.discount_amount),
              amountCurrency
            )}
          </div>
        )}
        {invoiceFee > 0 && (
          <div className='flex justify-between text-xs'>
            <span>{t('Invoice fee')}</span>
            <span>{formatPlanCurrencyAmount(invoiceFee, amountCurrency)}</span>
          </div>
        )}
      </div>

      {hasBalance && (
        <div className='space-y-2 rounded-md border p-3'>
          <div className='flex justify-between text-xs'>
            <span>{t('Required')}</span>
            <span>{formatQuota(balanceCost)}</span>
          </div>
          <div className='flex justify-between text-xs'>
            <span>{t('Available')}</span>
            <span>{formatQuota(Number(props.userQuota || 0))}</span>
          </div>
          {insufficientBalance && (
            <Alert variant='destructive'>
              <AlertDescription>{t('Insufficient balance')}</AlertDescription>
            </Alert>
          )}
          {balancePromoBlocked && (
            <Alert variant='destructive'>
              <AlertDescription>
                {t(
                  'Promo codes are disabled for balance subscription purchases.'
                )}
              </AlertDescription>
            </Alert>
          )}
          <Button
            className='w-full'
            variant='outline'
            disabled={
              paying ||
              amountLoading ||
              limitReached ||
              insufficientBalance ||
              balancePromoBlocked ||
              !invoiceValid
            }
            onClick={() => void pay('balance')}
          >
            {t('Pay with Balance')}
          </Button>
        </div>
      )}

      <Input
        value={promoCode}
        placeholder={t('Enter promo code')}
        onChange={(event) => setPromoCode(event.target.value)}
        onBlur={() =>
          void loadPreview(selectedPaymentMethod, invoiceRequest, promoCode)
        }
      />
      <InvoiceRequestForm
        config={invoiceConfig}
        value={invoiceRequest}
        onChange={(request) => {
          setInvoiceRequest(request)
          void loadPreview(selectedPaymentMethod, request, promoCode)
        }}
        invoiceFee={invoiceFee}
        disabled={paying}
      />

      <div className='grid grid-cols-2 gap-2'>
        {hasEpay &&
          props.epayMethods?.map((method) => (
            <Button
              key={method.type}
              variant={
                selectedPaymentKind === 'epay' &&
                selectedEpayMethod === method.type
                  ? 'default'
                  : 'outline'
              }
              disabled={paying || limitReached}
              onClick={() => {
                setSelectedPaymentKind('epay')
                setSelectedEpayMethod(method.type)
                void loadPreview(method.type)
              }}
            >
              {method.name || method.type}
            </Button>
          ))}
        {hasStripe && (
          <Button
            variant={selectedPaymentKind === 'stripe' ? 'default' : 'outline'}
            disabled={paying || limitReached}
            onClick={() => {
              setSelectedPaymentKind('stripe')
              void loadPreview('stripe')
            }}
          >
            Stripe
          </Button>
        )}
        {hasCreem && (
          <Button
            variant={selectedPaymentKind === 'creem' ? 'default' : 'outline'}
            disabled={paying || limitReached}
            onClick={() => {
              setSelectedPaymentKind('creem')
              void loadPreview('creem')
            }}
          >
            Creem
          </Button>
        )}
        {hasPancake && (
          <Button
            variant={selectedPaymentKind === 'pancake' ? 'default' : 'outline'}
            disabled={paying || limitReached}
            onClick={() => {
              setSelectedPaymentKind('pancake')
              void loadPreview('waffo_pancake')
            }}
          >
            Waffo Pancake
          </Button>
        )}
        {hasBepusdt && (
          <Button
            variant={selectedPaymentKind === 'bepusdt' ? 'default' : 'outline'}
            disabled={paying || limitReached || !selectedBepusdtTradeType}
            onClick={() => {
              setSelectedPaymentKind('bepusdt')
              void loadPreview('bepusdt')
            }}
          >
            USDT
          </Button>
        )}
        {hasOkpay && (
          <Button
            variant={selectedPaymentKind === 'okpay' ? 'default' : 'outline'}
            disabled={paying || limitReached}
            onClick={() => {
              setSelectedPaymentKind('okpay')
              void loadPreview('okpay')
            }}
          >
            OKPay
          </Button>
        )}
      </div>
      {selectedPaymentKind && selectedPaymentKind !== 'balance' && (
        <Button
          className='w-full'
          disabled={paying || amountLoading || limitReached || !invoiceValid}
          onClick={() => void pay(selectedPaymentKind)}
        >
          {t('Pay')}
        </Button>
      )}
      {hasBepusdt && (
        <select
          className='w-full rounded-md border p-2 text-sm'
          value={selectedBepusdtTradeType}
          onChange={(event) => {
            setSelectedBepusdtTradeType(event.target.value)
            setSelectedPaymentKind('bepusdt')
            void loadPreview('bepusdt', invoiceRequest, promoCode)
          }}
        >
          <option value=''>{t('Select USDT Network')}</option>
          {props.bepusdtChains?.map((chain) => (
            <option key={chain.trade_type} value={chain.trade_type}>
              {chain.name}
            </option>
          ))}
        </select>
      )}
      {paying && (
        <div className='text-muted-foreground flex items-center justify-center gap-2 text-xs'>
          <Loader2 className='h-3 w-3 animate-spin' />
          {t('Processing...')}
        </div>
      )}
    </Dialog>
  )
}
