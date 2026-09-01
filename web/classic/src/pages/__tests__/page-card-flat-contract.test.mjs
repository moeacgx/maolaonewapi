import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const root = dirname(fileURLToPath(import.meta.url));
const pageSource = (page) =>
  readFileSync(resolve(root, '..', page, 'index.jsx'), 'utf8');
const componentSource = (component) =>
  readFileSync(resolve(root, '..', '..', component), 'utf8');
const css = readFileSync(resolve(root, '..', '..', 'index.css'), 'utf8');

const pages = {
  dashboard: pageSource('Dashboard'),
  notificationCenter: pageSource('NotificationCenter'),
  invoice: pageSource('Invoice'),
  securityAudit: pageSource('SecurityAudit'),
  redemption: pageSource('Redemption'),
  affiliate: pageSource('Affiliate'),
  channelObservability: pageSource('ChannelObservability'),
  personalSetting: componentSource('components/settings/PersonalSetting.jsx'),
};

test('Dashboard remains the flat visual reference', () => {
  assert.match(pages.dashboard, /<Dashboard \/>/);
  assert.doesNotMatch(pages.dashboard, /<Card[\s\S]*<Dashboard \/>/);
});

test('NotificationCenter and SecurityAudit mark only their page Card as flat', () => {
  for (const name of ['notificationCenter', 'securityAudit']) {
    assert.match(pages[name], /<Card[^>]*className='classic-flat-page'/);
  }
  assert.doesNotMatch(pages.invoice, /classic-flat-page/);
});

test('Invoice uses a transparent glass card and a full-width right-pinned table', () => {
  assert.match(pages.invoice, /className='classic-glass-card'/);
  assert.match(pages.invoice, /scroll=\{\{ x: '100%' \}\}/);
  assert.match(pages.invoice, /title: t\('操作'\),[\s\S]*fixed: 'right'/);
  assert.match(
    css,
    /\.classic-glass-card\.semi-card\s*\{[\s\S]*background:[\s\S]*transparent[\s\S]*backdrop-filter:\s*blur\(/,
  );
});

test('flat page CSS removes the outer Card box without changing inner Cards', () => {
  assert.match(css, /\.classic-flat-page\.semi-card\s*\{/);
  assert.match(
    css,
    /\.classic-flat-page\.semi-card\s*\{[\s\S]*background:\s*transparent;[\s\S]*border:\s*0;[\s\S]*box-shadow:\s*none;/,
  );
  assert.match(pages.notificationCenter, /<Card\n\s+key=\{task\.id\}/);
  assert.match(pages.notificationCenter, /<Tabs type='line'/);
  assert.match(pages.invoice, /<Table[\s\S]*scroll=\{\{ x: '100%' \}\}/);
  assert.match(pages.securityAudit, /<Tabs type='line'/);
});

test('flat page body padding overrides only the direct semi-card-body child and beats the global !important rule', () => {
  // The global `.semi-card-body { padding: 10px !important; }` rule (search
  // "semi-ui 组件自定义样式" in index.css) always wins over a plain-specificity
  // or non-important declaration. The page-level override must therefore use
  // `!important` with higher specificity than that single-class global rule,
  // which a three-class compound + child selector guarantees regardless of
  // source order.
  assert.match(
    css,
    /\.classic-flat-page\.semi-card\s*>\s*\.semi-card-body\s*\{[^}]*padding:\s*16px\s*!important;[^}]*\}/,
    'the 16px override must target .classic-flat-page.semi-card > .semi-card-body with !important',
  );

  // NotificationCenter renders per-item business Cards (tasks/bots/deliveries)
  // as descendants of its `classic-flat-page` root Card (inside Tabs/Spin).
  // A bare descendant selector like `.classic-flat-page .semi-card-body` (or
  // `.classic-flat-page.semi-card .semi-card-body`, without `>`) would leak
  // the page-level 16px padding into those unrelated nested Cards. Only the
  // direct-child form scopes the override to the page's own root Card body.
  assert.doesNotMatch(
    css,
    /\.classic-flat-page(\.semi-card)?\s+\.semi-card-body/,
    'must not reintroduce a descendant selector that leaks into nested business Cards',
  );

  assert.match(pages.notificationCenter, /<Card key=\{delivery\.id\}>/);
});

test('all console pages keep the shared page and container shell', () => {
  for (const name of Object.keys(pages)) {
    assert.match(pages[name], /classic-console-page/);
    assert.match(pages[name], /classic-console-page-container/);
  }
});

test('shared console container stays full width for all shell pages', () => {
  assert.match(
    css,
    /\.classic-console-page-container\s*\{[\s\S]*?max-width:\s*none;[\s\S]*?margin:\s*0;/,
  );
  assert.doesNotMatch(
    pages.channelObservability,
    /mx-auto mt-\[60px\]|max-w-\[1600px\]/,
  );
});
