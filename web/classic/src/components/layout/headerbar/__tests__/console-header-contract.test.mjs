import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import {
  getConsoleSidebarToggleState,
  isConsolePath,
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

test('控制台路由按路径段匹配，避免误判相邻路径', () => {
  assert.equal(isConsolePath('/console'), true);
  assert.equal(isConsolePath('/console/'), true);
  assert.equal(isConsolePath('/console/channel'), true);
  assert.equal(isConsolePath('/console/extensions/module/page'), true);
  assert.equal(isConsolePath('/consolex'), false);
  assert.equal(isConsolePath('/console-preview'), false);
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

test('桌面不渲染侧栏入口，移动端保持默认入口', () => {
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
    false,
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

  assert.match(
    headerBarSource,
    /location\.pathname === '\/pricing' \|\| isConsoleShellRoute/,
  );
  assert.match(headerBarSource, /consoleSidebarToggle=\{/);
  assert.doesNotMatch(headerBarSource, /showOnDesktop/);
  assert.match(headerBarSource, /isConsoleMode=\{isConsoleShellRoute\}/);
  assert.match(headerBarSource, /location\.pathname === '\/notification-center'/);
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
  assert.match(pageLayoutSource, /isConsolePath\(location\.pathname\)/);
  assert.match(headerHookSource, /isConsolePath\(location\.pathname\)/);
  assert.doesNotMatch(pageLayoutSource, /startsWith\('\/console'\)/);
  assert.doesNotMatch(headerHookSource, /startsWith\('\/console'\)/);
  assert.doesNotMatch(siderBarSource, /useSidebarCollapsed/);
  assert.doesNotMatch(headerHookSource, /useSidebarCollapsed/);
});
