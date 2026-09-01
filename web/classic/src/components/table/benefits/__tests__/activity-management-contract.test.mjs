import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const readSource = (path) =>
  readFileSync(new URL(path, import.meta.url), 'utf8');

test('Classic report formats actual quota directly, with no ratio-based reconstruction', () => {
  const reportSource = readSource('../BenefitActivityReport.jsx');
  assert.match(reportSource, /renderQuota\(Number\(quota \|\| 0\)\)/);
  assert.doesNotMatch(reportSource, /reportAmountFromQuota/);
  // The claim threshold is a historical paid-recharge gate, not spendable
  // quota, and must never be reconstructed via a total_amount/total_quota
  // ratio (the exact anti-pattern the redesign forbids).
  assert.doesNotMatch(reportSource, /totalQuota \*[\s\S]{0,80}total_amount/);
  assert.match(reportSource, /claim_paid_threshold/);
});

test('Classic report labels expired counts as shares/vouchers, not vouchers alone', () => {
  const reportSource = readSource('../BenefitActivityReport.jsx');
  // report.expired_count aggregates both expired (never-claimed) shares and
  // expired (claimed) vouchers server-side, so the label must not imply it
  // is voucher-only.
  assert.match(reportSource, /Expired shares\/vouchers/);
  assert.doesNotMatch(reportSource, /label=\{t\('Expired vouchers'\)\}/);
  assert.doesNotMatch(reportSource, /\{expiredCount\} \{t\('vouchers'\)\}/);
  assert.doesNotMatch(reportSource, /\{expiredCount\} \{t\('voucher\(s\)'\)\}/);
});

test('Classic report top sections skip pure format-explainer text', () => {
  const reportSource = readSource('../BenefitActivityReport.jsx');
  assert.doesNotMatch(
    reportSource,
    /Quota is shown using the current display type/,
  );
});

test('Classic report never re-paginates vouchers or recomputes counts client-side', () => {
  const reportSource = readSource('../BenefitActivityReport.jsx');
  // No `vouchers` prop and no per-voucher loop: every count/quota total
  // must come straight off the report object from GET .../report.
  assert.doesNotMatch(reportSource, /\{ activity, report, vouchers \}/);
  assert.doesNotMatch(reportSource, /voucherList/);
  assert.doesNotMatch(reportSource, /\.reduce\(/);
  assert.doesNotMatch(reportSource, /\.filter\(/);
  assert.match(reportSource, /report\.total_count/);
  assert.match(reportSource, /report\.distributed_count/);
  assert.match(reportSource, /report\.used_count/);
  assert.match(reportSource, /report\.expired_count/);

  const panelSource = readSource('../BenefitActivitiesPanel.jsx');
  assert.doesNotMatch(panelSource, /fetchAllVouchersForReport/);
  assert.doesNotMatch(panelSource, /reportVouchers/);
  assert.doesNotMatch(panelSource, /REPORT_VOUCHER_PAGE_SIZE/);
  assert.match(
    panelSource,
    /<BenefitActivityReport\s+activity=\{detailActivity\}\s+report=\{detailData\}\s*\/>/,
  );
});

test('Classic report formats the claim threshold via the current display type, not a hardcoded CNY symbol', () => {
  const reportSource = readSource('../BenefitActivityReport.jsx');
  assert.match(
    reportSource,
    /value=\{formatDisplayAmount\(t, claimThreshold, currency\)\}/,
  );
  assert.match(reportSource, /const currency = getCurrencyConfig\(\)/);
  assert.doesNotMatch(reportSource, /claimThresholdCNY/);
  assert.doesNotMatch(reportSource, /`¥/);
  assert.doesNotMatch(reportSource, /[¥]|\$(?!\{)/);
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

test('BenefitActivityBatchActions maps skip reason codes to readable text', () => {
  const source = readSource('../BenefitActivityBatchActions.jsx');
  assert.match(source, /benefitActivityDeleteSkipReasonLabel/);
  assert.match(
    source,
    /benefitActivityDeleteSkipReasonLabel\(t, entry\.reason\)/,
  );
  assert.doesNotMatch(source, /\{entry\.reason\}/);
});

test('benefitLabels maps every real activity-delete and voucher-void skip reason code', () => {
  const source = readSource('../../../benefits/benefitLabels.js');
  assert.match(source, /not_found: 'Activity not found'/);
  assert.match(source, /has_claim_data:/);
  assert.match(source, /active_voucher:/);
  assert.match(source, /not_deletable:/);
  assert.match(source, /not_found: 'Voucher not found'/);
  assert.match(source, /not_active:/);
});

test('unrecognized skip reason codes fall back to a fixed readable string, never the raw code', () => {
  const source = readSource('../../../benefits/benefitLabels.js');
  assert.match(
    source,
    /BENEFIT_ACTIVITY_DELETE_SKIP_REASON_LABEL_KEYS\[reason\] \|\| 'Unknown reason'/,
  );
  assert.match(
    source,
    /BENEFIT_VOUCHER_VOID_SKIP_REASON_LABEL_KEYS\[reason\] \|\| 'Unknown reason'/,
  );
  // The old fallback chain re-inserted the raw backend code
  // (`... || reason || 'Unknown'`) when a code wasn't in the map; it must
  // not come back.
  assert.doesNotMatch(source, /\[reason\] \|\|\s*reason \|\|/);
});

test('Claim threshold hint stays short and never names a fixed currency', () => {
  const source = readSource('../BenefitActivitiesPanel.jsx');
  assert.match(
    source,
    /Minimum historical paid amount required; 0 means no threshold\./,
  );
  assert.doesNotMatch(source, /historical paid recharge/);
  assert.doesNotMatch(source, /\(CNY\)/);
});

test('Classic activity form submits amount_display_type alongside display-unit amounts', () => {
  const source = readSource('../BenefitActivitiesPanel.jsx');
  assert.match(source, /amount_display_type: currency\.type/);
  assert.match(source, /const currency = getCurrencyConfig\(\)/);
  assert.match(source, /const isTokens = currency\.type === 'TOKENS'/);
});

test('Classic activity edit form reads fixed_quota/min_quota/max_quota directly, no derived fallback', () => {
  const source = readSource('../BenefitActivitiesPanel.jsx');
  assert.match(
    source,
    /quotaToDisplayAmount\(Number\(activity\.fixed_quota \|\| 0\)\)/,
  );
  assert.match(
    source,
    /quotaToDisplayAmount\(Number\(activity\.min_quota \|\| 0\)\)/,
  );
  assert.match(
    source,
    /quotaToDisplayAmount\(Number\(activity\.max_quota \|\| 0\)\)/,
  );
  assert.doesNotMatch(source, /totalQuota \/ count/);
  assert.doesNotMatch(source, /not-yet-landed/);
  assert.doesNotMatch(source, /unreleased/i);
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

test('BenefitVoucherTable only allows batch-voiding active vouchers, maps skip reasons, and hides the bulk action until something is selected', () => {
  const source = readSource('../BenefitVoucherTable.jsx');
  assert.match(source, /disabled: record\.status !== 'active'/);
  assert.match(source, /'\/api\/benefit\/admin\/vouchers\/batch-void'/);
  assert.match(source, /updated_ids/);
  assert.match(source, /skipped/);
  assert.match(source, /benefitVoucherVoidSkipReasonLabel\(t, entry\.reason\)/);
  assert.doesNotMatch(source, /#\{entry\.id\}: \{entry\.reason\}/);
  assert.match(source, /\{selectedIds\.length > 0 && \(/);
});

test('BenefitVoucherTable consolidates per-row Ledger/Void into a single actions menu', () => {
  const source = readSource('../BenefitVoucherTable.jsx');
  assert.match(source, /<Dropdown[\s\S]*position='bottomRight'/);
  assert.match(
    source,
    /<Dropdown\.Item[\s\S]*onClick=\{\(\) => setLedgerVoucherId\(record\.id\)\}/,
  );
  assert.match(
    source,
    /<Dropdown\.Item[\s\S]*onClick=\{\(\) => openVoidModal\(\[record\.id\]\)\}/,
  );
  assert.match(source, /aria-label=\{t\('操作'\)\}/);
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

test('New admin containers use the 8px module radius, not the ad hoc 12px rounded-xl', () => {
  const files = [
    '../BenefitActivitiesPanel.jsx',
    '../BenefitActivityReport.jsx',
  ];
  for (const file of files) {
    const source = readSource(file);
    assert.doesNotMatch(
      source,
      /rounded-xl/,
      `${file} should not use rounded-xl`,
    );
  }
});
