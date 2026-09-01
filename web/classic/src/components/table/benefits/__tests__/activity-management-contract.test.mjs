import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const readSource = (path) =>
  readFileSync(new URL(path, import.meta.url), 'utf8');

test('Classic report formats actual quota directly, with no ratio-based reconstruction', () => {
  const reportSource = readSource('../BenefitActivityReport.jsx');
  assert.match(reportSource, /renderQuota\(Number\(quota \|\| 0\)\)/);
  assert.doesNotMatch(reportSource, /reportAmountFromQuota/);
  // The claim threshold is a historical CNY payment gate, not spendable
  // quota, and must never be reconstructed via a total_amount/total_quota
  // ratio (the exact anti-pattern the redesign forbids).
  assert.doesNotMatch(reportSource, /totalQuota \*[\s\S]{0,80}total_amount/);
  assert.match(reportSource, /claim_paid_threshold/);
});

test('Classic report avoids hardcoded currency symbols on quota-derived metrics', () => {
  const reportSource = readSource('../BenefitActivityReport.jsx');
  // The one deliberate exception is the CNY claim-threshold row, which is
  // always CNY regardless of quota_display_type; every other metric must
  // go through renderQuota().
  const withoutThresholdRow = reportSource.replace(
    /`¥\$\{claimThresholdCNY\.toFixed\(2\)\}`/,
    '',
  );
  assert.doesNotMatch(withoutThresholdRow, /[¥]|\$(?!\{)/);
});

test('Classic activity table selects deletable history rows and shows per-id results', () => {
  const source = readSource('../BenefitActivitiesPanel.jsx');
  assert.match(source, /rowSelection/);
  assert.match(source, /isBenefitActivityDeletable/);
  const batchActionsSource = readSource('../BenefitActivityBatchActions.jsx');
  assert.match(batchActionsSource, /deleted_ids/);
  assert.match(batchActionsSource, /skipped/);
  assert.match(
    batchActionsSource,
    /'\/api\/benefit\/admin\/activities\/batch'/,
  );
});

test('Classic activity form submits amount_display_type alongside display-unit amounts', () => {
  const source = readSource('../BenefitActivitiesPanel.jsx');
  assert.match(source, /amount_display_type: currency\.type/);
  assert.match(source, /const currency = getCurrencyConfig\(\)/);
  assert.match(source, /const isTokens = currency\.type === 'TOKENS'/);
});

test('Classic activity form uses dynamic step/precision per display type', () => {
  const source = readSource('../BenefitActivitiesPanel.jsx');
  assert.match(source, /const amountStep = isTokens \? 1 : 0\.01/);
  assert.match(source, /const amountPrecision = isTokens \? 0 : 2/);
  assert.match(source, /amountFieldLabel\(t, 'Total budget', currency\)/);
  assert.match(source, /amountFieldLabel\(t, 'Amount per voucher', currency\)/);
  assert.match(source, /amountFieldLabel\(t, 'Minimum amount', currency\)/);
  assert.match(source, /amountFieldLabel\(t, 'Maximum amount', currency\)/);
});

test('Classic activity status tag uses the shared status label/color map', () => {
  const source = readSource('../BenefitActivitiesPanel.jsx');
  assert.match(source, /benefitActivityStatusColor\(value\)/);
  assert.match(source, /benefitActivityStatusLabel\(t, value\)/);
  assert.doesNotMatch(source, /\{value\}\s*<\/Tag>/);
});

test('Classic activity report is delegated to the extracted BenefitActivityReport component', () => {
  const source = readSource('../BenefitActivitiesPanel.jsx');
  assert.match(
    source,
    /import BenefitActivityReport from '\.\/BenefitActivityReport'/,
  );
  assert.match(source, /<BenefitActivityReport/);
  assert.doesNotMatch(source, /BenefitActivityReportView/);
});

test('Classic voucher list is delegated to the extracted BenefitVoucherTable component', () => {
  const source = readSource('../BenefitActivitiesPanel.jsx');
  assert.match(
    source,
    /import BenefitVoucherTable from '\.\/BenefitVoucherTable'/,
  );
  assert.match(
    source,
    /<BenefitVoucherTable activityId=\{detail\.activityId\} \/>/,
  );
});

test('BenefitVoucherTable paginates and filters against the registered admin endpoint', () => {
  const source = readSource('../BenefitVoucherTable.jsx');
  assert.match(
    source,
    /\/api\/benefit\/admin\/activities\/\$\{activityId\}\/vouchers/,
  );
  assert.match(source, /p: String\(nextPage\)/);
  assert.match(source, /page_size/);
  assert.match(source, /keyword/);
  assert.match(source, /status/);
});

test('BenefitVoucherTable only allows batch-voiding active vouchers and shows skipped reasons', () => {
  const source = readSource('../BenefitVoucherTable.jsx');
  assert.match(source, /disabled: record\.status !== 'active'/);
  assert.match(source, /'\/api\/benefit\/admin\/vouchers\/batch-void'/);
  assert.match(source, /updated_ids/);
  assert.match(source, /skipped/);
});

test('BenefitVoucherTable formats every quota column through renderQuota', () => {
  const source = readSource('../BenefitVoucherTable.jsx');
  assert.match(source, /dataIndex: 'original_quota'[\s\S]{0,80}renderQuota/);
  assert.match(source, /dataIndex: 'used_quota'[\s\S]{0,80}renderQuota/);
  assert.match(source, /dataIndex: 'remaining_quota'[\s\S]{0,80}renderQuota/);
});

test('BenefitVoucherLedger shows admin-only operator/reason metadata via renderQuota deltas', () => {
  const source = readSource('../BenefitVoucherLedger.jsx');
  assert.match(
    source,
    /\/api\/benefit\/admin\/vouchers\/\$\{voucherId\}\/ledger/,
  );
  assert.match(source, /metadata\?\.operator_id/);
  assert.match(source, /metadata\?\.reason/);
  assert.match(source, /renderQuota\(entry\.balance_after\)/);
});
