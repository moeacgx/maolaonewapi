import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const readSource = (path) =>
  readFileSync(new URL(path, import.meta.url), 'utf8');

test('BenefitSummary formats every stat through renderQuota, never a bare quota literal', () => {
  const source = readSource('../BenefitSummary.jsx');
  assert.match(source, /import \{ renderQuota \} from '\.\.\/\.\.\/helpers'/);
  assert.match(source, /renderQuota\(availableQuota\)/);
  assert.match(source, /renderQuota\(usedQuota\)/);
  assert.match(source, /renderQuota\(expiringQuota\)/);
  assert.doesNotMatch(source, /[¥]|\$(?!\{)/);
});

test('BenefitSummary derives claimable-activity count from the shared eligibility rule', () => {
  const source = readSource('../BenefitSummary.jsx');
  assert.match(source, /isBenefitActivityClaimable/);
  assert.match(source, /isBenefitVoucherExpiringSoon/);
});

test('UserVoucherCard renders original/used/remaining amounts through renderQuota', () => {
  const source = readSource('../UserVoucherCard.jsx');
  assert.match(source, /renderQuota\(voucher\.remaining_quota\)/);
  assert.match(source, /renderQuota\(voucher\.used_quota\)/);
  assert.match(source, /renderQuota\(voucher\.original_quota\)/);
  assert.doesNotMatch(source, /\{voucher\.original_quota\}/);
  assert.doesNotMatch(source, /\{voucher\.used_quota\}/);
  assert.doesNotMatch(source, /\{voucher\.remaining_quota\}/);
  assert.doesNotMatch(source, /[¥]|\$(?!\{)/);
});

test('UserVoucherCard prefers the voucher-embedded activity/group name over the activities list lookup', () => {
  const source = readSource('../UserVoucherCard.jsx');
  assert.match(
    source,
    /const activityName = voucher\.activity_name \|\| activity\?\.name/,
  );
  assert.match(
    source,
    /const groupName =\s*voucher\.group_name_snapshot \|\| activity\?\.group_name_snapshot/,
  );
  assert.match(source, /\{activityName \|\| t\('Benefit voucher'\)\}/);
  assert.match(source, /\{groupName \|\| t\('Unknown'\)\}/);
});

test('UserVoucherCard shows claimed/expiry times without a ledger entry point', () => {
  const source = readSource('../UserVoucherCard.jsx');
  assert.match(source, /timestamp2string\(voucher\.claimed_at\)/);
  assert.match(source, /timestamp2string\(voucher\.expires_at\)/);
  assert.doesNotMatch(source, /onViewLedger|View ledger|<History\b/);
});

test('ClaimableActivityCard only enables the claim button when eligible and unclaimed', () => {
  const source = readSource('../ClaimableActivityCard.jsx');
  assert.match(source, /isBenefitActivityClaimable/);
  assert.match(source, /benefitClaimReasonLabel/);
  assert.match(source, /activity\.has_claimed/);
  assert.doesNotMatch(source, /[¥]|\$(?!\{)/);
});

test('ClaimableActivityCard shows remaining shares and personal validity without a hardcoded currency', () => {
  const source = readSource('../ClaimableActivityCard.jsx');
  assert.match(source, /activity\.remaining_count/);
  assert.match(source, /activity\.total_count/);
  assert.match(source, /activity\.personal_valid_hours/);
  assert.doesNotMatch(source, /[¥]|\$(?!\{)/);
});

test('ClaimableActivityCard shows the real per-share amount, not a total/count average', () => {
  const source = readSource('../ClaimableActivityCard.jsx');
  assert.doesNotMatch(source, /averageSharePerCount/);
  assert.match(source, /renderQuota\(activity\.fixed_quota\)/);
  assert.match(source, /renderQuota\(activity\.min_quota\)/);
  assert.match(source, /renderQuota\(activity\.max_quota\)/);
});

test('UserVoucherLedgerSheet has independent loading/error/data state and hides admin metadata', () => {
  const source = readSource('../UserVoucherLedgerSheet.jsx');
  assert.match(source, /loading/);
  assert.match(source, /error/);
  assert.match(source, /entries\.length === 0/);
  assert.match(source, /onRetry/);
  assert.doesNotMatch(source, /operator_id/);
  assert.doesNotMatch(source, /metadata/i);
});

test('UserVoucherLedgerSheet formats delta and balance through renderQuota', () => {
  const source = readSource('../UserVoucherLedgerSheet.jsx');
  assert.match(source, /renderQuota\(numeric\)/);
  assert.match(source, /renderQuota\(entry\.balance_after\)/);
  assert.match(source, /benefitLedgerTypeLabel/);
  assert.doesNotMatch(source, /[¥]|\$(?!\{)/);
});

test('Classic user benefits page composes summary, voucher, and activity components without a ledger entry point', () => {
  const source = readSource('../../../pages/Benefits/index.jsx');
  assert.match(
    source,
    /import BenefitSummary from '\.\.\/\.\.\/components\/benefits\/BenefitSummary'/,
  );
  assert.match(
    source,
    /import UserVoucherCard from '\.\.\/\.\.\/components\/benefits\/UserVoucherCard'/,
  );
  assert.match(
    source,
    /import ClaimableActivityCard from '\.\.\/\.\.\/components\/benefits\/ClaimableActivityCard'/,
  );
  assert.doesNotMatch(source, /UserVoucherLedgerSheet|ledgerVoucherId|loadVoucherLedger/);
  assert.match(source, /classic-console-panel/);
  assert.doesNotMatch(
    source,
    /voucher\.original_quota|voucher\.used_quota|voucher\.remaining_quota/,
  );
});

test('Classic user benefits page uses a responsive two-column card grid', () => {
  const source = readSource('../../../pages/Benefits/index.jsx');
  assert.match(source, /grid gap-3 md:grid-cols-2/);
});
