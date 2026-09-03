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
 * Activity batch-delete skip-reason label. Covers the backend's real
 * skip codes (not_found / has_claim_data / active_voucher / not_deletable);
 * an unrecognized code still renders a readable sentence instead of a raw
 * code, for any future code this list hasn't caught up with yet.
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
    case 'active_voucher':
      return t('Activity still has active vouchers')
    case 'not_deletable':
      return t('Activity is still active or not eligible for deletion')
    default:
      return t('Unknown reason')
  }
}
