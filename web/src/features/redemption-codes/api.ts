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
import { api } from '@/lib/api'

import type {
  Redemption,
  ApiResponse,
  BatchDeleteResult,
  GetRedemptionsParams,
  GetRedemptionsResponse,
  SearchRedemptionsParams,
  CreateRedemptionFormData,
  UpdateRedemptionFormData,
  PromoCode,
  PromoCodeFormData,
  GetPromoCodesParams,
  GetPromoCodesResponse,
} from './types'

// ============================================================================
// Redemption Code Management
// ============================================================================

// Get paginated redemption codes list
export async function getRedemptions(
  params: GetRedemptionsParams = {}
): Promise<GetRedemptionsResponse> {
  const { p = 1, page_size = 10 } = params
  const res = await api.get(`/api/redemption/?p=${p}&page_size=${page_size}`)
  return res.data
}

// Search redemption codes by keyword
export async function searchRedemptions(
  params: SearchRedemptionsParams
): Promise<GetRedemptionsResponse> {
  const { keyword = '', status = '', p = 1, page_size = 10 } = params
  const queryParams = new URLSearchParams()
  queryParams.set('keyword', keyword)
  if (status) queryParams.set('status', status)
  queryParams.set('p', String(p))
  queryParams.set('page_size', String(page_size))
  const res = await api.get(`/api/redemption/search?${queryParams.toString()}`)
  return res.data
}

// Get single redemption code by ID
export async function getRedemption(
  id: number
): Promise<ApiResponse<Redemption>> {
  const res = await api.get(`/api/redemption/${id}`)
  return res.data
}

// Create redemption code(s)
export async function createRedemption(
  data: CreateRedemptionFormData
): Promise<ApiResponse<string[]>> {
  const res = await api.post('/api/redemption/', data)
  return res.data
}

// Update redemption code
export async function updateRedemption(
  data: UpdateRedemptionFormData
): Promise<ApiResponse<Redemption>> {
  const res = await api.put('/api/redemption/', data)
  return res.data
}

// Update redemption code status (enable/disable)
export async function updateRedemptionStatus(
  id: number,
  status: number
): Promise<ApiResponse<Redemption>> {
  const res = await api.put('/api/redemption/?status_only=true', { id, status })
  return res.data
}

// Delete a single redemption code
export async function deleteRedemption(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/redemption/${id}/`)
  return res.data
}

// Batch delete selected redemption codes
export async function deleteRedemptions(
  ids: number[]
): Promise<ApiResponse<BatchDeleteResult>> {
  const res = await api.delete('/api/redemption/batch', { data: { ids } })
  return res.data
}

// Delete invalid redemption codes (used, disabled, expired)
// Note: this endpoint keeps the legacy contract and returns the deleted count as `data`.
export async function deleteInvalidRedemptions(): Promise<ApiResponse<number>> {
  const res = await api.delete('/api/redemption/invalid')
  return res.data
}

export async function getPromoCodes(
  params: GetPromoCodesParams = {}
): Promise<GetPromoCodesResponse> {
  const { p = 1, page_size = 10 } = params
  const res = await api.get(`/api/promo_code/?p=${p}&page_size=${page_size}`)
  return res.data
}

export async function createPromoCode(
  data: PromoCodeFormData
): Promise<ApiResponse<PromoCode>> {
  const res = await api.post('/api/promo_code/', data)
  return res.data
}

export async function updatePromoCode(
  data: PromoCodeFormData & { id: number }
): Promise<ApiResponse<PromoCode>> {
  const res = await api.put('/api/promo_code/', data)
  return res.data
}

export async function updatePromoCodeStatus(
  id: number,
  status: number
): Promise<ApiResponse<PromoCode>> {
  const res = await api.put('/api/promo_code/?status_only=true', { id, status })
  return res.data
}

export async function deletePromoCode(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/promo_code/${id}/`)
  return res.data
}

// Batch delete selected promo codes
export async function deletePromoCodes(
  ids: number[]
): Promise<ApiResponse<BatchDeleteResult>> {
  const res = await api.delete('/api/promo_code/batch', { data: { ids } })
  return res.data
}

// Delete invalid promo codes (disabled, exhausted, expired)
export async function deleteInvalidPromoCodes(): Promise<
  ApiResponse<BatchDeleteResult>
> {
  const res = await api.delete('/api/promo_code/invalid')
  return res.data
}
