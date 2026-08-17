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

export const MODEL_PRICE_UNITS = {
  REQUEST: 'request',
  SECOND: 'second',
} as const

export type ModelPriceUnit =
  (typeof MODEL_PRICE_UNITS)[keyof typeof MODEL_PRICE_UNITS]

export type ModelPriceVariantRule = {
  resolution?: string
  quality?: string
  price: number
}

export type ModelPriceVariantConfig = {
  resolution_enabled: boolean
  quality_enabled: boolean
  rules?: ModelPriceVariantRule[]
  inherited?: boolean
}

export type ModelRoutePriceVariants = Record<string, ModelPriceVariantConfig>

export type FixedPriceRange = {
  minimum: number
  maximum: number
  hasVariants: boolean
}

export function normalizeModelPriceUnit(value: unknown): ModelPriceUnit {
  return value === MODEL_PRICE_UNITS.SECOND
    ? MODEL_PRICE_UNITS.SECOND
    : MODEL_PRICE_UNITS.REQUEST
}

export function getModelPriceVariantRules(
  config: ModelPriceVariantConfig | null | undefined
): ModelPriceVariantRule[] {
  return (config?.rules ?? []).filter(
    (rule) => Number.isFinite(rule.price) && rule.price >= 0
  )
}

export function hasActiveModelPriceVariants(
  config: ModelPriceVariantConfig | null | undefined
): boolean {
  return Boolean(
    config &&
    (config.resolution_enabled || config.quality_enabled) &&
    getModelPriceVariantRules(config).length > 0
  )
}

/** Variant rule prices are final prices; base price is fallback, not an addend. */
export function getFixedPriceRange(
  basePrice: number | null | undefined,
  variants?: ModelPriceVariantConfig | null,
  routeVariants?: ModelRoutePriceVariants | null
): FixedPriceRange {
  const normalizedBase =
    typeof basePrice === 'number' && Number.isFinite(basePrice)
      ? Math.max(basePrice, 0)
      : 0
  const prices = [normalizedBase]
  let hasVariants = false

  if (hasActiveModelPriceVariants(variants)) {
    prices.push(
      ...getModelPriceVariantRules(variants).map((rule) => rule.price)
    )
    hasVariants = true
  }
  for (const config of Object.values(routeVariants ?? {})) {
    if (!hasActiveModelPriceVariants(config)) continue
    prices.push(...getModelPriceVariantRules(config).map((rule) => rule.price))
    hasVariants = true
  }

  return {
    minimum: Math.min(...prices),
    maximum: Math.max(...prices),
    hasVariants,
  }
}
