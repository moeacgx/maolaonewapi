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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { CHANNEL_TYPE_OPTIONS } from '@/features/channels/constants'

import {
  DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE,
  type DynamicRoutingRule,
} from '../types'

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
  const sourceGroups = (props.rule.source_groups ?? []).join(', ')
  const isImageToolBridge =
    props.rule.action === DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE

  return (
    <div className='grid gap-4 md:grid-cols-2'>
      <div className='grid gap-1.5'>
        <Label htmlFor={`dynamic-routing-${props.ruleIndex}-source-groups`}>
          {t('Source groups')}
        </Label>
        <Input
          id={`dynamic-routing-${props.ruleIndex}-source-groups`}
          value={sourceGroups}
          onChange={(event) =>
            props.onChange({
              ...props.rule,
              source_groups: event.target.value
                .split(',')
                .map((group) => group.trim())
                .filter(Boolean),
            })
          }
          placeholder='codex, default'
          disabled={props.disabled}
        />
        <p className='text-muted-foreground text-xs'>
          {t('Leave empty to match every effective source group.')}
        </p>
      </div>
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
          selected={
            isImageToolBridge
              ? ['/v1/responses']
              : (props.rule.request_paths ?? [])
          }
          onChange={(requestPaths) =>
            props.onChange({
              ...props.rule,
              request_paths: isImageToolBridge
                ? ['/v1/responses']
                : requestPaths,
            })
          }
          placeholder={
            isImageToolBridge ? '/v1/responses' : t('All request paths')
          }
          emptyText={t('No request paths available.')}
          allowCreate={!isImageToolBridge}
          createLabel={t('Add "{{value}}"')}
          disabled={props.disabled || isImageToolBridge}
          maxVisibleChips={2}
        />
        <p className='text-muted-foreground text-xs'>
          {t(
            isImageToolBridge
              ? 'Bridge requests use /v1/responses downstream; the target path is configurable.'
              : 'Leave empty to match every request path. Paths must be exact.'
          )}
        </p>
      </div>
    </div>
  )
}
