import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const readSource = (path) =>
  readFileSync(new URL(path, import.meta.url), 'utf8');

test('Classic benefit hook uses the registered benefit API paths', () => {
  const source = readSource('../useBenefitsData.jsx');
  assert.match(source, /\/api\/benefit\/activities/);
  assert.match(source, /\/api\/benefit\/vouchers/);
  assert.match(source, /\/api\/benefit\/activities\/\$\{activityId\}\/claim/);
  assert.doesNotMatch(source, /promo-code|promo_code/);
});

test('group details preserve the per-user concurrency limit field', () => {
  const source = readSource('../../../helpers/groupDetails.js');
  assert.match(source, /single_user_concurrency_limit/);
  assert.match(
    source,
    /buildGroupDetailsPayload[\s\S]*single_user_concurrency_limit/,
  );
});

test('Classic benefit activity form exposes validity and activity time fields', () => {
  const source = readSource(
    '../../../components/table/benefits/BenefitActivitiesPanel.jsx',
  );
  assert.match(source, /personal_valid_seconds/);
  assert.match(source, /starts_at/);
  assert.match(source, /ends_at/);
  assert.match(source, /活动开始时间/);
  assert.match(source, /活动结束时间/);
  assert.match(source, /个人券有效期/);
});

test('Classic benefit page uses the shared quota formatter export', () => {
  const source = readSource('../../../pages/Benefits/index.jsx');
  assert.match(source, /import \{ renderQuota \} from '\.\.\/\.\.\/helpers'/);
  assert.doesNotMatch(source, /formatQuota/);
});
