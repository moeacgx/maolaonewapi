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
import type { TaskLog } from '../types'

export type TaskLogModelDisplay = {
  requestModel: string
  actualModel?: string
}

export function getTaskLogModelDisplay(
  log: TaskLog
): TaskLogModelDisplay | null {
  const requestModel = log.properties?.origin_model_name?.trim() || ''
  const upstreamModel = log.properties?.upstream_model_name?.trim() || ''
  const displayModel = requestModel || upstreamModel
  if (!displayModel) return null
  const actualModel =
    upstreamModel && upstreamModel !== displayModel ? upstreamModel : undefined
  return { requestModel: displayModel, actualModel }
}
