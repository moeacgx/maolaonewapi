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

test('模型广场顶栏为用户名区保留独立的右侧间距', () => {
  const headerSource = readFileSync(
    new URL('../PricingTemplateHeader.jsx', import.meta.url),
    'utf8',
  );
  const stylesheet = readFileSync(
    new URL('../PricingTemplateHeader.css', import.meta.url),
    'utf8',
  );

  expect(headerSource).toContain('classic-pricing-template-user-area');
  expect(stylesheet).toMatch(
    /\.classic-pricing-template-user-area\s*\{[\s\S]*?margin-right:\s*-4px;[\s\S]*?margin-left:\s*4px;/,
  );
});
