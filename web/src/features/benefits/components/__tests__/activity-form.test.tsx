import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { BenefitActivityForm } from '../benefit-activity-form'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (value: string) => value }),
}))

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
    fireEvent.change(screen.getByLabelText('Fixed amount (yuan)'), {
      target: { value: '2' },
    })
    fireEvent.change(screen.getByLabelText('Total count'), {
      target: { value: '3' },
    })
    expect(screen.queryByLabelText('Total budget (yuan)')).toBeNull()
    expect(screen.getByText('Calculated total budget (yuan)')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Create draft' }))
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        total_amount: 6,
        fixed_amount: 2,
        total_count: 3,
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
    expect(screen.queryByLabelText('Fixed amount (yuan)')).toBeNull()
    expect(screen.getByLabelText('Total budget (yuan)')).toBeTruthy()
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
    fireEvent.change(screen.getByLabelText('Total budget (yuan)'), {
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

  it('rejects amounts with more than two decimal places', () => {
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
    fireEvent.change(screen.getByLabelText('Total budget (yuan)'), {
      target: { value: '10.001' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create draft' }))
    expect(
      screen.getByText('Amounts must use at most two decimal places')
    ).toBeTruthy()
    expect(submit).not.toHaveBeenCalled()
  })
})
