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

import { API } from '../../helpers/api';
import { extractGroupDetailsResponse } from '../../helpers/groupDetails';
import { cleanSecurityAuditFilter } from './eventFilter';

export { cleanSecurityAuditFilter } from './eventFilter';

const API_ROOT = '/api/security-audit';

const unwrap = (response) => {
  const payload = response?.data;
  if (!payload?.success) {
    throw new Error(payload?.message || 'Request failed');
  }
  return payload.data;
};

const requestConfig = {
  disableDuplicate: true,
  skipErrorHandler: true,
};

const UPSTREAM_POLICY_TARGET_TYPES = new Set(['all', 'channels', 'groups']);
const REQUEST_ARCHIVE_EVENT_SOURCES = new Set([
  'prompt_guard',
  'sensitive_word',
  'upstream_policy',
]);

const normalizePositiveIds = (values) =>
  Array.from(
    new Set(
      (Array.isArray(values) ? values : [])
        .map(Number)
        .filter((value) => Number.isInteger(value) && value > 0),
    ),
  );

const normalizeGroupCodes = (values) =>
  Array.from(
    new Set(
      (Array.isArray(values) ? values : [])
        .map((value) => String(value || '').trim())
        .filter((value) => value && value.toLowerCase() !== 'auto'),
    ),
  );

const normalizeRequestArchiveEventSources = (values) =>
  Array.from(
    new Set(
      (Array.isArray(values) ? values : [])
        .map((value) =>
          String(value || '')
            .trim()
            .toLowerCase(),
        )
        .filter((value) => REQUEST_ARCHIVE_EVENT_SOURCES.has(value)),
    ),
  );

export const builtinPolicyConfigToDraft = (policy = {}) => ({
  ...policy,
  upstream_policy_target_type: UPSTREAM_POLICY_TARGET_TYPES.has(
    policy.upstream_policy_target_type,
  )
    ? policy.upstream_policy_target_type
    : 'all',
  upstream_policy_channel_ids: normalizePositiveIds(
    policy.upstream_policy_channel_ids,
  ),
  upstream_policy_group_codes: normalizeGroupCodes(
    policy.upstream_policy_group_codes,
  ),
  cyber_session_block_enabled: policy.cyber_session_block_enabled === true,
  cyber_session_block_ttl_seconds: Number.isFinite(
    Number(policy.cyber_session_block_ttl_seconds),
  )
    ? Number(policy.cyber_session_block_ttl_seconds)
    : 3600,
  cyber_policy_auto_ban_exempt_group_codes: normalizeGroupCodes(
    policy.cyber_policy_auto_ban_exempt_group_codes,
  ),
});

export const configToDraft = (config) => ({
  ...config,
  mode: config?.effective_mode || 'off',
  scanners: config?.scanners || [],
  group_ids: config?.group_ids || [],
  endpoints: (config?.endpoints || []).map((endpoint) => ({
    ...endpoint,
    token_action: 'keep',
    token: '',
  })),
});

export const requestArchiveConfigToDraft = (config) => ({
  ...config,
  archive_scope: config?.archive_scope || 'all_requests',
  event_channel_ids: normalizePositiveIds(config?.event_channel_ids),
  event_group_codes: normalizeGroupCodes(config?.event_group_codes),
  event_sources: normalizeRequestArchiveEventSources(config?.event_sources),
  max_body_bytes: config?.max_body_bytes ?? 67108864,
  queue_max_bytes: config?.queue_max_bytes ?? 1073741824,
  targets: (config?.targets || []).map((target) => ({
    ...target,
    access_key_action: 'keep',
    access_key: '',
    secret_key_action: 'keep',
    secret_key: '',
  })),
});

const endpointToPayload = (endpoint) => ({
  id: String(endpoint.id || '').trim(),
  name: String(endpoint.name || '').trim(),
  protocol: 'openai_compatible',
  base_url: String(endpoint.base_url || '').trim(),
  model: String(endpoint.model || '').trim(),
  timeout_ms: Number(endpoint.timeout_ms),
  input_limit: Number(endpoint.input_limit),
  enabled: endpoint.enabled === true,
  token_action: endpoint.token_action || 'keep',
  ...(endpoint.token_action === 'replace'
    ? { token: String(endpoint.token || '').trim() }
    : {}),
});

const requestArchiveTargetToPayload = (target) => ({
  id: String(target.id || '').trim(),
  name: String(target.name || '').trim(),
  type: target.type || 'local',
  enabled: target.enabled === true,
  local_path: String(target.local_path || '').trim(),
  endpoint: String(target.endpoint || '').trim(),
  bucket: String(target.bucket || '').trim(),
  region: String(target.region || '').trim(),
  prefix: String(target.prefix || '').trim(),
  path_style: target.path_style === true,
  access_key_action: target.access_key_action || 'keep',
  ...(target.access_key_action === 'replace'
    ? { access_key: String(target.access_key || '').trim() }
    : {}),
  secret_key_action: target.secret_key_action || 'keep',
  ...(target.secret_key_action === 'replace'
    ? { secret_key: String(target.secret_key || '').trim() }
    : {}),
});

export const draftToUpdatePayload = (draft) => ({
  expected_version: draft.config_version,
  enabled: draft.mode !== 'off',
  blocking_enabled: draft.mode === 'blocking',
  store_pass_events: draft.store_pass_events === true,
  strategy: 'priority',
  worker_count: Number(draft.worker_count),
  queue_capacity: Number(draft.queue_capacity),
  retention_days: Number(draft.retention_days),
  scanners: draft.scanners || [],
  all_groups: draft.all_groups === true,
  group_ids: draft.all_groups ? [] : draft.group_ids || [],
  endpoints: (draft.endpoints || []).map(endpointToPayload),
});

export const requestArchiveDraftToUpdatePayload = (draft) => ({
  expected_version: Number(draft.config_version),
  enabled: draft.enabled === true,
  archive_scope: draft.archive_scope || 'all_requests',
  event_channel_ids: normalizePositiveIds(draft.event_channel_ids),
  event_group_codes: normalizeGroupCodes(draft.event_group_codes),
  event_sources: normalizeRequestArchiveEventSources(draft.event_sources),
  active_target_id: String(draft.active_target_id || ''),
  retention_days: Number(draft.retention_days),
  worker_count: Number(draft.worker_count),
  queue_capacity: Number(draft.queue_capacity),
  max_body_bytes: Number(draft.max_body_bytes),
  queue_max_bytes: Number(draft.queue_max_bytes),
  targets: (draft.targets || []).map(requestArchiveTargetToPayload),
});

export const getSecurityAuditConfig = async () =>
  unwrap(await API.get(`${API_ROOT}/config`, requestConfig));

export const getRequestArchiveConfig = async () =>
  unwrap(await API.get(`${API_ROOT}/request-archive/config`, requestConfig));

export const updateRequestArchiveConfig = async (draft) =>
  unwrap(
    await API.put(
      `${API_ROOT}/request-archive/config`,
      requestArchiveDraftToUpdatePayload(draft),
      { skipErrorHandler: true },
    ),
  );

export const getRequestArchiveRuntime = async () =>
  unwrap(await API.get(`${API_ROOT}/request-archive/runtime`, requestConfig));

export const probeRequestArchiveTarget = async (target) =>
  unwrap(
    await API.post(
      `${API_ROOT}/request-archive/targets/probe`,
      requestArchiveTargetToPayload(target),
      { skipErrorHandler: true },
    ),
  );

export const getSecurityAuditBuiltinPolicy = async () =>
  builtinPolicyConfigToDraft(
    unwrap(await API.get(`${API_ROOT}/builtin-policy`, requestConfig)),
  );

export const getSecurityAuditBuiltinPolicyChannels = async () => {
  const data = unwrap(
    await API.get(`${API_ROOT}/builtin-policy/channels`, requestConfig),
  );
  const channels = Array.isArray(data) ? data : data?.channels;
  return Array.isArray(channels) ? channels : [];
};

export const getSecurityAuditBuiltinPolicyGroups = async () => {
  const data = unwrap(
    await API.get(`${API_ROOT}/builtin-policy/groups`, requestConfig),
  );
  return Array.isArray(data) ? data : [];
};

export const updateSecurityAuditBuiltinPolicy = async (policy) =>
  builtinPolicyConfigToDraft(
    unwrap(
      await API.put(
        `${API_ROOT}/builtin-policy`,
        {
          expected_version: policy.config_version,
          upstream_policy_enabled: policy.upstream_policy_enabled === true,
          upstream_policy_target_type: UPSTREAM_POLICY_TARGET_TYPES.has(
            policy.upstream_policy_target_type,
          )
            ? policy.upstream_policy_target_type
            : 'all',
          upstream_policy_channel_ids: normalizePositiveIds(
            policy.upstream_policy_channel_ids,
          ),
          upstream_policy_group_codes: normalizeGroupCodes(
            policy.upstream_policy_group_codes,
          ),
          sensitive_word_audit_enabled:
            policy.sensitive_word_audit_enabled === true,
          cyber_session_block_enabled:
            policy.cyber_session_block_enabled === true,
          cyber_session_block_ttl_seconds:
            policy.cyber_session_block_ttl_seconds,
          cyber_policy_auto_ban_enabled:
            policy.cyber_policy_auto_ban_enabled === true,
          cyber_policy_auto_ban_exempt_group_codes: normalizeGroupCodes(
            policy.cyber_policy_auto_ban_exempt_group_codes,
          ),
          cyber_policy_ban_threshold: policy.cyber_policy_ban_threshold,
          cyber_policy_violation_window_hours:
            policy.cyber_policy_violation_window_hours,
          check_sensitive_enabled: policy.check_sensitive_enabled === true,
          check_sensitive_on_prompt_enabled:
            policy.check_sensitive_on_prompt_enabled === true,
          sensitive_rules: policy.sensitive_rules || '{"rules":[]}',
          sensitive_rule_channel_ids: policy.sensitive_rule_channel_ids || '[]',
        },
        { skipErrorHandler: true },
      ),
    ),
  );

export const updateSecurityAuditConfig = async (draft) =>
  unwrap(
    await API.put(`${API_ROOT}/config`, draftToUpdatePayload(draft), {
      skipErrorHandler: true,
    }),
  );

export const getSecurityAuditRuntime = async () =>
  unwrap(await API.get(`${API_ROOT}/runtime`, requestConfig));

export const getSecurityAuditGroups = async () => {
  const response = await API.get('/api/group/details', requestConfig);
  if (!response?.data?.success) {
    throw new Error(response?.data?.message || 'Request failed');
  }
  return extractGroupDetailsResponse(response.data) || [];
};

export const getSecurityAuditEvents = async (filter, page, pageSize) =>
  unwrap(
    await API.get(`${API_ROOT}/events`, {
      ...requestConfig,
      params: {
        ...cleanSecurityAuditFilter(filter),
        page,
        page_size: pageSize,
      },
    }),
  );

export const getSecurityAuditEvent = async (id) =>
  unwrap(
    await API.get(`${API_ROOT}/events/${id}`, {
      ...requestConfig,
      skipErrorHandler: true,
    }),
  );

export const deleteSecurityAuditEvent = async (id) =>
  unwrap(
    await API.delete(`${API_ROOT}/events/${id}`, {
      skipErrorHandler: true,
    }),
  );

export const batchDeleteSecurityAuditEvents = async (ids) =>
  unwrap(
    await API.post(
      `${API_ROOT}/events/batch-delete`,
      { ids },
      { skipErrorHandler: true },
    ),
  );

export const previewSecurityAuditDelete = async (filter) =>
  unwrap(
    await API.post(
      `${API_ROOT}/events/delete-preview`,
      cleanSecurityAuditFilter(filter),
      { skipErrorHandler: true },
    ),
  );

export const deleteSecurityAuditEventsByFilter = async (filter, preview) =>
  unwrap(
    await API.post(
      `${API_ROOT}/events/delete-by-filter`,
      {
        filter: cleanSecurityAuditFilter(filter),
        confirmation_token: preview.confirmation_token,
        confirm: true,
      },
      { skipErrorHandler: true },
    ),
  );

export const probeSecurityAuditEndpoint = async (endpoint) => {
  const payload = endpointToPayload(endpoint);
  return unwrap(
    await API.post(
      `${API_ROOT}/endpoints/probe`,
      {
        endpoint_id: payload.id,
        name: payload.name,
        base_url: payload.base_url,
        model: payload.model,
        timeout_ms: payload.timeout_ms,
        input_limit: payload.input_limit,
        token_action: payload.token_action,
        ...(payload.token ? { token: payload.token } : {}),
      },
      { skipErrorHandler: true },
    ),
  );
};
