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
import { useCallback, useMemo, useState, type SetStateAction } from 'react'
import axios from 'axios'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { RotateCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { MultiSelect } from '@/components/multi-select'
import {
  getSensitiveRuleChannels,
  getSensitiveRuleGroups,
} from '@/features/system-settings/api'
import {
  includeMissingSensitiveGroupOptions,
  includeMissingSensitiveRouteOptions,
  normalizeSensitiveGroupCodes,
  normalizeSensitiveRouteIds,
} from '@/features/system-settings/request-limits/sensitive-rule-config'
import {
  SensitiveWordsSection,
  type SensitiveFormValues,
} from '@/features/system-settings/request-limits/sensitive-words-section'
import type { SensitiveRuleChannel } from '@/features/system-settings/types'
import {
  getSecurityAuditBuiltinPolicy,
  updateSecurityAuditBuiltinPolicy,
} from './api'
import {
  getBuiltinPolicyScopeValidationError,
  normalizeBuiltinPolicyScope,
  setBuiltinPolicyTargetType,
} from './builtin-policy-scope'
import type { SecurityAuditBuiltinPolicy } from './types'

type BuiltinPolicyViewProps = {
  onSaved: (policy: SecurityAuditBuiltinPolicy) => void
}

function getChannelLabel(channel: SensitiveRuleChannel) {
  const name = channel.name?.trim()
  const label = name ? `${name} #${channel.id}` : `#${channel.id}`
  const tag = channel.tag?.trim()
  return tag ? `${label} · ${tag}` : label
}

export function SecurityAuditBuiltinPolicyView({
  onSaved,
}: BuiltinPolicyViewProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [draftOverride, setDraftOverride] =
    useState<SecurityAuditBuiltinPolicy | null>(null)
  const [saving, setSaving] = useState(false)

  const policyQuery = useQuery({
    queryKey: ['security-audit', 'builtin-policy'],
    queryFn: getSecurityAuditBuiltinPolicy,
    staleTime: 15_000,
  })
  const channelsQuery = useQuery({
    queryKey: ['security-audit', 'builtin-policy', 'channels'],
    queryFn: getSensitiveRuleChannels,
  })
  const groupsQuery = useQuery({
    queryKey: ['security-audit', 'builtin-policy', 'groups'],
    queryFn: getSensitiveRuleGroups,
  })
  const channelOptions = useMemo(
    () =>
      [...(channelsQuery.data?.data ?? [])]
        .filter((channel) => Number.isInteger(channel.id) && channel.id > 0)
        .sort((left, right) => {
          const nameCompare = getChannelLabel(left).localeCompare(
            getChannelLabel(right)
          )
          return nameCompare === 0 ? left.id - right.id : nameCompare
        })
        .map((channel) => ({
          value: String(channel.id),
          label: getChannelLabel(channel),
        })),
    [channelsQuery.data?.data]
  )
  const groupOptions = useMemo(
    () =>
      [...(groupsQuery.data?.data ?? [])]
        .filter((group) => group.id > 0 && group.code.trim().length > 0)
        .sort((left, right) => left.name.localeCompare(right.name))
        .map((group) => ({
          value: group.code,
          label: `${group.name || group.code} #${group.id}`,
        })),
    [groupsQuery.data?.data]
  )

  const draft =
    draftOverride &&
    policyQuery.data &&
    draftOverride.config_version === policyQuery.data.config_version
      ? draftOverride
      : (policyQuery.data ?? null)
  const draftScope = draft ? normalizeBuiltinPolicyScope(draft) : null
  const baselineScope = policyQuery.data
    ? normalizeBuiltinPolicyScope(policyQuery.data)
    : null
  const scopeValidationError = draftScope
    ? getBuiltinPolicyScopeValidationError(draftScope)
    : null

  const setDraft = useCallback(
    (next: SetStateAction<SecurityAuditBuiltinPolicy | null>) => {
      setDraftOverride((currentOverride) => {
        const current =
          currentOverride &&
          policyQuery.data &&
          currentOverride.config_version === policyQuery.data.config_version
            ? currentOverride
            : (policyQuery.data ?? null)
        return typeof next === 'function' ? next(current) : next
      })
    },
    [policyQuery.data]
  )

  const policySwitchesDirty = Boolean(
    draft &&
    policyQuery.data &&
    (draft.upstream_policy_enabled !==
      policyQuery.data.upstream_policy_enabled ||
      draft.cyber_policy_conversation_block_enabled !==
        policyQuery.data.cyber_policy_conversation_block_enabled ||
      draft.sensitive_word_audit_enabled !==
        policyQuery.data.sensitive_word_audit_enabled ||
      draft.cyber_policy_auto_ban_enabled !==
        policyQuery.data.cyber_policy_auto_ban_enabled ||
      JSON.stringify(draft.cyber_policy_auto_ban_exempt_group_codes ?? []) !==
        JSON.stringify(
          policyQuery.data.cyber_policy_auto_ban_exempt_group_codes ?? []
        ) ||
      draft.cyber_policy_ban_threshold !==
        policyQuery.data.cyber_policy_ban_threshold ||
      draft.cyber_policy_violation_window_hours !==
        policyQuery.data.cyber_policy_violation_window_hours ||
      JSON.stringify(draftScope) !== JSON.stringify(baselineScope))
  )

  const resetPolicySwitches = () => {
    if (policyQuery.data) setDraft(policyQuery.data)
  }

  const savePolicy = async (values: SensitiveFormValues) => {
    if (!draft) return
    const scope = normalizeBuiltinPolicyScope(draft)
    const scopeError = getBuiltinPolicyScopeValidationError(scope)
    if (scopeError === 'channels') {
      toast.error(t('Choose at least one channel for this scope.'))
      return
    }
    if (scopeError === 'groups') {
      toast.error(t('Choose at least one group for this scope.'))
      return
    }

    setSaving(true)
    try {
      const updated = await updateSecurityAuditBuiltinPolicy({
        expected_version: draft.config_version,
        upstream_policy_enabled: draft.upstream_policy_enabled,
        cyber_policy_conversation_block_enabled:
          draft.cyber_policy_conversation_block_enabled,
        ...scope,
        sensitive_word_audit_enabled: draft.sensitive_word_audit_enabled,
        cyber_policy_auto_ban_enabled: draft.cyber_policy_auto_ban_enabled,
        cyber_policy_auto_ban_exempt_group_codes:
          draft.cyber_policy_auto_ban_exempt_group_codes ?? [],
        cyber_policy_ban_threshold: draft.cyber_policy_ban_threshold,
        cyber_policy_violation_window_hours:
          draft.cyber_policy_violation_window_hours,
        check_sensitive_enabled: values.CheckSensitiveEnabled,
        check_sensitive_on_prompt_enabled: values.CheckSensitiveOnPromptEnabled,
        sensitive_rules: values.SensitiveRules || '{"rules":[]}',
        sensitive_rule_channel_ids: values.SensitiveRuleChannelIds || '[]',
      })
      queryClient.setQueryData(['security-audit', 'builtin-policy'], updated)
      setDraft(updated)
      onSaved(updated)
      await queryClient.invalidateQueries({
        queryKey: ['security-audit', 'runtime'],
      })
      toast.success(t('Built-in safety policy saved'))
    } catch (error) {
      if (axios.isAxiosError(error) && error.response?.status === 409) {
        await policyQuery.refetch()
        toast.error(
          t(
            'The configuration changed on the server. Latest values were reloaded; review and save again.'
          )
        )
        return
      }
      toast.error(t('Failed to save built-in safety policy'))
    } finally {
      setSaving(false)
    }
  }

  if (policyQuery.isError) {
    return (
      <Alert variant='destructive'>
        <AlertTitle>{t('Failed to load built-in safety policy')}</AlertTitle>
        <AlertDescription className='flex flex-wrap items-center gap-2'>
          <span>{t('Check your Root permissions and try again.')}</span>
          <Button
            variant='outline'
            size='sm'
            onClick={() => void policyQuery.refetch()}
          >
            {t('Retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  }

  if (!draft) {
    return (
      <div className='flex flex-col gap-4'>
        <Skeleton className='h-44 w-full rounded-xl' />
        <Skeleton className='h-96 w-full rounded-xl' />
      </div>
    )
  }

  const activeScope = normalizeBuiltinPolicyScope(draft)
  const sensitiveValues: SensitiveFormValues = {
    CheckSensitiveEnabled: draft.check_sensitive_enabled,
    CheckSensitiveOnPromptEnabled: draft.check_sensitive_on_prompt_enabled,
    SensitiveWords: draft.sensitive_words,
    SensitiveRules: draft.sensitive_rules,
    SensitiveRuleChannelIds: draft.sensitive_rule_channel_ids,
  }

  return (
    <div className='flex flex-col gap-4'>
      <Alert>
        <AlertTitle>{t('Guard nodes are optional')}</AlertTitle>
        <AlertDescription>
          {t(
            'Sensitive word filtering runs locally, and upstream policy events are recognized from exact cyber_policy error codes even when no Guard node is configured.'
          )}
        </AlertDescription>
      </Alert>

      <Card>
        <CardHeader>
          <CardTitle>{t('Built-in safety policy')}</CardTitle>
          <CardDescription>
            {t(
              'Choose which built-in detections are recorded in the unified audit event list.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <FieldGroup>
            <Field orientation='horizontal'>
              <FieldContent>
                <FieldLabel htmlFor='audit-upstream-policy-enabled'>
                  {t('Recognize upstream policy events')}
                </FieldLabel>
                <FieldDescription>
                  {t(
                    'Record exact cyber_policy rejections returned by HTTP, streaming, or Realtime upstreams.'
                  )}
                </FieldDescription>
              </FieldContent>
              <Switch
                id='audit-upstream-policy-enabled'
                checked={draft.upstream_policy_enabled}
                onCheckedChange={(upstreamPolicyEnabled) =>
                  setDraft((current) =>
                    current
                      ? {
                          ...current,
                          upstream_policy_enabled: upstreamPolicyEnabled,
                          cyber_policy_auto_ban_enabled:
                            upstreamPolicyEnabled &&
                            current.cyber_policy_auto_ban_enabled,
                        }
                      : current
                  )
                }
              />
            </Field>
            <div className='space-y-3 border-t pt-4'>
              <div>
                <Label>{t('Official risk control scope')}</Label>
                <p className='text-muted-foreground mt-1 text-xs'>
                  {t(
                    'Choose where official cyber_policy events are written to security audit. Detection still runs globally; this scope only controls audit records and automatic bans.'
                  )}
                </p>
              </div>
              <ToggleGroup
                value={[activeScope.upstream_policy_target_type]}
                onValueChange={(targetTypes) => {
                  const targetType = targetTypes[0]
                  if (
                    targetType !== 'all' &&
                    targetType !== 'channels' &&
                    targetType !== 'groups'
                  ) {
                    return
                  }
                  setDraft((current) =>
                    current
                      ? {
                          ...current,
                          ...setBuiltinPolicyTargetType(
                            normalizeBuiltinPolicyScope(current),
                            targetType
                          ),
                        }
                      : current
                  )
                }}
                variant='outline'
                size='sm'
                aria-label={t('Official risk control scope')}
                className='w-full sm:w-fit'
              >
                <ToggleGroupItem
                  value='all'
                  className='min-w-0 flex-1 sm:flex-none'
                >
                  {t('All channels')}
                </ToggleGroupItem>
                <ToggleGroupItem
                  value='channels'
                  className='min-w-0 flex-1 sm:flex-none'
                >
                  {t('Specified channels')}
                </ToggleGroupItem>
                <ToggleGroupItem
                  value='groups'
                  className='min-w-0 flex-1 sm:flex-none'
                >
                  {t('Specified groups')}
                </ToggleGroupItem>
              </ToggleGroup>

              {activeScope.upstream_policy_target_type === 'all' ? (
                <p className='text-muted-foreground text-xs'>
                  {t('Audit cyber_policy events from every channel.')}
                </p>
              ) : activeScope.upstream_policy_target_type === 'channels' ? (
                <div className='space-y-1.5'>
                  <Label htmlFor='audit-upstream-policy-channel-ids'>
                    {t('Applied channels')}
                  </Label>
                  <MultiSelect
                    id='audit-upstream-policy-channel-ids'
                    options={includeMissingSensitiveRouteOptions(
                      channelOptions,
                      activeScope.upstream_policy_channel_ids,
                      t('Unavailable channel')
                    )}
                    selected={activeScope.upstream_policy_channel_ids.map(
                      String
                    )}
                    onChange={(channelIds) =>
                      setDraft((current) =>
                        current
                          ? {
                              ...current,
                              upstream_policy_channel_ids:
                                normalizeSensitiveRouteIds(channelIds),
                            }
                          : current
                      )
                    }
                    placeholder={t('Select channels...')}
                    emptyText={t('No channels available.')}
                    disabled={channelsQuery.isLoading || channelsQuery.isError}
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
                  ) : scopeValidationError === 'channels' ? (
                    <p className='text-destructive text-xs'>
                      {t('Choose at least one channel for this scope.')}
                    </p>
                  ) : (
                    <p className='text-muted-foreground text-xs'>
                      {t(
                        'Audit cyber_policy events only when one of the selected channels is used.'
                      )}
                    </p>
                  )}
                </div>
              ) : (
                <div className='space-y-1.5'>
                  <Label htmlFor='audit-upstream-policy-group-codes'>
                    {t('Applied groups')}
                  </Label>
                  <MultiSelect
                    id='audit-upstream-policy-group-codes'
                    options={includeMissingSensitiveGroupOptions(
                      groupOptions,
                      activeScope.upstream_policy_group_codes,
                      t('Unavailable group')
                    )}
                    selected={activeScope.upstream_policy_group_codes}
                    onChange={(groupCodes) =>
                      setDraft((current) =>
                        current
                          ? {
                              ...current,
                              upstream_policy_group_codes:
                                normalizeSensitiveGroupCodes(groupCodes),
                            }
                          : current
                      )
                    }
                    placeholder={t('Select groups...')}
                    emptyText={t('No groups available.')}
                    disabled={groupsQuery.isLoading || groupsQuery.isError}
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
                  ) : scopeValidationError === 'groups' ? (
                    <p className='text-destructive text-xs'>
                      {t('Choose at least one group for this scope.')}
                    </p>
                  ) : (
                    <>
                      <p className='text-muted-foreground text-xs'>
                        {t(
                          'Audit cyber_policy events for channels assigned to any selected group.'
                        )}
                      </p>
                      <p className='text-muted-foreground text-xs'>
                        {t(
                          'Business groups use stable codes from group management, not channel tags.'
                        )}
                      </p>
                    </>
                  )}
                </div>
              )}
            </div>
            <Field orientation='horizontal'>
              <FieldContent>
                <FieldLabel htmlFor='audit-cyber-policy-conversation-block-enabled'>
                  {t(
                    'Block subsequent requests in the same cyber_policy conversation'
                  )}
                </FieldLabel>
                <FieldDescription>
                  {t(
                    'After an exact cyber_policy rejection, requests with the same user and stable conversation identifier are rejected before channel selection and billing. This switch is independent of automatic user bans.'
                  )}
                </FieldDescription>
              </FieldContent>
              <Switch
                id='audit-cyber-policy-conversation-block-enabled'
                checked={draft.cyber_policy_conversation_block_enabled}
                onCheckedChange={(cyberPolicyConversationBlockEnabled) =>
                  setDraft((current) =>
                    current
                      ? {
                          ...current,
                          cyber_policy_conversation_block_enabled:
                            cyberPolicyConversationBlockEnabled,
                        }
                      : current
                  )
                }
              />
            </Field>
            <Field orientation='horizontal'>
              <FieldContent>
                <FieldLabel htmlFor='audit-cyber-policy-auto-ban-enabled'>
                  {t(
                    'Automatically disable users after cyber_policy violations'
                  )}
                </FieldLabel>
                <FieldDescription>
                  {t(
                    'Only ordinary users are affected. Administrators and Root accounts are never disabled automatically.'
                  )}
                </FieldDescription>
              </FieldContent>
              <Switch
                id='audit-cyber-policy-auto-ban-enabled'
                checked={draft.cyber_policy_auto_ban_enabled}
                onCheckedChange={(cyberPolicyAutoBanEnabled) =>
                  setDraft((current) =>
                    current
                      ? {
                          ...current,
                          cyber_policy_auto_ban_enabled:
                            cyberPolicyAutoBanEnabled,
                          upstream_policy_enabled:
                            cyberPolicyAutoBanEnabled ||
                            current.upstream_policy_enabled,
                        }
                      : current
                  )
                }
              />
            </Field>
            <div className='grid gap-4 sm:grid-cols-2'>
              <Field>
                <FieldLabel htmlFor='audit-cyber-policy-ban-threshold'>
                  {t('Violation threshold')}
                </FieldLabel>
                <Input
                  id='audit-cyber-policy-ban-threshold'
                  type='number'
                  min={1}
                  max={1_000_000}
                  step={1}
                  value={draft.cyber_policy_ban_threshold}
                  disabled={!draft.cyber_policy_auto_ban_enabled}
                  onChange={(event) => {
                    const value = event.target.valueAsNumber
                    if (Number.isInteger(value)) {
                      setDraft((current) =>
                        current
                          ? {
                              ...current,
                              cyber_policy_ban_threshold: value,
                            }
                          : current
                      )
                    }
                  }}
                />
                <FieldDescription>
                  {t('Set to 1 to disable the user after the first violation.')}
                </FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor='audit-cyber-policy-window-hours'>
                  {t('Rolling window (hours)')}
                </FieldLabel>
                <Input
                  id='audit-cyber-policy-window-hours'
                  type='number'
                  min={1}
                  max={87_600}
                  step={1}
                  value={draft.cyber_policy_violation_window_hours}
                  disabled={!draft.cyber_policy_auto_ban_enabled}
                  onChange={(event) => {
                    const value = event.target.valueAsNumber
                    if (Number.isInteger(value)) {
                      setDraft((current) =>
                        current
                          ? {
                              ...current,
                              cyber_policy_violation_window_hours: value,
                            }
                          : current
                      )
                    }
                  }}
                />
                <FieldDescription>
                  {t(
                    'Only exact cyber_policy events in this period are counted.'
                  )}
                </FieldDescription>
              </Field>
            </div>
            <Field>
              <FieldLabel htmlFor='audit-cyber-policy-exempt-groups'>
                {t('Automatic ban group whitelist')}
              </FieldLabel>
              <MultiSelect
                id='audit-cyber-policy-exempt-groups'
                options={includeMissingSensitiveGroupOptions(
                  groupOptions,
                  draft.cyber_policy_auto_ban_exempt_group_codes ?? [],
                  t('Unavailable group')
                )}
                selected={draft.cyber_policy_auto_ban_exempt_group_codes ?? []}
                onChange={(groupCodes) =>
                  setDraft((current) =>
                    current
                      ? {
                          ...current,
                          cyber_policy_auto_ban_exempt_group_codes:
                            normalizeSensitiveGroupCodes(groupCodes),
                        }
                      : current
                  )
                }
                placeholder={t('Select groups exempt from automatic bans...')}
                emptyText={t('No groups available.')}
                disabled={
                  !draft.cyber_policy_auto_ban_enabled ||
                  groupsQuery.isLoading ||
                  groupsQuery.isError
                }
                maxVisibleChips={3}
              />
              <FieldDescription>
                {t(
                  'Selected business groups remain in the audit log but do not count toward cyber_policy bans; other groups still follow the threshold.'
                )}
              </FieldDescription>
              <p className='text-xs text-amber-600 dark:text-amber-400'>
                {t(
                  'The whitelist can only narrow automatic bans. To apply bans to every non-whitelisted group, set the official risk control scope above to All channels.'
                )}
              </p>
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
            </Field>
            <Field orientation='horizontal'>
              <FieldContent>
                <FieldLabel htmlFor='audit-sensitive-events-enabled'>
                  {t('Record sensitive word events')}
                </FieldLabel>
                <FieldDescription>
                  {t(
                    'Write one unified event for each deduplicated request, response, or Realtime sensitive-word match.'
                  )}
                </FieldDescription>
              </FieldContent>
              <Switch
                id='audit-sensitive-events-enabled'
                checked={draft.sensitive_word_audit_enabled}
                onCheckedChange={(sensitiveWordAuditEnabled) =>
                  setDraft((current) =>
                    current
                      ? {
                          ...current,
                          sensitive_word_audit_enabled:
                            sensitiveWordAuditEnabled,
                        }
                      : current
                  )
                }
              />
            </Field>
          </FieldGroup>
        </CardContent>
      </Card>

      {draft.uses_legacy_sensitive_words ? (
        <Alert>
          <AlertTitle>{t('Legacy sensitive words detected')}</AlertTitle>
          <AlertDescription>
            {t(
              'They are shown as a structured blocking rule. Saving keeps the legacy data intact and switches the active editor to structured rules.'
            )}
          </AlertDescription>
        </Alert>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>{t('Sensitive word filtering')}</CardTitle>
          <CardDescription>
            {t(
              'These existing rules were moved from System Settings and continue using the same stored configuration.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <SensitiveWordsSection
            key={draft.config_version}
            defaultValues={sensitiveValues}
            inlineActions
            hideTitle
            externalDirty={policySwitchesDirty}
            externalInvalid={scopeValidationError !== null}
            isSaving={saving}
            onSaveValues={savePolicy}
            onResetExternal={resetPolicySwitches}
          />
        </CardContent>
      </Card>
    </div>
  )
}
