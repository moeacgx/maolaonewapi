import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const readSource = (relativePath) =>
  readFileSync(new URL(relativePath, import.meta.url), 'utf8');

const pageSources = [
  ['Dashboard', '../index.jsx'],
  ['NotificationCenter', '../../NotificationCenter/index.jsx'],
  ['Invoice', '../../Invoice/index.jsx'],
  ['SecurityAudit', '../../SecurityAudit/index.jsx'],
  ['Redemption', '../../Redemption/index.jsx'],
  ['Affiliate', '../../Affiliate/index.jsx'],
  ['PersonalSetting', '../../../components/settings/PersonalSetting.jsx'],
];

test('Classic 控制台页面统一使用独立平铺外壳', () => {
  const stylesheet = readSource('../../../index.css');
  const pageLayout = readSource('../../../components/layout/PageLayout.jsx');
  const shellRule = stylesheet.slice(
    stylesheet.indexOf('.classic-console-page {'),
    stylesheet.indexOf('.classic-console-page-container {'),
  );
  const mobileShellRule = stylesheet.slice(
    stylesheet.indexOf('  .classic-console-page {'),
    stylesheet.indexOf('  .classic-console-page-container {'),
  );
  const contentOffsetExpression =
    /var\(--classic-console-header-height\)\s*-\s*var\(--classic-console-content-padding\)/;

  assert.match(stylesheet, /\.classic-console-page\s*\{/);
  assert.match(stylesheet, /\.classic-console-page-container\s*\{/);
  assert.match(
    stylesheet,
    /\.classic-console-page-container[\s\S]*?max-width:\s*1440px/,
  );
  assert.match(
    stylesheet,
    /\.classic-console-dashboard-container[\s\S]*?max-width:\s*none/,
  );
  assert.match(
    stylesheet,
    /\.classic-console-dashboard-container[\s\S]*?margin:\s*0;/,
  );
  assert.match(shellRule, /--classic-console-header-height:\s*64px/);
  assert.match(shellRule, /--classic-console-content-padding:\s*24px/);
  assert.match(shellRule, contentOffsetExpression);
  assert.match(
    shellRule,
    /padding:\s*calc\([\s\S]*?var\(--classic-console-header-height\)[\s\S]*?var\(--classic-console-content-padding\)/,
  );
  assert.match(mobileShellRule, /--classic-console-content-padding:\s*5px/);
  assert.match(mobileShellRule, contentOffsetExpression);
  assert.match(
    mobileShellRule,
    /padding-top:\s*calc\([\s\S]*?var\(--classic-console-header-height\)[\s\S]*?var\(--classic-console-content-padding\)/,
  );
  assert.match(
    pageLayout,
    /padding: shouldInnerPadding \? \(isMobile \? '5px' : '24px'\) : '0'/,
  );

  for (const [name, path] of pageSources) {
    const source = readSource(path);
    assert.match(
      source,
      /className=['"][^'"]*\bclassic-console-page\b[^'"]*['"]|className=\{['"]classic-console-page['"]\}/,
      `${name} 应使用统一页面外壳`,
    );
    assert.match(
      source,
      /classic-console-page-container/,
      `${name} 应使用统一内容容器`,
    );
  }
});

test('Dashboard 作为宽屏概览页不再居中收窄', () => {
  const source = readSource('../index.jsx');
  const stylesheet = readSource('../../../index.css');

  assert.match(source, /classic-console-dashboard-container/);
  assert.match(
    stylesheet,
    /\.classic-console-dashboard-container[\s\S]*?max-width:\s*none/,
  );
  assert.match(
    stylesheet,
    /\.classic-console-dashboard-container[\s\S]*?margin:\s*0;/,
  );
});

test('Classic 页面外壳移除各自的宽度和重复居中规则', () => {
  const redemption = readSource('../../Redemption/index.jsx');
  const personalSetting = readSource(
    '../../../components/settings/PersonalSetting.jsx',
  );
  const pageSourcesToNormalize = [
    '../../NotificationCenter/index.jsx',
    '../../Invoice/index.jsx',
    '../../SecurityAudit/index.jsx',
    '../../Affiliate/index.jsx',
  ];

  assert.doesNotMatch(redemption, /mt-\[60px\]|max-w-/);
  assert.doesNotMatch(
    personalSetting,
    /<div className=['"]flex justify-center['"]>/,
  );
  assert.doesNotMatch(personalSetting, /max-w-7xl\s+mx-auto/);

  for (const path of pageSourcesToNormalize) {
    assert.doesNotMatch(
      readSource(path),
      /mx-auto mt-\[60px\]|max-w-7xl mx-auto/,
    );
  }
});

test('通知、发票和安全审计保留业务 Card 作为内部内容分组', () => {
  for (const path of [
    '../../NotificationCenter/index.jsx',
    '../../Invoice/index.jsx',
    '../../SecurityAudit/index.jsx',
  ]) {
    assert.match(readSource(path), /<Card[\s\S]*?>/);
  }
});
