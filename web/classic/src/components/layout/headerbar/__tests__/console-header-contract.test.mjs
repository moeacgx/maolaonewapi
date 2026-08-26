import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import {
  getConsoleSidebarToggleState,
  isConsoleHomePath,
  shouldRenderConsoleSidebarToggle,
} from '../consoleHeaderBehavior.js';

const readSource = (relativePath) =>
  readFileSync(new URL(relativePath, import.meta.url), 'utf8');

test('控制台首页识别 /console、/console/，且 query/hash 由 pathname 自然忽略', () => {
  assert.equal(isConsoleHomePath('/console'), true);
  assert.equal(isConsoleHomePath('/console/'), true);

  const queryLocation = new URL('https://example.test/console?tab=usage');
  const hashLocation = new URL('https://example.test/console/#overview');

  assert.equal(isConsoleHomePath(queryLocation.pathname), true);
  assert.equal(isConsoleHomePath(hashLocation.pathname), true);
  assert.equal(isConsoleHomePath('/console/channel'), false);
});

test('侧栏按钮在桌面和移动端使用各自的打开状态', () => {
  assert.deepEqual(
    getConsoleSidebarToggleState({
      isMobile: false,
      drawerOpen: false,
      collapsed: true,
    }),
    { isOpen: false, ariaLabel: '打开侧边栏' },
  );
  assert.deepEqual(
    getConsoleSidebarToggleState({
      isMobile: false,
      drawerOpen: false,
      collapsed: false,
    }),
    { isOpen: true, ariaLabel: '关闭侧边栏' },
  );
  assert.deepEqual(
    getConsoleSidebarToggleState({
      isMobile: true,
      drawerOpen: true,
      collapsed: true,
    }),
    { isOpen: true, ariaLabel: '关闭侧边栏' },
  );
});

test('桌面侧栏入口只在显式允许时出现，移动端保持默认入口', () => {
  assert.equal(
    shouldRenderConsoleSidebarToggle({
      isConsoleRoute: true,
      isMobile: false,
      showOnDesktop: false,
    }),
    false,
  );
  assert.equal(
    shouldRenderConsoleSidebarToggle({
      isConsoleRoute: true,
      isMobile: false,
      showOnDesktop: true,
    }),
    true,
  );
  assert.equal(
    shouldRenderConsoleSidebarToggle({
      isConsoleRoute: true,
      isMobile: true,
      showOnDesktop: false,
    }),
    true,
  );
});

test('PageLayout 将唯一侧栏状态传给 HeaderBar 和 SiderBar', () => {
  const headerBarSource = readSource('../index.jsx');
  const templateSource = readSource('../PricingTemplateHeader.jsx');
  const pageLayoutSource = readSource('../../PageLayout.jsx');
  const siderBarSource = readSource('../../SiderBar.jsx');
  const headerHookSource = readSource('../../../../hooks/common/useHeaderBar.js');

  assert.match(headerBarSource, /isConsoleHomePath\(location\.pathname\)/);
  assert.match(headerBarSource, /consoleSidebarToggle=\{/);
  assert.match(headerBarSource, /showOnDesktop/);
  const headerHookResult = headerBarSource.match(
    /const \{([\s\S]*?)\} = useHeaderBar\(/,
  );
  assert.ok(headerHookResult);
  assert.doesNotMatch(headerHookResult[1], /\bcollapsed\b/);
  assert.match(templateSource, /const \[mobileOpen, setMobileOpen\] = React\.useState\(false\)/);
  assert.match(templateSource, /setMobileOpen\(\(open\) => !open\)/);
  assert.match(templateSource, /isConsoleMode && consoleSidebarToggle/);
  assert.match(
    pageLayoutSource,
    /const \[collapsed, toggleCollapsed, setCollapsed\] = useSidebarCollapsed\(\);/,
  );
  assert.match(pageLayoutSource, /<HeaderBar[\s\S]*?collapsed=\{collapsed\}/);
  assert.match(
    pageLayoutSource,
    /<SiderBar[\s\S]*?collapsed=\{collapsed\}[\s\S]*?onSidebarToggle=\{toggleCollapsed\}/,
  );
  assert.doesNotMatch(siderBarSource, /useSidebarCollapsed/);
  assert.doesNotMatch(headerHookSource, /useSidebarCollapsed/);
});
