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

export type BillingAdjustmentLabel = {
  kind: 'discount'
  key: '{{discount}} fold'
  value: string
}

function normalizeBillingFactor(value: number): number {
  return Number.isFinite(value) ? value : 1
}

function formatBillingNumber(value: number, digits: number): string {
  return value
    .toFixed(digits)
    .replace(/(\.\d*?[1-9])0+$/u, '$1')
    .replace(/\.0+$/u, '')
}

/**
 * 计算 v243 定价界面使用的展示因子。
 * 分组倍率与充值/汇率因子相乘，徽标展示相对于供应商美元价格的最终价格。
 */
export function getBillingCompositeFactor(options: {
  groupRatio?: number
  priceRate?: number
  usdExchangeRate?: number
}): number {
  const groupRatio = normalizeBillingFactor(options.groupRatio ?? 1)
  const priceRate = normalizeBillingFactor(options.priceRate ?? 1)
  const usdExchangeRate = normalizeBillingFactor(options.usdExchangeRate ?? 1)
  const forexFactor = usdExchangeRate > 0 ? priceRate / usdExchangeRate : 1

  return forexFactor * groupRatio
}

/** 将每个分组的综合因子统一格式化为折数插值。 */
export function getBillingAdjustmentLabel(
  factor: number
): BillingAdjustmentLabel {
  const normalizedFactor = normalizeBillingFactor(factor)
  return {
    kind: 'discount',
    key: '{{discount}} fold',
    value: formatBillingNumber(normalizedFactor * 10, 1),
  }
}

/** 沿用 v243 的颜色阈值：低于 0.5 为红色，其余为绿色。 */
export function getBillingAdjustmentClassName(factor: number): string {
  return normalizeBillingFactor(factor) < 0.5
    ? 'bg-red-500/15 text-red-800 dark:bg-red-400/15 dark:text-red-300'
    : 'bg-green-500/15 text-green-800 dark:bg-green-400/15 dark:text-green-300'
}
