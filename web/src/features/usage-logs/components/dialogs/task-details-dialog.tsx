/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { GroupBadge } from '@/components/group-badge'
import { StatusBadge } from '@/components/status-badge'
import { Label } from '@/components/ui/label'
import { formatTimestampToDate } from '@/lib/format'

import { formatDuration } from '../../lib/format'
import {
  taskActionMapper,
  taskPlatformMapper,
  taskStatusMapper,
} from '../../lib/mappers'
import { getTaskLogModelDisplay } from '../../lib/task-log-format'
import type { TaskLog } from '../../types'
import { ModelBadge } from '../model-badge'

function TaskDetailRow(props: {
  label: React.ReactNode
  value: React.ReactNode
  mono?: boolean
}) {
  return (
    <div className='grid min-w-0 grid-cols-[6rem_minmax(0,1fr)] gap-3 text-sm sm:grid-cols-[7rem_minmax(0,1fr)]'>
      <span className='text-muted-foreground text-xs'>{props.label}</span>
      <span
        className={
          props.mono
            ? 'min-w-0 font-mono text-xs break-all'
            : 'min-w-0 text-xs break-all'
        }
      >
        {props.value || '-'}
      </span>
    </div>
  )
}

export function TaskDetailsDialog(props: {
  task: TaskLog
  open: boolean
  onOpenChange: (open: boolean) => void
  isAdmin: boolean
}) {
  const { t } = useTranslation()
  const { task } = props
  const model = getTaskLogModelDisplay(task)
  const platform =
    task.display_platform?.trim() ||
    taskPlatformMapper.getLabel(task.platform, task.platform)
  const duration = formatDuration(task.submit_time, task.finish_time, 'seconds')

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={`${t('Task ID')} ${task.task_id}`}
      contentClassName='sm:max-w-3xl'
      contentHeight='min(72dvh, 680px)'
    >
      <div className='space-y-4'>
        <section className='bg-muted/20 space-y-2 rounded-md border p-3'>
          <Label className='text-xs font-semibold'>{t('Details')}</Label>
          <TaskDetailRow label={t('Task ID')} value={task.task_id} mono />
          <TaskDetailRow label={t('Platform')} value={t(platform)} />
          <TaskDetailRow
            label={t('Action')}
            value={t(taskActionMapper.getLabel(task.action))}
          />
          <TaskDetailRow
            label={t('Status')}
            value={
              <StatusBadge
                label={t(
                  taskStatusMapper.getLabel(
                    task.status,
                    task.status || 'Submitting'
                  )
                )}
                variant={taskStatusMapper.getVariant(task.status)}
                copyable={false}
              />
            }
          />
          <TaskDetailRow
            label={t('Progress')}
            value={task.progress || '-'}
            mono
          />
        </section>
        {model ? (
          <section className='bg-muted/20 space-y-2 rounded-md border p-3'>
            <Label className='text-xs font-semibold'>{t('Model')}</Label>
            <TaskDetailRow
              label={t('Model')}
              value={
                <ModelBadge
                  modelName={model.requestModel}
                  actualModel={props.isAdmin ? model.actualModel : undefined}
                />
              }
            />
            {props.isAdmin && model.actualModel ? (
              <TaskDetailRow
                label={t('Actual Model')}
                value={model.actualModel}
                mono
              />
            ) : null}
          </section>
        ) : null}
        <section className='bg-muted/20 space-y-2 rounded-md border p-3'>
          <Label className='text-xs font-semibold'>{t('Routing')}</Label>
          {props.isAdmin ? (
            <>
              <TaskDetailRow
                label={t('User')}
                value={task.username || `#${task.user_id}`}
              />
              <TaskDetailRow
                label={t('Channel')}
                value={task.channel_id ? `#${task.channel_id}` : '-'}
                mono
              />
            </>
          ) : null}
          <TaskDetailRow
            label={t('Group')}
            value={
              task.group ? (
                <GroupBadge
                  group={task.group}
                  label={task.group_name || undefined}
                />
              ) : (
                '-'
              )
            }
          />
        </section>
        <section className='bg-muted/20 space-y-2 rounded-md border p-3'>
          <Label className='text-xs font-semibold'>{t('Timing')}</Label>
          <TaskDetailRow
            label={t('Submit Time')}
            value={
              task.submit_time
                ? formatTimestampToDate(task.submit_time, 'seconds')
                : '-'
            }
            mono
          />
          <TaskDetailRow
            label={t('Start Time')}
            value={
              task.start_time
                ? formatTimestampToDate(task.start_time, 'seconds')
                : '-'
            }
            mono
          />
          <TaskDetailRow
            label={t('Finish Time')}
            value={
              task.finish_time
                ? formatTimestampToDate(task.finish_time, 'seconds')
                : '-'
            }
            mono
          />
          <TaskDetailRow
            label={t('Duration')}
            value={duration ? `${duration.durationSec.toFixed(1)}s` : '-'}
            mono
          />
        </section>
        {task.result_url || task.fail_reason ? (
          <section className='bg-muted/20 space-y-2 rounded-md border p-3'>
            <Label className='text-xs font-semibold'>{t('Result')}</Label>
            {task.result_url ? (
              <TaskDetailRow label={t('URL')} value={task.result_url} mono />
            ) : null}
            {task.fail_reason ? (
              <TaskDetailRow label={t('Details')} value={task.fail_reason} />
            ) : null}
          </section>
        ) : null}
      </div>
    </Dialog>
  )
}
