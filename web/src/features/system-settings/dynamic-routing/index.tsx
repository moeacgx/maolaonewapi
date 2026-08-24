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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { getEnabledModels } from '@/features/channels/api'
import { DynamicRoutingRulesEditor } from '@/features/dynamic-routing/components/dynamic-routing-rules-editor'
import {
  normalizeDynamicRoutingRules,
  parseDynamicRoutingRules,
  validateDynamicRoutingRules,
} from '@/features/dynamic-routing/lib/rules'
import type { DynamicRoutingRule } from '@/features/dynamic-routing/types'

import { getGroupDetails, updateSystemOption } from '../api'
import { SettingsSwitchField } from '../components/settings-form-layout'
import { SettingsPage } from '../components/settings-page'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'

const dynamicRoutingDefaults = {
  'dynamic_routing.enabled': false,
  'dynamic_routing.rules': [] as DynamicRoutingRule[],
}

type DynamicRoutingSettings = typeof dynamicRoutingDefaults

function serializeRules(rules: DynamicRoutingRule[]): string {
  return JSON.stringify(normalizeDynamicRoutingRules(rules))
}

function DynamicRoutingSettingsForm(props: {
  settings: DynamicRoutingSettings
}) {
  const settingsKey = `${props.settings['dynamic_routing.enabled']}:${serializeRules(
    props.settings['dynamic_routing.rules']
  )}`

  return (
    <DynamicRoutingSettingsEditor key={settingsKey} settings={props.settings} />
  )
}

function DynamicRoutingSettingsEditor(props: {
  settings: DynamicRoutingSettings
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const groupDetailsQuery = useQuery({
    queryKey: ['system-settings', 'group-details'],
    queryFn: getGroupDetails,
    refetchOnWindowFocus: false,
  })
  const enabledModelsQuery = useQuery({
    queryKey: ['dynamic-routing', 'enabled-models'],
    queryFn: getEnabledModels,
    refetchOnWindowFocus: false,
  })
  const modelOptions = useMemo(
    () =>
      [...new Set(enabledModelsQuery.data?.data ?? [])]
        .map((model) => model.trim())
        .filter(Boolean),
    [enabledModelsQuery.data?.data]
  )
  const targetGroupOptions = useMemo(
    () =>
      (groupDetailsQuery.data?.groups ?? [])
        .filter((group) => group.code.trim())
        .map((group) => ({
          value: group.code,
          label: group.name || group.code,
        })),
    [groupDetailsQuery.data?.groups]
  )
  const [enabled, setEnabled] = useState(
    props.settings['dynamic_routing.enabled']
  )
  const [rules, setRules] = useState<DynamicRoutingRule[]>(() =>
    normalizeDynamicRoutingRules(props.settings['dynamic_routing.rules'])
  )
  const [saving, setSaving] = useState(false)

  const savedRules = useMemo(
    () => serializeRules(props.settings['dynamic_routing.rules']),
    [props.settings]
  )
  const currentRules = useMemo(() => serializeRules(rules), [rules])
  const isDirty =
    enabled !== props.settings['dynamic_routing.enabled'] ||
    currentRules !== savedRules

  const handleSave = async () => {
    const normalizedRules = normalizeDynamicRoutingRules(rules)
    const validationError = validateDynamicRoutingRules(normalizedRules)
    if (validationError) {
      toast.error(t(validationError))
      return
    }

    setSaving(true)
    try {
      if (JSON.stringify(normalizedRules) !== savedRules) {
        const result = await updateSystemOption({
          key: 'dynamic_routing.rules',
          value: JSON.stringify(normalizedRules),
        })
        if (!result.success) {
          throw new Error(result.message || t('Failed to update setting'))
        }
      }

      if (enabled !== props.settings['dynamic_routing.enabled']) {
        const result = await updateSystemOption({
          key: 'dynamic_routing.enabled',
          value: String(enabled),
        })
        if (!result.success) {
          throw new Error(result.message || t('Failed to update setting'))
        }
      }

      await queryClient.invalidateQueries({ queryKey: ['system-options'] })
      toast.success(t('Dynamic routing settings saved successfully'))
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to save settings')
      )
    } finally {
      setSaving(false)
    }
  }

  return (
    <SettingsSection title={t('Dynamic Routing')}>
      <form
        className='space-y-6'
        onSubmit={(event) => {
          event.preventDefault()
          void handleSave()
        }}
      >
        <SettingsPageFormActions
          onSave={() => void handleSave()}
          isSaving={saving}
          isSaveDisabled={!isDirty}
          saveLabel={t('Save dynamic routing settings')}
        />
        <SettingsSwitchField
          checked={enabled}
          onCheckedChange={setEnabled}
          label={t('Enable dynamic routing')}
          description={t(
            'Disabled by default. Existing static model mappings continue to work when this switch is off or no rule matches.'
          )}
          disabled={saving}
        />

        <div className='bg-muted/20 rounded-lg border p-4 text-sm'>
          <p className='font-medium'>{t('Available routing actions')}</p>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t(
              'Model redirect changes only the final upstream model. It does not change the request endpoint, response format, or billing contract.'
            )}
          </p>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t(
              'Responses image tool bridge is a separate action: an explicit image_generation tool choice on downstream /v1/responses is sent to the target path configured by the rule, returned as a Responses image_generation_call, and billed as the target image model.'
            )}
          </p>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t(
              'Model fields suggest currently enabled models. Target groups are selected by display name and saved using their internal code.'
            )}
          </p>
        </div>

        <DynamicRoutingRulesEditor
          rules={rules}
          onChange={setRules}
          disabled={saving}
          sourceModelOptions={modelOptions}
          targetModelOptions={modelOptions}
          targetGroupOptions={targetGroupOptions}
        />
      </form>
    </SettingsSection>
  )
}

export function DynamicRoutingSettings() {
  return (
    <SettingsPage
      routePath='/_authenticated/system-settings/dynamic-routing/'
      defaultSettings={dynamicRoutingDefaults}
      defaultSection='general'
      getSectionMeta={() => ({ titleKey: 'Dynamic Routing' })}
      resolveSettings={(settings) => ({
        ...settings,
        'dynamic_routing.rules': parseDynamicRoutingRules(
          settings['dynamic_routing.rules']
        ),
      })}
      getSectionContent={(_sectionId, settings) => (
        <DynamicRoutingSettingsForm settings={settings} />
      )}
    />
  )
}
