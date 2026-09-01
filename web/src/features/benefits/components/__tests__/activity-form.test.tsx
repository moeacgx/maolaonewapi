import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { BenefitActivityForm } from '../benefit-activity-form'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (value: string) => value }),
}))

describe('benefit activity form', () => {
  it('blocks fixed activity submission when budget does not match count', () => {
    const submit = vi.fn()
    render(<BenefitActivityForm onSubmit={submit} onCancel={vi.fn()} />)
    fireEvent.change(screen.getByLabelText('Activity name'), {
      target: { value: 'Weekend' },
    })
    fireEvent.change(screen.getByLabelText('Benefit group ID'), {
      target: { value: '7' },
    })
    fireEvent.change(screen.getByLabelText('Total count'), {
      target: { value: '2' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create draft' }))
    expect(
      screen.getByText('Fixed amount times count must equal total budget')
    ).toBeTruthy()
    expect(submit).not.toHaveBeenCalled()
  })

  it('submits display amounts without a manually entered quota', () => {
    const submit = vi.fn().mockResolvedValue(undefined)
    render(<BenefitActivityForm onSubmit={submit} onCancel={vi.fn()} />)
    fireEvent.change(screen.getByLabelText('Activity name'), {
      target: { value: 'Weekend' },
    })
    fireEvent.change(screen.getByLabelText('Benefit group ID'), {
      target: { value: '7' },
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

  it('rejects amounts with more than two decimal places', () => {
    const submit = vi.fn()
    render(<BenefitActivityForm onSubmit={submit} onCancel={vi.fn()} />)
    fireEvent.change(screen.getByLabelText('Activity name'), {
      target: { value: 'Weekend' },
    })
    fireEvent.change(screen.getByLabelText('Benefit group ID'), {
      target: { value: '7' },
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
