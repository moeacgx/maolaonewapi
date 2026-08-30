import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import { getCustomNavIconComponent } from '../../../../helpers/customNav';
import { isPricingHeaderScrolled } from '../pricingHeaderScroll.js';

test('模型广场顶栏把自定义图标作为组件渲染', () => {
  const Icon = getCustomNavIconComponent('Home');
  const headerSource = readFileSync(
    new URL('../PricingTemplateHeader.jsx', import.meta.url),
    'utf8',
  );

  assert.equal(React.isValidElement(Icon), false);
  assert.equal(React.isValidElement(React.createElement(Icon, { size: 16 })), true);
  assert.match(headerSource, /getCustomNavIconComponent\(link\.iconName\)/);
});

test('模型广场顶栏会跟随实际的布局滚动容器收缩', () => {
  const headerSource = readFileSync(
    new URL('../PricingTemplateHeader.jsx', import.meta.url),
    'utf8',
  );

  assert.equal(isPricingHeaderScrolled([{ scrollTop: 0 }, { scrollTop: 21 }]), true);
  assert.equal(
    isPricingHeaderScrolled([{ scrollTop: 0 }, { scrollTop: 0 }], 21),
    true,
  );
  assert.equal(isPricingHeaderScrolled([{ scrollTop: 20 }]), false);
  assert.match(
    headerSource,
    /document\.querySelectorAll\('section\.semi-layout'\)/,
  );
});

test('模型广场顶栏在宽屏为用户名区扩展可见空间', () => {
  const headerSource = readFileSync(
    new URL('../PricingTemplateHeader.jsx', import.meta.url),
    'utf8',
  );
  const stylesheet = readFileSync(
    new URL('../PricingTemplateHeader.css', import.meta.url),
    'utf8',
  );

  assert.match(headerSource, /classic-pricing-template-user-area/);
  assert.match(headerSource, /PRICING_TEMPLATE_MOBILE_BREAKPOINT = 1100/);
  assert.match(stylesheet, /max-width: 1440px;/);
  assert.match(stylesheet, /max-width: min\(1440px, calc\(100% - 32px\)\);/);
  assert.match(
    stylesheet,
    /\.classic-pricing-template-user-area \.semi-button\s*\{[\s\S]*?width:\s*auto !important;[\s\S]*?min-width:\s*max-content !important;/,
  );
  assert.match(
    stylesheet,
    /\.classic-pricing-template-user-area\s*\{[\s\S]*?margin-right:\s*-4px;[\s\S]*?margin-left:\s*4px;/,
  );
});

test('控制台所有页面复用模板顶栏且桌面不渲染侧栏按钮', () => {
  const headerBarSource = readFileSync(
    new URL('../index.jsx', import.meta.url),
    'utf8',
  );
  const templateSource = readFileSync(
    new URL('../PricingTemplateHeader.jsx', import.meta.url),
    'utf8',
  );
  const mobileMenuSource = readFileSync(
    new URL('../MobileMenuButton.jsx', import.meta.url),
    'utf8',
  );

  assert.match(headerBarSource, /isConsoleShellRoute/);
  assert.match(
    headerBarSource,
    /location\.pathname === '\/pricing' \|\| isConsoleShellRoute/,
  );
  assert.match(headerBarSource, /consoleSidebarToggle=\{/);
  assert.match(headerBarSource, /isConsoleMode=\{isConsoleShellRoute\}/);
  assert.match(
    headerBarSource,
    /location\.pathname === '\/notification-center'/,
  );
  assert.doesNotMatch(headerBarSource, /showOnDesktop/);
  assert.match(templateSource, /classic-pricing-template-console-toggle/);
  assert.match(templateSource, /setMobileOpen\(\(open\) => !open\)/);
  assert.match(mobileMenuSource, /shouldRenderConsoleSidebarToggle/);
  assert.doesNotMatch(mobileMenuSource, /showOnDesktop/);
});
