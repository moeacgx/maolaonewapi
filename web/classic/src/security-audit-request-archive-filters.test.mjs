import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import { runInNewContext } from 'node:vm';

const root = dirname(fileURLToPath(import.meta.url));
const readSource = (...parts) => readFileSync(resolve(root, ...parts), 'utf8');

const apiSource = readSource('pages/SecurityAudit/api.js');
const tabSource = readSource('pages/SecurityAudit/RequestArchiveTab.jsx');

function loadArchiveTransforms() {
  const helpersStart = apiSource.indexOf(
    'const UPSTREAM_POLICY_TARGET_TYPES = new Set',
  );
  const helpersEnd = apiSource.indexOf('export const getSecurityAuditConfig');
  assert.notEqual(helpersStart, -1);
  assert.notEqual(helpersEnd, -1);

  const context = {};
  const source = apiSource
    .slice(helpersStart, helpersEnd)
    .replaceAll('export const ', 'const ');
  runInNewContext(
    `${source}
globalThis.requestArchiveConfigToDraft = requestArchiveConfigToDraft;
globalThis.requestArchiveDraftToUpdatePayload = requestArchiveDraftToUpdatePayload;`,
    context,
  );
  return context;
}

test('Classic normalizes request archive event filters', () => {
  const { requestArchiveConfigToDraft } = loadArchiveTransforms();
  const draft = JSON.parse(
    JSON.stringify(
      requestArchiveConfigToDraft({
        event_channel_ids: ['9', 2, 9, 0],
        event_group_codes: [' vip ', 'auto', '', 'vip', 'default'],
        event_sources: [
          ' UPSTREAM_POLICY ',
          'unknown',
          'prompt_guard',
          'biological_risk',
          'upstream_policy',
        ],
      }),
    ),
  );

  assert.deepEqual(draft.event_channel_ids, [9, 2]);
  assert.deepEqual(draft.event_group_codes, ['vip', 'default']);
  assert.deepEqual(draft.event_sources, [
    'upstream_policy',
    'prompt_guard',
    'biological_risk',
  ]);
});

test('Classic preserves event filter selections for all-request scope', () => {
  const { requestArchiveDraftToUpdatePayload } = loadArchiveTransforms();
  const payload = JSON.parse(
    JSON.stringify(
      requestArchiveDraftToUpdatePayload({
        config_version: 7,
        enabled: true,
        archive_scope: 'all_requests',
        event_channel_ids: [31],
        event_group_codes: ['vip'],
        event_sources: ['upstream_policy'],
        targets: [],
      }),
    ),
  );

  assert.equal(payload.archive_scope, 'all_requests');
  assert.deepEqual(payload.event_channel_ids, [31]);
  assert.deepEqual(payload.event_group_codes, ['vip']);
  assert.deepEqual(payload.event_sources, ['upstream_policy']);
});

test('Classic request archive filters use the stable backend contract', () => {
  assert.match(apiSource, /event_channel_ids:/);
  assert.match(apiSource, /event_group_codes:/);
  assert.match(apiSource, /event_sources:/);
  assert.match(tabSource, /draft\.event_channel_ids/);
  assert.match(tabSource, /draft\.event_group_codes/);
  assert.match(tabSource, /draft\.event_sources/);
  assert.match(
    tabSource,
    /<Select\.Option key=\{group\.code\} value=\{group\.code\}>/,
  );
});

test('Classic disables filters outside audit-event scope without clearing them', () => {
  assert.match(
    tabSource,
    /const archiveFiltersEnabled = draft\?\.archive_scope === 'audit_events'/,
  );
  assert.match(
    tabSource,
    /disabled=\{!archiveFiltersEnabled \|\| channelsError\}/,
  );
  assert.match(
    tabSource,
    /disabled=\{!archiveFiltersEnabled \|\| groupsError\}/,
  );
  assert.match(tabSource, /disabled=\{!archiveFiltersEnabled\}/);
  assert.doesNotMatch(
    tabSource,
    /onChange=\{\(archive_scope\)[\s\S]{0,240}event_(?:channel_ids|group_codes|sources): \[\]/,
  );
});

test('Classic keeps missing channel and group selections visible', () => {
  assert.match(tabSource, /missing-archive-channel-/);
  assert.match(tabSource, /missing-archive-group-/);
  assert.match(tabSource, /getSecurityAuditBuiltinPolicyChannels\(\)/);
  assert.match(tabSource, /getSecurityAuditBuiltinPolicyGroups\(\)/);
});

test('Classic explains OR within dimensions and AND across dimensions', () => {
  assert.match(
    tabSource,
    /Leave a filter empty to match any value\. Values within one filter use OR; different non-empty filters use AND\./,
  );
  assert.match(tabSource, /value='prompt_guard'/);
  assert.match(tabSource, /value='sensitive_word'/);
  assert.match(tabSource, /value='upstream_policy'/);
  assert.match(tabSource, /value='biological_risk'/);
  assert.match(tabSource, /Official risk control \(cyber_policy\)/);
});
