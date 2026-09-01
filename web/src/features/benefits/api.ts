import { api } from '@/lib/api'

import type {
  BenefitActivity,
  BenefitActivityUserView,
  BenefitReport,
  BenefitLedgerEntry,
  BenefitVoucher,
} from './types'

type ApiResponse<T> = { success: boolean; message?: string; data?: T }

export type BenefitActivityInput = Omit<
  BenefitActivity,
  | 'id'
  | 'status'
  | 'group_code_snapshot'
  | 'group_name_snapshot'
  | 'published_at'
  | 'total_quota'
  | 'total_amount_cents'
  | 'fixed_amount_cents'
  | 'min_amount_cents'
  | 'max_amount_cents'
  | 'claim_paid_threshold_cents'
> & { id?: number }

export async function getBenefitActivities(): Promise<
  BenefitActivityUserView[]
> {
  const response = await api.get<ApiResponse<BenefitActivityUserView[]>>(
    '/api/benefit/activities'
  )
  if (!response.data.success) {
    throw new Error(
      response.data.message || 'Unable to load benefit activities'
    )
  }
  return response.data.data ?? []
}

export async function getBenefitVouchers(): Promise<BenefitVoucher[]> {
  const response = await api.get<ApiResponse<BenefitVoucher[]>>(
    '/api/benefit/vouchers'
  )
  if (!response.data.success) {
    throw new Error(response.data.message || 'Unable to load benefit vouchers')
  }
  return response.data.data ?? []
}

export async function claimBenefitActivity(id: number) {
  const response = await api.post<ApiResponse<BenefitVoucher>>(
    `/api/benefit/activities/${id}/claim`
  )
  return response.data
}

export async function getAdminBenefitActivities() {
  const response = await api.get<
    ApiResponse<{ items: BenefitActivity[]; total: number }>
  >('/api/benefit/admin/activities?p=1&page_size=100')
  if (!response.data.success) {
    throw new Error(
      response.data.message || 'Unable to load benefit activities'
    )
  }
  return response.data.data?.items ?? []
}

export async function getAdminBenefitVouchers(id: number) {
  const response = await api.get<ApiResponse<BenefitVoucher[]>>(
    `/api/benefit/admin/activities/${id}/vouchers`
  )
  return response.data.data ?? []
}

export async function getAdminBenefitVoucherLedger(id: number) {
  const response = await api.get<ApiResponse<BenefitLedgerEntry[]>>(
    `/api/benefit/admin/vouchers/${id}/ledger`
  )
  return response.data.data ?? []
}

export async function voidAdminBenefitVoucher(id: number, reason: string) {
  const response = await api.post<ApiResponse<null>>(
    `/api/benefit/admin/vouchers/${id}/void`,
    { confirm: true, reason }
  )
  return response.data
}

export async function createAdminBenefitActivity(input: BenefitActivityInput) {
  const response = await api.post<ApiResponse<BenefitActivity>>(
    '/api/benefit/admin/activities',
    input
  )
  return response.data
}

export async function updateAdminBenefitActivity(
  input: BenefitActivityInput & { id: number }
) {
  const response = await api.put<ApiResponse<BenefitActivity>>(
    `/api/benefit/admin/activities/${input.id}`,
    input
  )
  return response.data
}

export async function publishAdminBenefitActivity(id: number) {
  const response = await api.post<ApiResponse<BenefitActivity>>(
    `/api/benefit/admin/activities/${id}/publish`
  )
  return response.data
}

export async function transitionAdminBenefitActivity(
  id: number,
  action: 'pause' | 'resume' | 'end'
) {
  const response = await api.post<ApiResponse<BenefitActivity>>(
    `/api/benefit/admin/activities/${id}/${action}`
  )
  return response.data
}

export async function terminateAdminBenefitActivity(
  id: number,
  mode: 'unused' | 'all',
  reason: string
) {
  const response = await api.post<ApiResponse<null>>(
    `/api/benefit/admin/activities/${id}/terminate`,
    { mode, reason, confirm: true }
  )
  return response.data
}

export async function getAdminBenefitReport(id: number) {
  const response = await api.get<ApiResponse<BenefitReport>>(
    `/api/benefit/admin/activities/${id}/report`
  )
  if (!response.data.success || !response.data.data) {
    throw new Error(response.data.message || 'Failed to load benefit report')
  }
  return response.data.data
}
