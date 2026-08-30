import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { createInstance } from 'i18next'
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

import en from '../../../../i18n/locales/en.json'
import fr from '../../../../i18n/locales/fr.json'
import ja from '../../../../i18n/locales/ja.json'
import ru from '../../../../i18n/locales/ru.json'
import vi from '../../../../i18n/locales/vi.json'
import zhTW from '../../../../i18n/locales/zh-TW.json'
import zh from '../../../../i18n/locales/zh.json'
import { QUOTA_TYPES } from '../../constants'
import type { PricingModel } from '../../types'
import {
  getBillingAdjustmentClassName,
  getBillingAdjustmentLabel,
  getBillingCompositeFactor,
} from '../billing-adjustment'
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

  test('formats every group pricing factor as a fold label', () => {
    expect(
      getBillingCompositeFactor({
        groupRatio: 0.2,
        priceRate: 7.2,
        usdExchangeRate: 7,
      })
    ).toBeCloseTo(0.2057, 4)
    expect(getBillingAdjustmentLabel(0.2057)).toEqual({
      kind: 'discount',
      key: '{{discount}} fold',
      discount: '2.1',
      multiplier: '0.21',
    })
    expect(getBillingAdjustmentLabel(1)).toEqual({
      kind: 'discount',
      key: '{{discount}} fold',
      discount: '10',
      multiplier: '1',
    })
    expect(getBillingAdjustmentLabel(2.06)).toEqual({
      kind: 'discount',
      key: '{{discount}} fold',
      discount: '20.6',
      multiplier: '2.06',
    })
  })

  test('keeps model cards wired to the combined billing adjustment', () => {
    const cardSource = readFileSync(
      join(process.cwd(), 'src/features/pricing/components/model-card.tsx'),
      'utf8'
    )

    expect(cardSource).toContain('getBillingCompositeFactor')
    expect(cardSource).toContain('getBillingAdjustmentLabel')
    expect(cardSource).toContain('showBillingDiscount')
    expect(cardSource).toContain('billingFactor < 0.9995')
  })

  test('keeps model detail badges hidden for original price or markup factors', () => {
    const detailSource = readFileSync(
      join(process.cwd(), 'src/features/pricing/components/model-details.tsx'),
      'utf8'
    )

    expect(detailSource).toContain('function BillingAdjustmentBadge')
    expect(detailSource).toContain('factor >= 0.9995')
    expect(detailSource).toContain('return null')
  })

  test('renders fold values only for Chinese and real multipliers for other locales', async () => {
    const label = getBillingAdjustmentLabel(1)
    const cases = [
      { language: 'en', locale: en, expected: '1x' },
      { language: 'zh', locale: zh, expected: '10折' },
      { language: 'zh-TW', locale: zhTW, expected: '10折' },
      { language: 'fr', locale: fr, expected: '1 fois' },
      { language: 'ru', locale: ru, expected: '1x' },
      { language: 'ja', locale: ja, expected: '1倍' },
      { language: 'vi', locale: vi, expected: '1 lần' },
    ]

    for (const testCase of cases) {
      const i18n = createInstance()
      await i18n.init({
        lng: testCase.language,
        resources: {
          [testCase.language]: testCase.locale,
        },
      })
      expect(i18n.t(label.key, label)).toBe(testCase.expected)
    }
  })

  test('uses red for deep discounts and green for all other factors', () => {
    expect(getBillingAdjustmentClassName(0.4999)).toContain('text-red-800')
    expect(getBillingAdjustmentClassName(0.5)).toContain('text-green-800')
  })
})
