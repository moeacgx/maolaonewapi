/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

type BenefitTerminateDialogProps = {
  onConfirm: (mode: 'unused' | 'all', reason: string) => Promise<void>
  onCancel: () => void
}

export function BenefitTerminateDialog(props: BenefitTerminateDialogProps) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<'unused' | 'all'>('unused')
  const [reason, setReason] = useState('')
  const [confirmed, setConfirmed] = useState(false)
  const canConfirm = confirmed && reason.trim().length > 0

  return (
    <div
      role='dialog'
      aria-label={t('Terminate benefit activity')}
      className='border-border bg-background ring-foreground/10 grid gap-3 p-4 shadow-lg ring-1'
    >
      <label className='flex items-center gap-2 text-sm'>
        <input
          type='radio'
          name='terminate-mode'
          checked={mode === 'unused'}
          onChange={() => setMode('unused')}
        />
        {t('Void unclaimed vouchers')}
      </label>
      <label className='flex items-center gap-2 text-sm'>
        <input
          type='radio'
          name='terminate-mode'
          checked={mode === 'all'}
          onChange={() => setMode('all')}
        />
        {t('Void all vouchers')}
      </label>
      <Input
        aria-label={t('Reason')}
        placeholder={t('Reason')}
        value={reason}
        onChange={(event) => setReason(event.target.value)}
      />
      <label className='flex items-center gap-2 text-sm'>
        <input
          type='checkbox'
          checked={confirmed}
          onChange={(event) => setConfirmed(event.target.checked)}
        />
        {t('I confirm this irreversible action')}
      </label>
      <div className='flex gap-2'>
        <Button
          type='button'
          variant='destructive'
          disabled={!canConfirm}
          onClick={() => void props.onConfirm(mode, reason)}
        >
          {t('Terminate')}
        </Button>
        <Button type='button' variant='outline' onClick={props.onCancel}>
          {t('Cancel')}
        </Button>
      </div>
    </div>
  )
}
