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
export const DYNAMIC_ROUTING_ACTION_MODEL_REDIRECT = 'model_redirect' as const
export const DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE =
  'responses_image_tool_bridge' as const
export const DYNAMIC_ROUTING_RESPONSES_PATH = '/v1/responses' as const
export const DYNAMIC_ROUTING_IMAGE_GENERATION_PATH =
  '/v1/images/generations' as const
export const DYNAMIC_ROUTING_IMAGE_TARGET_PATHS = [
  DYNAMIC_ROUTING_RESPONSES_PATH,
  DYNAMIC_ROUTING_IMAGE_GENERATION_PATH,
] as const

export const DYNAMIC_ROUTING_ACTIONS = [
  DYNAMIC_ROUTING_ACTION_MODEL_REDIRECT,
  DYNAMIC_ROUTING_ACTION_RESPONSES_IMAGE_TOOL_BRIDGE,
] as const

export const DYNAMIC_ROUTING_CONDITION_REASONING_EFFORT =
  'reasoning_effort' as const

export const DYNAMIC_ROUTING_CONDITION_REQUEST_PREFIX = 'request.' as const

export const DYNAMIC_ROUTING_OPERATORS = [
  'equals',
  'not_equals',
  'exists',
  'not_exists',
] as const

export type DynamicRoutingAction = (typeof DYNAMIC_ROUTING_ACTIONS)[number]

export type DynamicRoutingOperator = (typeof DYNAMIC_ROUTING_OPERATORS)[number]

export interface DynamicRoutingCondition {
  field: string
  operator?: DynamicRoutingOperator
  value?: string
}

export interface DynamicRoutingRule {
  id: string
  enabled: boolean
  action?: DynamicRoutingAction
  source_model: string
  target_model: string
  target_path?: string
  source_groups?: string[]
  target_group?: string
  channel_types?: number[]
  request_paths?: string[]
  conditions?: DynamicRoutingCondition[]
  priority?: number
}

export interface DynamicRoutingChannelConfig {
  enabled?: boolean
  rules?: DynamicRoutingRule[]
}

export type DynamicRoutingChannelMode = 'inherit' | 'enabled' | 'disabled'
