/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQuery } from '@tanstack/react-query'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { getAdminPlans } from '@/features/subscriptions/api'
import { formatQuota } from '@/lib/format'

import {
  createPromoCode,
  deletePromoCode,
  getPromoCodes,
  updatePromoCode,
  updatePromoCodeStatus,
} from '../api'
import {
  buildPromoCodePayload,
  EMPTY_PROMO_CODE_FORM,
  promoCodeToForm,
  type PromoCodeFormState,
} from '../lib'
import type { PromoCode } from '../types'

const ENABLED = 1
const DISABLED = 2

export function PromoCodesPanel() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [open, setOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [form, setForm] = useState<PromoCodeFormState>(EMPTY_PROMO_CODE_FORM)
  const pageSize = 20

  const { data, isFetching, refetch } = useQuery({
    queryKey: ['promo-codes', page, pageSize],
    queryFn: async () => {
      const response = await getPromoCodes({ p: page, page_size: pageSize })
      return response.data ?? { items: [], total: 0, page, page_size: pageSize }
    },
  })
  const { data: plansResponse } = useQuery({
    queryKey: ['promo-code-subscription-plans'],
    queryFn: getAdminPlans,
  })
  const plans = useMemo(
    () => (plansResponse?.data || []).map((record) => record.plan),
    [plansResponse?.data]
  )
  const pageCount = Math.max(1, Math.ceil((data?.total || 0) / pageSize))

  const updateField = <K extends keyof PromoCodeFormState>(
    key: K,
    value: PromoCodeFormState[K]
  ) => setForm((current) => ({ ...current, [key]: value }))

  const submit = async () => {
    if (!form.name.trim() || !form.code.trim() || form.discount_value <= 0) {
      toast.error(t('Please complete all required promo code fields'))
      return
    }
    if (form.discount_type === 'percent' && form.discount_value > 100) {
      toast.error(t('Discount percentage cannot exceed 100'))
      return
    }
    if (
      !form.applies_to_topup &&
      !form.applies_to_all_subscription &&
      form.subscription_plan_ids.length === 0
    ) {
      toast.error(t('Please select at least one promo code scope'))
      return
    }

    setSaving(true)
    try {
      const payload = buildPromoCodePayload(form)
      const response = form.id
        ? await updatePromoCode({ ...payload, id: form.id })
        : await createPromoCode(payload)
      if (!response.success) {
        toast.error(response.message || t('Failed to save promo code'))
        return
      }
      toast.success(t('Promo code saved successfully'))
      setOpen(false)
      await refetch()
    } finally {
      setSaving(false)
    }
  }

  const toggleStatus = async (promo: PromoCode) => {
    const response = await updatePromoCodeStatus(
      promo.id,
      promo.status === ENABLED ? DISABLED : ENABLED
    )
    if (response.success) await refetch()
  }

  const remove = async (promo: PromoCode) => {
    if (!window.confirm(t('Delete this promo code?'))) return
    const response = await deletePromoCode(promo.id)
    if (response.success) await refetch()
  }

  return (
    <div className='space-y-4'>
      <div className='flex justify-end'>
        <Button
          size='sm'
          onClick={() => {
            setForm(EMPTY_PROMO_CODE_FORM)
            setOpen(true)
          }}
        >
          <Plus />
          {t('Create Promo Code')}
        </Button>
      </div>

      <div className='rounded-md border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('ID')}</TableHead>
              <TableHead>{t('Name')}</TableHead>
              <TableHead>{t('Promo Code')}</TableHead>
              <TableHead>{t('Discount')}</TableHead>
              <TableHead>{t('Redeemed')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead className='text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {(data?.items || []).map((promo) => (
              <TableRow key={promo.id}>
                <TableCell>
                  <TableId value={promo.id} />
                </TableCell>
                <TableCell>{promo.name}</TableCell>
                <TableCell className='font-mono'>{promo.code}</TableCell>
                <TableCell>
                  {promo.discount_type === 'percent'
                    ? `${promo.discount_value}%`
                    : formatQuota(promo.discount_value)}
                </TableCell>
                <TableCell className='font-mono'>
                  {promo.redeemed_count}/
                  {promo.max_redeem_count || t('Unlimited')}
                </TableCell>
                <TableCell>
                  <StatusBadge
                    label={t(promo.status === ENABLED ? 'Enabled' : 'Disabled')}
                    variant={promo.status === ENABLED ? 'success' : 'neutral'}
                    copyable={false}
                  />
                </TableCell>
                <TableCell>
                  <div className='flex justify-end gap-1'>
                    <Button
                      size='icon-sm'
                      variant='ghost'
                      aria-label={t('Edit')}
                      onClick={() => {
                        setForm(promoCodeToForm(promo))
                        setOpen(true)
                      }}
                    >
                      <Pencil />
                    </Button>
                    <Button
                      size='sm'
                      variant='ghost'
                      onClick={() => toggleStatus(promo)}
                    >
                      {t(promo.status === ENABLED ? 'Disable' : 'Enable')}
                    </Button>
                    <Button
                      size='icon-sm'
                      variant='ghost'
                      aria-label={t('Delete')}
                      onClick={() => remove(promo)}
                    >
                      <Trash2 />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
            {!isFetching && (data?.items || []).length === 0 && (
              <TableRow>
                <TableCell colSpan={7} className='h-24 text-center'>
                  {t('No Promo Codes Found')}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>

      <div className='flex justify-end gap-2'>
        <Button
          size='sm'
          variant='outline'
          disabled={page <= 1}
          onClick={() => setPage((value) => Math.max(1, value - 1))}
        >
          {t('Previous')}
        </Button>
        <Button
          size='sm'
          variant='outline'
          disabled={page >= pageCount}
          onClick={() => setPage((value) => Math.min(pageCount, value + 1))}
        >
          {t('Next')}
        </Button>
      </div>

      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent className='sm:max-w-[600px]'>
          <SheetHeader>
            <SheetTitle>
              {form.id ? t('Update Promo Code') : t('Create Promo Code')}
            </SheetTitle>
          </SheetHeader>
          <div className='space-y-4 overflow-y-auto py-4'>
            <Input
              value={form.name}
              placeholder={t('Name')}
              onChange={(event) => updateField('name', event.target.value)}
            />
            <Input
              value={form.code}
              placeholder={t('Promo Code')}
              onChange={(event) => updateField('code', event.target.value)}
            />
            <Select
              items={[
                { value: 'percent', label: t('Percentage') },
                { value: 'fixed', label: t('Fixed quota') },
              ]}
              value={form.discount_type}
              onValueChange={(value) => {
                if (value === 'percent' || value === 'fixed') {
                  updateField('discount_type', value)
                }
              }}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem value='percent'>{t('Percentage')}</SelectItem>
                  <SelectItem value='fixed'>{t('Fixed quota')}</SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
            <Input
              type='number'
              min='0'
              step={form.discount_type === 'fixed' ? '0.01' : '1'}
              value={form.discount_value}
              placeholder={t('Discount Value')}
              onChange={(event) =>
                updateField('discount_value', Number(event.target.value) || 0)
              }
            />
            <Input
              type='number'
              min='0'
              step='1'
              value={form.max_redeem_count}
              placeholder={t('Usage limit, 0 for unlimited')}
              onChange={(event) =>
                updateField('max_redeem_count', Number(event.target.value) || 0)
              }
            />
            <Input
              type='datetime-local'
              value={form.expired_time_text}
              onChange={(event) =>
                updateField('expired_time_text', event.target.value)
              }
            />
            <label className='flex items-center gap-2 text-sm'>
              <Checkbox
                checked={form.applies_to_topup}
                onCheckedChange={(checked) =>
                  updateField('applies_to_topup', checked === true)
                }
              />
              {t('Balance recharge')}
            </label>
            <label className='flex items-center gap-2 text-sm'>
              <Checkbox
                checked={form.applies_to_all_subscription}
                onCheckedChange={(checked) => {
                  updateField('applies_to_all_subscription', checked === true)
                  if (checked === true) updateField('subscription_plan_ids', [])
                }}
              />
              {t('All subscriptions')}
            </label>
            {!form.applies_to_all_subscription && (
              <div className='grid gap-2 rounded-md border p-3 sm:grid-cols-2'>
                {plans.map((plan) => (
                  <label
                    key={plan.id}
                    className='flex items-center gap-2 text-sm'
                  >
                    <Checkbox
                      checked={form.subscription_plan_ids.includes(plan.id)}
                      onCheckedChange={() =>
                        updateField(
                          'subscription_plan_ids',
                          form.subscription_plan_ids.includes(plan.id)
                            ? form.subscription_plan_ids.filter(
                                (id) => id !== plan.id
                              )
                            : [...form.subscription_plan_ids, plan.id]
                        )
                      }
                    />
                    <span className='truncate'>{plan.title}</span>
                  </label>
                ))}
              </div>
            )}
          </div>
          <SheetFooter>
            <SheetClose render={<Button variant='outline' />}>
              {t('Close')}
            </SheetClose>
            <Button disabled={saving} onClick={submit}>
              {saving ? t('Saving...') : t('Save changes')}
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>
    </div>
  )
}
