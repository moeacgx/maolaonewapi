import { api } from '@/lib/api'
import { useSystemConfigStore } from '@/stores/system-config-store'

import type {
  BenefitActivity,
  BenefitActivityBatchDeleteResult,
  BenefitActivityUserView,
  BenefitLedgerEntry,
  BenefitReport,
  BenefitVoucher,
  BenefitVoucherAdminView,
  BenefitVoucherBatchResult,
  BenefitVoucherListFilter,
  BenefitVoucherListResult,
} from './types'

type ApiResponse<T> = { success: boolean; message?: string; data?: T }

export type BenefitGroupOption = {
  value: string
  label: string
  desc?: string
  ratio?: number | string
  id: number
}

type BenefitGroupDetails = {
  id: number
  code: string
  name: string
  status: number
  ratio?: number | string
}

export type BenefitActivityInput = Omit<
  BenefitActivity,
  | 'id'
  | 'status'
  | 'group_code_snapshot'
  | 'group_name_snapshot'
  | 'published_at'
  | 'total_quota'
  | 'fixed_quota'
  | 'min_quota'
  | 'max_quota'
> & { id?: number }

export async function getBenefitGroupOptions(): Promise<BenefitGroupOption[]> {
  const response =
    await api.get<ApiResponse<BenefitGroupDetails[]>>('/api/group/details')
  if (!response.data.success || !Array.isArray(response.data.data)) {
    throw new Error(response.data.message || 'Unable to load groups')
  }
  const groups = response.data.data.filter(
    (group) =>
      Number.isInteger(Number(group.id)) &&
      Number(group.id) > 0 &&
      Number(group.status) === 1
  )
  const nameCounts = new Map<string, number>()
  groups.forEach((group) => {
    const name =
      String(group.name ?? '').trim() || String(group.code ?? '').trim()
    nameCounts.set(name, (nameCounts.get(name) ?? 0) + 1)
  })
  return groups.map((group) => {
    const id = Number(group.id)
    const name =
      String(group.name ?? '').trim() || String(group.code ?? '').trim()
    const code = String(group.code ?? '').trim()
    const count = nameCounts.get(name) ?? 0
    const label = count > 1 && code ? `${name} · ${code}` : name
    return {
      value: String(id),
      label,
      ratio: group.ratio,
      id,
    }
  })
}

/** The current system display type, sent alongside admin-entered amounts. */
export function getCurrentAmountDisplayType() {
  return useSystemConfigStore.getState().config.currency.quotaDisplayType
}

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

/** A user's own voucher ledger; separate from the admin ledger route below. */
export async function getBenefitVoucherLedger(
  id: number
): Promise<BenefitLedgerEntry[]> {
  const response = await api.get<ApiResponse<BenefitLedgerEntry[]>>(
    `/api/benefit/vouchers/${id}/ledger`
  )
  if (!response.data.success) {
    throw new Error(response.data.message || 'Unable to load voucher ledger')
  }
  return response.data.data ?? []
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

export type BenefitVoucherListParams = {
  activityId: number
  page: number
  pageSize: number
  filter?: BenefitVoucherListFilter
}

/** Paginated, filterable admin voucher list for one activity. */
export async function getAdminBenefitVouchers(
  params: BenefitVoucherListParams
): Promise<BenefitVoucherListResult> {
  const response = await api.get<ApiResponse<BenefitVoucherListResult>>(
    `/api/benefit/admin/activities/${params.activityId}/vouchers`,
    {
      params: {
        p: params.page,
        page_size: params.pageSize,
        keyword: params.filter?.keyword || undefined,
        status: params.filter?.status || undefined,
      },
    }
  )
  if (!response.data.success) {
    throw new Error(response.data.message || 'Unable to load vouchers')
  }
  const data = response.data.data
  return {
    items: data?.items ?? [],
    total: data?.total ?? 0,
    page: data?.page ?? params.page,
    page_size: data?.page_size ?? params.pageSize,
  }
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

/** Batch voucher void; only active vouchers are eligible. */
export async function voidAdminBenefitVouchers(ids: number[], reason: string) {
  const response = await api.post<ApiResponse<BenefitVoucherBatchResult>>(
    '/api/benefit/admin/vouchers/batch-void',
    { ids, reason, confirm: true }
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

/**
 * Batch-deletes historical activities (also used for a single-id delete,
 * since there is no dedicated `/:id` delete route). Returns the actual
 * `deleted_ids` and per-id `skipped` reasons.
 */
export async function deleteAdminBenefitActivities(ids: number[]) {
  const response = await api.delete<
    ApiResponse<BenefitActivityBatchDeleteResult>
  >('/api/benefit/admin/activities/batch', { data: { ids } })
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

export type { BenefitVoucherAdminView }
