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
import { useTranslation } from 'react-i18next'

import { MultiSelect } from '@/components/multi-select'
import { Label } from '@/components/ui/label'
import { CHANNEL_TYPE_OPTIONS } from '@/features/channels/constants'

import type { DynamicRoutingRule } from '../types'

const REQUEST_PATH_OPTIONS = [
  '/v1/chat/completions',
  '/v1/responses',
  '/v1/messages',
  '/v1/images/generations',
  '/v1/images/edits',
  '/v1/images/tasks',
]

type DynamicRoutingScopeEditorProps = {
  rule: DynamicRoutingRule
  ruleIndex: number
  onChange: (rule: DynamicRoutingRule) => void
  disabled?: boolean
}

export function DynamicRoutingScopeEditor(
  props: DynamicRoutingScopeEditorProps
) {
  const { t } = useTranslation()
  const channelTypes = (props.rule.channel_types ?? []).map(String)

  return (
    <div className='grid gap-4 md:grid-cols-2'>
      <div className='grid gap-1.5'>
        <Label htmlFor={`dynamic-routing-${props.ruleIndex}-channel-types`}>
          {t('Upstream channel types')}
        </Label>
        <MultiSelect
          id={`dynamic-routing-${props.ruleIndex}-channel-types`}
          options={CHANNEL_TYPE_OPTIONS.map((option) => ({
            value: String(option.value),
            label: t(option.label),
          }))}
          selected={channelTypes}
          onChange={(selected) =>
            props.onChange({
              ...props.rule,
              channel_types: selected
                .map(Number)
                .filter(
                  (channelType) =>
                    Number.isInteger(channelType) && channelType > 0
                ),
            })
          }
          placeholder={t('All channel types')}
          emptyText={t('No channel types available.')}
          disabled={props.disabled}
          maxVisibleChips={3}
        />
        <p className='text-muted-foreground text-xs'>
          {t('Leave empty to match every upstream channel type.')}
        </p>
      </div>
      <div className='grid gap-1.5'>
        <Label htmlFor={`dynamic-routing-${props.ruleIndex}-request-paths`}>
          {t('Request paths')}
        </Label>
        <MultiSelect
          id={`dynamic-routing-${props.ruleIndex}-request-paths`}
          options={REQUEST_PATH_OPTIONS.map((path) => ({
            value: path,
            label: path,
          }))}
          selected={props.rule.request_paths ?? []}
          onChange={(requestPaths) =>
            props.onChange({ ...props.rule, request_paths: requestPaths })
          }
          placeholder={t('All request paths')}
          emptyText={t('No request paths available.')}
          allowCreate
          createLabel={t('Add "{{value}}"')}
          disabled={props.disabled}
          maxVisibleChips={2}
        />
        <p className='text-muted-foreground text-xs'>
          {t('Leave empty to match every request path. Paths must be exact.')}
        </p>
      </div>
    </div>
  )
}
