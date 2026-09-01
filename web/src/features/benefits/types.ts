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
  /** 展示金额（元）；后端会转换为计费 quota。 */
  total_amount: number
  total_quota: number
  total_count: number
  fixed_amount: number
  min_amount: number
  max_amount: number
  claim_paid_threshold: number
  /** 金额契约迁移前服务端可能返回的旧字段。 */
  total_amount_cents?: number
  fixed_amount_cents?: number
  min_amount_cents?: number
  max_amount_cents?: number
  claim_paid_threshold_cents?: number
  personal_valid_seconds: number
  starts_at: number
  ends_at: number
  published_at: number
}

export type BenefitVoucher = {
  id: number
  activity_id: number
  user_id: number
  /** 展示金额（元）；后端保存对应的计费 quota。 */
  original_amount: number
  original_quota: number
  remaining_quota: number
  used_quota: number
  status: 'active' | 'exhausted' | 'expired' | 'voided'
  claimed_at: number
  expires_at: number
  void_reason?: string
  original_amount_cents?: number
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
