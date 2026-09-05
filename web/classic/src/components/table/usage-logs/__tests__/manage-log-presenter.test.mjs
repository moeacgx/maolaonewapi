import assert from 'node:assert/strict';
import test from 'node:test';

import { getManageLogSummary } from '../manage-log-presenter.js';

const zh = (key) => key;

test('renders quota management logs in the active locale', () => {
  const log = { type: 3 };

  assert.equal(
    getManageLogSummary(
      log,
      {
        op: { action: 'user.quota_add', params: { quota: '＄0.100000 额度' } },
      },
      zh,
    ),
    '管理员增加用户额度 ＄0.100000 额度',
  );
  assert.equal(
    getManageLogSummary(
      log,
      {
        op: {
          action: 'user.quota_subtract',
          params: { quota: '＄0.050000 额度' },
        },
      },
      zh,
    ),
    '管理员减少用户额度 ＄0.050000 额度',
  );
  assert.equal(
    getManageLogSummary(
      log,
      {
        op: {
          action: 'user.quota_override',
          params: { from: '＄0.100000 额度', to: '＄0.200000 额度' },
        },
      },
      zh,
    ),
    '管理员覆盖用户额度 ＄0.100000 额度 为 ＄0.200000 额度',
  );
});
