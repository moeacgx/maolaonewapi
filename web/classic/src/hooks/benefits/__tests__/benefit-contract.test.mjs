import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const readSource = (path) =>
  readFileSync(new URL(path, import.meta.url), 'utf8');

test('Classic benefit hook uses the registered benefit API paths', () => {
  const source = readSource('../useBenefitsData.jsx');
  assert.match(source, /\/api\/benefit\/activities/);
  assert.match(source, /\/api\/benefit\/vouchers/);
  assert.match(source, /\/api\/benefit\/activities\/\$\{activityId\}\/claim/);
  assert.doesNotMatch(source, /promo-code|promo_code/);
});

test('group details preserve the per-user concurrency limit field', () => {
  const source = readSource('../../../helpers/groupDetails.js');
  assert.match(source, /single_user_concurrency_limit/);
  assert.match(
    source,
    /buildGroupDetailsPayload[\s\S]*single_user_concurrency_limit/,
  );
});

test('Classic benefit activity form exposes validity and activity time fields', () => {
  const source = readSource(
    '../../../components/table/benefits/BenefitActivitiesPanel.jsx',
  );
  assert.match(source, /personal_valid_hours/);
  assert.doesNotMatch(source, /field='personal_valid_seconds'/);
  assert.match(source, /starts_at/);
  assert.match(source, /ends_at/);
  assert.match(source, /活动开始时间/);
  assert.match(source, /活动结束时间/);
  assert.match(source, /个人券有效期/);
  assert.match(source, /个人券有效期（小时）/);
});

test('Classic marketing benefits keeps visual hierarchy and edits activities in a side sheet', () => {
  const pageSource = readSource('../../../pages/Redemption/index.jsx');
  const panelSource = readSource(
    '../../../components/table/benefits/BenefitActivitiesPanel.jsx',
  );
  const styles = readSource('../../../index.css');

  assert.match(pageSource, /marketing-benefits-tabs/);
  assert.match(pageSource, /!font-bold/);
  assert.match(panelSource, /<SideSheet/);
  assert.match(panelSource, /<Table/);
  assert.match(panelSource, /title: t\('操作'\)/);
  assert.match(panelSource, /label={t\('总预算（元）'\)}/);
  assert.match(panelSource, /extraText=\{t\(\s*'活动全部券的基础金额/);
  assert.doesNotMatch(panelSource, /field='total_quota'/);
  assert.doesNotMatch(
    panelSource,
    /总预算（分）|美分|固定面额（分）|实付门槛（分）/,
  );
  assert.match(panelSource, /field='total_amount'/);
  assert.match(panelSource, /field='fixed_amount'/);
  assert.match(panelSource, /step=\{0\.01\}/);
  assert.doesNotMatch(panelSource, /总额度（quota）/);
  assert.match(styles, /\.marketing-benefits-tabs/);
});

test('Classic benefit activity keeps one create entry and reopens the editor cleanly', () => {
  const source = readSource(
    '../../../components/table/benefits/BenefitActivitiesPanel.jsx',
  );

  assert.equal((source.match(/<Plus\b/g) || []).length, 1);
  assert.match(
    source,
    /const \[editorSessionKey, setEditorSessionKey\] = useState\(0\)/,
  );
  assert.match(source, /key=\{editorSessionKey\}/);
  assert.match(
    source,
    /const closeEditor = \(\) => \{[\s\S]*formApiRef\.current = null;[\s\S]*setEditorVisible\(false\);/,
  );
  assert.doesNotMatch(source, /formApiRef\.current\?\.reset\(values\)/);
  assert.doesNotMatch(
    source,
    /formApiRef\.current\?\.reset\(toFormValues\(activity\)\)/,
  );
});

test('Classic benefit activity selects an active group by name and submits its stable ID', () => {
  const source = readSource(
    '../../../components/table/benefits/BenefitActivitiesPanel.jsx',
  );

  assert.match(source, /extractGroupDetailsResponse/);
  assert.match(source, /createGroupOptions/);
  assert.match(source, /API\.get\('\/api\/group\/details'\)/);
  assert.match(source, /group\.status === 1/);
  assert.match(source, /value: group\.id/);
  assert.match(source, /group\.name/);
  assert.match(source, /group\.code/);
  assert.doesNotMatch(source, /group\.description/);
  assert.match(source, /field='group_id'/);
  assert.match(source, /<Form\.Select[\s\S]*field='group_id'/);
  assert.match(source, /optionList=\{editorGroupOptions\}/);
  assert.doesNotMatch(source, /<Form\.InputNumber[\s\S]*field='group_id'/);
  assert.doesNotMatch(source, /绑定分组 ID/);
  assert.match(source, /loading=\{groupLoading\}/);
  assert.match(source, /nameCounts/);
  assert.doesNotMatch(source, /group\.description,\s*\]/);
});

test('Classic benefit activity separates fixed and random budget inputs', () => {
  const source = readSource(
    '../../../components/table/benefits/BenefitActivitiesPanel.jsx',
  );
  assert.match(source, /fixedTotalAmount/);
  assert.match(source, /可行总预算范围/);
  assert.match(source, /field='fixed_amount'[\s\S]*field='total_count'/);
  assert.match(source, /amountMode === 'fixed'\s*\? fixedAmount : 0/);
  assert.match(
    source,
    /amountMode === 'random'\s*\? Number\(values\.min_amount/,
  );
});

test('Classic benefit report presents human-readable budget and delivery details', () => {
  const source = readSource(
    '../../../components/table/benefits/BenefitActivitiesPanel.jsx',
  );

  assert.match(source, /BenefitActivityReportView/);
  assert.match(source, /const isDraft = activity\?\.status === 'draft'/);
  assert.match(source, /资金使用进度/);
  assert.match(source, /金额去向/);
  assert.match(source, /reportVouchers/);
  assert.match(source, /已领取用户/);
  assert.doesNotMatch(source, /Object\.entries\(detailData\)/);
});

test('Classic benefit pages keep visible boundaries around independent modules', () => {
  const benefitsPage = readSource('../../../pages/Benefits/index.jsx');
  const redemptionPage = readSource('../../../pages/Redemption/index.jsx');
  const styles = readSource('../../../index.css');

  assert.match(benefitsPage, /classic-console-panel/);
  assert.match(redemptionPage, /classic-console-panel/);
  assert.match(
    styles,
    /\.classic-console-panel\s*\{[\s\S]*border:\s*1px solid var\(--semi-color-border\)/,
  );
  assert.match(
    styles,
    /\.classic-console-panel-header\s*\{[\s\S]*border-bottom:\s*1px solid var\(--semi-color-border\)/,
  );
});

test('Classic benefit activity aggregates row operations into one menu', () => {
  const source = readSource(
    '../../../components/table/benefits/BenefitActivitiesPanel.jsx',
  );

  assert.match(source, /<Dropdown[\s\S]*position='bottomRight'/);
  assert.match(source, /<Dropdown\.Item disabled>\{t\('活动管理'\)\}/);
  assert.match(source, /<Dropdown\.Item disabled>\{t\('数据查看'\)\}/);
  assert.match(source, /aria-label=\{t\('操作'\)\}/);
  assert.doesNotMatch(source, /width: 330/);
});

test('Classic benefit page uses the shared quota formatter export', () => {
  const source = readSource('../../../pages/Benefits/index.jsx');
  assert.match(source, /import \{ renderQuota \} from '\.\.\/\.\.\/helpers'/);
  assert.doesNotMatch(source, /formatQuota/);
});

test('Classic defaults expose benefits and wallet links to the registered route', () => {
  const sidebar = readSource('../../../hooks/common/useSidebar.js');
  const sidebarView = readSource('../../../components/layout/SiderBar.jsx');
  const topup = readSource('../../../components/topup/index.jsx');

  assert.match(sidebar, /personal:\s*\{[\s\S]*?benefits:\s*true/);
  assert.match(
    sidebarView,
    /itemKey:\s*'benefits',[\s\S]*?to:\s*'\/console\/benefits'/,
  );
  assert.match(topup, /<Link[\s\S]*?to='\/console\/benefits'/);
});
