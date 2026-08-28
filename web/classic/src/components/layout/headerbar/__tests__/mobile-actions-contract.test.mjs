import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const root = dirname(fileURLToPath(import.meta.url));

const readSource = (relativePath) =>
  readFileSync(resolve(root, relativePath), 'utf8');

test('移动端模板顶栏保留通知和个人中心入口', () => {
  const headerSource = readSource('../PricingTemplateHeader.jsx');
  const userSource = readSource('../UserArea.jsx');
  const notificationSource = readSource('../NotificationButton.jsx');
  const mobileStart = headerSource.indexOf(
    "<div className='classic-pricing-template-mobile-actions'>",
  );
  const mobileEnd = headerSource.indexOf(
    "</div>\n          </nav>",
    mobileStart,
  );

  assert.ok(mobileStart >= 0, '应存在移动端 actions 容器');
  assert.ok(mobileEnd > mobileStart, '应能定位移动端 actions 容器边界');

  const mobileActions = headerSource.slice(mobileStart, mobileEnd);
  assert.match(mobileActions, /<NotificationButton/);
  assert.match(mobileActions, /unreadCount=\{unreadCount\}/);
  assert.match(mobileActions, /onNoticeOpen=\{onNoticeOpen\}/);
  assert.match(mobileActions, /<UserArea/);
  assert.match(mobileActions, /isMobile=\{true\}/);
  assert.match(mobileActions, /userState=\{userState\}/);
  assert.match(mobileActions, /logout=\{logout\}/);
  assert.match(notificationSource, /'aria-label': t\('系统公告'\)/);
  assert.match(notificationSource, /title: t\('系统公告'\)/);
  assert.match(userSource, /aria-label=\{t\('个人中心'\)\}/);
  assert.match(userSource, /title=\{t\('个人中心'\)\}/);
});

test('移动端模板菜单与控制台侧栏继续使用独立状态', () => {
  const headerSource = readSource('../PricingTemplateHeader.jsx');
  const shellSource = readSource('../index.jsx');

  assert.match(
    headerSource,
    /const \[mobileOpen, setMobileOpen\] = React\.useState\(false\)/,
  );
  assert.match(headerSource, /setMobileOpen\(\(open\) => !open\)/);
  assert.match(shellSource, /drawerOpen=\{drawerOpen\}/);
  assert.match(shellSource, /onToggle=\{handleMobileMenuToggle\}/);
});
