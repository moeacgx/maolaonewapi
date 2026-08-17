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
} from '../system-settings/request-limits/sensitive-rule-config.ts'
import type { UpstreamPolicyTargetType } from './types'

export type BuiltinPolicyScope = {
  upstream_policy_target_type: UpstreamPolicyTargetType
  upstream_policy_channel_ids: number[]
  upstream_policy_group_codes: string[]
}

export type BuiltinPolicyScopeValidationError = 'channels' | 'groups' | null

export function normalizeUpstreamPolicyTargetType(
  value: unknown
): UpstreamPolicyTargetType {
  return value === 'channels' || value === 'groups' ? value : 'all'
}

export function normalizeBuiltinPolicyScope(
  input: Partial<BuiltinPolicyScope>
): BuiltinPolicyScope {
  return {
    upstream_policy_target_type: normalizeUpstreamPolicyTargetType(
      input.upstream_policy_target_type
    ),
    upstream_policy_channel_ids: normalizeSensitiveRouteIds(
      input.upstream_policy_channel_ids ?? []
    ),
    upstream_policy_group_codes: normalizeSensitiveGroupCodes(
      input.upstream_policy_group_codes ?? []
    ),
  }
}

export function setBuiltinPolicyTargetType(
  scope: BuiltinPolicyScope,
  targetType: UpstreamPolicyTargetType
): BuiltinPolicyScope {
  return {
    ...scope,
    upstream_policy_target_type: targetType,
  }
}

export function getBuiltinPolicyScopeValidationError(
  scope: BuiltinPolicyScope
): BuiltinPolicyScopeValidationError {
  if (
    scope.upstream_policy_target_type === 'channels' &&
    scope.upstream_policy_channel_ids.length === 0
  ) {
    return 'channels'
  }
  if (
    scope.upstream_policy_target_type === 'groups' &&
    scope.upstream_policy_group_codes.length === 0
  ) {
    return 'groups'
  }
  return null
}
