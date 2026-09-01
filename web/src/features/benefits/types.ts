import type { CurrencyDisplayType } from '@/stores/system-config-store'

export type BenefitActivityStatus =
  | 'draft'
  | 'published'
  | 'paused'
  | 'ended'
  | 'terminated'

export type BenefitAmountMode = 'fixed' | 'random'

export type BenefitActivity = {
  id: number
  name: string
  description: string
  group_id: number
  group_code_snapshot: string
  group_name_snapshot: string
  status: BenefitActivityStatus
  amount_mode: BenefitAmountMode
  /** Display-unit type the amount fields below are expressed in. */
  amount_display_type: CurrencyDisplayType
  /** Amount fields are expressed in the current system display unit. */
  total_amount: number
  /** Real quota total; the only safe source for formatted display. */
  total_quota: number
  total_count: number
  fixed_amount: number
  min_amount: number
  max_amount: number
  claim_paid_threshold: number
  personal_valid_hours: number
  starts_at: number
  ends_at: number
  published_at: number
}

export type BenefitVoucherStatus = 'active' | 'exhausted' | 'expired' | 'voided'

export type BenefitVoucher = {
  id: number
  activity_id: number
  user_id: number
  original_quota: number
  remaining_quota: number
  used_quota: number
  status: BenefitVoucherStatus
  claimed_at: number
  expires_at: number
  voided_at?: number
  void_reason?: string
}

/** Admin voucher list row, enriched with activity/user context for a single query. */
export type BenefitVoucherAdminView = BenefitVoucher & {
  activity_name: string
  group_name_snapshot: string
  username: string
}

export type BenefitActivityUserView = BenefitActivity & {
  eligible: boolean
  eligibility_reason?: string
  has_claimed: boolean
  claimed_voucher?: BenefitVoucher
  single_user_concurrency_limit: number
}

export type BenefitReport = {
  total_quota: number
  undistributed_quota: number
  distributed_quota: number
  used_quota: number
  expired_unused_quota: number
  /** Share counters; only present once the backend report includes them. */
  total_shares?: number
  distributed_shares?: number
  used_up_shares?: number
  expired_shares?: number
}

export type BenefitLedgerEntry = {
  id: number
  activity_id: number
  voucher_id: number
  user_id: number
  request_id: string
  log_id: number
  type: string
  quota_delta: number
  balance_after: number
  created_at: number
  /** Admin-only metadata (operator, void reason, ...); absent for user-facing views. */
  metadata?: string
}

export type BenefitVoucherListFilter = {
  keyword?: string
  status?: string
}

export type BenefitVoucherListResult = {
  items: BenefitVoucherAdminView[]
  total: number
  page: number
  page_size: number
}

export type BenefitVoucherBatchSkip = {
  id: number
  reason: string
}

export type BenefitVoucherBatchResult = {
  updated_ids: number[]
  skipped: BenefitVoucherBatchSkip[]
}

/** Matches the activity batch-delete response actually returned today (counts only). */
export type BenefitActivityBatchDeleteResult = {
  deleted: number
  skipped: number
}
