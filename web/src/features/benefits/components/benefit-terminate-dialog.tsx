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
