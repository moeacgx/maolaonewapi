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
import { useEffect, useMemo, useState } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
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
import { Textarea } from '@/components/ui/textarea'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  createErrorMessageReplacementRule,
  ERROR_MESSAGE_REPLACEMENT_MODES,
  parseErrorMessageReplacementRules,
  serializeErrorMessageReplacementRules,
  type ErrorMessageReplacementRule,
  validateErrorMessageReplacementRules,
} from './error-message-rules'

const MODE_LABELS = {
  contains: 'Contains',
  exact: 'Exact match',
  regex: 'Regular expression',
} as const

const parseStatusCodeInput = (value: string): number | undefined =>
  value.trim() === '' ? undefined : Number(value)

type Props = {
  defaultValue: string
}

export function ErrorMessageSettingsSection(props: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const initialRules = useMemo(
    () => parseErrorMessageReplacementRules(props.defaultValue),
    [props.defaultValue]
  )
  const [rules, setRules] =
    useState<ErrorMessageReplacementRule[]>(initialRules)

  useEffect(() => setRules(initialRules), [initialRules])

  const updateRule = (
    index: number,
    changes: Partial<ErrorMessageReplacementRule>
  ) => {
    setRules((current) =>
      current.map((rule, currentIndex) =>
        currentIndex === index ? { ...rule, ...changes } : rule
      )
    )
  }

  const save = async () => {
    if (!validateErrorMessageReplacementRules(rules)) {
      toast.error(
        t(
          'Every rule needs a match value and replacement message. Status codes must be between 100 and 599.'
        )
      )
      return
    }
    const result = await updateOption.mutateAsync({
      key: 'ErrorMessageReplacementRules',
      value: serializeErrorMessageReplacementRules(rules),
    })
    if (result.success) {
      setRules(
        parseErrorMessageReplacementRules(
          serializeErrorMessageReplacementRules(rules)
        )
      )
    }
  }

  return (
    <SettingsSection title={t('Client error messages')}>
      <SettingsPageFormActions
        onSave={save}
        isSaving={updateOption.isPending}
        saveLabel='Save error message rules'
      />
      <p className='text-muted-foreground text-sm'>
        {t(
          'Rules are checked in order. An optional original status code can be combined with the error text, then both the client status code and message can be replaced. Upstream errors still drive retries, channel disabling, and security audit.'
        )}
      </p>
      <div className='flex flex-col gap-3'>
        {rules.map((rule, index) => (
          <div
            key={index}
            className='border-border grid gap-4 rounded-md border p-4 md:grid-cols-2 xl:grid-cols-[9rem_minmax(0,1fr)_11rem_minmax(0,1fr)_9rem_2.25rem]'
          >
            <label className='grid content-start gap-2'>
              <span className='text-sm font-medium'>
                {t('Original status code (optional)')}
              </span>
              <Input
                type='number'
                min={100}
                max={599}
                step={1}
                value={rule.statusCode ?? ''}
                placeholder='403'
                onChange={(event) =>
                  updateRule(index, {
                    statusCode: parseStatusCodeInput(event.target.value),
                  })
                }
              />
            </label>
            <label className='grid gap-2'>
              <span className='text-sm font-medium'>{t('Match')}</span>
              <Textarea
                value={rule.match}
                rows={2}
                maxLength={4096}
                placeholder={t('Text from the original error message')}
                onChange={(event) =>
                  updateRule(index, { match: event.target.value })
                }
              />
            </label>
            <div className='grid content-start gap-2'>
              <Label>{t('Match mode')}</Label>
              <Select
                value={rule.mode}
                onValueChange={(value) =>
                  updateRule(index, {
                    mode: value as ErrorMessageReplacementRule['mode'],
                  })
                }
              >
                <SelectTrigger className='w-full'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    {ERROR_MESSAGE_REPLACEMENT_MODES.map((mode) => (
                      <SelectItem key={mode} value={mode}>
                        {t(MODE_LABELS[mode])}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
            <label className='grid gap-2'>
              <span className='text-sm font-medium'>{t('Replace with')}</span>
              <Textarea
                value={rule.replace}
                rows={2}
                maxLength={4096}
                placeholder={t('Message returned to the client')}
                onChange={(event) =>
                  updateRule(index, { replace: event.target.value })
                }
              />
            </label>
            <label className='grid content-start gap-2'>
              <span className='text-sm font-medium'>
                {t('New status code (optional)')}
              </span>
              <Input
                type='number'
                min={100}
                max={599}
                step={1}
                value={rule.replaceStatusCode ?? ''}
                placeholder='429'
                onChange={(event) =>
                  updateRule(index, {
                    replaceStatusCode: parseStatusCodeInput(event.target.value),
                  })
                }
              />
            </label>
            <Button
              type='button'
              variant='ghost'
              size='icon'
              className='self-end'
              aria-label={t('Delete rule')}
              title={t('Delete rule')}
              onClick={() =>
                setRules((current) =>
                  current.filter((_, currentIndex) => currentIndex !== index)
                )
              }
            >
              <Trash2 className='size-4' />
            </Button>
          </div>
        ))}
        {rules.length === 0 && (
          <div className='border-border text-muted-foreground rounded-md border border-dashed px-4 py-8 text-center text-sm'>
            {t('No replacement rules configured')}
          </div>
        )}
      </div>
      <Button
        type='button'
        variant='outline'
        className='w-fit'
        disabled={rules.length >= 100}
        onClick={() =>
          setRules((current) => [
            ...current,
            createErrorMessageReplacementRule(),
          ])
        }
      >
        <Plus className='size-4' />
        {t('Add rule')}
      </Button>
    </SettingsSection>
  )
}
