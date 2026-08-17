/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import type { TaskLog } from '../types'

export interface TaskLogModelDisplay {
  requestModel: string
  actualModel?: string
}

export function getTaskLogModelDisplay(
  log: Pick<TaskLog, 'properties'>
): TaskLogModelDisplay | null {
  const requestModel = log.properties?.origin_model_name?.trim() || ''
  const upstreamModel = log.properties?.upstream_model_name?.trim() || ''
  const displayModel = requestModel || upstreamModel
  if (!displayModel) return null
  return {
    requestModel: displayModel,
    actualModel:
      upstreamModel && upstreamModel !== displayModel
        ? upstreamModel
        : undefined,
  }
}

export function getTaskResultKind(
  log: Pick<
    TaskLog,
    'status' | 'image_urls' | 'result_expired' | 'result_url' | 'fail_reason'
  >
): 'images' | 'expired' | 'result' | 'error' | 'empty' {
  if (log.status === 'SUCCESS' && log.image_urls?.some((url) => url.trim())) {
    return 'images'
  }
  if (log.status === 'SUCCESS' && log.result_expired) return 'expired'
  if (
    log.status === 'SUCCESS' &&
    (log.result_url || log.fail_reason?.startsWith('http'))
  ) {
    return 'result'
  }
  if (log.fail_reason) return 'error'
  return 'empty'
}
