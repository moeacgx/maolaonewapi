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
import { describe, expect, test } from 'vitest'

import { QUOTA_TYPES } from '../../constants'
import type { PricingModel } from '../../types'
import { filterByQuotaType } from '../filters'
import {
  getFixedPriceRange,
  MODEL_PRICE_UNITS,
  normalizeModelPriceUnit,
} from '../fixed-price'
import { getGroupDisplayName } from '../group-names'
import { getConfiguredGroupRatio, getDisplayGroupRatio } from '../model-helpers'

const fixedModel: PricingModel = {
  id: 1,
  model_name: 'video-model',
  quota_type: 1,
  model_ratio: 0,
  completion_ratio: 0,
  model_price: 0.4,
  model_price_unit: 'second',
  enable_groups: ['free', 'paid'],
  group_ratio: { free: 0, paid: 2 },
}

describe('retained pricing contracts', () => {
  test('preserves zero group ratios for selected and best-price displays', () => {
    expect(getConfiguredGroupRatio({ free: 0 }, 'free')).toBe(0)
    expect(getDisplayGroupRatio(fixedModel, 'free')).toBe(0)
    expect(getDisplayGroupRatio(fixedModel)).toBe(0)
  })

  test('treats variant and route prices as final alternatives to fallback', () => {
    expect(
      getFixedPriceRange(
        0.4,
        {
          resolution_enabled: true,
          quality_enabled: false,
          rules: [{ resolution: '1080p', price: 0.7 }],
        },
        {
          'image.edit': {
            resolution_enabled: false,
            quality_enabled: true,
            rules: [{ quality: 'high', price: 0.2 }],
          },
        }
      )
    ).toEqual({ minimum: 0.2, maximum: 0.7, hasVariants: true })
  })

  test('defaults unknown fixed units to request and separates second filters', () => {
    expect(normalizeModelPriceUnit('unknown')).toBe(MODEL_PRICE_UNITS.REQUEST)
    const requestModel = {
      ...fixedModel,
      id: 2,
      model_name: 'image-model',
      model_price_unit: '' as const,
    }
    expect(
      filterByQuotaType([fixedModel, requestModel], QUOTA_TYPES.SECOND)
    ).toEqual([fixedModel])
    expect(
      filterByQuotaType([fixedModel, requestModel], QUOTA_TYPES.REQUEST)
    ).toEqual([requestModel])
  })

  test('uses group_names while safely falling back to internal code', () => {
    expect(getGroupDisplayName('vip', { vip: 'Premium' })).toBe('Premium')
    expect(getGroupDisplayName('legacy', { legacy: '  ' })).toBe('legacy')
  })
})
