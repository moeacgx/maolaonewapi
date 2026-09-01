/*
Copyright (C) 2025 QuantumNous

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

// Shared label/status mapping for the benefits feature (user + admin views).
// Centralized here because both `components/benefits/**` (user-facing) and
// `components/table/benefits/**` (admin-facing) need the exact same mapping
// for voucher/activity/ledger status text and colors.

export const BENEFIT_VOUCHER_STATUS_LABEL_KEYS = {
  active: 'Active',
  exhausted: 'Exhausted',
  expired: 'Expired',
  voided: 'Voided',
};

export const BENEFIT_VOUCHER_STATUS_TAG_COLORS = {
  active: 'green',
  exhausted: 'blue',
  expired: 'grey',
  voided: 'red',
};

export const benefitVoucherStatusLabel = (t, status) =>
  t(BENEFIT_VOUCHER_STATUS_LABEL_KEYS[status] || status || 'Unknown');

export const benefitVoucherStatusColor = (status) =>
  BENEFIT_VOUCHER_STATUS_TAG_COLORS[status] || 'grey';

export const BENEFIT_ACTIVITY_STATUS_LABEL_KEYS = {
  draft: 'Draft',
  published: 'Published',
  paused: 'Paused',
  ended: 'Ended',
  terminated: 'Terminated',
};

export const BENEFIT_ACTIVITY_STATUS_TAG_COLORS = {
  draft: 'grey',
  published: 'green',
  paused: 'orange',
  ended: 'grey',
  terminated: 'red',
};

export const benefitActivityStatusLabel = (t, status) =>
  t(BENEFIT_ACTIVITY_STATUS_LABEL_KEYS[status] || status || 'Unknown');

export const benefitActivityStatusColor = (status) =>
  BENEFIT_ACTIVITY_STATUS_TAG_COLORS[status] || 'grey';

// Activities can be selected for deletion while `draft` (no shares issued
// yet), or once they are no longer accepting claims (`ended`/`terminated`).
// The server re-validates this against live share/voucher state; the client
// list only needs to avoid offering an obviously-wrong selection.
export const BENEFIT_ACTIVITY_DELETABLE_STATUSES = [
  'draft',
  'ended',
  'terminated',
];

export const isBenefitActivityDeletable = (status) =>
  BENEFIT_ACTIVITY_DELETABLE_STATUSES.includes(status);

export const BENEFIT_CLAIM_REASON_LABEL_KEYS = {
  ineligible: 'Not eligible yet',
  claimed: 'Already claimed',
  sold_out: 'Fully claimed',
  inactive: 'Not currently claimable',
  not_started: 'Not started',
  ended: 'Ended',
};

export const benefitClaimReasonLabel = (t, reason) =>
  t(BENEFIT_CLAIM_REASON_LABEL_KEYS[reason] || 'Not eligible yet');

// Only an activity view where the user is eligible, has not claimed yet, is
// currently claimable, and still has shares left renders an enabled claim
// button; every other combination surfaces a status tag instead.
export const isBenefitActivityClaimable = (activity) =>
  Boolean(activity?.eligible) && !activity?.has_claimed;

export const BENEFIT_LEDGER_TYPE_LABEL_KEYS = {
  pre_consume: 'Reserved for a request',
  settle_delta: 'Settlement adjustment',
  settle_rollback: 'Settlement reverted',
  refund_additional: 'Extra reservation released',
  refund: 'Refunded',
  void: 'Voided',
  expire: 'Expired',
};

export const benefitLedgerTypeLabel = (t, type) =>
  t(BENEFIT_LEDGER_TYPE_LABEL_KEYS[type] || type || 'Unknown');

// A voucher counts as "expiring soon" once it is still active/usable and its
// expiry falls inside this window. 72h gives users enough lead time to spend
// a voucher before it lapses without flagging nearly every active voucher;
// the design spec does not pin an exact figure, so this is a deliberate,
// documented choice rather than a derived contract value.
export const BENEFIT_VOUCHER_EXPIRING_SOON_WINDOW_SECONDS = 72 * 3600;

export const isBenefitVoucherExpiringSoon = (
  voucher,
  nowSeconds,
  windowSeconds = BENEFIT_VOUCHER_EXPIRING_SOON_WINDOW_SECONDS,
) =>
  voucher?.status === 'active' &&
  Number(voucher?.remaining_quota || 0) > 0 &&
  Number(voucher?.expires_at || 0) > nowSeconds &&
  Number(voucher?.expires_at || 0) <= nowSeconds + windowSeconds;

// Reason codes returned by DeleteBenefitActivitiesByIDs (model/marketing_delete.go)
// for a skipped activity id.
export const BENEFIT_ACTIVITY_DELETE_SKIP_REASON_LABEL_KEYS = {
  not_found: 'Activity not found',
  has_claim_data: 'Draft activity already has claim data',
  active_voucher: 'Still has active vouchers with a balance',
  not_deletable: 'Activity is still running',
};

export const benefitActivityDeleteSkipReasonLabel = (t, reason) =>
  t(
    BENEFIT_ACTIVITY_DELETE_SKIP_REASON_LABEL_KEYS[reason] ||
      reason ||
      'Unknown',
  );

// Reason codes returned by VoidBenefitVouchers (model/benefit_voucher.go) for
// a skipped voucher id.
export const BENEFIT_VOUCHER_VOID_SKIP_REASON_LABEL_KEYS = {
  not_found: 'Voucher not found',
  not_active: 'Voucher is not active or has no remaining balance',
};

export const benefitVoucherVoidSkipReasonLabel = (t, reason) =>
  t(BENEFIT_VOUCHER_VOID_SKIP_REASON_LABEL_KEYS[reason] || reason || 'Unknown');

// Formats an amount that the backend has already converted into the site's
// current quota_display_type (e.g. activity total/fixed/min/max amounts and
// the claim threshold) — NOT a raw internal quota value, so this must never
// be passed through renderQuota(), which expects raw quota and would
// double-convert an already-converted amount.
export const formatDisplayAmount = (t, amount, currency) => {
  const number = Number(amount || 0);
  if (currency.type === 'TOKENS') {
    return `${Math.round(number).toLocaleString()} ${t('Tokens')}`;
  }
  return `${currency.symbol}${number.toFixed(2)}`;
};
