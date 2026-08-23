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
import { Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

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

import type { DynamicRoutingCondition, DynamicRoutingOperator } from '../types'

const OPERATOR_OPTIONS: Array<{
  value: DynamicRoutingOperator
  label: string
}> = [
  { value: 'equals', label: 'Equals' },
  { value: 'not_equals', label: 'Does not equal' },
  { value: 'exists', label: 'Exists' },
  { value: 'not_exists', label: 'Does not exist' },
]

type DynamicRoutingConditionEditorProps = {
  condition: DynamicRoutingCondition
  index: number
  onChange: (condition: DynamicRoutingCondition) => void
  onRemove: () => void
  disabled?: boolean
}

export function DynamicRoutingConditionEditor(
  props: DynamicRoutingConditionEditorProps
) {
  const { t } = useTranslation()
  const operator = props.condition.operator ?? 'equals'
  const requiresValue = operator !== 'exists' && operator !== 'not_exists'
  const inputPrefix = `dynamic-routing-condition-${props.index}`

  const updateCondition = (patch: Partial<DynamicRoutingCondition>) => {
    const next = { ...props.condition, ...patch }
    if (!requiresValue && 'value' in next) delete next.value
    props.onChange(next)
  }

  return (
    <div className='grid gap-3 rounded-md border p-3 md:grid-cols-[minmax(0,1.15fr)_minmax(0,0.9fr)_minmax(0,1fr)_auto] md:items-end'>
      <div className='grid gap-1.5'>
        <Label htmlFor={`${inputPrefix}-field`}>{t('Condition field')}</Label>
        <Input
          id={`${inputPrefix}-field`}
          value={props.condition.field}
          onChange={(event) => updateCondition({ field: event.target.value })}
          placeholder='reasoning_effort'
          disabled={props.disabled}
        />
      </div>
      <div className='grid gap-1.5'>
        <Label>{t('Operator')}</Label>
        <Select
          items={OPERATOR_OPTIONS.map((option) => ({
            value: option.value,
            label: t(option.label),
          }))}
          value={operator}
          onValueChange={(value) => {
            if (!value) return
            const nextOperator = value as DynamicRoutingOperator
            const next: DynamicRoutingCondition = {
              ...props.condition,
              operator: nextOperator,
            }
            if (nextOperator === 'exists' || nextOperator === 'not_exists') {
              delete next.value
            }
            props.onChange(next)
          }}
          disabled={props.disabled}
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              {OPERATOR_OPTIONS.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {t(option.label)}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      </div>
      <div className='grid gap-1.5'>
        <Label htmlFor={`${inputPrefix}-value`}>{t('Value')}</Label>
        <Input
          id={`${inputPrefix}-value`}
          value={props.condition.value ?? ''}
          onChange={(event) => updateCondition({ value: event.target.value })}
          placeholder='high'
          disabled={props.disabled || !requiresValue}
        />
      </div>
      <Button
        type='button'
        variant='ghost'
        size='icon'
        className='h-10 w-10'
        onClick={props.onRemove}
        disabled={props.disabled}
        aria-label={t('Delete condition')}
      >
        <Trash2 className='h-4 w-4' aria-hidden='true' />
      </Button>
    </div>
  )
}
