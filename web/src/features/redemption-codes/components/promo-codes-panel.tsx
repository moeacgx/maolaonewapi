/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useQuery } from '@tanstack/react-query'
import { MoreHorizontal, Pencil, Plus, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { DataTableRowActionMenu } from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
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
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { formatQuota, getEditableQuotaStep } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

import {
  createPromoCode,
  deleteInvalidPromoCodes,
  deletePromoCode,
  deletePromoCodes,
  getPromoCodes,
  updatePromoCode,
  updatePromoCodeStatus,
} from '../api'
import { PROMO_CODE_ERROR_MESSAGES } from '../constants'
import {
  buildPromoCodePayload,
  EMPTY_PROMO_CODE_FORM,
  getBatchDeleteSkipReasonMessage,
  promoCodeToForm,
  type PromoCodeFormState,
} from '../lib'
import { isTimestampExpired } from '../lib/utils'
import type { PromoCode } from '../types'

const ENABLED = 1
const DISABLED = 2
const EXHAUSTED = 3

export function PromoCodesPanel() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [open, setOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [form, setForm] = useState<PromoCodeFormState>(EMPTY_PROMO_CODE_FORM)
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())
  const [showBulkDeleteConfirm, setShowBulkDeleteConfirm] = useState(false)
  const [showDeleteInvalidConfirm, setShowDeleteInvalidConfirm] =
    useState(false)
  const [showSingleDeleteConfirm, setShowSingleDeleteConfirm] = useState(false)
  const [singleDeleteTarget, setSingleDeleteTarget] =
    useState<PromoCode | null>(null)
  const [isBulkDeleting, setIsBulkDeleting] = useState(false)
  const [isDeletingInvalid, setIsDeletingInvalid] = useState(false)
  const [isSingleDeleting, setIsSingleDeleting] = useState(false)
  const [isTogglingStatus, setIsTogglingStatus] = useState<number | null>(null)
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
  const items = data?.items || []

  // Every page change goes through this so a stale ID from a previous page can never reach a batch delete request.
  const changePage = (next: number | ((current: number) => number)) => {
    setPage(next)
    setSelectedIds(new Set())
  }

  const allSelected =
    items.length > 0 && items.every((promo) => selectedIds.has(promo.id))
  const someSelected = items.some((promo) => selectedIds.has(promo.id))

  const toggleSelectAll = (checked: boolean) => {
    setSelectedIds(
      checked ? new Set(items.map((promo) => promo.id)) : new Set()
    )
  }

  const toggleRow = (id: number, checked: boolean) => {
    setSelectedIds((current) => {
      const next = new Set(current)
      if (checked) {
        next.add(id)
      } else {
        next.delete(id)
      }
      return next
    })
  }

  const goToValidPageAfterDelete = async () => {
    const result = await refetch()
    const remaining = result.data?.items.length ?? 0
    if (remaining === 0 && page > 1) {
      changePage((value) => Math.max(1, value - 1))
    }
  }

  const handleBulkDelete = async () => {
    const ids = [...selectedIds]
    if (ids.length === 0) return

    setIsBulkDeleting(true)
    try {
      const response = await deletePromoCodes(ids)
      if (!response.success) {
        toast.error(
          response.message || t(PROMO_CODE_ERROR_MESSAGES.BATCH_DELETE_FAILED)
        )
        return
      }

      const deletedIds = response.data?.deleted_ids ?? []
      const skipped = response.data?.skipped ?? []

      setSelectedIds(new Set())
      setShowBulkDeleteConfirm(false)

      if (skipped.length > 0) {
        const skippedList = skipped
          .map(
            (entry) =>
              `ID ${entry.id}: ${getBatchDeleteSkipReasonMessage(entry.reason, t)}`
          )
          .join('; ')
        toast.warning(
          `${t(
            'Deleted {{deletedCount}} promo code(s), skipped {{skippedCount}}',
            {
              deletedCount: deletedIds.length,
              skippedCount: skipped.length,
            }
          )}. ${t('Skipped')}: ${skippedList}`
        )
      } else {
        toast.success(
          t('Successfully deleted {{count}} promo code(s)', {
            count: deletedIds.length,
          })
        )
      }

      await goToValidPageAfterDelete()
    } catch (error: unknown) {
      handleServerError(error)
    } finally {
      setIsBulkDeleting(false)
    }
  }

  const handleDeleteInvalid = async () => {
    setIsDeletingInvalid(true)
    try {
      const response = await deleteInvalidPromoCodes()
      if (!response.success) {
        toast.error(
          response.message || t(PROMO_CODE_ERROR_MESSAGES.DELETE_INVALID_FAILED)
        )
        return
      }

      const deletedIds = response.data?.deleted_ids ?? []
      const skipped = response.data?.skipped ?? []

      setShowDeleteInvalidConfirm(false)

      if (skipped.length > 0) {
        const skippedList = skipped
          .map(
            (entry) =>
              `ID ${entry.id}: ${getBatchDeleteSkipReasonMessage(entry.reason, t)}`
          )
          .join('; ')
        toast.warning(
          `${t(
            'Deleted {{deletedCount}} promo code(s), skipped {{skippedCount}}',
            {
              deletedCount: deletedIds.length,
              skippedCount: skipped.length,
            }
          )}. ${t('Skipped')}: ${skippedList}`
        )
      } else {
        toast.success(
          t('Successfully deleted {{count}} invalid promo codes', {
            count: deletedIds.length,
          })
        )
      }

      await goToValidPageAfterDelete()
    } catch (error: unknown) {
      handleServerError(error)
    } finally {
      setIsDeletingInvalid(false)
    }
  }

  const handleSingleDelete = async () => {
    if (!singleDeleteTarget) return

    setIsSingleDeleting(true)
    try {
      const response = await deletePromoCode(singleDeleteTarget.id)
      if (!response.success) {
        toast.error(
          response.message || t(PROMO_CODE_ERROR_MESSAGES.DELETE_FAILED)
        )
        return
      }
      toast.success(t('Promo code deleted successfully'))
      setShowSingleDeleteConfirm(false)
      setSingleDeleteTarget(null)
      await goToValidPageAfterDelete()
    } catch (error: unknown) {
      handleServerError(error)
    } finally {
      setIsSingleDeleting(false)
    }
  }

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
    } catch (error: unknown) {
      handleServerError(error)
    } finally {
      setSaving(false)
    }
  }

  const toggleStatus = async (promo: PromoCode) => {
    setIsTogglingStatus(promo.id)
    try {
      const response = await updatePromoCodeStatus(
        promo.id,
        promo.status === ENABLED ? DISABLED : ENABLED
      )
      if (!response.success) {
        toast.error(response.message || t('Failed to update status'))
        return
      }
      toast.success(t('Status updated successfully'))
      await refetch()
    } catch (error: unknown) {
      handleServerError(error)
    } finally {
      setIsTogglingStatus(null)
    }
  }

  const openDeleteConfirm = (promo: PromoCode) => {
    setSingleDeleteTarget(promo)
    setShowSingleDeleteConfirm(true)
  }

  const getPromoStatus = (promo: PromoCode) => {
    if (promo.status === EXHAUSTED) {
      return { label: t('Exhausted'), variant: 'neutral' as const }
    }
    if (promo.status === ENABLED && isTimestampExpired(promo.expired_time)) {
      return { label: t('Expired'), variant: 'neutral' as const }
    }
    if (promo.status === DISABLED) {
      return { label: t('Disabled'), variant: 'neutral' as const }
    }
    return { label: t('Enabled'), variant: 'success' as const }
  }

  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'
  const quotaStep = getEditableQuotaStep()
  const discountLabel =
    form.discount_type === 'fixed'
      ? t('Discount Value ({{currency}})', { currency: currencyLabel })
      : t('Discount Value (%)')
  let discountPlaceholder: string
  if (form.discount_type === 'fixed') {
    discountPlaceholder = tokensOnly
      ? t('Enter discount in tokens')
      : t('Enter discount in {{currency}}', { currency: currencyLabel })
  } else {
    discountPlaceholder = t('Enter percentage (0-100)')
  }

  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-center justify-end gap-2'>
        {selectedIds.size > 0 && (
          <Button
            size='sm'
            variant='outline'
            onClick={() => setShowBulkDeleteConfirm(true)}
          >
            <Trash2 className='text-destructive h-4 w-4' />
            {t('Delete selected ({{count}})', { count: selectedIds.size })}
          </Button>
        )}
        <DropdownMenu>
          <DropdownMenuTrigger render={<Button size='sm' variant='outline' />}>
            <MoreHorizontal />
            {t('More')}
          </DropdownMenuTrigger>
          <DropdownMenuContent align='end'>
            <DropdownMenuItem
              className='text-destructive'
              onClick={() => setShowDeleteInvalidConfirm(true)}
            >
              <Trash2 className='h-4 w-4' />
              {t('Delete Invalid')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
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
              <TableHead className='w-10'>
                <Checkbox
                  checked={allSelected}
                  indeterminate={!allSelected && someSelected}
                  onCheckedChange={(checked) =>
                    toggleSelectAll(checked === true)
                  }
                  aria-label={t('Select all')}
                />
              </TableHead>
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
            {items.map((promo) => {
              const status = getPromoStatus(promo)
              return (
                <TableRow key={promo.id}>
                  <TableCell>
                    <Checkbox
                      checked={selectedIds.has(promo.id)}
                      onCheckedChange={(checked) =>
                        toggleRow(promo.id, checked === true)
                      }
                      aria-label={t('Select row')}
                    />
                  </TableCell>
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
                      label={status.label}
                      variant={status.variant}
                      copyable={false}
                    />
                  </TableCell>
                  <TableCell>
                    <div className='flex justify-end'>
                      <DataTableRowActionMenu ariaLabel={t('Open menu')}>
                        <DropdownMenuItem
                          onClick={() => {
                            setForm(promoCodeToForm(promo))
                            setOpen(true)
                          }}
                        >
                          <Pencil className='h-4 w-4' />
                          {t('Edit')}
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          disabled={isTogglingStatus === promo.id}
                          onClick={() => toggleStatus(promo)}
                        >
                          {promo.status === ENABLED
                            ? t('Disable')
                            : t('Enable')}
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          className='text-destructive'
                          onClick={() => openDeleteConfirm(promo)}
                        >
                          <Trash2 className='h-4 w-4' />
                          {t('Delete')}
                        </DropdownMenuItem>
                      </DataTableRowActionMenu>
                    </div>
                  </TableCell>
                </TableRow>
              )
            })}
            {!isFetching && items.length === 0 && (
              <TableRow>
                <TableCell colSpan={8} className='h-24 text-center'>
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
          onClick={() => changePage((value) => Math.max(1, value - 1))}
        >
          {t('Previous')}
        </Button>
        <Button
          size='sm'
          variant='outline'
          disabled={page >= pageCount}
          onClick={() => changePage((value) => Math.min(pageCount, value + 1))}
        >
          {t('Next')}
        </Button>
      </div>

      <ConfirmDialog
        destructive
        open={showBulkDeleteConfirm}
        onOpenChange={setShowBulkDeleteConfirm}
        handleConfirm={handleBulkDelete}
        isLoading={isBulkDeleting}
        className='max-w-md'
        title={t('Delete {{count}} promo code(s)?', {
          count: selectedIds.size,
        })}
        desc={
          <>
            {t('You are about to delete {{count}} promo code(s).', {
              count: selectedIds.size,
            })}{' '}
            <br />
            {t(
              'Existing payment reservations, topup logs, and usage history will not be affected.'
            )}{' '}
            <br />
            {t('This action cannot be undone.')}
          </>
        }
        confirmText={t('Delete')}
      />

      <ConfirmDialog
        destructive
        open={showDeleteInvalidConfirm}
        onOpenChange={setShowDeleteInvalidConfirm}
        handleConfirm={handleDeleteInvalid}
        isLoading={isDeletingInvalid}
        className='max-w-md'
        title={t('Delete Invalid Promo Codes?')}
        desc={
          <>
            {t('This will delete all')} <strong>{t('disabled')}</strong>,{' '}
            <strong>{t('exhausted')}</strong>
            {t(', and')} <strong>{t('expired')}</strong> {t('promo codes.')}
            <br />
            {t('This action cannot be undone.')}
          </>
        }
        confirmText={t('Delete Invalid')}
      />

      <ConfirmDialog
        destructive
        open={showSingleDeleteConfirm}
        onOpenChange={setShowSingleDeleteConfirm}
        handleConfirm={handleSingleDelete}
        isLoading={isSingleDeleting}
        className='max-w-md'
        title={t('Delete promo code?')}
        desc={
          <>
            {t('You are about to delete promo code')}{' '}
            <strong>{singleDeleteTarget?.name}</strong>.
            <br />
            {t('This action cannot be undone.')}
          </>
        }
        confirmText={t('Delete')}
      />

      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent className='sm:max-w-[600px]'>
          <SheetHeader>
            <SheetTitle>
              {form.id ? t('Update Promo Code') : t('Create Promo Code')}
            </SheetTitle>
          </SheetHeader>
          <div className='space-y-4 overflow-y-auto py-4'>
            <div>
              <Label>{t('Name')}</Label>
              <Input
                value={form.name}
                placeholder={t('Name')}
                onChange={(event) => updateField('name', event.target.value)}
              />
            </div>
            <div>
              <Label>{t('Promo Code')}</Label>
              <Input
                value={form.code}
                placeholder={t('Promo Code')}
                onChange={(event) => updateField('code', event.target.value)}
              />
            </div>
            <div>
              <Label>{t('Discount Type')}</Label>
              <Select
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
            </div>
            <div>
              <Label>{discountLabel}</Label>
              <Input
                type='number'
                min='0'
                step={form.discount_type === 'fixed' ? quotaStep : '1'}
                value={form.discount_value}
                placeholder={discountPlaceholder}
                onChange={(event) =>
                  updateField('discount_value', Number(event.target.value) || 0)
                }
              />
              {form.discount_type === 'fixed' && (
                <p className='text-muted-foreground mt-1 text-sm'>
                  {tokensOnly
                    ? t('Enter the discount amount in tokens')
                    : t('Enter the discount amount in {{currency}}', {
                        currency: currencyLabel,
                      })}
                </p>
              )}
            </div>
            <div>
              <Label>{t('Usage Limit')}</Label>
              <Input
                type='number'
                min='0'
                step='1'
                value={form.max_redeem_count}
                placeholder={t('Usage limit, 0 for unlimited')}
                onChange={(event) =>
                  updateField(
                    'max_redeem_count',
                    Number(event.target.value) || 0
                  )
                }
              />
            </div>
            <div>
              <Label>{t('Expiration Time')}</Label>
              <Input
                type='datetime-local'
                value={form.expired_time_text}
                onChange={(event) =>
                  updateField('expired_time_text', event.target.value)
                }
              />
            </div>
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
