import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const readSource = () =>
  readFileSync(
    new URL('../PricingTemplateHeader.jsx', import.meta.url),
    'utf8',
  );

test('mobile menu exposes account, notification, language, and theme controls', () => {
  const source = readSource();
  const mobileMenu = source.slice(
    source.indexOf("id='classic-pricing-template-mobile-menu'"),
  );

  assert.match(mobileMenu, /classic-pricing-template-mobile-controls/);
  assert.match(mobileMenu, /<UserArea[\s\S]*isMobile=\{true\}/);
  assert.match(mobileMenu, /<NotificationButton/);
  assert.match(mobileMenu, /<LanguageSelector/);
  assert.match(mobileMenu, /<ThemeToggle/);
});

test('mobile top bar only keeps the menu toggle', () => {
  const source = readSource();
  const mobileActions = source.slice(
    source.indexOf("className='classic-pricing-template-mobile-actions'"),
    source.indexOf("id='classic-pricing-template-mobile-menu'"),
  );

  assert.match(mobileActions, /classic-pricing-template-menu-button/);
  assert.doesNotMatch(mobileActions, /<ThemeToggle/);
  assert.doesNotMatch(mobileActions, /<NotificationButton/);
  assert.doesNotMatch(mobileActions, /<LanguageSelector/);
});
