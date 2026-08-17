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
export function parsePositiveInteger(value: string): number {
  if (value === '') return 0
  const parsed = Number(value)
  if (!Number.isFinite(parsed)) return 0
  return Math.max(0, Math.trunc(parsed))
}

export function toUnixSeconds(value: string): number {
  if (!value) return 0
  const milliseconds = new Date(value).getTime()
  if (Number.isNaN(milliseconds)) return 0
  return Math.floor(milliseconds / 1000)
}

export function canPlacePredictionBet(
  status: string | undefined,
  closeTime: number | undefined,
  nowSeconds: number
): boolean {
  return status === 'open' && (!closeTime || closeTime > nowSeconds)
}
