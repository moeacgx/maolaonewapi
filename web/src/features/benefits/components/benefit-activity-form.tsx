import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { ApiKeyGroupCombobox } from '@/features/keys/components/api-key-group-combobox'

import type { BenefitActivityInput, BenefitGroupOption } from '../api'

type BenefitActivityFormProps = {
  onSubmit: (input: BenefitActivityInput) => Promise<void>
  onCancel: () => void
  initial?: Partial<BenefitActivityInput>
  groupOptions?: readonly BenefitGroupOption[]
}

const defaultForm: BenefitActivityInput = {
  name: '',
  description: '',
  group_id: 0,
  amount_mode: 'fixed',
  total_amount: 10,
  total_count: 10,
  fixed_amount: 1,
  min_amount: 0.5,
  max_amount: 2,
  claim_paid_threshold: 0,
  personal_valid_hours: 24,
  starts_at: Math.floor(Date.now() / 1000),
  ends_at: Math.floor(Date.now() / 1000) + 86400,
}

type LegacyBenefitActivityAmounts = {
  total_amount_cents?: number
  fixed_amount_cents?: number
  min_amount_cents?: number
  max_amount_cents?: number
  claim_paid_threshold_cents?: number
  personal_valid_seconds?: number
}

function amountFromInitial(
  amount: number | undefined,
  legacyAmount: number | undefined,
  fallback: number
) {
  if (typeof amount === 'number' && Number.isFinite(amount)) return amount
  if (typeof legacyAmount === 'number' && Number.isFinite(legacyAmount)) {
    return legacyAmount / 100
  }
  return fallback
}

function normalizeInitial(initial?: Partial<BenefitActivityInput>) {
  const legacy = initial as Partial<LegacyBenefitActivityAmounts> | undefined
  return {
    ...defaultForm,
    ...initial,
    total_amount: amountFromInitial(
      initial?.total_amount,
      legacy?.total_amount_cents,
      defaultForm.total_amount
    ),
    fixed_amount: amountFromInitial(
      initial?.fixed_amount,
      legacy?.fixed_amount_cents,
      defaultForm.fixed_amount
    ),
    min_amount: amountFromInitial(
      initial?.min_amount,
      legacy?.min_amount_cents,
      defaultForm.min_amount
    ),
    max_amount: amountFromInitial(
      initial?.max_amount,
      legacy?.max_amount_cents,
      defaultForm.max_amount
    ),
    claim_paid_threshold: amountFromInitial(
      initial?.claim_paid_threshold,
      legacy?.claim_paid_threshold_cents,
      defaultForm.claim_paid_threshold
    ),
    personal_valid_hours: validityHoursFromInitial(
      initial?.personal_valid_hours,
      legacy?.personal_valid_seconds,
      defaultForm.personal_valid_hours
    ),
  }
}

function amountInMinorUnits(value: number) {
  if (!Number.isFinite(value) || value < 0) return null
  const minor = Math.round(value * 100)
  return Math.abs(value * 100 - minor) < 1e-7 ? minor : null
}

function validityHoursFromInitial(
  hours: number | undefined,
  legacySeconds: number | undefined,
  fallback: number
) {
  if (typeof hours === 'number' && Number.isFinite(hours)) return hours
  if (typeof legacySeconds === 'number' && Number.isFinite(legacySeconds)) {
    return legacySeconds / 3600
  }
  return fallback
}

function fixedTotalAmount(amount: number, count: number) {
  const minorAmount = amountInMinorUnits(amount)
  if (minorAmount === null || !Number.isInteger(count) || count <= 0) {
    return 0
  }
  return (minorAmount * count) / 100
}

function amountRange(minimum: number, maximum: number, count: number) {
  const minimumMinor = amountInMinorUnits(minimum)
  const maximumMinor = amountInMinorUnits(maximum)
  if (
    minimumMinor === null ||
    maximumMinor === null ||
    !Number.isInteger(count) ||
    count <= 0
  ) {
    return null
  }
  return {
    minimum: (minimumMinor * count) / 100,
    maximum: (maximumMinor * count) / 100,
  }
}

function toDateTimeLocal(timestamp: number) {
  const parts = new Intl.DateTimeFormat('en-CA', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
  }).formatToParts(new Date(timestamp * 1000))
  const values = Object.fromEntries(
    parts.map((part) => [part.type, part.value])
  )
  return `${values.year}-${values.month}-${values.day}T${values.hour}:${values.minute}`
}

function fromDateTimeLocal(value: string) {
  const timestamp = Math.floor(new Date(`${value}:00+08:00`).getTime() / 1000)
  return Number.isFinite(timestamp) ? timestamp : 0
}

export function BenefitActivityForm(props: BenefitActivityFormProps) {
  const { t } = useTranslation()
  const [form, setForm] = useState(() => normalizeInitial(props.initial))
  const [error, setError] = useState('')

  const update = <K extends keyof BenefitActivityInput>(
    key: K,
    value: BenefitActivityInput[K]
  ) => setForm((current) => ({ ...current, [key]: value }))

  const computedTotalAmount =
    form.amount_mode === 'fixed'
      ? fixedTotalAmount(form.fixed_amount, form.total_count)
      : form.total_amount
  const computedRange = amountRange(
    form.min_amount,
    form.max_amount,
    form.total_count
  )

  const submit = async () => {
    if (!form.name.trim() || form.group_id <= 0 || form.total_count <= 0) {
      setError(t('Please complete the required benefit activity fields'))
      return
    }
    const amounts =
      form.amount_mode === 'fixed'
        ? [form.fixed_amount, form.claim_paid_threshold]
        : [
            form.total_amount,
            form.min_amount,
            form.max_amount,
            form.claim_paid_threshold,
          ]
    if (amounts.some((amount) => amountInMinorUnits(amount) === null)) {
      setError(t('Amounts must use at most two decimal places'))
      return
    }
    if (
      computedTotalAmount <= 0 ||
      (form.amount_mode === 'fixed' && form.fixed_amount <= 0) ||
      (form.amount_mode === 'random' &&
        (form.min_amount <= 0 || form.max_amount <= 0)) ||
      form.claim_paid_threshold < 0
    ) {
      setError(t('Amounts must be valid'))
      return
    }
    if (
      form.amount_mode === 'random' &&
      (!computedRange ||
        form.total_amount < computedRange.minimum ||
        form.total_amount > computedRange.maximum)
    ) {
      setError(t('Random amount bounds cannot satisfy the total budget'))
      return
    }
    setError('')
    await props.onSubmit({
      id: form.id,
      name: form.name,
      description: form.description,
      group_id: form.group_id,
      amount_mode: form.amount_mode,
      total_amount: computedTotalAmount,
      total_count: form.total_count,
      fixed_amount: form.amount_mode === 'fixed' ? form.fixed_amount : 0,
      min_amount: form.amount_mode === 'random' ? form.min_amount : 0,
      max_amount: form.amount_mode === 'random' ? form.max_amount : 0,
      claim_paid_threshold: form.claim_paid_threshold,
      personal_valid_hours: form.personal_valid_hours,
      starts_at: form.starts_at,
      ends_at: form.ends_at,
    })
  }

  return (
    <div className='border-border grid gap-3 border-b pb-4 md:grid-cols-2'>
      <Input
        aria-label={t('Activity name')}
        placeholder={t('Activity name')}
        value={form.name}
        onChange={(event) => update('name', event.target.value)}
      />
      <Textarea
        aria-label={t('Activity description')}
        placeholder={t('Activity description')}
        value={form.description}
        onChange={(event) => update('description', event.target.value)}
      />
      <label className='grid gap-1 text-sm'>
        <span>{t('Benefit group')}</span>
        <ApiKeyGroupCombobox
          options={props.groupOptions ? [...props.groupOptions] : []}
          value={form.group_id > 0 ? String(form.group_id) : undefined}
          onValueChange={(value) => update('group_id', Number(value ?? 0))}
          placeholder={t('Select a group')}
          disabled={!props.groupOptions?.length}
        />
      </label>
      <label className='grid gap-1 text-sm'>
        <span>{t('Amount mode')}</span>
        <select
          className='border-input bg-background h-9 rounded-md border px-3'
          value={form.amount_mode}
          onChange={(event) =>
            update(
              'amount_mode',
              event.target.value as BenefitActivityInput['amount_mode']
            )
          }
        >
          <option value='fixed'>{t('Fixed amount')}</option>
          <option value='random'>{t('Random amount')}</option>
        </select>
      </label>
      {form.amount_mode === 'fixed' ? (
        <>
          <Input
            aria-label={t('Fixed amount (yuan)')}
            type='number'
            min={0.01}
            step={0.01}
            value={form.fixed_amount}
            onChange={(event) =>
              update('fixed_amount', Number(event.target.value))
            }
          />
          <Input
            aria-label={t('Total count')}
            type='number'
            min={1}
            value={form.total_count}
            onChange={(event) =>
              update('total_count', Number(event.target.value))
            }
          />
          <div className='bg-muted/40 border-border grid gap-1 rounded-md border p-3 md:col-span-2'>
            <span className='text-sm font-medium'>
              {t('Calculated total budget (yuan)')}
            </span>
            <span className='text-primary text-lg font-semibold tabular-nums'>
              ¥{computedTotalAmount.toFixed(2)}
            </span>
            <span className='text-muted-foreground text-xs'>
              {t('Calculated from amount per voucher and total count')}
            </span>
          </div>
        </>
      ) : (
        <>
          <Input
            aria-label={t('Total budget (yuan)')}
            type='number'
            min={0.01}
            step={0.01}
            value={form.total_amount}
            onChange={(event) =>
              update('total_amount', Number(event.target.value))
            }
          />
          <Input
            aria-label={t('Total count')}
            type='number'
            min={1}
            value={form.total_count}
            onChange={(event) =>
              update('total_count', Number(event.target.value))
            }
          />
          <Input
            aria-label={t('Minimum amount (yuan)')}
            type='number'
            min={0.01}
            step={0.01}
            value={form.min_amount}
            onChange={(event) =>
              update('min_amount', Number(event.target.value))
            }
          />
          <Input
            aria-label={t('Maximum amount (yuan)')}
            type='number'
            min={0.01}
            step={0.01}
            value={form.max_amount}
            onChange={(event) =>
              update('max_amount', Number(event.target.value))
            }
          />
          <div className='border-border bg-muted/40 grid gap-1 rounded-md border p-3 md:col-span-2'>
            <span className='text-sm font-medium'>
              {t('Possible total budget range')}
            </span>
            <span className='text-primary font-semibold tabular-nums'>
              {computedRange
                ? `¥${computedRange.minimum.toFixed(2)} ~ ¥${computedRange.maximum.toFixed(2)}`
                : '-'}
            </span>
            <span className='text-muted-foreground text-xs'>
              {t('Total budget must stay within this range')}
            </span>
          </div>
        </>
      )}
      <Input
        aria-label={t('Paid threshold (yuan)')}
        type='number'
        min={0}
        step={0.01}
        value={form.claim_paid_threshold}
        onChange={(event) =>
          update('claim_paid_threshold', Number(event.target.value))
        }
      />
      <Input
        aria-label={t('Personal validity in hours')}
        type='number'
        min={1}
        value={form.personal_valid_hours}
        onChange={(event) =>
          update('personal_valid_hours', Number(event.target.value))
        }
      />
      <label className='grid gap-1 text-sm'>
        <span>{t('Activity starts')}</span>
        <Input
          type='datetime-local'
          value={toDateTimeLocal(form.starts_at)}
          onChange={(event) =>
            update('starts_at', fromDateTimeLocal(event.target.value))
          }
        />
      </label>
      <label className='grid gap-1 text-sm'>
        <span>{t('Activity ends')}</span>
        <Input
          type='datetime-local'
          value={toDateTimeLocal(form.ends_at)}
          onChange={(event) =>
            update('ends_at', fromDateTimeLocal(event.target.value))
          }
        />
      </label>
      {error ? (
        <p className='text-destructive text-sm md:col-span-2'>{error}</p>
      ) : null}
      <div className='flex gap-2 md:col-span-2'>
        <Button type='button' onClick={() => void submit()}>
          {props.initial?.id ? t('Save activity') : t('Create draft')}
        </Button>
        <Button type='button' variant='outline' onClick={props.onCancel}>
          {t('Cancel')}
        </Button>
      </div>
    </div>
  )
}
