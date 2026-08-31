export type BenefitActivityStatus =
  | 'draft'
  | 'published'
  | 'paused'
  | 'ended'
  | 'terminated'

export type BenefitActivity = {
  id: number
  name: string
  description: string
  group_id: number
  group_code_snapshot: string
  group_name_snapshot: string
  status: BenefitActivityStatus
  amount_mode: 'fixed' | 'random'
  total_amount_cents: number
  total_quota: number
  total_count: number
  fixed_amount_cents: number
  min_amount_cents: number
  max_amount_cents: number
  claim_paid_threshold_cents: number
  personal_valid_seconds: number
  starts_at: number
  ends_at: number
  published_at: number
}

export type BenefitVoucher = {
  id: number
  activity_id: number
  user_id: number
  original_amount_cents: number
  original_quota: number
  remaining_quota: number
  used_quota: number
  status: 'active' | 'exhausted' | 'expired' | 'voided'
  claimed_at: number
  expires_at: number
  void_reason?: string
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
}
