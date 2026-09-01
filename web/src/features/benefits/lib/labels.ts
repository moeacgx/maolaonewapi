import type { TFunction } from 'i18next'

import type { BenefitActivityStatus, BenefitVoucherStatus } from '../types'

/** Voucher status label. Explicit t() calls keep every value scannable for i18n sync. */
export function voucherStatusLabel(
  status: BenefitVoucherStatus,
  t: TFunction
): string {
  switch (status) {
    case 'active':
      return t('Active')
    case 'exhausted':
      return t('Exhausted')
    case 'expired':
      return t('Expired')
    case 'voided':
      return t('Voided')
    default:
      return status
  }
}

/** Activity status label. Explicit t() calls keep every value scannable for i18n sync. */
export function activityStatusLabel(
  status: BenefitActivityStatus,
  t: TFunction
): string {
  switch (status) {
    case 'draft':
      return t('Draft')
    case 'published':
      return t('Published')
    case 'paused':
      return t('Paused')
    case 'ended':
      return t('Ended')
    case 'terminated':
      return t('Terminated')
    default:
      return status
  }
}

/**
 * Claim eligibility reason label. Mirrors the backend's
 * `BenefitClaimReason*` constants (ineligible/claimed/sold_out/inactive/
 * not_started/ended); explicit t() calls keep every value scannable.
 */
export function claimEligibilityLabel(
  reason: string | undefined,
  t: TFunction
): string {
  switch (reason) {
    case 'ineligible':
      return t('Not eligible for this group')
    case 'claimed':
      return t('Already claimed')
    case 'sold_out':
      return t('Fully claimed')
    case 'inactive':
      return t('Activity is not active')
    case 'not_started':
      return t('Activity has not started')
    case 'ended':
      return t('Activity has ended')
    default:
      return t('Not eligible')
  }
}

export function ledgerEntryTypeLabel(type: string, t: TFunction): string {
  switch (type) {
    case 'pre_consume':
      return t('Pre-consume')
    case 'settle_delta':
      return t('Settlement adjustment')
    case 'settle_rollback':
      return t('Settlement rollback')
    case 'refund_additional':
      return t('Additional refund')
    case 'refund':
      return t('Refund')
    case 'void':
      return t('Voided')
    case 'expire':
      return t('Expired')
    default:
      return type
  }
}

/**
 * Activity batch-delete skip-reason label. Codes mirror the deletion
 * state matrix (draft-with-claims / still-active / has-active-vouchers /
 * not-found); an unrecognized code still renders a readable sentence
 * instead of a raw code, since the exact backend enum isn't pinned here.
 */
export function activityDeleteSkipReasonLabel(
  reason: string,
  t: TFunction
): string {
  switch (reason) {
    case 'not_found':
      return t('Activity not found')
    case 'has_claim_data':
      return t('Draft activity already has claim data')
    case 'status_active':
    case 'not_ended':
    case 'active_status':
      return t('Activity is still published or paused')
    case 'has_active_vouchers':
    case 'active_vouchers':
      return t('Activity still has active vouchers')
    default:
      return t('Not eligible for deletion ({{reason}})', { reason })
  }
}
