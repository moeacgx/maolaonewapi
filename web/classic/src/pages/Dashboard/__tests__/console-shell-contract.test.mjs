import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const readSource = (relativePath) =>
  readFileSync(new URL(relativePath, import.meta.url), 'utf8');

test('控制台首页使用独立外壳并保留问候与操作入口', () => {
  const dashboardPage = readSource('../index.jsx');
  const dashboardSource = readSource('../../../components/dashboard/index.jsx');
  const dashboardHeader = readSource(
    '../../../components/dashboard/DashboardHeader.jsx',
  );
  const stylesheet = readSource('../../../index.css');
  const heroRule = stylesheet.slice(
    stylesheet.indexOf('.classic-console-dashboard-hero'),
    stylesheet.indexOf('.classic-console-dashboard-greeting'),
  );

  assert.match(dashboardPage, /classic-console-dashboard-page/);
  assert.match(dashboardPage, /classic-console-dashboard-container/);
  assert.match(dashboardHeader, /classic-console-dashboard-hero/);
  assert.match(dashboardHeader, /\{getGreeting\}/);
  assert.match(dashboardHeader, /showSearchModal/);
  assert.match(dashboardHeader, /onClick=\{refresh\}/);
  assert.match(dashboardSource, /<StatsCards/);
  assert.match(dashboardSource, /dashboardData\.isAdminUser/);
  assert.match(dashboardSource, /<PerformanceOverviewPanel/);
  assert.match(dashboardSource, /<RevenuePanel/);
  assert.doesNotMatch(heroRule, /border|border-radius|background|box-shadow/);
});
