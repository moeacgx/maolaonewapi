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
import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronDown, ChevronRight, Plus, RotateCw, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
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
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { MultiSelect } from '@/components/multi-select'
import { getSensitiveRuleChannels, getSensitiveRuleGroups } from '../api'
import {
  SettingsForm,
  SettingsSwitchField,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import type { SensitiveRuleChannel } from '../types'
import {
  ACTION_BLOCK,
  ACTION_MASK,
  createSensitiveRuleDraft,
  DEFAULT_REPLACEMENT,
  expandInvalidSensitiveRule,
  getEmptySensitiveRuleTarget,
  getInitialSensitiveRuleExpansion,
  includeMissingSensitiveRouteOptions,
  includeMissingSensitiveGroupOptions,
  normalizeSensitiveGroupCodes,
  normalizeSensitiveRouteIds,
  parseSensitiveRuleChannelIds,
  parseSensitiveRulesConfig,
  removeSensitiveRuleExpansion,
  SCOPE_BOTH,
  SCOPE_REQUEST,
  SCOPE_RESPONSE,
  serializeSensitiveRules,
  TARGET_ALL,
  TARGET_CHANNEL_TAGS,
  TARGET_ROUTES,
  toggleSensitiveRuleExpansion,
  type SensitiveRuleDraft,
} from './sensitive-rule-config'

export type SensitiveFormValues = {
  CheckSensitiveEnabled: boolean
  CheckSensitiveOnPromptEnabled: boolean
  SensitiveWords?: string
  SensitiveRules?: string
  SensitiveRuleChannelIds?: string
}

type SensitiveWordsSectionProps = {
  defaultValues: SensitiveFormValues
  inlineActions?: boolean
  hideTitle?: boolean
  externalDirty?: boolean
  externalInvalid?: boolean
  isSaving?: boolean
  onSaveValues?: (values: SensitiveFormValues) => Promise<void>
  onResetExternal?: () => void
}

function getChannelLabel(channel: SensitiveRuleChannel) {
  const name = channel.name?.trim()
  const channelLabel = name ? `${name} #${channel.id}` : `#${channel.id}`
  const tag = channel.tag?.trim()
  return tag ? `${channelLabel} · ${tag}` : channelLabel
}

export function SensitiveWordsSection({
  defaultValues,
  inlineActions = false,
  hideTitle = false,
  externalDirty = false,
  externalInvalid = false,
  isSaving: externalSaving = false,
  onSaveValues,
  onResetExternal,
}: SensitiveWordsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const channelsQuery = useQuery({
    queryKey: ['security-audit', 'builtin-policy', 'channels'],
    queryFn: getSensitiveRuleChannels,
  })
  const groupsQuery = useQuery({
    queryKey: ['security-audit', 'builtin-policy', 'groups'],
    queryFn: getSensitiveRuleGroups,
  })
  const channels = useMemo(() => {
    return [...(channelsQuery.data?.data ?? [])]
      .filter((channel) => Number.isInteger(channel.id) && channel.id > 0)
      .sort((a, b) => {
        const nameCompare = getChannelLabel(a).localeCompare(getChannelLabel(b))
        return nameCompare === 0 ? a.id - b.id : nameCompare
      })
  }, [channelsQuery.data?.data])
  const channelOptions = useMemo(
    () =>
      channels.map((channel) => ({
        value: String(channel.id),
        label: getChannelLabel(channel),
      })),
    [channels]
  )
  const groupOptions = useMemo(
    () =>
      [...(groupsQuery.data?.data ?? [])]
        .filter((group) => group.id > 0 && group.code.trim().length > 0)
        .sort((a, b) => a.name.localeCompare(b.name))
        .map((group) => ({
          value: group.code,
          label: `${group.name || group.code} #${group.id}`,
        })),
    [groupsQuery.data?.data]
  )
  const [filterEnabled, setFilterEnabled] = useState(
    defaultValues.CheckSensitiveEnabled
  )
  const [promptEnabled, setPromptEnabled] = useState(
    defaultValues.CheckSensitiveOnPromptEnabled
  )
  const legacyChannelIds = useMemo(
    () => parseSensitiveRuleChannelIds(defaultValues.SensitiveRuleChannelIds),
    [defaultValues.SensitiveRuleChannelIds]
  )
  const [rules, setRules] = useState<SensitiveRuleDraft[]>(() =>
    parseSensitiveRulesConfig(
      defaultValues.SensitiveRules,
      defaultValues.SensitiveWords,
      parseSensitiveRuleChannelIds(defaultValues.SensitiveRuleChannelIds)
    )
  )
  const [expandedRules, setExpandedRules] = useState(() =>
    getInitialSensitiveRuleExpansion(rules)
  )

  const initialRulesValue = useMemo(
    () =>
      serializeSensitiveRules(
        parseSensitiveRulesConfig(
          defaultValues.SensitiveRules,
          defaultValues.SensitiveWords,
          legacyChannelIds
        )
      ),
    [
      defaultValues.SensitiveRules,
      defaultValues.SensitiveWords,
      legacyChannelIds,
    ]
  )
  const currentRulesValue = useMemo(
    () => serializeSensitiveRules(rules),
    [rules]
  )
  const invalidTargetsByRuleId = useMemo(
    () =>
      new Map(
        rules
          .map((rule) => [rule.id, getEmptySensitiveRuleTarget(rule)] as const)
          .filter((entry) => entry[1] !== null)
      ),
    [rules]
  )
  const hasInvalidTargets = invalidTargetsByRuleId.size > 0
  const hasChanges =
    externalDirty ||
    filterEnabled !== defaultValues.CheckSensitiveEnabled ||
    promptEnabled !== defaultValues.CheckSensitiveOnPromptEnabled ||
    currentRulesValue !== initialRulesValue

  const updateRule = (id: string, patch: Partial<SensitiveRuleDraft>) => {
    const currentRule = rules.find((rule) => rule.id === id)
    if (currentRule) {
      const nextRule = { ...currentRule, ...patch }
      setExpandedRules((current) =>
        expandInvalidSensitiveRule(current, nextRule)
      )
    }
    setRules((prev) =>
      prev.map((rule) => (rule.id === id ? { ...rule, ...patch } : rule))
    )
  }

  const addRule = () => {
    const rule = createSensitiveRuleDraft()
    setRules((prev) => [...prev, rule])
    setExpandedRules((current) => ({ ...current, [rule.id]: true }))
  }

  const toggleRule = (id: string) => {
    setExpandedRules((current) => toggleSensitiveRuleExpansion(current, id))
  }

  const deleteRule = (id: string) => {
    setRules((prev) => prev.filter((item) => item.id !== id))
    setExpandedRules((current) => removeSensitiveRuleExpansion(current, id))
  }

  const isRuleExpanded = (id: string) => expandedRules[id] === true

  const getRuleSummary = (rule: SensitiveRuleDraft) => {
    const action = t(rule.action === ACTION_BLOCK ? 'Block' : 'Mask')
    const scope = t(
      rule.scope === SCOPE_RESPONSE
        ? 'Response'
        : rule.scope === SCOPE_BOTH
          ? 'Both'
          : 'Request'
    )
    const target =
      rule.targetType === TARGET_ALL
        ? t('All channels')
        : rule.targetType === TARGET_CHANNEL_TAGS
          ? `${t('Legacy channel groups')} (${rule.channelTags.length})`
          : `${t('Specified channels')} (${rule.channelIds.length}) · ${t('Specified groups')} (${rule.groupCodes.length})`
    return `${action} · ${scope} · ${target}`
  }

  const onSubmit = async () => {
    if (hasInvalidTargets || externalInvalid) return

    if (onSaveValues) {
      await onSaveValues({
        CheckSensitiveEnabled: filterEnabled,
        CheckSensitiveOnPromptEnabled: promptEnabled,
        SensitiveWords: defaultValues.SensitiveWords,
        SensitiveRules: currentRulesValue,
        SensitiveRuleChannelIds: defaultValues.SensitiveRuleChannelIds || '[]',
      })
      return
    }

    const updates: Array<{ key: string; value: string | boolean }> = []
    if (filterEnabled !== defaultValues.CheckSensitiveEnabled) {
      updates.push({ key: 'CheckSensitiveEnabled', value: filterEnabled })
    }
    if (promptEnabled !== defaultValues.CheckSensitiveOnPromptEnabled) {
      updates.push({
        key: 'CheckSensitiveOnPromptEnabled',
        value: promptEnabled,
      })
    }
    if (currentRulesValue !== initialRulesValue) {
      updates.push({ key: 'SensitiveRules', value: currentRulesValue })
      if ((defaultValues.SensitiveWords ?? '').trim() !== '') {
        updates.push({ key: 'SensitiveWords', value: '' })
      }
    }
    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }
  }

  const onReset = () => {
    setFilterEnabled(defaultValues.CheckSensitiveEnabled)
    setPromptEnabled(defaultValues.CheckSensitiveOnPromptEnabled)
    const nextRules = parseSensitiveRulesConfig(
      defaultValues.SensitiveRules,
      defaultValues.SensitiveWords,
      legacyChannelIds
    )
    setRules(nextRules)
    setExpandedRules(getInitialSensitiveRuleExpansion(nextRules))
    onResetExternal?.()
  }

  const isSaving = externalSaving || updateOption.isPending

  return (
    <SettingsSection
      title={t('Sensitive Words')}
      titleProps={hideTitle ? { className: 'sr-only' } : undefined}
    >
      <SettingsForm
        onSubmit={(event) => {
          event.preventDefault()
          void onSubmit()
        }}
      >
        {inlineActions ? (
          <div
            data-settings-form-span='full'
            className='flex flex-wrap items-center justify-end gap-2'
          >
            <Button
              type='button'
              size='sm'
              variant='outline'
              onClick={onReset}
              disabled={!hasChanges || isSaving}
            >
              {t('Reset')}
            </Button>
            <Button
              type='submit'
              size='sm'
              disabled={
                !hasChanges || isSaving || hasInvalidTargets || externalInvalid
              }
            >
              {isSaving ? <Spinner data-icon='inline-start' /> : null}
              {t(isSaving ? 'Saving...' : 'Save sensitive rules')}
            </Button>
          </div>
        ) : (
          <SettingsPageFormActions
            onSave={() => void onSubmit()}
            onReset={onReset}
            isSaving={isSaving}
            isSaveDisabled={!hasChanges || hasInvalidTargets || externalInvalid}
            isResetDisabled={!hasChanges}
            saveLabel='Save sensitive rules'
          />
        )}

        <div data-settings-form-span='full' className='space-y-4'>
          <SettingsSwitchField
            checked={filterEnabled}
            onCheckedChange={setFilterEnabled}
            label={t('Enable filtering')}
            description={t(
              'Inspect request text before it reaches upstream models.'
            )}
          />
          <SettingsSwitchField
            checked={promptEnabled}
            onCheckedChange={setPromptEnabled}
            label={t('Inspect user prompts')}
            description={t(
              'Rules are applied to prompt-like text fields in supported request formats.'
            )}
          />
        </div>

        <div data-settings-form-span='full' className='space-y-3'>
          <div className='flex flex-wrap items-center justify-between gap-3'>
            <div className='min-w-0'>
              <h3 className='text-sm font-medium'>{t('Filter rules')}</h3>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Each rule can mask or block matching request or response text.'
                )}
              </p>
            </div>
            <Button type='button' variant='outline' size='sm' onClick={addRule}>
              <Plus data-icon='inline-start' />
              <span>{t('Add rule')}</span>
            </Button>
          </div>

          {rules.length === 0 ? (
            <div className='text-muted-foreground rounded-lg border border-dashed p-4 text-sm'>
              {t('No sensitive rules configured.')}
            </div>
          ) : (
            <div className='space-y-3'>
              {rules.map((rule, index) => (
                <div
                  key={rule.id}
                  className='bg-card text-card-foreground rounded-lg border'
                >
                  <div className='flex items-center justify-between gap-3 p-3'>
                    <button
                      type='button'
                      className='flex min-w-0 flex-1 flex-wrap items-center gap-2 text-left'
                      aria-expanded={isRuleExpanded(rule.id)}
                      aria-controls={`sensitive-rule-${index}`}
                      onClick={() => toggleRule(rule.id)}
                    >
                      {isRuleExpanded(rule.id) ? (
                        <ChevronDown className='size-4 shrink-0' />
                      ) : (
                        <ChevronRight className='size-4 shrink-0' />
                      )}
                      <Badge variant={rule.enabled ? 'default' : 'outline'}>
                        {rule.enabled ? t('Enabled') : t('Disabled')}
                      </Badge>
                      <span className='min-w-0 flex-1 truncate text-sm font-medium'>
                        {rule.name ||
                          t('Rule {{number}}', { number: index + 1 })}
                      </span>
                      {invalidTargetsByRuleId.has(rule.id) ? (
                        <Badge variant='destructive'>{t('Error')}</Badge>
                      ) : null}
                      <span className='text-muted-foreground basis-full truncate pl-6 text-xs sm:basis-auto sm:pl-0'>
                        {getRuleSummary(rule)}
                      </span>
                    </button>
                    <div className='flex shrink-0 items-center gap-2'>
                      <Switch
                        checked={rule.enabled}
                        onCheckedChange={(enabled) =>
                          updateRule(rule.id, { enabled })
                        }
                        aria-label={t('Enable rule')}
                      />
                      <Button
                        type='button'
                        variant='ghost'
                        size='icon-sm'
                        aria-label={t('Delete rule')}
                        onClick={() => deleteRule(rule.id)}
                      >
                        <Trash2 />
                      </Button>
                    </div>
                  </div>

                  {isRuleExpanded(rule.id) ? (
                    <div
                      id={`sensitive-rule-${index}`}
                      role='region'
                      aria-label={
                        rule.name || t('Rule {{number}}', { number: index + 1 })
                      }
                      className='space-y-3 border-t px-3 pt-3 pb-3'
                    >
                      <div className='grid gap-3 md:grid-cols-[minmax(0,1fr)_150px_150px]'>
                        <div className='space-y-1.5'>
                          <Label htmlFor={`${rule.id}-name`}>
                            {t('Rule name')}
                          </Label>
                          <Input
                            id={`${rule.id}-name`}
                            value={rule.name}
                            placeholder={t('Rule name')}
                            onChange={(event) =>
                              updateRule(rule.id, { name: event.target.value })
                            }
                          />
                        </div>
                        <div className='space-y-1.5'>
                          <Label>{t('Action')}</Label>
                          <Select
                            value={rule.action}
                            onValueChange={(value) => {
                              if (
                                value !== ACTION_MASK &&
                                value !== ACTION_BLOCK
                              ) {
                                return
                              }
                              updateRule(rule.id, {
                                action: value,
                                replacement:
                                  value === ACTION_MASK
                                    ? rule.replacement || DEFAULT_REPLACEMENT
                                    : rule.replacement,
                              })
                            }}
                          >
                            <SelectTrigger className='w-full'>
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent alignItemWithTrigger={false}>
                              <SelectGroup>
                                <SelectItem value={ACTION_MASK}>
                                  {t('Mask')}
                                </SelectItem>
                                <SelectItem value={ACTION_BLOCK}>
                                  {t('Block')}
                                </SelectItem>
                              </SelectGroup>
                            </SelectContent>
                          </Select>
                        </div>
                        <div className='space-y-1.5'>
                          <Label>{t('Scope')}</Label>
                          <Select
                            value={rule.scope}
                            onValueChange={(value) => {
                              if (
                                value !== SCOPE_REQUEST &&
                                value !== SCOPE_RESPONSE &&
                                value !== SCOPE_BOTH
                              ) {
                                return
                              }
                              updateRule(rule.id, { scope: value })
                            }}
                          >
                            <SelectTrigger className='w-full'>
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent alignItemWithTrigger={false}>
                              <SelectGroup>
                                <SelectItem value={SCOPE_REQUEST}>
                                  {t('Request')}
                                </SelectItem>
                                <SelectItem value={SCOPE_RESPONSE}>
                                  {t('Response')}
                                </SelectItem>
                                <SelectItem value={SCOPE_BOTH}>
                                  {t('Both')}
                                </SelectItem>
                              </SelectGroup>
                            </SelectContent>
                          </Select>
                        </div>
                      </div>

                      {rule.action === ACTION_MASK ? (
                        <div className='space-y-1.5'>
                          <Label htmlFor={`${rule.id}-replacement`}>
                            {t('Replacement text')}
                          </Label>
                          <Input
                            id={`${rule.id}-replacement`}
                            value={rule.replacement ?? ''}
                            placeholder={DEFAULT_REPLACEMENT}
                            onChange={(event) =>
                              updateRule(rule.id, {
                                replacement: event.target.value,
                              })
                            }
                          />
                        </div>
                      ) : null}

                      <div className='flex flex-col gap-3'>
                        <div className='flex flex-col gap-1.5'>
                          <Label>{t('Applied scope')}</Label>
                          <ToggleGroup
                            value={[rule.targetType]}
                            onValueChange={(targetTypes) => {
                              const targetType = targetTypes[0]
                              if (
                                targetType !== TARGET_ROUTES &&
                                targetType !== TARGET_ALL
                              ) {
                                return
                              }
                              updateRule(rule.id, { targetType })
                            }}
                            variant='outline'
                            size='sm'
                            aria-label={t('Applied scope')}
                            className='w-full sm:w-fit'
                          >
                            <ToggleGroupItem
                              value={TARGET_ALL}
                              className='min-w-0 flex-1 sm:flex-none'
                            >
                              {t('All channels')}
                            </ToggleGroupItem>
                            <ToggleGroupItem
                              value={TARGET_ROUTES}
                              className='min-w-0 flex-1 sm:flex-none'
                            >
                              {t('Specified scope')}
                            </ToggleGroupItem>
                          </ToggleGroup>
                        </div>

                        {rule.targetType === TARGET_ALL ? (
                          <p className='text-muted-foreground text-xs'>
                            {t('This rule runs for every channel.')}
                          </p>
                        ) : rule.targetType === TARGET_CHANNEL_TAGS ? (
                          <div className='flex flex-col gap-1.5'>
                            <Label>{t('Legacy channel groups')}</Label>
                            <p className='text-muted-foreground text-xs'>
                              {t(
                                'This legacy rule keeps its existing channel-group scope until you switch to a specified scope.'
                              )}
                            </p>
                          </div>
                        ) : (
                          <div className='grid gap-3 lg:grid-cols-2'>
                            <div className='flex flex-col gap-1.5'>
                              <Label htmlFor={`${rule.id}-channel-ids`}>
                                {t('Applied channels')}
                              </Label>
                              <MultiSelect
                                id={`${rule.id}-channel-ids`}
                                options={includeMissingSensitiveRouteOptions(
                                  channelOptions,
                                  rule.channelIds,
                                  t('Unavailable channel')
                                )}
                                selected={rule.channelIds.map(String)}
                                onChange={(channelIds) =>
                                  updateRule(rule.id, {
                                    channelIds:
                                      normalizeSensitiveRouteIds(channelIds),
                                  })
                                }
                                placeholder={t('Select channels...')}
                                emptyText={t('No channels available.')}
                                disabled={
                                  channelsQuery.isLoading ||
                                  channelsQuery.isError
                                }
                                maxVisibleChips={3}
                              />
                              {channelsQuery.isError ? (
                                <div className='text-destructive flex flex-wrap items-center gap-2 text-xs'>
                                  <span>{t('Unable to load channels')}</span>
                                  <Button
                                    type='button'
                                    variant='ghost'
                                    size='sm'
                                    onClick={() => void channelsQuery.refetch()}
                                  >
                                    <RotateCw data-icon='inline-start' />
                                    {t('Retry')}
                                  </Button>
                                </div>
                              ) : null}
                            </div>
                            <div className='flex flex-col gap-1.5'>
                              <Label htmlFor={`${rule.id}-group-codes`}>
                                {t('Applied groups')}
                              </Label>
                              <MultiSelect
                                id={`${rule.id}-group-codes`}
                                options={includeMissingSensitiveGroupOptions(
                                  groupOptions,
                                  rule.groupCodes,
                                  t('Unavailable group')
                                )}
                                selected={rule.groupCodes}
                                onChange={(groupCodes) =>
                                  updateRule(rule.id, {
                                    groupCodes:
                                      normalizeSensitiveGroupCodes(groupCodes),
                                  })
                                }
                                placeholder={t('Select groups...')}
                                emptyText={t('No groups available.')}
                                disabled={
                                  groupsQuery.isLoading || groupsQuery.isError
                                }
                                maxVisibleChips={3}
                              />
                              {groupsQuery.isError ? (
                                <div className='text-destructive flex flex-wrap items-center gap-2 text-xs'>
                                  <span>{t('Unable to load groups')}</span>
                                  <Button
                                    type='button'
                                    variant='ghost'
                                    size='sm'
                                    onClick={() => void groupsQuery.refetch()}
                                  >
                                    <RotateCw data-icon='inline-start' />
                                    {t('Retry')}
                                  </Button>
                                </div>
                              ) : null}
                            </div>
                            {invalidTargetsByRuleId.get(rule.id) ===
                            TARGET_ROUTES ? (
                              <p className='text-destructive text-xs lg:col-span-2'>
                                {t(
                                  'Choose at least one channel or group for an enabled rule.'
                                )}
                              </p>
                            ) : (
                              <p className='text-muted-foreground text-xs lg:col-span-2'>
                                {t(
                                  'This rule runs when any selected channel or group is used.'
                                )}
                              </p>
                            )}
                          </div>
                        )}
                      </div>

                      <div className='space-y-1.5'>
                        <Label htmlFor={`${rule.id}-keywords`}>
                          {t('Keywords')}
                        </Label>
                        <Textarea
                          id={`${rule.id}-keywords`}
                          rows={5}
                          value={rule.keywordsText}
                          placeholder={t('Enter one keyword per line')}
                          onChange={(event) =>
                            updateRule(rule.id, {
                              keywordsText: event.target.value,
                            })
                          }
                        />
                        <p className='text-muted-foreground text-xs'>
                          {t('Empty lines and duplicate keywords are ignored.')}{' '}
                          {t(
                            'Latin keywords match complete word boundaries; Chinese keywords keep substring matching.'
                          )}
                        </p>
                      </div>
                    </div>
                  ) : null}
                </div>
              ))}
            </div>
          )}
        </div>
      </SettingsForm>
    </SettingsSection>
  )
}
