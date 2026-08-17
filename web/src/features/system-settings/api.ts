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
import { api } from '@/lib/api'

import type {
  ConfirmPaymentComplianceResponse,
  FetchUpstreamRatiosRequest,
  GroupCodeMigrationResponse,
  GroupCodeMigrationSummary,
  GroupDetailsData,
  GroupDetailsResponse,
  LogCleanupTask,
  OkpayRatePreviewResponse,
  SensitiveRuleChannelTagsResponse,
  SensitiveRuleChannelsResponse,
  SensitiveRuleGroupsResponse,
  SystemOptionsResponse,
  SystemTaskListResponse,
  SystemTaskResponse,
  TokenGroupMigrationRequest,
  TokenGroupMigrationResponse,
  TokenGroupMigrationSummary,
  UpdateGroupDetailsRequest,
  UpdateGroupDetailsResponse,
  UpdateOptionRequest,
  UpdateOptionResponse,
  UpstreamChannelsResponse,
  UpstreamRatiosResponse,
} from './types'

export async function getSystemOptions() {
  const res = await api.get<SystemOptionsResponse>('/api/option/')
  return res.data
}

export async function updateSystemOption(request: UpdateOptionRequest) {
  const res = await api.put<UpdateOptionResponse>('/api/option/', request)
  return res.data
}

export async function getGroupDetails(): Promise<GroupDetailsData> {
  const res = await api.get<GroupDetailsResponse>('/api/group/details')
  if (Array.isArray(res.data)) {
    return {
      groups: res.data,
      autoGroup: { user_selectable: false, description: '' },
    }
  }
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to load groups')
  }
  return {
    groups: res.data.data,
    autoGroup: res.data.auto_group ?? {
      user_selectable: false,
      description: '',
    },
  }
}

export async function updateGroupDetails(
  request: UpdateGroupDetailsRequest
): Promise<UpdateGroupDetailsResponse> {
  const res = await api.put<UpdateGroupDetailsResponse>(
    '/api/group/details',
    request
  )
  return res.data
}

export async function previewTokenGroupMigration(
  request: TokenGroupMigrationRequest
): Promise<TokenGroupMigrationSummary> {
  const res = await api.post<TokenGroupMigrationResponse>(
    '/api/group/token-migration/preview',
    request
  )
  if (!res.data.success || !res.data.data) {
    throw new Error(res.data.message || 'Failed to preview token migration')
  }
  return res.data.data
}

export async function migrateTokenGroup(
  request: TokenGroupMigrationRequest
): Promise<TokenGroupMigrationSummary> {
  const res = await api.post<TokenGroupMigrationResponse>(
    '/api/group/token-migration',
    request
  )
  if (!res.data.success || !res.data.data) {
    throw new Error(res.data.message || 'Failed to migrate tokens')
  }
  return res.data.data
}

export async function previewGroupCodeMigration(): Promise<GroupCodeMigrationSummary> {
  const res = await api.post<GroupCodeMigrationResponse>(
    '/api/group/code-migration/preview'
  )
  if (!res.data.success || !res.data.data) {
    throw new Error(
      res.data.message || 'Failed to preview group code migration'
    )
  }
  return res.data.data
}

export async function migrateGroupCodes(): Promise<GroupCodeMigrationSummary> {
  const res = await api.post<GroupCodeMigrationResponse>(
    '/api/group/code-migration',
    { confirm: true }
  )
  if (!res.data.success || !res.data.data) {
    throw new Error(res.data.message || 'Failed to migrate group codes')
  }
  return res.data.data
}

export async function confirmPaymentCompliance() {
  const res = await api.post<ConfirmPaymentComplianceResponse>(
    '/api/option/payment_compliance',
    { confirmed: true }
  )
  return res.data
}

export async function previewOkpayRate() {
  const res = await api.get<OkpayRatePreviewResponse>(
    '/api/option/okpay/rate-preview'
  )
  return res.data
}

export async function startLogCleanupTask(targetTimestamp: number) {
  const res = await api.post<SystemTaskResponse<LogCleanupTask>>(
    '/api/system-task/log-cleanup',
    null,
    {
      params: { target_timestamp: targetTimestamp },
    }
  )
  return res.data
}

export async function getCurrentLogCleanupTask() {
  const res = await api.get<SystemTaskResponse<LogCleanupTask | null>>(
    '/api/system-task/current',
    {
      params: { type: 'log_cleanup' },
    }
  )
  return res.data
}

export async function getSystemTask(taskId: string) {
  const res = await api.get<SystemTaskResponse<LogCleanupTask>>(
    `/api/system-task/${taskId}`
  )
  return res.data
}

export async function listSystemTasks(limit = 20) {
  const res = await api.get<SystemTaskListResponse>('/api/system-task/list', {
    params: { limit },
  })
  return res.data
}

export async function resetModelRatios() {
  const res = await api.post<UpdateOptionResponse>(
    '/api/option/rest_model_ratio'
  )
  return res.data
}

export async function getUpstreamChannels() {
  const res = await api.get<UpstreamChannelsResponse>(
    '/api/ratio_sync/channels'
  )
  return res.data
}

export async function getSensitiveRuleChannels() {
  const res = await api.get<SensitiveRuleChannelsResponse>(
    '/api/security-audit/builtin-policy/channels'
  )
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to load channels')
  }
  return res.data
}

export async function getSensitiveRuleChannelTags() {
  const res = await api.get<SensitiveRuleChannelTagsResponse>(
    '/api/security-audit/builtin-policy/channel-tags'
  )
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to load channel groups')
  }
  return res.data
}

export async function getSensitiveRuleGroups() {
  const res = await api.get<SensitiveRuleGroupsResponse>(
    '/api/security-audit/builtin-policy/groups'
  )
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to load groups')
  }
  return res.data
}

export async function fetchUpstreamRatios(request: FetchUpstreamRatiosRequest) {
  const res = await api.post<UpstreamRatiosResponse>(
    '/api/ratio_sync/fetch',
    request
  )
  return res.data
}
