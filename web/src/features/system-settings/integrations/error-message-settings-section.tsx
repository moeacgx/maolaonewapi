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
import {
  Add01Icon,
  ArrowDown01Icon,
  ArrowRight01Icon,
  Delete02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
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
  MAX_ERROR_MESSAGE_MATCHES_PER_RULE,
  MAX_ERROR_MESSAGE_REPLACEMENT_RULES,
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
  const matchInputRefs = useRef<Record<string, HTMLTextAreaElement | null>>({})
  const pendingMatchFocus = useRef<string | null>(null)
  const initialRules = useMemo(
    () => parseErrorMessageReplacementRules(props.defaultValue),
    [props.defaultValue]
  )
  const [rules, setRules] =
    useState<ErrorMessageReplacementRule[]>(initialRules)
  const [expandedRules, setExpandedRules] = useState<Record<number, boolean>>(
    {}
  )
  const nextEditorId = useRef(0)
  const ruleEditorIds = useRef<string[]>([])
  const matchEditorIds = useRef<Record<string, string[]>>({})

  while (ruleEditorIds.current.length < rules.length) {
    ruleEditorIds.current.push(`error-rule-${nextEditorId.current++}`)
  }
  if (ruleEditorIds.current.length > rules.length) {
    ruleEditorIds.current.splice(rules.length)
  }

  const ruleRows = rules.map((rule, index) => {
    const editorId = ruleEditorIds.current[index]
    if (editorId === undefined) {
      throw new Error('Error message rule editor ID is missing')
    }
    const matchIds = matchEditorIds.current[editorId] ?? []
    while (matchIds.length < rule.matches.length) {
      matchIds.push(`${editorId}-match-${nextEditorId.current++}`)
    }
    if (matchIds.length > rule.matches.length) {
      matchIds.splice(rule.matches.length)
    }
    matchEditorIds.current[editorId] = matchIds
    const matches = rule.matches.map((match, matchIndex) => {
      const matchEditorId = matchIds[matchIndex]
      if (matchEditorId === undefined) {
        throw new Error('Error message match editor ID is missing')
      }
      return { match, matchEditorId, matchIndex }
    })
    return { editorId, index, matches, rule }
  })

  useEffect(() => {
    ruleEditorIds.current = initialRules.map(
      () => `error-rule-${nextEditorId.current++}`
    )
    matchEditorIds.current = {}
    setRules(initialRules)
    setExpandedRules({})
  }, [initialRules])

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

  const updateMatch = (
    ruleIndex: number,
    matchIndex: number,
    value: string
  ) => {
    const matches = [...rules[ruleIndex].matches]
    matches[matchIndex] = value
    updateRule(ruleIndex, { matches })
  }

  const addMatch = (ruleIndex: number, afterIndex?: number) => {
    const matches = [...rules[ruleIndex].matches]
    if (matches.length >= MAX_ERROR_MESSAGE_MATCHES_PER_RULE) return
    const insertAt = afterIndex === undefined ? matches.length : afterIndex + 1
    const ruleEditorId = ruleEditorIds.current[ruleIndex]
    if (ruleEditorId === undefined) {
      throw new Error('Error message rule editor ID is missing')
    }
    const matchIds = matchEditorIds.current[ruleEditorId] ?? []
    matchIds.splice(
      insertAt,
      0,
      `${ruleEditorId}-match-${nextEditorId.current++}`
    )
    matchEditorIds.current[ruleEditorId] = matchIds
    matches.splice(insertAt, 0, '')
    pendingMatchFocus.current = `${ruleIndex}-${insertAt}`
    updateRule(ruleIndex, { matches })
  }

  useEffect(() => {
    if (pendingMatchFocus.current === null) return
    const input = matchInputRefs.current[pendingMatchFocus.current]
    pendingMatchFocus.current = null
    input?.focus()
  }, [rules])

  const removeMatch = (ruleIndex: number, matchIndex: number) => {
    const matches = rules[ruleIndex].matches.filter(
      (_, currentIndex) => currentIndex !== matchIndex
    )
    const ruleEditorId = ruleEditorIds.current[ruleIndex]
    if (ruleEditorId !== undefined && matches.length > 0) {
      matchEditorIds.current[ruleEditorId]?.splice(matchIndex, 1)
    }
    updateRule(ruleIndex, { matches: matches.length > 0 ? matches : [''] })
  }

  const removeRule = (ruleIndex: number) => {
    const removedEditorId = ruleEditorIds.current.splice(ruleIndex, 1)[0]
    if (removedEditorId !== undefined) {
      delete matchEditorIds.current[removedEditorId]
    }
    setRules((current) =>
      current.filter((_, currentIndex) => currentIndex !== ruleIndex)
    )
    setExpandedRules({})
  }

  const addRule = () => {
    const nextIndex = rules.length
    const editorId = `error-rule-${nextEditorId.current++}`
    ruleEditorIds.current.push(editorId)
    matchEditorIds.current[editorId] = [
      `${editorId}-match-${nextEditorId.current++}`,
    ]
    setRules((current) => [...current, createErrorMessageReplacementRule()])
    setExpandedRules((expanded) => ({ ...expanded, [nextIndex]: true }))
  }

  const save = async () => {
    if (!validateErrorMessageReplacementRules(rules)) {
      toast.error(
        t(
          'Every rule needs at least one non-empty match value and a replacement message. Original status codes must be between 100 and 599, and replacement status codes must be between 400 and 599.'
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
          'Rules are checked in order. Match values within one rule use OR logic, while an optional original status code is combined with them using AND logic, then both the client status code and message can be replaced. Retries, channel disabling, security audit, and internal metrics still use upstream errors; error logs show the replacement first and keep the upstream original in expanded details.'
        )}
      </p>
      <div className='flex flex-col gap-3'>
        {ruleRows.map(({ editorId, index, matches, rule }) => (
          <Collapsible
            key={editorId}
            open={expandedRules[index] === true}
            onOpenChange={(open) =>
              setExpandedRules((current) => ({ ...current, [index]: open }))
            }
            className='border-border overflow-hidden rounded-md border'
          >
            <div className='flex items-center gap-2 p-2'>
              <CollapsibleTrigger className='hover:bg-muted flex min-w-0 flex-1 items-center gap-2 rounded-md px-2 py-1.5 text-left'>
                <HugeiconsIcon
                  icon={
                    expandedRules[index] === true
                      ? ArrowDown01Icon
                      : ArrowRight01Icon
                  }
                  className='size-4 shrink-0'
                  strokeWidth={2}
                />
                <span className='min-w-0 truncate text-sm font-medium'>
                  {t('Rule {{number}}', { number: index + 1 })}
                </span>
                <span className='text-muted-foreground min-w-0 truncate text-sm'>
                  {t(MODE_LABELS[rule.mode])} ·{' '}
                  {t('{{count}} / {{max}} match values', {
                    count: rule.matches.length,
                    max: MAX_ERROR_MESSAGE_MATCHES_PER_RULE,
                  })}
                </span>
              </CollapsibleTrigger>
              <Button
                type='button'
                variant='destructive'
                size='icon-sm'
                aria-label={t('Delete rule')}
                title={t('Delete rule')}
                onClick={() => removeRule(index)}
              >
                <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} />
              </Button>
            </div>
            <CollapsibleContent>
              <div className='border-border flex flex-col gap-5 border-t p-4'>
                <FieldGroup className='grid gap-4 md:grid-cols-3'>
                  <Field>
                    <FieldLabel htmlFor={`error-rule-${index}-status`}>
                      {t('Original status code (optional)')}
                    </FieldLabel>
                    <Input
                      id={`error-rule-${index}-status`}
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
                  </Field>
                  <Field>
                    <FieldLabel>{t('Match mode')}</FieldLabel>
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
                  </Field>
                  <Field>
                    <FieldLabel htmlFor={`error-rule-${index}-new-status`}>
                      {t('New status code (optional)')}
                    </FieldLabel>
                    <Input
                      id={`error-rule-${index}-new-status`}
                      type='number'
                      min={400}
                      max={599}
                      step={1}
                      value={rule.replaceStatusCode ?? ''}
                      placeholder='429'
                      onChange={(event) =>
                        updateRule(index, {
                          replaceStatusCode: parseStatusCodeInput(
                            event.target.value
                          ),
                        })
                      }
                    />
                  </Field>
                </FieldGroup>

                <FieldSet>
                  <FieldLegend variant='label'>{t('Match values')}</FieldLegend>
                  <FieldGroup className='gap-3'>
                    {matches.map(({ match, matchEditorId, matchIndex }) => (
                      <Field
                        key={matchEditorId}
                        orientation='horizontal'
                        className='items-start'
                      >
                        <Textarea
                          ref={(element) => {
                            matchInputRefs.current[`${index}-${matchIndex}`] =
                              element
                          }}
                          value={match}
                          rows={2}
                          maxLength={4096}
                          aria-label={t('Match value')}
                          placeholder={t(
                            'Text from the original error message'
                          )}
                          onChange={(event) =>
                            updateMatch(index, matchIndex, event.target.value)
                          }
                          onKeyDown={(event) => {
                            if (
                              event.key === 'Enter' &&
                              !event.shiftKey &&
                              match.trim()
                            ) {
                              event.preventDefault()
                              addMatch(index, matchIndex)
                            }
                          }}
                        />
                        <Button
                          type='button'
                          variant='ghost'
                          size='icon'
                          aria-label={t('Remove match value')}
                          title={t('Remove match value')}
                          onClick={() => removeMatch(index, matchIndex)}
                        >
                          <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} />
                        </Button>
                      </Field>
                    ))}
                  </FieldGroup>
                  <div className='flex flex-wrap items-center justify-between gap-2'>
                    <FieldDescription>
                      {t('{{count}} / {{max}} match values', {
                        count: rule.matches.length,
                        max: MAX_ERROR_MESSAGE_MATCHES_PER_RULE,
                      })}
                    </FieldDescription>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      disabled={
                        rule.matches.length >=
                        MAX_ERROR_MESSAGE_MATCHES_PER_RULE
                      }
                      onClick={() => addMatch(index)}
                    >
                      <HugeiconsIcon
                        icon={Add01Icon}
                        data-icon='inline-start'
                        strokeWidth={2}
                      />
                      {t('Add match value')}
                    </Button>
                  </div>
                </FieldSet>

                <Field>
                  <FieldLabel htmlFor={`error-rule-${index}-replacement`}>
                    {t('Replace with')}
                  </FieldLabel>
                  <Textarea
                    id={`error-rule-${index}-replacement`}
                    value={rule.replace}
                    rows={2}
                    maxLength={4096}
                    placeholder={t('Message returned to the client')}
                    onChange={(event) =>
                      updateRule(index, { replace: event.target.value })
                    }
                  />
                </Field>
              </div>
            </CollapsibleContent>
          </Collapsible>
        ))}
        {rules.length === 0 && (
          <Empty className='border-border border'>
            <EmptyHeader>
              <EmptyTitle>{t('No replacement rules configured')}</EmptyTitle>
              <EmptyDescription>
                {t('Add a rule to replace matching client error messages.')}
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        )}
      </div>
      <Button
        type='button'
        variant='outline'
        className='w-fit'
        disabled={rules.length >= MAX_ERROR_MESSAGE_REPLACEMENT_RULES}
        onClick={addRule}
      >
        <HugeiconsIcon
          icon={Add01Icon}
          data-icon='inline-start'
          strokeWidth={2}
        />
        {t('Add rule')}
      </Button>
    </SettingsSection>
  )
}
