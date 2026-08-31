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
})
