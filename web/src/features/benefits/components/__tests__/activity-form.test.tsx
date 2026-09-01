import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { getCurrencyLabel } from '@/lib/currency'
import { getEditableQuotaStep } from '@/lib/format'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
  type CurrencyConfig,
} from '@/stores/system-config-store'

import { BenefitActivityForm } from '../benefit-activity-form'

function setCurrencyConfig(overrides: Partial<CurrencyConfig>) {
  useSystemConfigStore.getState().setConfig({
    currency: { ...DEFAULT_CURRENCY_CONFIG, ...overrides },
  })
}

afterEach(() => {
  setCurrencyConfig(DEFAULT_CURRENCY_CONFIG)
})

describe('benefit activity form', () => {
  const groupOptions = [{ value: '7', label: 'Codex-Hack', id: 7 }]

  it('derives the fixed total budget from amount and count', async () => {
    const submit = vi.fn().mockResolvedValue(undefined)
    render(
      <BenefitActivityForm
        onSubmit={submit}
        onCancel={vi.fn()}
        groupOptions={groupOptions}
        initial={{ group_id: 7 }}
      />
    )
    fireEvent.change(screen.getByLabelText('Activity name'), {
      target: { value: 'Weekend' },
    })
    fireEvent.change(screen.getByLabelText('Fixed amount (USD)'), {
      target: { value: '2' },
    })
    fireEvent.change(screen.getByLabelText('Total count'), {
      target: { value: '3' },
    })
    expect(screen.queryByLabelText('Total budget (USD)')).toBeNull()
    expect(screen.getByText('Calculated total budget (USD)')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Create draft' }))
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        total_amount: 6,
        fixed_amount: 2,
        total_count: 3,
        amount_display_type: 'USD',
      })
    )
  })

  it('shows only random amount fields and the valid budget range', () => {
    render(
      <BenefitActivityForm
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        groupOptions={groupOptions}
        initial={{ group_id: 7 }}
      />
    )
    fireEvent.change(screen.getByLabelText('Amount mode'), {
      target: { value: 'random' },
    })
    expect(screen.queryByLabelText('Fixed amount (USD)')).toBeNull()
    expect(screen.getByLabelText('Total budget (USD)')).toBeTruthy()
    expect(screen.getByText('Possible total budget range')).toBeTruthy()
  })

  it('blocks random activity submission when the budget is outside its range', () => {
    const submit = vi.fn()
    render(
      <BenefitActivityForm
        onSubmit={submit}
        onCancel={vi.fn()}
        groupOptions={groupOptions}
        initial={{ group_id: 7 }}
      />
    )
    fireEvent.change(screen.getByLabelText('Activity name'), {
      target: { value: 'Weekend' },
    })
    fireEvent.change(screen.getByLabelText('Amount mode'), {
      target: { value: 'random' },
    })
    fireEvent.change(screen.getByLabelText('Total budget (USD)'), {
      target: { value: '25' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create draft' }))
    expect(
      screen.getByText('Random amount bounds cannot satisfy the total budget')
    ).toBeTruthy()
    expect(submit).not.toHaveBeenCalled()
  })

  it('submits display amounts without a manually entered quota', () => {
    const submit = vi.fn().mockResolvedValue(undefined)
    render(
      <BenefitActivityForm
        onSubmit={submit}
        onCancel={vi.fn()}
        groupOptions={groupOptions}
        initial={{ group_id: 7 }}
      />
    )
    fireEvent.change(screen.getByLabelText('Activity name'), {
      target: { value: 'Weekend' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create draft' }))
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        total_amount: 10,
        fixed_amount: 1,
      })
    )
    expect(submit.mock.calls[0][0]).not.toHaveProperty('total_quota')
  })

  it('submits personal voucher validity in hours', () => {
    const submit = vi.fn().mockResolvedValue(undefined)
    render(
      <BenefitActivityForm
        onSubmit={submit}
        onCancel={vi.fn()}
        groupOptions={groupOptions}
        initial={{ group_id: 7 }}
      />
    )
    fireEvent.change(screen.getByLabelText('Activity name'), {
      target: { value: 'Weekend' },
    })
    fireEvent.change(screen.getByLabelText('Personal validity in hours'), {
      target: { value: '48' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create draft' }))
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({ personal_valid_hours: 48 })
    )
    expect(submit.mock.calls[0][0]).not.toHaveProperty('personal_valid_seconds')
  })

  it('rejects currency amounts with more than two decimal places', () => {
    const submit = vi.fn()
    render(
      <BenefitActivityForm
        onSubmit={submit}
        onCancel={vi.fn()}
        groupOptions={groupOptions}
        initial={{ group_id: 7 }}
      />
    )
    fireEvent.change(screen.getByLabelText('Activity name'), {
      target: { value: 'Weekend' },
    })
    fireEvent.change(screen.getByLabelText('Amount mode'), {
      target: { value: 'random' },
    })
    fireEvent.change(screen.getByLabelText('Total budget (USD)'), {
      target: { value: '10.001' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create draft' }))
    expect(
      screen.getByText('Amounts must use at most two decimal places')
    ).toBeTruthy()
    expect(submit).not.toHaveBeenCalled()
  })

  it.each([
    ['USD' as const],
    ['CNY' as const],
    ['CUSTOM' as const],
    ['TOKENS' as const],
  ])(
    'labels and steps the total budget input for the %s display type',
    (displayType) => {
      setCurrencyConfig({ quotaDisplayType: displayType })
      render(
        <BenefitActivityForm
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
          groupOptions={groupOptions}
          initial={{ group_id: 7 }}
        />
      )
      fireEvent.change(screen.getByLabelText('Amount mode'), {
        target: { value: 'random' },
      })
      const currencyLabel = getCurrencyLabel()
      const expectedStep = String(getEditableQuotaStep())
      const input = screen.getByLabelText(`Total budget (${currencyLabel})`)
      expect(input).toHaveAttribute('step', expectedStep)
    }
  )

  it('accepts only whole-number amounts in TOKENS mode', () => {
    setCurrencyConfig({ quotaDisplayType: 'TOKENS' })
    const submit = vi.fn()
    render(
      <BenefitActivityForm
        onSubmit={submit}
        onCancel={vi.fn()}
        groupOptions={groupOptions}
        initial={{ group_id: 7 }}
      />
    )
    fireEvent.change(screen.getByLabelText('Activity name'), {
      target: { value: 'Weekend' },
    })
    fireEvent.change(screen.getByLabelText('Fixed amount (Tokens)'), {
      target: { value: '2.5' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create draft' }))
    expect(screen.getByText('Token amounts must be whole numbers')).toBeTruthy()
    expect(submit).not.toHaveBeenCalled()
  })

  it('submits the current amount_display_type with the activity', () => {
    setCurrencyConfig({ quotaDisplayType: 'CNY' })
    const submit = vi.fn().mockResolvedValue(undefined)
    render(
      <BenefitActivityForm
        onSubmit={submit}
        onCancel={vi.fn()}
        groupOptions={groupOptions}
        initial={{ group_id: 7 }}
      />
    )
    fireEvent.change(screen.getByLabelText('Activity name'), {
      target: { value: 'Weekend' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create draft' }))
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({ amount_display_type: 'CNY' })
    )
  })

  it('formats the fixed budget preview through the shared quota formatter, never as raw quota', () => {
    render(
      <BenefitActivityForm
        onSubmit={vi.fn()}
        onCancel={vi.fn()}
        groupOptions={groupOptions}
        initial={{ group_id: 7 }}
      />
    )
    fireEvent.change(screen.getByLabelText('Fixed amount (USD)'), {
      target: { value: '2' },
    })
    fireEvent.change(screen.getByLabelText('Total count'), {
      target: { value: '3' },
    })
    expect(screen.queryByText('3000000')).toBeNull()
    expect(screen.queryByText('500000')).toBeNull()
  })
})
