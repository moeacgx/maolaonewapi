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
  total_amount_cents: 1000,
  total_quota: 100000,
  total_count: 10,
  fixed_amount_cents: 100,
  min_amount_cents: 50,
  max_amount_cents: 200,
  claim_paid_threshold_cents: 0,
  personal_valid_seconds: 86400,
  starts_at: Math.floor(Date.now() / 1000),
  ends_at: Math.floor(Date.now() / 1000) + 86400,
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
  const [form, setForm] = useState({ ...defaultForm, ...props.initial })
  const [error, setError] = useState('')

  const update = <K extends keyof BenefitActivityInput>(
    key: K,
    value: BenefitActivityInput[K]
  ) => setForm((current) => ({ ...current, [key]: value }))

  const submit = async () => {
    if (
      !form.name.trim() ||
      form.group_id <= 0 ||
      form.total_count <= 0 ||
      form.total_quota <= 0
    ) {
      setError(t('Please complete the required benefit activity fields'))
      return
    }
    if (
      form.amount_mode === 'fixed' &&
      form.fixed_amount_cents * form.total_count !== form.total_amount_cents
    ) {
      setError(t('Fixed amount times count must equal total budget'))
      return
    }
    if (
      form.amount_mode === 'random' &&
      (form.total_amount_cents < form.min_amount_cents * form.total_count ||
        form.total_amount_cents > form.max_amount_cents * form.total_count)
    ) {
      setError(t('Random amount bounds cannot satisfy the total budget'))
      return
    }
    setError('')
    await props.onSubmit(form)
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
        aria-label={t('Total budget in cents')}
        type='number'
        min={1}
        value={form.total_amount_cents}
        onChange={(event) =>
          update('total_amount_cents', Number(event.target.value))
        }
      />
      <Input
        aria-label={t('Total quota')}
        type='number'
        min={1}
        value={form.total_quota}
        onChange={(event) => update('total_quota', Number(event.target.value))}
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
        aria-label={t('Fixed amount in cents')}
        type='number'
        min={1}
        value={form.fixed_amount_cents}
        onChange={(event) =>
          update('fixed_amount_cents', Number(event.target.value))
        }
      />
      {form.amount_mode === 'random' ? (
        <>
          <Input
            aria-label={t('Minimum amount in cents')}
            type='number'
            min={1}
            value={form.min_amount_cents}
            onChange={(event) =>
              update('min_amount_cents', Number(event.target.value))
            }
          />
          <Input
            aria-label={t('Maximum amount in cents')}
            type='number'
            min={1}
            value={form.max_amount_cents}
            onChange={(event) =>
              update('max_amount_cents', Number(event.target.value))
            }
          />
        </>
      ) : null}
      <Input
        aria-label={t('Paid threshold in cents')}
        type='number'
        min={0}
        value={form.claim_paid_threshold_cents}
        onChange={(event) =>
          update('claim_paid_threshold_cents', Number(event.target.value))
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
