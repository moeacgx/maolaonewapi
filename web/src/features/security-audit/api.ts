/*
Copyright (C) 2023-2026 QuantumNous

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
import {
  normalizeSensitiveGroupCodes,
  normalizeSensitiveRouteIds,
} from '@/features/system-settings/request-limits/sensitive-rule-config'
import { api } from '@/lib/api'

import { cleanSecurityAuditEventFilter } from './event-filter'
import type {
  ApiEnvelope,
  SecurityAuditBuiltinPolicy,
  SecurityAuditBuiltinPolicyUpdate,
  SecurityAuditConfig,
  SecurityAuditConfigDraft,
  SecurityAuditConfigUpdate,
  SecurityAuditDeletePreview,
  SecurityAuditDeleteResult,
  SecurityAuditEndpointDraft,
  SecurityAuditEventDetail,
  SecurityAuditEventFilter,
  SecurityAuditEventPage,
  SecurityAuditGroup,
  SecurityAuditProbeResult,
  SecurityAuditRuntime,
  RequestArchiveConfig,
  RequestArchiveConfigDraft,
  RequestArchiveConfigUpdate,
  RequestArchiveAuditSource,
  RequestArchiveProbeResult,
  RequestArchiveRuntime,
  RequestArchiveTargetDraft,
} from './types'

export { hasSecurityAuditEventFilter } from './event-filter'

const API_ROOT = '/api/security-audit'

const REQUEST_ARCHIVE_AUDIT_SOURCES: readonly RequestArchiveAuditSource[] = [
  'prompt_guard',
  'sensitive_word',
  'upstream_policy',
]

function normalizeRequestArchiveAuditSources(
  value: unknown
): RequestArchiveAuditSource[] {
  if (!Array.isArray(value)) return []
  const allowed = new Set<string>(REQUEST_ARCHIVE_AUDIT_SOURCES)
  return [
    ...new Set(
      value
        .map((source) =>
          String(source ?? '')
            .trim()
            .toLowerCase()
        )
        .filter((source): source is RequestArchiveAuditSource =>
          allowed.has(source)
        )
    ),
  ].sort()
}

function unwrap<T>(response: ApiEnvelope<T>): T {
  if (response.success === false || response.data === undefined) {
    throw new Error(response.message || 'Request failed')
  }
  return response.data
}

type UnknownRecord = Record<string, unknown>

function asRecord(value: unknown): UnknownRecord {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? (value as UnknownRecord)
    : {}
}

function readValue(record: UnknownRecord, ...keys: string[]) {
  for (const key of keys) {
    if (record[key] !== undefined && record[key] !== null) return record[key]
  }
  return undefined
}

function readString(record: UnknownRecord, ...keys: string[]) {
  const value = readValue(record, ...keys)
  return typeof value === 'string' ? value : ''
}

function readNumber(record: UnknownRecord, ...keys: string[]) {
  const value = readValue(record, ...keys)
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string' && value.trim() !== '') {
    const parsed = Number(value)
    if (Number.isFinite(parsed)) return parsed
  }
  return 0
}

function readBoolean(record: UnknownRecord, ...keys: string[]) {
  const value = readValue(record, ...keys)
  if (typeof value === 'boolean') return value
  if (typeof value === 'number') return value !== 0
  if (typeof value === 'string') return value.toLowerCase() === 'true'
  return false
}

function normalizeBuiltinPolicy(
  policy: SecurityAuditBuiltinPolicy
): SecurityAuditBuiltinPolicy {
  return {
    ...policy,
    upstream_policy_channel_ids: Array.isArray(
      policy.upstream_policy_channel_ids
    )
      ? policy.upstream_policy_channel_ids
      : [],
    upstream_policy_group_codes: normalizeSensitiveGroupCodes(
      policy.upstream_policy_group_codes ?? []
    ),
    cyber_session_block_enabled: policy.cyber_session_block_enabled === true,
    cyber_session_block_ttl_seconds: Number.isFinite(
      Number(policy.cyber_session_block_ttl_seconds)
    )
      ? Number(policy.cyber_session_block_ttl_seconds)
      : 3600,
    cyber_policy_auto_ban_exempt_group_codes: normalizeSensitiveGroupCodes(
      policy.cyber_policy_auto_ban_exempt_group_codes ?? []
    ),
  }
}

function normalizeMode(value: string): SecurityAuditRuntime['effective_mode'] {
  if (value === 'async_audit' || value === 'blocking') return value
  return 'off'
}

function normalizeQueue(value: unknown): SecurityAuditRuntime['queue'] {
  const queue = asRecord(value)
  return {
    queued: readNumber(queue, 'queued', 'Queued'),
    processing: readNumber(queue, 'processing', 'Processing'),
    retry: readNumber(queue, 'retry', 'Retry'),
    done: readNumber(queue, 'done', 'Done'),
    failed: readNumber(queue, 'failed', 'Failed'),
    active: readNumber(queue, 'active', 'Active'),
    capacity: readNumber(queue, 'capacity', 'Capacity'),
    oldest_queued_at: readNumber(queue, 'oldest_queued_at', 'OldestQueuedAt'),
  }
}

function normalizeMetrics(value: unknown): SecurityAuditRuntime['metrics'] {
  const metrics = asRecord(value)
  return {
    total: readNumber(metrics, 'total', 'Total'),
    allowed: readNumber(metrics, 'allowed', 'Allowed'),
    flagged: readNumber(metrics, 'flagged', 'Flagged'),
    blocked: readNumber(metrics, 'blocked', 'Blocked'),
    unavailable: readNumber(metrics, 'unavailable', 'Unavailable'),
    invalid: readNumber(metrics, 'invalid', 'Invalid'),
    timeouts: readNumber(metrics, 'timeouts', 'Timeouts'),
    failovers: readNumber(metrics, 'failovers', 'Failovers'),
    bulkhead_full: readNumber(metrics, 'bulkhead_full', 'BulkheadFull'),
    record_failed: readNumber(metrics, 'record_failed', 'RecordFailed'),
    enqueued: readNumber(metrics, 'enqueued', 'Enqueued'),
    dropped: readNumber(metrics, 'dropped', 'Dropped'),
    processed: readNumber(metrics, 'processed', 'Processed'),
    failed: readNumber(metrics, 'failed', 'Failed'),
  }
}

function normalizeEndpointHealth(value: unknown, fallbackId = '') {
  const endpoint = asRecord(value)
  return {
    id: readString(endpoint, 'id', 'Id') || fallbackId,
    name:
      readString(endpoint, 'name', 'Name') ||
      readString(endpoint, 'id', 'Id') ||
      fallbackId,
    enabled: readBoolean(endpoint, 'enabled', 'Enabled'),
    healthy: readBoolean(endpoint, 'healthy', 'Healthy', 'ok', 'Ok'),
    status: readString(endpoint, 'status', 'Status'),
    latency_ms: readNumber(endpoint, 'latency_ms', 'LatencyMs'),
    checked_at: readNumber(endpoint, 'checked_at', 'CheckedAt'),
    error_code: readString(endpoint, 'error_code', 'ErrorCode') || undefined,
  }
}

function normalizeEndpointHealthList(value: unknown) {
  if (Array.isArray(value)) {
    return value.map((endpoint) => normalizeEndpointHealth(endpoint))
  }
  return Object.entries(asRecord(value)).map(([id, endpoint]) =>
    normalizeEndpointHealth(endpoint, id)
  )
}

function normalizeRuntime(value: unknown): SecurityAuditRuntime {
  const runtime = asRecord(value)
  const processStatus =
    readString(runtime, 'process_status', 'ProcessStatus') ||
    (readBoolean(runtime, 'worker_running', 'WorkerRunning')
      ? 'running'
      : 'stopped')

  return {
    process_status: processStatus,
    effective_mode: normalizeMode(
      readString(runtime, 'effective_mode', 'EffectiveMode', 'mode', 'Mode')
    ),
    config_version: readNumber(runtime, 'config_version', 'ConfigVersion'),
    crypto_ready: readBoolean(runtime, 'crypto_ready', 'CryptoReady'),
    worker_total: readNumber(
      runtime,
      'worker_total',
      'WorkerTotal',
      'worker_count',
      'WorkerCount'
    ),
    worker_active: readNumber(runtime, 'worker_active', 'WorkerActive'),
    worker_heartbeat_at: readNumber(
      runtime,
      'worker_heartbeat_at',
      'WorkerHeartbeatAt'
    ),
    queue: normalizeQueue(readValue(runtime, 'queue', 'Queue')),
    queue_delay_ms: readNumber(runtime, 'queue_delay_ms', 'QueueDelayMs'),
    metrics: normalizeMetrics(readValue(runtime, 'metrics', 'Metrics')),
    last_processed_at: readNumber(
      runtime,
      'last_processed_at',
      'LastProcessedAt'
    ),
    last_error_code:
      readString(runtime, 'last_error_code', 'LastErrorCode') || undefined,
    endpoints: normalizeEndpointHealthList(
      readValue(runtime, 'endpoints', 'Endpoints')
    ),
    generated_at: readNumber(runtime, 'generated_at', 'GeneratedAt'),
  }
}

export function configToDraft(
  config: SecurityAuditConfig
): SecurityAuditConfigDraft {
  let mode = config.effective_mode
  if (!mode) {
    if (config.blocking_enabled) mode = 'blocking'
    else if (config.enabled) mode = 'async_audit'
    else mode = 'off'
  }

  return {
    ...config,
    mode,
    scanners: config.scanners || [],
    group_ids: config.group_ids || [],
    // 数据库初始配置没有节点时，Go 的 nil slice 会序列化为 null；
    // 页面草稿必须把它归一化为空数组，避免首次打开独立页面崩溃。
    endpoints: (config.endpoints || []).map((endpoint) => ({
      ...endpoint,
      token_action: 'keep',
      token: '',
    })),
  }
}

export function requestArchiveConfigToDraft(
  config: RequestArchiveConfig
): RequestArchiveConfigDraft {
  return {
    ...config,
    event_channel_ids: normalizeSensitiveRouteIds(
      config.event_channel_ids ?? []
    ),
    event_group_codes: normalizeSensitiveGroupCodes(
      config.event_group_codes ?? []
    ),
    event_sources: normalizeRequestArchiveAuditSources(config.event_sources),
    max_body_bytes: config.max_body_bytes ?? 67_108_864,
    queue_max_bytes: config.queue_max_bytes ?? 1_073_741_824,
    targets: (config.targets || []).map((target) => ({
      ...target,
      access_key_action: 'keep',
      access_key: '',
      secret_key_action: 'keep',
      secret_key: '',
    })),
  }
}

function endpointToInput(endpoint: SecurityAuditEndpointDraft) {
  return {
    id: endpoint.id.trim(),
    name: endpoint.name.trim(),
    protocol: 'openai_compatible' as const,
    base_url: endpoint.base_url.trim(),
    model: endpoint.model.trim(),
    timeout_ms: endpoint.timeout_ms,
    input_limit: endpoint.input_limit,
    enabled: endpoint.enabled,
    token_action: endpoint.token_action,
    ...(endpoint.token_action === 'replace'
      ? { token: endpoint.token.trim() }
      : {}),
  }
}

function requestArchiveTargetToInput(target: RequestArchiveTargetDraft) {
  return {
    id: target.id.trim(),
    name: target.name.trim(),
    type: target.type,
    enabled: target.enabled,
    local_path: target.local_path?.trim() || '',
    endpoint: target.endpoint?.trim() || '',
    bucket: target.bucket?.trim() || '',
    region: target.region?.trim() || '',
    prefix: target.prefix?.trim() || '',
    path_style: target.path_style,
    access_key_action: target.access_key_action,
    ...(target.access_key_action === 'replace'
      ? { access_key: target.access_key.trim() }
      : {}),
    secret_key_action: target.secret_key_action,
    ...(target.secret_key_action === 'replace'
      ? { secret_key: target.secret_key.trim() }
      : {}),
  }
}

function normalizeProbeResult(value: unknown): SecurityAuditProbeResult {
  const result = asRecord(value)
  return {
    endpoint_id: readString(result, 'endpoint_id', 'EndpointId'),
    healthy: readBoolean(result, 'healthy', 'Healthy', 'ok', 'Ok'),
    latency_ms: readNumber(result, 'latency_ms', 'LatencyMs'),
    status: readString(result, 'status', 'Status'),
    error_code: readString(result, 'error_code', 'ErrorCode') || undefined,
    message: readString(result, 'message', 'Message') || undefined,
  }
}

export function draftToConfigUpdate(
  draft: SecurityAuditConfigDraft
): SecurityAuditConfigUpdate {
  return {
    expected_version: draft.config_version,
    enabled: draft.mode !== 'off',
    blocking_enabled: draft.mode === 'blocking',
    store_pass_events: draft.store_pass_events,
    strategy: 'priority',
    worker_count: draft.worker_count,
    queue_capacity: draft.queue_capacity,
    retention_days: draft.retention_days,
    scanners: draft.scanners,
    all_groups: draft.all_groups,
    group_ids: draft.all_groups ? [] : draft.group_ids,
    endpoints: draft.endpoints.map(endpointToInput),
  }
}

export function requestArchiveDraftToConfigUpdate(
  draft: RequestArchiveConfigDraft
): RequestArchiveConfigUpdate {
  return {
    expected_version: draft.config_version,
    enabled: draft.enabled,
    archive_scope: draft.archive_scope || 'all_requests',
    event_channel_ids: normalizeSensitiveRouteIds(
      draft.event_channel_ids ?? []
    ),
    event_group_codes: normalizeSensitiveGroupCodes(
      draft.event_group_codes ?? []
    ),
    event_sources: normalizeRequestArchiveAuditSources(draft.event_sources),
    active_target_id: draft.active_target_id,
    retention_days: draft.retention_days,
    worker_count: draft.worker_count,
    queue_capacity: draft.queue_capacity,
    max_body_bytes: draft.max_body_bytes,
    queue_max_bytes: draft.queue_max_bytes,
    targets: draft.targets.map(requestArchiveTargetToInput),
  }
}

export async function getSecurityAuditConfig() {
  const response = await api.get<ApiEnvelope<SecurityAuditConfig>>(
    `${API_ROOT}/config`,
    { disableDuplicate: true }
  )
  return unwrap(response.data)
}

export async function getRequestArchiveConfig() {
  const response = await api.get<ApiEnvelope<RequestArchiveConfig>>(
    `${API_ROOT}/request-archive/config`,
    { disableDuplicate: true }
  )
  return unwrap(response.data)
}

export async function updateRequestArchiveConfig(
  input: RequestArchiveConfigUpdate
) {
  const response = await api.put<ApiEnvelope<RequestArchiveConfig>>(
    `${API_ROOT}/request-archive/config`,
    input,
    { skipBusinessError: true }
  )
  return unwrap(response.data)
}

export async function getRequestArchiveRuntime() {
  const response = await api.get<ApiEnvelope<RequestArchiveRuntime>>(
    `${API_ROOT}/request-archive/runtime`,
    { disableDuplicate: true }
  )
  return unwrap(response.data)
}

export async function probeRequestArchiveTarget(
  target: RequestArchiveTargetDraft
) {
  const response = await api.post<ApiEnvelope<RequestArchiveProbeResult>>(
    `${API_ROOT}/request-archive/targets/probe`,
    requestArchiveTargetToInput(target),
    { skipBusinessError: true }
  )
  return unwrap(response.data)
}

export async function getSecurityAuditBuiltinPolicy() {
  const response = await api.get<ApiEnvelope<SecurityAuditBuiltinPolicy>>(
    `${API_ROOT}/builtin-policy`,
    { disableDuplicate: true }
  )
  return normalizeBuiltinPolicy(unwrap(response.data))
}

export async function updateSecurityAuditBuiltinPolicy(
  input: SecurityAuditBuiltinPolicyUpdate
) {
  const response = await api.put<ApiEnvelope<SecurityAuditBuiltinPolicy>>(
    `${API_ROOT}/builtin-policy`,
    input,
    { skipBusinessError: true }
  )
  return normalizeBuiltinPolicy(unwrap(response.data))
}

export async function updateSecurityAuditConfig(
  input: SecurityAuditConfigUpdate
) {
  const response = await api.put<ApiEnvelope<SecurityAuditConfig>>(
    `${API_ROOT}/config`,
    input,
    { skipBusinessError: true }
  )
  return unwrap(response.data)
}

export async function probeSecurityAuditEndpoint(
  endpoint: SecurityAuditEndpointDraft
) {
  const response = await api.post<ApiEnvelope<unknown>>(
    `${API_ROOT}/endpoints/probe`,
    {
      endpoint_id: endpoint.id.trim(),
      name: endpoint.name.trim(),
      base_url: endpoint.base_url.trim(),
      model: endpoint.model.trim(),
      timeout_ms: endpoint.timeout_ms,
      input_limit: endpoint.input_limit,
      token_action: endpoint.token_action,
      ...(endpoint.token_action === 'replace' && endpoint.token.trim() !== ''
        ? { token: endpoint.token.trim() }
        : {}),
    },
    { skipBusinessError: true }
  )
  return normalizeProbeResult(unwrap(response.data))
}

export async function getSecurityAuditRuntime() {
  const response = await api.get<ApiEnvelope<unknown>>(`${API_ROOT}/runtime`, {
    disableDuplicate: true,
  })
  return normalizeRuntime(unwrap(response.data))
}

export async function getSecurityAuditEvents(
  filter: SecurityAuditEventFilter,
  page: number,
  pageSize: number
) {
  const response = await api.get<ApiEnvelope<SecurityAuditEventPage>>(
    `${API_ROOT}/events`,
    {
      params: {
        ...cleanSecurityAuditEventFilter(filter),
        page,
        page_size: pageSize,
      },
      disableDuplicate: true,
    }
  )
  return unwrap(response.data)
}

export async function getSecurityAuditEvent(id: number) {
  const response = await api.get<ApiEnvelope<SecurityAuditEventDetail>>(
    `${API_ROOT}/events/${id}`,
    { disableDuplicate: true, skipBusinessError: true }
  )
  return unwrap(response.data)
}

export async function deleteSecurityAuditEvent(id: number) {
  const response = await api.delete<ApiEnvelope<SecurityAuditDeleteResult>>(
    `${API_ROOT}/events/${id}`,
    { skipBusinessError: true }
  )
  return unwrap(response.data)
}

export async function batchDeleteSecurityAuditEvents(ids: number[]) {
  const response = await api.post<ApiEnvelope<SecurityAuditDeleteResult>>(
    `${API_ROOT}/events/batch-delete`,
    { ids },
    { skipBusinessError: true }
  )
  return unwrap(response.data)
}

export async function previewSecurityAuditDelete(
  filter: SecurityAuditEventFilter
) {
  const response = await api.post<ApiEnvelope<SecurityAuditDeletePreview>>(
    `${API_ROOT}/events/delete-preview`,
    cleanSecurityAuditEventFilter(filter),
    { skipBusinessError: true }
  )
  return unwrap(response.data)
}

export async function deleteSecurityAuditEventsByFilter(
  filter: SecurityAuditEventFilter,
  preview: SecurityAuditDeletePreview
) {
  const response = await api.post<ApiEnvelope<SecurityAuditDeleteResult>>(
    `${API_ROOT}/events/delete-by-filter`,
    {
      filter: cleanSecurityAuditEventFilter(filter),
      confirmation_token: preview.confirmation_token,
      confirm: true,
    },
    { skipBusinessError: true }
  )
  return unwrap(response.data)
}

export async function getSecurityAuditGroups() {
  const response = await api.get<
    ApiEnvelope<
      Array<{
        id: number
        code: string
        name: string
        description?: string
      }>
    >
  >('/api/group/details')
  const groups = unwrap(response.data)
  return groups.map<SecurityAuditGroup>((group) => ({
    id: group.id,
    code: group.code,
    name: group.name,
    description: group.description,
  }))
}
