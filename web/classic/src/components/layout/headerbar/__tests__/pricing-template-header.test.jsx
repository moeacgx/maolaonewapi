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

import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import React from 'react';
import { getCustomNavIconComponent } from '../../../../helpers/customNav';
import { isPricingHeaderScrolled } from '../pricingHeaderScroll';

test('模型广场顶栏把自定义图标作为组件渲染', () => {
  const Icon = getCustomNavIconComponent('Home');
  const headerSource = readFileSync(
    new URL('../PricingTemplateHeader.jsx', import.meta.url),
    'utf8',
  );

  expect(React.isValidElement(Icon)).toBe(false);
  expect(React.isValidElement(<Icon size={16} />)).toBe(true);
  expect(headerSource).toContain('getCustomNavIconComponent(link.iconName)');
});

test('模型广场顶栏会跟随实际的布局滚动容器收缩', () => {
  const headerSource = readFileSync(
    new URL('../PricingTemplateHeader.jsx', import.meta.url),
    'utf8',
  );

  expect(isPricingHeaderScrolled([{ scrollTop: 0 }, { scrollTop: 21 }])).toBe(
    true,
  );
  expect(
    isPricingHeaderScrolled([{ scrollTop: 0 }, { scrollTop: 0 }], 21),
  ).toBe(true);
  expect(isPricingHeaderScrolled([{ scrollTop: 20 }])).toBe(false);
  expect(headerSource).toContain(
    "document.querySelectorAll('section.semi-layout')",
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

  expect(headerSource).toContain('classic-pricing-template-user-area');
  expect(headerSource).toContain('PRICING_TEMPLATE_MOBILE_BREAKPOINT = 1100');
  expect(stylesheet).toContain('max-width: 1440px;');
  expect(stylesheet).toContain('max-width: min(1440px, calc(100% - 32px));');
  expect(stylesheet).toMatch(
    /\.classic-pricing-template-user-area \.semi-button\s*\{[\s\S]*?width:\s*auto !important;[\s\S]*?min-width:\s*max-content !important;/,
  );
  expect(stylesheet).toMatch(
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

  expect(headerBarSource).toContain('isConsoleShellRoute');
  expect(headerBarSource).toContain(
    "location.pathname === '/pricing' || isConsoleShellRoute",
  );
  expect(headerBarSource).toContain('consoleSidebarToggle={');
  expect(headerBarSource).toContain('isConsoleMode={isConsoleShellRoute}');
  expect(headerBarSource).toContain(
    "location.pathname === '/notification-center'",
  );
  expect(headerBarSource).not.toContain('showOnDesktop');
  expect(templateSource).toContain('classic-pricing-template-console-toggle');
  expect(templateSource).toContain('setMobileOpen((open) => !open)');
  expect(mobileMenuSource).toContain('shouldRenderConsoleSidebarToggle');
  expect(mobileMenuSource).not.toContain('showOnDesktop');
});
