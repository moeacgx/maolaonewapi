import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'

import type { BenefitActivityInput } from '../api'

type BenefitActivityFormProps = {
  onSubmit: (input: BenefitActivityInput) => Promise<void>
  onCancel: () => void
  initial?: Partial<BenefitActivityInput>
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
  personal_valid_seconds: 86400,
  starts_at: Math.floor(Date.now() / 1000),
  ends_at: Math.floor(Date.now() / 1000) + 86400,
}

type LegacyBenefitActivityAmounts = {
  total_amount_cents?: number
  fixed_amount_cents?: number
  min_amount_cents?: number
  max_amount_cents?: number
  claim_paid_threshold_cents?: number
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
  }
}

function amountInMinorUnits(value: number) {
  if (!Number.isFinite(value) || value < 0) return null
  const minor = Math.round(value * 100)
  return Math.abs(value * 100 - minor) < 1e-7 ? minor : null
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

  const submit = async () => {
    if (
      !form.name.trim() ||
      form.group_id <= 0 ||
      form.total_count <= 0
    ) {
      setError(t('Please complete the required benefit activity fields'))
      return
    }
    const amounts = [
      form.total_amount,
      form.fixed_amount,
      form.min_amount,
      form.max_amount,
      form.claim_paid_threshold,
    ]
    if (amounts.some((amount) => amountInMinorUnits(amount) === null)) {
      setError(t('Amounts must use at most two decimal places'))
      return
    }
    if (
      form.total_amount <= 0 ||
      (form.amount_mode === 'fixed' && form.fixed_amount <= 0) ||
      (form.amount_mode === 'random' &&
        (form.min_amount <= 0 || form.max_amount <= 0)) ||
      form.claim_paid_threshold < 0
    ) {
      setError(t('Amounts must be valid'))
      return
    }
    if (
      form.amount_mode === 'fixed' &&
      amountInMinorUnits(form.fixed_amount)! * form.total_count !==
        amountInMinorUnits(form.total_amount)!
    ) {
      setError(t('Fixed amount times count must equal total budget'))
      return
    }
    if (
      form.amount_mode === 'random' &&
      (amountInMinorUnits(form.total_amount)! <
        amountInMinorUnits(form.min_amount)! * form.total_count ||
        amountInMinorUnits(form.total_amount)! >
          amountInMinorUnits(form.max_amount)! * form.total_count)
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
      total_amount: form.total_amount,
      total_count: form.total_count,
      fixed_amount: form.fixed_amount,
      min_amount: form.min_amount,
      max_amount: form.max_amount,
      claim_paid_threshold: form.claim_paid_threshold,
      personal_valid_seconds: form.personal_valid_seconds,
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
      <Input
        aria-label={t('Benefit group ID')}
        type='number'
        min={1}
        value={form.group_id || ''}
        onChange={(event) => update('group_id', Number(event.target.value))}
      />
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
        onChange={(event) => update('total_count', Number(event.target.value))}
      />
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
      {form.amount_mode === 'random' ? (
        <>
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
        </>
      ) : null}
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
        aria-label={t('Personal validity in seconds')}
        type='number'
        min={1}
        value={form.personal_valid_seconds}
        onChange={(event) =>
          update('personal_valid_seconds', Number(event.target.value))
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
