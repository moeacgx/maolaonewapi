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

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import {
  buildDynamicRoutingChannelConfig,
  getDynamicRoutingChannelMode,
} from '../lib/rules'
import type {
  DynamicRoutingChannelConfig,
  DynamicRoutingChannelMode,
  DynamicRoutingRule,
} from '../types'
import { DynamicRoutingRulesEditor } from './dynamic-routing-rules-editor'

type ChannelDynamicRoutingEditorProps = {
  value: DynamicRoutingChannelConfig | undefined
  onChange: (value: DynamicRoutingChannelConfig | undefined) => void
  disabled?: boolean
  sourceModelOptions?: string[]
  targetModelOptions?: string[]
  targetGroupOptions?: Array<{ value: string; label: string }>
}

export function ChannelDynamicRoutingEditor(
  props: ChannelDynamicRoutingEditorProps
) {
  const { t } = useTranslation()
  const mode = getDynamicRoutingChannelMode(props.value)
  const rules = props.value?.rules ?? []

  const update = (
    nextMode: DynamicRoutingChannelMode,
    nextRules: DynamicRoutingRule[]
  ) => {
    props.onChange(buildDynamicRoutingChannelConfig(nextMode, nextRules))
  }

  return (
    <div className='space-y-4'>
      <Alert>
        <AlertDescription className='text-xs'>
          {t(
            'Channel rules take precedence over global rules with the same source model and scope. Global rules remain available only when this channel has no rule covering that model and scope.'
          )}
        </AlertDescription>
      </Alert>
      <div className='grid gap-1.5'>
        <Label>{t('Channel dynamic routing switch')}</Label>
        <Select
          items={[
            { value: 'inherit', label: t('Inherit global switch') },
            { value: 'enabled', label: t('Enable for this channel') },
            { value: 'disabled', label: t('Disable for this channel') },
          ]}
          value={mode}
          onValueChange={(value) => {
            if (!value) return
            update(value as DynamicRoutingChannelMode, rules)
          }}
          disabled={props.disabled}
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              <SelectItem value='inherit'>
                {t('Inherit global switch')}
              </SelectItem>
              <SelectItem value='enabled'>
                {t('Enable for this channel')}
              </SelectItem>
              <SelectItem value='disabled'>
                {t('Disable for this channel')}
              </SelectItem>
            </SelectGroup>
          </SelectContent>
        </Select>
      </div>
      <DynamicRoutingRulesEditor
        rules={rules}
        onChange={(nextRules) => update(mode, nextRules)}
        disabled={props.disabled}
        sourceModelOptions={props.sourceModelOptions}
        targetModelOptions={props.targetModelOptions}
        targetGroupOptions={props.targetGroupOptions}
      />
    </div>
  )
}
