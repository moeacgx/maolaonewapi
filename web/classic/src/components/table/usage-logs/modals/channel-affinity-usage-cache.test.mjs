import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildChannelAffinityUsageCacheTarget,
  hasChannelAffinityUsageCacheIdentity,
  hasChannelAffinityUsageCacheMetric,
} from './channel-affinity-usage-cache.js';

test('渠道亲和性日志字段映射保留 key_fp 并允许 reason 回退规则名', () => {
  const target = buildChannelAffinityUsageCacheTarget({
    reason: 'prompt-cache',
    using_group: 'default',
    key_hint: 'sess...tion',
    key_fp: 'a1b2c3d4',
  });

  assert.deepEqual(target, {
    rule_name: 'prompt-cache',
    using_group: 'default',
    key_hint: 'sess...tion',
    key_fp: 'a1b2c3d4',
  });
  assert.equal(hasChannelAffinityUsageCacheIdentity(target), true);
});

test('缺失 key_fp 不发起详情请求，保持真实空态条件', () => {
  const target = buildChannelAffinityUsageCacheTarget({
    rule_name: 'prompt-cache',
    key_hint: 'sess...tion',
  });

  assert.equal(hasChannelAffinityUsageCacheIdentity(target), false);
});

test('支持的 provider 保留显式零值，缺失字段不伪造统计', () => {
  const stats = { prompt_tokens: 0, cached_tokens: 0 };

  assert.equal(
    hasChannelAffinityUsageCacheMetric(stats, 'cached_tokens', true),
    true,
  );
  assert.equal(
    hasChannelAffinityUsageCacheMetric(stats, 'prompt_cache_hit_tokens', true),
    false,
  );
  assert.equal(
    hasChannelAffinityUsageCacheMetric({}, 'cached_tokens', true),
    false,
  );
  assert.equal(
    hasChannelAffinityUsageCacheMetric(stats, 'cached_tokens', false),
    false,
  );
});
