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
import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import {
  createDynamicRoutingRule,
  createDynamicRoutingRuleFromPreset,
  DYNAMIC_ROUTING_PRESETS,
} from '../lib/rules'
import type { DynamicRoutingRule } from '../types'
import { DynamicRoutingRuleEditor } from './dynamic-routing-rule-editor'

const ruleEditorKeys = new WeakMap<DynamicRoutingRule, string>()
let nextRuleEditorKey = 0

function getRuleEditorKey(rule: DynamicRoutingRule): string {
  const existingKey = ruleEditorKeys.get(rule)
  if (existingKey) return existingKey

  nextRuleEditorKey += 1
  const key = `dynamic-routing-rule-${nextRuleEditorKey}`
  ruleEditorKeys.set(rule, key)
  return key
}

type DynamicRoutingRulesEditorProps = {
  rules: DynamicRoutingRule[]
  onChange: (rules: DynamicRoutingRule[]) => void
  disabled?: boolean
  sourceModelOptions?: string[]
  targetModelOptions?: string[]
}

export function DynamicRoutingRulesEditor(
  props: DynamicRoutingRulesEditorProps
) {
  const { t } = useTranslation()

  const updateRule = (index: number, rule: DynamicRoutingRule) => {
    props.onChange(
      props.rules.map((existingRule, existingIndex) =>
        existingIndex === index ? rule : existingRule
      )
    )
  }

  const removeRule = (index: number) => {
    props.onChange(props.rules.filter((_, ruleIndex) => ruleIndex !== index))
  }

  const addRule = () => {
    props.onChange([...props.rules, createDynamicRoutingRule()])
  }

  const addPreset = (
    preset: (typeof DYNAMIC_ROUTING_PRESETS)[number]['id']
  ) => {
    props.onChange([...props.rules, createDynamicRoutingRuleFromPreset(preset)])
  }

  const cannotAddRule = props.disabled || props.rules.length >= 100

  return (
    <div className='space-y-4'>
      <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Rules are evaluated by priority. When priorities tie, the first matching rule is used.'
          )}
        </p>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={addRule}
          disabled={cannotAddRule}
        >
          <Plus className='mr-2 h-4 w-4' aria-hidden='true' />
          {t('Add routing rule')}
        </Button>
      </div>

      <section className='space-y-3 rounded-lg border border-dashed p-4'>
        <div className='space-y-1'>
          <h3 className='text-sm font-medium'>
            {t('Quick Setup from Preset')}
          </h3>
          <p className='text-muted-foreground text-xs'>
            {t('Pick the closest preset, then adjust the values shown below.')}
          </p>
        </div>
        <div className='grid gap-2 sm:grid-cols-2'>
          {DYNAMIC_ROUTING_PRESETS.map((preset) => (
            <Button
              key={preset.id}
              type='button'
              variant='outline'
              className='h-auto min-h-20 justify-start px-3 py-2 text-left whitespace-normal'
              onClick={() => addPreset(preset.id)}
              disabled={cannotAddRule}
            >
              <span className='flex flex-col items-start gap-1'>
                <span className='font-medium'>{t(preset.label)}</span>
                <span className='text-muted-foreground text-xs font-normal'>
                  {t(preset.description)}
                </span>
              </span>
            </Button>
          ))}
        </div>
      </section>

      {props.rules.map((rule, index) => (
        <DynamicRoutingRuleEditor
          key={getRuleEditorKey(rule)}
          rule={rule}
          index={index}
          onChange={(nextRule) => updateRule(index, nextRule)}
          onRemove={() => removeRule(index)}
          disabled={props.disabled}
          sourceModelOptions={props.sourceModelOptions}
          targetModelOptions={props.targetModelOptions}
        />
      ))}

      {props.rules.length === 0 && (
        <div className='text-muted-foreground flex min-h-28 items-center justify-center rounded-lg border border-dashed px-4 text-center text-sm'>
          {t('No dynamic routing rules configured. Add a rule to get started.')}
        </div>
      )}
    </div>
  )
}
