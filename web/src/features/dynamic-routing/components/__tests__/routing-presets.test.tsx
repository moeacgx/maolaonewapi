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
*/
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { DynamicRoutingRulesEditor } =
  await import('../dynamic-routing-rules-editor')
const { DynamicRoutingRuleEditor } =
  await import('../dynamic-routing-rule-editor')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Add routing rule': 'Add routing rule',
        'Basic model redirect': 'Basic model redirect',
        'Bridge an explicitly selected image_generation tool to /v1/images/generations.':
          'Bridge an explicitly selected image_generation tool to /v1/images/generations.',
        'Bridge an explicitly selected image_generation tool to a Responses-capable image model.':
          'Bridge an explicitly selected image_generation tool to a Responses-capable image model.',
        'Keep the request endpoint and rewrite only the final upstream model.':
          'Keep the request endpoint and rewrite only the final upstream model.',
        'Pick the closest preset, then adjust the values shown below.':
          'Pick the closest preset, then adjust the values shown below.',
        'Quick Setup from Preset': 'Quick Setup from Preset',
        'Reasoning effort redirect': 'Reasoning effort redirect',
        'Responses image tool to Images API':
          'Responses image tool to Images API',
        'Responses image tool to Responses':
          'Responses image tool to Responses',
        'Route requests with reasoning_effort=high to a dedicated upstream model.':
          'Route requests with reasoning_effort=high to a dedicated upstream model.',
        'Rules are evaluated by priority. When priorities tie, the first matching rule is used.':
          'Rules are evaluated by priority. When priorities tie, the first matching rule is used.',
      },
    },
  },
})

describe('dynamic routing presets', () => {
  test('adds the Images API bridge template with its safe fixed paths', () => {
    const onChange = vi.fn()

    render(
      <I18nextProvider i18n={i18n}>
        <DynamicRoutingRulesEditor rules={[]} onChange={onChange} />
      </I18nextProvider>
    )

    fireEvent.click(
      screen.getByRole('button', {
        name: /Responses image tool to Images API/,
      })
    )

    expect(onChange).toHaveBeenCalledWith([
      expect.objectContaining({
        action: 'responses_image_tool_bridge',
        request_paths: ['/v1/responses'],
        target_path: '/v1/images/generations',
        source_model: '',
        target_model: '',
      }),
    ])
  })

  test('selects a group by display name and submits its code', () => {
    const onChange = vi.fn()

    render(
      <I18nextProvider i18n={i18n}>
        <DynamicRoutingRuleEditor
          rule={{
            id: 'image-route',
            enabled: true,
            action: 'responses_image_tool_bridge',
            source_model: 'gpt-5.6-sol',
            target_model: 'gpt-image-2',
            priority: 0,
          }}
          index={0}
          onChange={onChange}
          onRemove={vi.fn()}
          targetGroupOptions={[{ value: 'image', label: '生图专用分组' }]}
        />
      </I18nextProvider>
    )

    const groupSelect = screen.getByRole('combobox', {
      name: 'Target group (optional)',
    })
    fireEvent.pointerDown(groupSelect)
    fireEvent.click(groupSelect)
    const groupOption = screen.getByRole('option', { name: '生图专用分组' })
    fireEvent.pointerDown(groupOption)
    fireEvent.click(groupOption)

    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ target_group: 'image' })
    )
  })
})
