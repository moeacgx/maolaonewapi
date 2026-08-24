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
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
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
  targetGroupOptions?: Array<{ value: string; label: string }>
}

const ACTION_OPTIONS: Array<{ value: DynamicRoutingAction; label: string }> = [
  { value: DYNAMIC_ROUTING_ACTION_MODEL_REDIRECT, label: 'Model redirect' },
  {
    value: DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE,
    label: 'Responses image tool bridge',
  },
]

const INHERIT_TARGET_GROUP_VALUE = '__dynamic_routing_inherit_group__'

export function DynamicRoutingRuleEditor(props: DynamicRoutingRuleEditorProps) {
  const { t } = useTranslation()
  const action = props.rule.action ?? DYNAMIC_ROUTING_ACTION_MODEL_REDIRECT
  const sourceModelOptions = (props.sourceModelOptions ?? []).map((model) => ({
    value: model,
    label: model,
  }))
  const targetModelOptions = (props.targetModelOptions ?? []).map((model) => ({
    value: model,
    label: model,
  }))
  const configuredTargetGroup = props.rule.target_group?.trim() ?? ''
  const targetGroupOptions = [...(props.targetGroupOptions ?? [])]
  if (
    configuredTargetGroup &&
    !targetGroupOptions.some((option) => option.value === configuredTargetGroup)
  ) {
    targetGroupOptions.unshift({
      value: configuredTargetGroup,
      label: t('Unknown configured group'),
    })
  }
  const missingRequiredFields = props.rule.enabled
    ? [
        !props.rule.id.trim() ? t('Rule ID') : '',
        !props.rule.source_model.trim() ? t('Public source model') : '',
        !props.rule.target_model.trim()
          ? t(
              action === DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE
                ? 'Target image model'
                : 'Final upstream model'
            )
          : '',
      ].filter(Boolean)
    : []

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

      {missingRequiredFields.length > 0 && (
        <p className='rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300'>
          {t('This enabled rule is incomplete. Fill in: {{fields}}.', {
            fields: missingRequiredFields.join(', '),
          })}
        </p>
      )}

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
            {t('Public source model *')}
          </Label>
          <Combobox
            options={sourceModelOptions}
            id={`dynamic-routing-${props.index}-source-model`}
            value={props.rule.source_model}
            onValueChange={(value) => updateRule({ source_model: value ?? '' })}
            placeholder={t('Model sent by the client, for example gpt-5.6-sol')}
            searchPlaceholder={t('Search or enter a model')}
            emptyText={t('No matching model. Press Enter to use custom text.')}
            allowCustomValue
            openOnFocus={false}
            className='w-full'
          />
          <p className='text-muted-foreground text-xs'>
            {t('Use the model name sent in the client request.')}
          </p>
        </div>
        <div className='grid gap-1.5'>
          <Label htmlFor={`dynamic-routing-${props.index}-target-model`}>
            {t(
              action === DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE
                ? 'Target image model *'
                : 'Final upstream model *'
            )}
          </Label>
          <Combobox
            options={targetModelOptions}
            id={`dynamic-routing-${props.index}-target-model`}
            value={props.rule.target_model}
            onValueChange={(value) => updateRule({ target_model: value ?? '' })}
            placeholder={t(
              action === DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE
                ? 'Target model, for example gpt-image-2'
                : 'Target model, for example gemini-3.7-flash-high'
            )}
            searchPlaceholder={t('Search or enter a model')}
            emptyText={t('No matching model. Press Enter to use custom text.')}
            allowCustomValue
            openOnFocus={false}
            className='w-full'
          />
          <p className='text-muted-foreground text-xs'>
            {t('The target channel must have this model configured.')}
          </p>
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
              {t('Target group (optional)')}
            </Label>
            <Select
              items={[
                {
                  value: INHERIT_TARGET_GROUP_VALUE,
                  label: t('Inherit current effective group'),
                },
                ...targetGroupOptions,
              ]}
              value={configuredTargetGroup || INHERIT_TARGET_GROUP_VALUE}
              onValueChange={(value) => {
                if (!value) return
                updateRule({
                  target_group:
                    value === INHERIT_TARGET_GROUP_VALUE ? undefined : value,
                })
              }}
              disabled={props.disabled}
            >
              <SelectTrigger
                id={`dynamic-routing-${props.index}-target-group`}
                className='w-full'
              >
                <SelectValue
                  placeholder={t('Inherit current effective group')}
                />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  <SelectItem value={INHERIT_TARGET_GROUP_VALUE}>
                    {t('Inherit current effective group')}
                  </SelectItem>
                  {targetGroupOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Select a group by its display name. The selected group code is saved automatically; inherit keeps the current effective group.'
              )}
            </p>
          </div>
        </div>
      )}

      <DynamicRoutingScopeEditor
        rule={props.rule}
        ruleIndex={props.index}
        onChange={props.onChange}
        disabled={props.disabled}
        groupOptions={props.targetGroupOptions}
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
    </section>
  )
}
