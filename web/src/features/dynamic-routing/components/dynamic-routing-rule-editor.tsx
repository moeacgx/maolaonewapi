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
import { Plus, Trash2 } from 'lucide-react'
import { useId } from 'react'
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
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'

import {
  DYNAMIC_ROUTING_ACTION_MODEL_REDIRECT,
  DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE,
  DYNAMIC_ROUTING_IMAGE_TARGET_PATHS,
  DYNAMIC_ROUTING_IMAGE_GENERATION_PATH,
  type DynamicRoutingAction,
  type DynamicRoutingCondition,
  type DynamicRoutingRule,
} from '../types'
import { DynamicRoutingConditionEditor } from './dynamic-routing-condition-editor'
import { DynamicRoutingScopeEditor } from './dynamic-routing-scope-editor'

type DynamicRoutingRuleEditorProps = {
  rule: DynamicRoutingRule
  index: number
  onChange: (rule: DynamicRoutingRule) => void
  onRemove: () => void
  disabled?: boolean
  sourceModelOptions?: string[]
  targetModelOptions?: string[]
}

const ACTION_OPTIONS: Array<{ value: DynamicRoutingAction; label: string }> = [
  { value: DYNAMIC_ROUTING_ACTION_MODEL_REDIRECT, label: 'Model redirect' },
  {
    value: DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE,
    label: 'Responses image tool bridge',
  },
]

export function DynamicRoutingRuleEditor(props: DynamicRoutingRuleEditorProps) {
  const { t } = useTranslation()
  const sourceListId = useId()
  const targetListId = useId()
  const action = props.rule.action ?? DYNAMIC_ROUTING_ACTION_MODEL_REDIRECT

  const updateRule = (patch: Partial<DynamicRoutingRule>) => {
    props.onChange({ ...props.rule, ...patch })
  }

  const updateCondition = (
    conditionIndex: number,
    condition: DynamicRoutingCondition
  ) => {
    const conditions = [...(props.rule.conditions ?? [])]
    conditions[conditionIndex] = condition
    updateRule({ conditions })
  }

  const removeCondition = (conditionIndex: number) => {
    updateRule({
      conditions: (props.rule.conditions ?? []).filter(
        (_, index) => index !== conditionIndex
      ),
    })
  }

  const addCondition = () => {
    updateRule({
      conditions: [
        ...(props.rule.conditions ?? []),
        { field: 'reasoning_effort', operator: 'equals', value: 'high' },
      ],
    })
  }

  return (
    <section className='space-y-4 rounded-lg border p-4'>
      <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
        <div className='flex flex-wrap items-center gap-2'>
          <h3 className='font-medium'>
            {t('Routing rule {{number}}', { number: props.index + 1 })}
          </h3>
          <Badge variant='outline'>
            {t(
              action === DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE
                ? 'Responses image tool bridge'
                : 'Model redirect'
            )}
          </Badge>
        </div>
        <div className='flex items-center gap-3'>
          <Label htmlFor={`dynamic-routing-${props.index}-enabled`}>
            {t('Enable rule')}
          </Label>
          <Switch
            id={`dynamic-routing-${props.index}-enabled`}
            checked={props.rule.enabled}
            onCheckedChange={(enabled) => updateRule({ enabled })}
            disabled={props.disabled}
          />
          <Button
            type='button'
            variant='ghost'
            size='icon'
            className='text-destructive hover:text-destructive h-8 w-8'
            onClick={props.onRemove}
            disabled={props.disabled}
            aria-label={t('Delete routing rule')}
          >
            <Trash2 className='h-4 w-4' aria-hidden='true' />
          </Button>
        </div>
      </div>

      <div className='grid gap-4 md:grid-cols-[minmax(0,1fr)_9rem_14rem]'>
        <div className='grid gap-1.5'>
          <Label htmlFor={`dynamic-routing-${props.index}-id`}>
            {t('Rule ID')}
          </Label>
          <Input
            id={`dynamic-routing-${props.index}-id`}
            value={props.rule.id}
            onChange={(event) => updateRule({ id: event.target.value })}
            placeholder='gemini-high-reasoning'
            disabled={props.disabled}
          />
        </div>
        <div className='grid gap-1.5'>
          <Label htmlFor={`dynamic-routing-${props.index}-priority`}>
            {t('Priority')}
          </Label>
          <Input
            id={`dynamic-routing-${props.index}-priority`}
            type='number'
            min={-1000}
            max={1000}
            value={props.rule.priority ?? 0}
            onChange={(event) =>
              updateRule({
                priority:
                  event.target.value === '' ? 0 : Number(event.target.value),
              })
            }
            disabled={props.disabled}
          />
        </div>
        <div className='grid gap-1.5'>
          <Label htmlFor={`dynamic-routing-${props.index}-action`}>
            {t('Action')}
          </Label>
          <Select
            items={ACTION_OPTIONS.map((option) => ({
              value: option.value,
              label: t(option.label),
            }))}
            value={action}
            onValueChange={(value) => {
              if (!value) return
              const nextAction = value as DynamicRoutingAction
              updateRule({
                action: nextAction,
                target_path:
                  nextAction ===
                  DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE
                    ? DYNAMIC_ROUTING_IMAGE_GENERATION_PATH
                    : undefined,
                request_paths:
                  nextAction ===
                  DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE
                    ? ['/v1/responses']
                    : props.rule.request_paths,
              })
            }}
            disabled={props.disabled}
          >
            <SelectTrigger id={`dynamic-routing-${props.index}-action`}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                {ACTION_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {t(option.label)}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>
      </div>

      <div className='grid gap-4 md:grid-cols-2'>
        <div className='grid gap-1.5'>
          <Label htmlFor={`dynamic-routing-${props.index}-source-model`}>
            {t('Public source model')}
          </Label>
          <Input
            id={`dynamic-routing-${props.index}-source-model`}
            value={props.rule.source_model}
            onChange={(event) =>
              updateRule({ source_model: event.target.value })
            }
            placeholder='gemini-3.7-flash'
            list={sourceListId}
            disabled={props.disabled}
          />
        </div>
        <div className='grid gap-1.5'>
          <Label htmlFor={`dynamic-routing-${props.index}-target-model`}>
            {t(
              action === DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE
                ? 'Target image model'
                : 'Final upstream model'
            )}
          </Label>
          <Input
            id={`dynamic-routing-${props.index}-target-model`}
            value={props.rule.target_model}
            onChange={(event) =>
              updateRule({ target_model: event.target.value })
            }
            placeholder={
              action === DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE
                ? 'gpt-image-2'
                : 'gemini-3.7-flash-high'
            }
            list={targetListId}
            disabled={props.disabled}
          />
        </div>
      </div>

      {action === DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE && (
        <div className='grid gap-4 md:grid-cols-2'>
          <div className='grid gap-1.5'>
            <Label htmlFor={`dynamic-routing-${props.index}-target-path`}>
              {t('Target request path')}
            </Label>
            <Select
              items={DYNAMIC_ROUTING_IMAGE_TARGET_PATHS.map((path) => ({
                value: path,
                label: path,
              }))}
              value={
                props.rule.target_path ?? DYNAMIC_ROUTING_IMAGE_GENERATION_PATH
              }
              onValueChange={(value) => {
                if (value) updateRule({ target_path: value })
              }}
              disabled={props.disabled}
            >
              <SelectTrigger id={`dynamic-routing-${props.index}-target-path`}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {DYNAMIC_ROUTING_IMAGE_TARGET_PATHS.map((path) => (
                    <SelectItem key={path} value={path}>
                      {path}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <p className='text-muted-foreground text-xs'>
              {t(
                'The target request path determines whether the Responses or Images API is used.'
              )}
            </p>
          </div>
          <div className='grid gap-1.5'>
            <Label htmlFor={`dynamic-routing-${props.index}-target-group`}>
              {t('Target group')}
            </Label>
            <Input
              id={`dynamic-routing-${props.index}-target-group`}
              value={props.rule.target_group ?? ''}
              onChange={(event) =>
                updateRule({ target_group: event.target.value })
              }
              placeholder='image-generation'
              disabled={props.disabled}
            />
            <p className='text-muted-foreground text-xs'>
              {t('Leave empty to inherit the current effective group.')}
            </p>
          </div>
        </div>
      )}

      <DynamicRoutingScopeEditor
        rule={props.rule}
        ruleIndex={props.index}
        onChange={props.onChange}
        disabled={props.disabled}
      />

      <Separator />

      <div className='space-y-3'>
        <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
          <div className='space-y-1'>
            <Label>{t('Request conditions')}</Label>
            <p className='text-muted-foreground text-xs'>
              {t(
                action === DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE
                  ? 'This action only runs when tool_choice explicitly selects image_generation; the target path is controlled by the rule.'
                  : 'All conditions must match. Use reasoning_effort or request.<simple_json_path>.'
              )}
            </p>
          </div>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={addCondition}
            disabled={
              props.disabled || (props.rule.conditions?.length ?? 0) >= 8
            }
          >
            <Plus className='mr-2 h-4 w-4' aria-hidden='true' />
            {t('Add condition')}
          </Button>
        </div>
        {props.rule.conditions?.map((condition, conditionIndex) => (
          <DynamicRoutingConditionEditor
            key={`${condition.field}-${condition.operator}-${condition.value ?? ''}`}
            condition={condition}
            index={conditionIndex}
            onChange={(nextCondition) =>
              updateCondition(conditionIndex, nextCondition)
            }
            onRemove={() => removeCondition(conditionIndex)}
            disabled={props.disabled}
          />
        ))}
        {(props.rule.conditions?.length ?? 0) === 0 && (
          <p className='text-muted-foreground rounded-md border border-dashed px-3 py-2 text-xs'>
            {t(
              'No request conditions. This rule matches every request in its scope.'
            )}
          </p>
        )}
      </div>

      {props.sourceModelOptions && props.sourceModelOptions.length > 0 && (
        <datalist id={sourceListId}>
          {props.sourceModelOptions.map((model) => (
            <option key={model} value={model} />
          ))}
        </datalist>
      )}
      {props.targetModelOptions && props.targetModelOptions.length > 0 && (
        <datalist id={targetListId}>
          {props.targetModelOptions.map((model) => (
            <option key={model} value={model} />
          ))}
        </datalist>
      )}
    </section>
  )
}
