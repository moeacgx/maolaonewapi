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
import { useTranslation } from 'react-i18next'
import { formatTimestampToDate } from '@/lib/format'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import { GroupBadge } from '@/components/group-badge'
import { StatusBadge } from '@/components/status-badge'
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

function TaskTimeRows({ task }: { task: TaskLog }) {
  const { t } = useTranslation()
  const duration = formatDuration(task.submit_time, task.finish_time, 'seconds')
  return (
    <>
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
    </>
  )
}

export function TaskDetailsDialog({
  task,
  open,
  onOpenChange,
  isAdmin,
}: {
  task: TaskLog
  open: boolean
  onOpenChange: (open: boolean) => void
  isAdmin: boolean
}) {
  const { t } = useTranslation()
  const model = getTaskLogModelDisplay(task)
  const platform =
    task.display_platform ||
    taskPlatformMapper.getLabel(task.platform, task.platform)

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-3xl'>
        <DialogHeader>
          <DialogTitle>
            {t('Task ID')} {task.task_id}
          </DialogTitle>
        </DialogHeader>
        <ScrollArea className='max-h-[72vh] pr-3'>
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
            <section className='bg-muted/20 space-y-2 rounded-md border p-3'>
              <Label className='text-xs font-semibold'>{t('Model')}</Label>
              <TaskDetailRow
                label={t('Model')}
                value={
                  model ? (
                    <ModelBadge
                      modelName={model.requestModel}
                      actualModel={isAdmin ? model.actualModel : undefined}
                    />
                  ) : (
                    '-'
                  )
                }
              />
              {isAdmin && (
                <>
                  <TaskDetailRow
                    label={t('Request Model')}
                    value={task.properties?.origin_model_name || '-'}
                    mono
                  />
                  <TaskDetailRow
                    label={t('Actual Model')}
                    value={task.properties?.upstream_model_name || '-'}
                    mono
                  />
                </>
              )}
            </section>
            <section className='bg-muted/20 space-y-2 rounded-md border p-3'>
              <Label className='text-xs font-semibold'>{t('Routing')}</Label>
              <TaskDetailRow
                label={t('User')}
                value={task.username || `#${task.user_id}`}
              />
              <TaskDetailRow
                label={t('Channel')}
                value={task.channel_id ? `#${task.channel_id}` : '-'}
                mono
              />
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
              <TaskTimeRows task={task} />
            </section>
            {(task.result_url || task.fail_reason) && (
              <section className='bg-muted/20 space-y-2 rounded-md border p-3'>
                <Label className='text-xs font-semibold'>{t('Result')}</Label>
                <TaskDetailRow
                  label={t('URL')}
                  value={task.result_url || '-'}
                  mono
                />
                <TaskDetailRow
                  label={t('Details')}
                  value={task.fail_reason || '-'}
                />
              </section>
            )}
          </div>
        </ScrollArea>
      </DialogContent>
    </Dialog>
  )
}
