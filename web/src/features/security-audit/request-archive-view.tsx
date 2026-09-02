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
  Add01Icon,
  Alert02Icon,
  Database01Icon,
  Delete02Icon,
  FloppyDiskIcon,
  RefreshIcon,
  TestTube01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import axios from 'axios'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { MultiSelect } from '@/components/multi-select'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
  FieldTitle,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import {
  getSensitiveRuleChannels,
  getSensitiveRuleGroups,
} from '@/features/system-settings/api'
import {
  includeMissingSensitiveGroupOptions,
  includeMissingSensitiveRouteOptions,
  normalizeSensitiveGroupCodes,
  normalizeSensitiveRouteIds,
} from '@/features/system-settings/request-limits/sensitive-rule-config'
import type { SensitiveRuleChannel } from '@/features/system-settings/types'

import {
  getRequestArchiveConfig,
  getRequestArchiveRuntime,
  probeRequestArchiveTarget,
  requestArchiveConfigToDraft,
  requestArchiveDraftToConfigUpdate,
  updateRequestArchiveConfig,
} from './api'
import type {
  RequestArchiveAuditSource,
  RequestArchiveApiErrorResponse,
  RequestArchiveConfigDraft,
  RequestArchiveRuntime,
  RequestArchiveSecretAction,
  RequestArchiveTargetDraft,
  RequestArchiveTargetType,
} from './types'
import { formatAuditInteger, formatAuditTime } from './utils'

function formatArchiveBytes(bytes?: number) {
  if (typeof bytes !== 'number' || !Number.isFinite(bytes) || bytes < 0) {
    return '-'
  }
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let value = bytes / 1024
  for (const unit of units) {
    if (value < 1024 || unit === units.at(-1)) {
      return `${value.toFixed(value >= 10 ? 1 : 2)} ${unit}`
    }
    value /= 1024
  }
  return `${bytes} B`
}

function createTarget(): RequestArchiveTargetDraft {
  const suffix =
    typeof crypto !== 'undefined' && 'randomUUID' in crypto
      ? crypto.randomUUID().slice(0, 8)
      : Date.now().toString(36)

  return {
    id: `archive-${suffix}`,
    name: '',
    type: 'local',
    enabled: true,
    local_path: '',
    endpoint: '',
    bucket: '',
    region: 'us-east-1',
    prefix: '',
    path_style: false,
    access_key_configured: false,
    secret_key_configured: false,
    created_at: 0,
    updated_at: 0,
    access_key_action: 'keep',
    access_key: '',
    secret_key_action: 'keep',
    secret_key: '',
  }
}

function getChannelLabel(channel: SensitiveRuleChannel) {
  const name = channel.name?.trim()
  const label = name ? `${name} #${channel.id}` : `#${channel.id}`
  const tag = channel.tag?.trim()
  return tag ? `${label} · ${tag}` : label
}

function validateDraft(
  draft: RequestArchiveConfigDraft,
  t: (key: string) => string
) {
  if (
    !Number.isInteger(draft.retention_days) ||
    draft.retention_days < 1 ||
    draft.retention_days > 3650
  ) {
    return t('Archive retention must be between 1 and 3650 days.')
  }
  if (
    !Number.isInteger(draft.worker_count) ||
    draft.worker_count < 1 ||
    draft.worker_count > 32
  ) {
    return t('Archive worker count must be between 1 and 32.')
  }
  if (
    !Number.isInteger(draft.queue_capacity) ||
    draft.queue_capacity < 1 ||
    draft.queue_capacity > 1048576
  ) {
    return t('Archive queue capacity must be between 1 and 1048576.')
  }
  if (
    !Number.isInteger(draft.max_body_bytes) ||
    draft.max_body_bytes < 1024 ||
    draft.max_body_bytes > 128 * 1024 * 1024
  ) {
    return t('Maximum archive body size must be between 1 KiB and 128 MiB.')
  }
  if (
    !Number.isInteger(draft.queue_max_bytes) ||
    draft.queue_max_bytes < draft.max_body_bytes ||
    draft.queue_max_bytes > 64 * 1024 * 1024 * 1024
  ) {
    return t(
      'Archive queue body limit must be at least the per-request limit and at most 64 GiB.'
    )
  }
  if (draft.targets.length > 64) {
    return t('At most 64 archive storage targets are supported.')
  }

  const targetIds = new Set<string>()
  for (const target of draft.targets) {
    if (!target.id.trim() || !target.name.trim()) {
      return t('Every archive storage target requires an ID and name.')
    }
    if (targetIds.has(target.id.trim())) {
      return `${t('Duplicate')}: ${t('Storage target ID')}`
    }
    targetIds.add(target.id.trim())

    if (target.type === 'local') {
      if (!target.local_path?.trim()) {
        return t('Local archive storage requires an absolute directory path.')
      }
      continue
    }

    if (!target.bucket?.trim()) {
      return t('S3-compatible archive storage requires a bucket.')
    }
    if (target.access_key_action === 'replace' && !target.access_key.trim()) {
      return t('Enter the replacement access key.')
    }
    if (target.secret_key_action === 'replace' && !target.secret_key.trim()) {
      return t('Enter the replacement secret key.')
    }
    if (
      target.enabled &&
      !target.access_key_configured &&
      target.access_key_action !== 'replace'
    ) {
      return t('Configure an access key for every new S3-compatible target.')
    }
    if (
      target.enabled &&
      !target.secret_key_configured &&
      target.secret_key_action !== 'replace'
    ) {
      return t('Configure a secret key for every new S3-compatible target.')
    }
    const clearsAccessKey = target.access_key_action === 'clear'
    const clearsSecretKey = target.secret_key_action === 'clear'
    if (clearsAccessKey !== clearsSecretKey) {
      return t('Clear both S3-compatible credentials together.')
    }
    if (clearsAccessKey && target.enabled) {
      return t(
        'Disable the S3-compatible target before clearing its credentials.'
      )
    }
  }

  if (draft.enabled) {
    const activeTarget = draft.targets.find(
      (target) => target.id === draft.active_target_id
    )
    if (!activeTarget?.enabled) {
      return t(
        'Select an enabled active archive storage target before enabling archiving.'
      )
    }
  }
  return null
}

function targetTypeLabel(
  type: RequestArchiveTargetType,
  t: (key: string) => string
) {
  return type === 'local' ? t('Local storage') : t('S3-compatible storage')
}

function secretStatus(
  configured: boolean,
  action: RequestArchiveSecretAction,
  value: string,
  t: (key: string) => string
) {
  if (action === 'clear') return t('Will be cleared')
  if (action === 'replace' && value) return t('Pending replacement')
  return configured ? t('Configured') : t('Not configured')
}

function getErrorMessage(error: unknown, fallback: string) {
  if (axios.isAxiosError<RequestArchiveApiErrorResponse>(error)) {
    return error.response?.data?.message || error.message || fallback
  }
  return error instanceof Error ? error.message : fallback
}

function isRequestArchiveConfigConflict(error: unknown) {
  return (
    axios.isAxiosError<RequestArchiveApiErrorResponse>(error) &&
    error.response?.status === 409 &&
    error.response.data?.code === 'request_archive_config_conflict'
  )
}

function RuntimeOverview({
  runtime,
  loading,
  fetching,
  error,
  onRetry,
}: {
  runtime?: RequestArchiveRuntime
  loading: boolean
  fetching: boolean
  error: unknown
  onRetry: () => void
}) {
  const { t } = useTranslation()
  if (loading && !runtime) {
    return (
      <div className='grid gap-4 md:grid-cols-2 xl:grid-cols-4'>
        {['status', 'worker', 'queue', 'target'].map((section) => (
          <Skeleton key={section} className='h-32 rounded-xl' />
        ))}
      </div>
    )
  }

  if (error && !runtime) {
    return (
      <Alert variant='destructive'>
        <HugeiconsIcon icon={Alert02Icon} strokeWidth={2} />
        <AlertTitle>{t('Request archive runtime is unavailable')}</AlertTitle>
        <AlertDescription className='flex flex-wrap items-center gap-2'>
          <span>{getErrorMessage(error, t('Refresh failed'))}</span>
          <Button
            variant='outline'
            size='sm'
            onClick={onRetry}
            disabled={fetching}
          >
            {fetching ? (
              <Spinner data-icon='inline-start' />
            ) : (
              <HugeiconsIcon
                icon={RefreshIcon}
                strokeWidth={2}
                data-icon='inline-start'
              />
            )}
            {t('Retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  }

  const queue = runtime?.queue
  const utilization = queue?.capacity
    ? Math.min(100, Math.round((queue.active / queue.capacity) * 100))
    : 0

  return (
    <div className='flex flex-col gap-4'>
      {error ? (
        <Alert variant='destructive'>
          <HugeiconsIcon icon={Alert02Icon} strokeWidth={2} />
          <AlertTitle>
            {t(
              'Showing the last known runtime because the latest refresh failed.'
            )}
          </AlertTitle>
          <AlertDescription className='flex flex-wrap items-center gap-2'>
            <span>{getErrorMessage(error, t('Refresh failed'))}</span>
            <Button
              variant='outline'
              size='sm'
              onClick={onRetry}
              disabled={fetching}
            >
              {fetching ? (
                <Spinner data-icon='inline-start' />
              ) : (
                <HugeiconsIcon
                  icon={RefreshIcon}
                  strokeWidth={2}
                  data-icon='inline-start'
                />
              )}
              {t('Retry')}
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
      <div className='grid gap-4 md:grid-cols-2 xl:grid-cols-4'>
        <Card>
          <CardHeader>
            <CardTitle>{t('Archive worker')}</CardTitle>
            <CardDescription>
              {t('Background request archive worker')}
            </CardDescription>
          </CardHeader>
          <CardContent className='flex items-center gap-2'>
            <span className='text-2xl font-semibold tabular-nums'>
              {formatAuditInteger(runtime?.worker_active)} /{' '}
              {formatAuditInteger(runtime?.worker_count)}
            </span>
            <Badge variant={runtime?.worker_running ? 'default' : 'secondary'}>
              {runtime?.worker_running ? t('Running') : t('Stopped')}
            </Badge>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>{t('Archive queue')}</CardTitle>
            <CardDescription>
              {t('Enqueued: {{enqueued}} - Dropped: {{dropped}}', {
                enqueued: formatAuditInteger(runtime?.enqueued),
                dropped: formatAuditInteger(runtime?.dropped),
              })}
            </CardDescription>
          </CardHeader>
          <CardContent className='text-2xl font-semibold tabular-nums'>
            <div>
              {formatAuditInteger(queue?.active)} /{' '}
              {formatAuditInteger(queue?.capacity)}
            </div>
            <div className='text-muted-foreground mt-2 text-sm font-normal'>
              {t('Queue body byte limit')}:{' '}
              {formatArchiveBytes(queue?.active_bytes)}
              {' / '}
              {formatArchiveBytes(queue?.capacity_bytes)}
            </div>
            <div className='mt-3 grid grid-cols-[minmax(0,1fr)_auto] gap-x-3 gap-y-1 text-sm font-normal'>
              <span className='text-muted-foreground'>{t('Queued')}</span>
              <span>{formatAuditInteger(queue?.queued)}</span>
              <span className='text-muted-foreground'>{t('Processing')}</span>
              <span>{formatAuditInteger(queue?.processing)}</span>
              <span className='text-muted-foreground'>{t('Retrying')}</span>
              <span>{formatAuditInteger(queue?.retry)}</span>
              <span className='text-muted-foreground'>{t('Failed')}</span>
              <span>{formatAuditInteger(queue?.failed)}</span>
              <span className='text-muted-foreground'>{t('Queue delay')}</span>
              <span>
                {t('{{latency}} ms', {
                  latency: formatAuditInteger(runtime?.queue_delay_ms),
                })}
              </span>
            </div>
            <p className='text-muted-foreground mt-3 text-xs font-normal'>
              {t(
                'Failed jobs retain their queued payloads and continue using queue capacity until retention cleanup.'
              )}
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>{t('Last archived request')}</CardTitle>
            <CardDescription>
              {t('Last successful background archive')}
            </CardDescription>
          </CardHeader>
          <CardContent className='font-medium'>
            {formatAuditTime(runtime?.last_processed_at ?? 0)}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>{t('Latest archive status')}</CardTitle>
            <CardDescription>
              {t('Queue utilization: {{percent}}%', { percent: utilization })}
            </CardDescription>
          </CardHeader>
          <CardContent className='flex flex-col gap-2 text-sm'>
            <div className='flex flex-wrap justify-between gap-2'>
              <span className='text-muted-foreground'>{t('Worker error')}</span>
              <span className='font-medium break-all'>
                {runtime?.last_error_code || t('None')}
              </span>
            </div>
            <div className='flex flex-wrap justify-between gap-2'>
              <span className='text-muted-foreground'>
                {t('Enqueue status')}
              </span>
              <span className='font-medium break-all'>
                {runtime?.last_enqueue_code || t('None')}
              </span>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

function ArchiveTargetCard({
  target,
  active,
  probing,
  onChange,
  onRemove,
  onProbe,
}: {
  target: RequestArchiveTargetDraft
  active: boolean
  probing: boolean
  onChange: (patch: Partial<RequestArchiveTargetDraft>) => void
  onRemove: () => void
  onProbe: () => void
}) {
  const { t } = useTranslation()
  const setType = (type: RequestArchiveTargetType) => {
    if (type === 'local') {
      onChange({
        type,
        endpoint: '',
        bucket: '',
        region: '',
        prefix: '',
        path_style: false,
        access_key_action: 'keep',
        access_key: '',
        secret_key_action: 'keep',
        secret_key: '',
      })
      return
    }
    onChange({
      type,
      access_key_action: target.access_key_configured ? 'keep' : 'replace',
      secret_key_action: target.secret_key_configured ? 'keep' : 'replace',
      region: target.region || 'us-east-1',
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className='flex min-w-0 flex-wrap items-center gap-2'>
          <span className='min-w-0 break-words'>
            {target.name || t('Unnamed storage target')}
          </span>
          <Badge variant={target.enabled ? 'default' : 'secondary'}>
            {target.enabled ? t('Enabled') : t('Disabled')}
          </Badge>
          <Badge variant='outline'>{targetTypeLabel(target.type, t)}</Badge>
          {active ? <Badge variant='outline'>{t('Active')}</Badge> : null}
        </CardTitle>
        <CardDescription className='break-all'>
          {target.type === 'local'
            ? target.local_path || t('Local path not configured')
            : target.bucket || t('Bucket not configured')}
        </CardDescription>
        <CardAction>
          <Button
            variant='ghost'
            size='icon-sm'
            aria-label={t('Remove storage target')}
            onClick={onRemove}
          >
            <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} />
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent>
        <FieldGroup>
          <div className='grid gap-4 sm:grid-cols-2'>
            <Field>
              <FieldLabel>{t('Name')}</FieldLabel>
              <Input
                value={target.name}
                placeholder={t('Archive primary storage')}
                onChange={(event) => onChange({ name: event.target.value })}
              />
            </Field>
            <Field>
              <FieldLabel>{t('Storage type')}</FieldLabel>
              <Select
                value={target.type}
                onValueChange={(value) => {
                  if (value === 'local' || value === 's3') setType(value)
                }}
                items={[
                  { value: 'local', label: t('Local storage') },
                  {
                    value: 's3',
                    label: t('S3-compatible storage / Cloudflare R2'),
                  },
                ]}
              >
                <SelectTrigger aria-label={t('Storage type')}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    <SelectItem value='local'>{t('Local storage')}</SelectItem>
                    <SelectItem value='s3'>
                      {t('S3-compatible storage / Cloudflare R2')}
                    </SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
          </div>

          <Field orientation='horizontal'>
            <FieldContent>
              <FieldTitle>{t('Enabled')}</FieldTitle>
              <FieldDescription>
                {t(
                  'Disabled targets remain available for previously queued archive jobs only.'
                )}
              </FieldDescription>
            </FieldContent>
            <Switch
              checked={target.enabled}
              onCheckedChange={(enabled) => onChange({ enabled })}
              aria-label={t('Enabled')}
            />
          </Field>

          {target.type === 'local' ? (
            <Field>
              <FieldLabel>{t('Local archive directory')}</FieldLabel>
              <Input
                value={target.local_path || ''}
                placeholder='D:\\newapi-archive'
                onChange={(event) =>
                  onChange({ local_path: event.target.value })
                }
              />
              <FieldDescription>
                {t(
                  'Use an absolute directory path on the server running New API.'
                )}
              </FieldDescription>
            </Field>
          ) : (
            <>
              <div className='grid gap-4 sm:grid-cols-2'>
                <Field>
                  <FieldLabel>{t('S3 endpoint')}</FieldLabel>
                  <Input
                    type='url'
                    value={target.endpoint || ''}
                    placeholder='https://<account-id>.r2.cloudflarestorage.com'
                    onChange={(event) =>
                      onChange({ endpoint: event.target.value })
                    }
                  />
                  <FieldDescription>
                    {t(
                      'Leave empty for the AWS endpoint. Use a Cloudflare R2 or other S3-compatible endpoint when needed.'
                    )}
                  </FieldDescription>
                </Field>
                <Field>
                  <FieldLabel>{t('Bucket')}</FieldLabel>
                  <Input
                    value={target.bucket || ''}
                    onChange={(event) =>
                      onChange({ bucket: event.target.value })
                    }
                  />
                </Field>
                <Field>
                  <FieldLabel>{t('Region')}</FieldLabel>
                  <Input
                    value={target.region || ''}
                    placeholder='us-east-1'
                    onChange={(event) =>
                      onChange({ region: event.target.value })
                    }
                  />
                </Field>
                <Field>
                  <FieldLabel>{t('Object prefix')}</FieldLabel>
                  <Input
                    value={target.prefix || ''}
                    placeholder='request-archive'
                    onChange={(event) =>
                      onChange({ prefix: event.target.value })
                    }
                  />
                </Field>
              </div>
              <Field orientation='horizontal'>
                <FieldContent>
                  <FieldTitle>{t('Use path-style S3 addressing')}</FieldTitle>
                  <FieldDescription>
                    {t(
                      'Enable this for storage services that require bucket names in the request path.'
                    )}
                  </FieldDescription>
                </FieldContent>
                <Switch
                  checked={target.path_style}
                  onCheckedChange={(path_style) => onChange({ path_style })}
                  aria-label={t('Use path-style S3 addressing')}
                />
              </Field>
              <div className='grid gap-4 sm:grid-cols-2'>
                <Field>
                  <FieldLabel>{t('Access key action')}</FieldLabel>
                  <Select
                    value={target.access_key_action}
                    onValueChange={(value) => {
                      if (
                        value === 'keep' ||
                        value === 'replace' ||
                        value === 'clear'
                      ) {
                        onChange({ access_key_action: value, access_key: '' })
                      }
                    }}
                    items={[
                      { value: 'keep', label: t('Keep') },
                      { value: 'replace', label: t('Replace') },
                      { value: 'clear', label: t('Clear') },
                    ]}
                  >
                    <SelectTrigger aria-label={t('Access key action')}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        <SelectItem
                          value='keep'
                          disabled={!target.access_key_configured}
                        >
                          {t('Keep')}
                        </SelectItem>
                        <SelectItem value='replace'>{t('Replace')}</SelectItem>
                        <SelectItem value='clear'>{t('Clear')}</SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                  <FieldDescription>
                    {secretStatus(
                      target.access_key_configured,
                      target.access_key_action,
                      target.access_key,
                      t
                    )}
                  </FieldDescription>
                </Field>
                <Field>
                  <FieldLabel>{t('Secret key action')}</FieldLabel>
                  <Select
                    value={target.secret_key_action}
                    onValueChange={(value) => {
                      if (
                        value === 'keep' ||
                        value === 'replace' ||
                        value === 'clear'
                      ) {
                        onChange({ secret_key_action: value, secret_key: '' })
                      }
                    }}
                    items={[
                      { value: 'keep', label: t('Keep') },
                      { value: 'replace', label: t('Replace') },
                      { value: 'clear', label: t('Clear') },
                    ]}
                  >
                    <SelectTrigger aria-label={t('Secret key action')}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        <SelectItem
                          value='keep'
                          disabled={!target.secret_key_configured}
                        >
                          {t('Keep')}
                        </SelectItem>
                        <SelectItem value='replace'>{t('Replace')}</SelectItem>
                        <SelectItem value='clear'>{t('Clear')}</SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                  <FieldDescription>
                    {secretStatus(
                      target.secret_key_configured,
                      target.secret_key_action,
                      target.secret_key,
                      t
                    )}
                  </FieldDescription>
                </Field>
              </div>
              {target.access_key_action === 'replace' ? (
                <Field>
                  <FieldLabel>{t('New access key')}</FieldLabel>
                  <Input
                    type='password'
                    autoComplete='new-password'
                    value={target.access_key}
                    onChange={(event) =>
                      onChange({ access_key: event.target.value })
                    }
                  />
                </Field>
              ) : null}
              {target.secret_key_action === 'replace' ? (
                <Field>
                  <FieldLabel>{t('New secret key')}</FieldLabel>
                  <Input
                    type='password'
                    autoComplete='new-password'
                    value={target.secret_key}
                    onChange={(event) =>
                      onChange({ secret_key: event.target.value })
                    }
                  />
                </Field>
              ) : null}
            </>
          )}
        </FieldGroup>
      </CardContent>
      <CardFooter>
        <Button
          variant='outline'
          size='sm'
          onClick={onProbe}
          disabled={probing}
        >
          {probing ? (
            <Spinner data-icon='inline-start' />
          ) : (
            <HugeiconsIcon
              icon={TestTube01Icon}
              strokeWidth={2}
              data-icon='inline-start'
            />
          )}
          {t('Probe')}
        </Button>
      </CardFooter>
    </Card>
  )
}

export function SecurityAuditRequestArchiveView() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<RequestArchiveConfigDraft | null>(null)
  const [saving, setSaving] = useState(false)
  const [probingId, setProbingId] = useState<string | null>(null)
  const [deleteTargetId, setDeleteTargetId] = useState<string | null>(null)

  const configQuery = useQuery({
    queryKey: ['security-audit', 'request-archive', 'config'],
    queryFn: getRequestArchiveConfig,
    staleTime: 15_000,
    refetchOnWindowFocus: false,
  })
  const runtimeQuery = useQuery({
    queryKey: ['security-audit', 'request-archive', 'runtime'],
    queryFn: getRequestArchiveRuntime,
    refetchInterval: 10_000,
  })
  const channelsQuery = useQuery({
    queryKey: ['security-audit', 'request-archive', 'channels'],
    queryFn: getSensitiveRuleChannels,
    staleTime: 15_000,
    refetchOnWindowFocus: false,
  })
  const groupsQuery = useQuery({
    queryKey: ['security-audit', 'request-archive', 'groups'],
    queryFn: getSensitiveRuleGroups,
    staleTime: 15_000,
    refetchOnWindowFocus: false,
  })

  const channelOptions = useMemo(
    () =>
      [...(channelsQuery.data?.data ?? [])]
        .filter((channel) => Number.isInteger(channel.id) && channel.id > 0)
        .sort((left, right) => {
          const nameCompare = getChannelLabel(left).localeCompare(
            getChannelLabel(right)
          )
          return nameCompare === 0 ? left.id - right.id : nameCompare
        })
        .map((channel) => ({
          value: String(channel.id),
          label: getChannelLabel(channel),
        })),
    [channelsQuery.data?.data]
  )
  const groupOptions = useMemo(
    () =>
      [...(groupsQuery.data?.data ?? [])]
        .filter(
          (group) =>
            Number.isInteger(group.id) &&
            group.id > 0 &&
            group.code.trim().length > 0
        )
        .sort((left, right) => {
          const leftLabel = left.name || left.code
          const rightLabel = right.name || right.code
          const nameCompare = leftLabel.localeCompare(rightLabel)
          return nameCompare === 0 ? left.id - right.id : nameCompare
        })
        .map((group) => ({
          value: group.code,
          label: `${group.name || group.code} #${group.id}`,
        })),
    [groupsQuery.data?.data]
  )
  const auditSourceOptions = useMemo<
    Array<{ value: RequestArchiveAuditSource; label: string }>
  >(
    () => [
      {
        value: 'upstream_policy',
        label: t('Official risk control (cyber_policy)'),
      },
      {
        value: 'biological_risk',
        label: t('Biological risk (upstream)'),
      },
      { value: 'sensitive_word', label: t('Sensitive words') },
      { value: 'prompt_guard', label: t('Prompt Guard') },
    ],
    [t]
  )

  useEffect(() => {
    if (configQuery.data) {
      setDraft(requestArchiveConfigToDraft(configQuery.data))
    }
  }, [configQuery.data])

  const baseline = useMemo(
    () =>
      configQuery.data ? requestArchiveConfigToDraft(configQuery.data) : null,
    [configQuery.data]
  )
  const dirty = Boolean(
    draft && baseline && JSON.stringify(draft) !== JSON.stringify(baseline)
  )

  const updateDraft = (patch: Partial<RequestArchiveConfigDraft>) => {
    setDraft((current) => (current ? { ...current, ...patch } : current))
  }
  const updateTarget = (
    id: string,
    patch: Partial<RequestArchiveTargetDraft>
  ) => {
    setDraft((current) =>
      current
        ? {
            ...current,
            targets: current.targets.map((target) =>
              target.id === id ? { ...target, ...patch } : target
            ),
          }
        : current
    )
  }

  const refresh = async () => {
    await Promise.all([
      configQuery.refetch(),
      runtimeQuery.refetch(),
      channelsQuery.refetch(),
      groupsQuery.refetch(),
    ])
  }

  const save = async () => {
    if (!draft) return
    const validationError = validateDraft(draft, t)
    if (validationError) {
      toast.error(validationError)
      return
    }
    setSaving(true)
    try {
      const updated = await updateRequestArchiveConfig(
        requestArchiveDraftToConfigUpdate(draft)
      )
      queryClient.setQueryData(
        ['security-audit', 'request-archive', 'config'],
        updated
      )
      setDraft(requestArchiveConfigToDraft(updated))
      await runtimeQuery.refetch()
      toast.success(t('Request archive configuration saved'))
    } catch (error) {
      if (isRequestArchiveConfigConflict(error)) {
        await configQuery.refetch()
        toast.error(
          t(
            'The archive configuration changed on the server. Latest values were reloaded; review and save again.'
          )
        )
        return
      }
      toast.error(
        getErrorMessage(
          error,
          t('Failed to save request archive configuration')
        )
      )
    } finally {
      setSaving(false)
    }
  }

  const probe = async (target: RequestArchiveTargetDraft) => {
    const validationError = validateDraft(
      {
        ...(draft as RequestArchiveConfigDraft),
        enabled: false,
        active_target_id: '',
        targets: [target],
      },
      t
    )
    if (validationError) {
      toast.error(validationError)
      return
    }
    setProbingId(target.id)
    try {
      const result = await probeRequestArchiveTarget(target)
      if (result.healthy) {
        toast.success(
          t('Archive storage responded in {{latency}} ms', {
            latency: result.latency_ms,
          })
        )
      } else {
        toast.error(result.message || t('Archive storage is unavailable'))
      }
    } catch (error) {
      toast.error(getErrorMessage(error, t('Archive storage probe failed')))
    } finally {
      setProbingId(null)
    }
  }

  if (configQuery.isLoading && !draft) {
    return <Skeleton className='h-96 w-full rounded-xl' />
  }

  if (configQuery.isError || !draft) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t('Request archive is unavailable')}</CardTitle>
          <CardDescription>
            {t('Check your Root permissions and try again.')}
          </CardDescription>
        </CardHeader>
        <CardFooter>
          <Button onClick={() => void configQuery.refetch()}>
            <HugeiconsIcon
              icon={RefreshIcon}
              strokeWidth={2}
              data-icon='inline-start'
            />
            {t('Retry')}
          </Button>
        </CardFooter>
      </Card>
    )
  }

  return (
    <div className='flex flex-col gap-4'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div>
          <h3 className='font-medium'>{t('Request archive')}</h3>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Asynchronously archives authenticated HTTP request bodies and every Realtime client frame as administrator-readable plain-text JSON. Historical encrypted archives still require the original CRYPTO_SECRET.'
            )}
          </p>
        </div>
        <div className='flex flex-wrap gap-2'>
          <Button
            variant='outline'
            size='sm'
            onClick={() => void refresh()}
            disabled={
              configQuery.isFetching ||
              runtimeQuery.isFetching ||
              channelsQuery.isFetching ||
              groupsQuery.isFetching
            }
          >
            {configQuery.isFetching ||
            runtimeQuery.isFetching ||
            channelsQuery.isFetching ||
            groupsQuery.isFetching ? (
              <Spinner data-icon='inline-start' />
            ) : (
              <HugeiconsIcon
                icon={RefreshIcon}
                strokeWidth={2}
                data-icon='inline-start'
              />
            )}
            {t('Refresh')}
          </Button>
          <Button
            size='sm'
            onClick={() => void save()}
            disabled={!dirty || saving}
          >
            {saving ? (
              <Spinner data-icon='inline-start' />
            ) : (
              <HugeiconsIcon
                icon={FloppyDiskIcon}
                strokeWidth={2}
                data-icon='inline-start'
              />
            )}
            {t('Save changes')}
          </Button>
        </div>
      </div>

      <Alert>
        <HugeiconsIcon icon={Alert02Icon} strokeWidth={2} />
        <AlertTitle>{t('Privacy and capacity notice')}</AlertTitle>
        <AlertDescription>
          {t(
            'Realtime archives include text JSON, binary JSON, and raw binary audio. These payloads may contain sensitive data, and audio can quickly consume queue and storage capacity. Set retention and access controls accordingly.'
          )}
        </AlertDescription>
      </Alert>

      <RuntimeOverview
        runtime={runtimeQuery.data}
        loading={runtimeQuery.isLoading}
        fetching={runtimeQuery.isFetching}
        error={runtimeQuery.isError ? runtimeQuery.error : null}
        onRetry={() => void runtimeQuery.refetch()}
      />

      <Card>
        <CardHeader>
          <CardTitle>{t('Archive policy')}</CardTitle>
          <CardDescription>
            {t(
              'External storage writes run in the background; archive failures never reject the client request.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <FieldGroup>
            <Field orientation='horizontal'>
              <FieldContent>
                <FieldTitle>{t('Enable request archive')}</FieldTitle>
                <FieldDescription>
                  {t(
                    'Store HTTP request bodies and Realtime client frames as administrator-readable plain-text JSON in the selected active target.'
                  )}
                </FieldDescription>
              </FieldContent>
              <Switch
                checked={draft.enabled}
                onCheckedChange={(enabled) => updateDraft({ enabled })}
                aria-label={t('Enable request archive')}
              />
            </Field>
            <Field>
              <FieldLabel>{t('Archive scope')}</FieldLabel>
              <Select
                value={draft.archive_scope || 'all_requests'}
                onValueChange={(value) => {
                  const archiveScope = (value ??
                    'all_requests') as RequestArchiveConfigDraft['archive_scope']
                  updateDraft({ archive_scope: archiveScope })
                }}
                items={[
                  { value: 'all_requests', label: t('All eligible requests') },
                  {
                    value: 'audit_events',
                    label: t('Only requests with audit events'),
                  },
                ]}
              >
                <SelectTrigger aria-label={t('Archive scope')}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    <SelectItem value='all_requests'>
                      {t('All eligible requests')}
                    </SelectItem>
                    <SelectItem value='audit_events'>
                      {t('Only requests with audit events')}
                    </SelectItem>
                  </SelectGroup>
                </SelectContent>
              </Select>
              <FieldDescription>
                {t(
                  'Audit-event mode archives the original request only after an audit event is stored successfully.'
                )}
              </FieldDescription>
            </Field>
            <FieldSet className='rounded-lg border p-4'>
              <FieldLegend>{t('Archive request filters')}</FieldLegend>
              <FieldDescription>
                {t(
                  'Limit archives by actual channels, route groups, and audit sources.'
                )}
              </FieldDescription>
              <FieldGroup className='grid gap-4 lg:grid-cols-2'>
                <Field
                  data-disabled={
                    draft.archive_scope !== 'audit_events' ||
                    channelsQuery.isLoading ||
                    channelsQuery.isError
                  }
                >
                  <FieldLabel htmlFor='archive-event-channel-ids'>
                    {t('Event channels')}
                  </FieldLabel>
                  <MultiSelect
                    id='archive-event-channel-ids'
                    options={includeMissingSensitiveRouteOptions(
                      channelOptions,
                      draft.event_channel_ids ?? [],
                      t('Unavailable channel')
                    )}
                    selected={(draft.event_channel_ids ?? []).map(String)}
                    onChange={(channelIds) =>
                      updateDraft({
                        event_channel_ids:
                          normalizeSensitiveRouteIds(channelIds),
                      })
                    }
                    placeholder={t('Select channels...')}
                    emptyText={t('No channels available.')}
                    disabled={
                      draft.archive_scope !== 'audit_events' ||
                      channelsQuery.isLoading ||
                      channelsQuery.isError
                    }
                    maxVisibleChips={3}
                  />
                  {channelsQuery.isError ? (
                    <div className='text-destructive flex flex-wrap items-center gap-2 text-xs'>
                      <span>{t('Unable to load channels')}</span>
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        onClick={() => void channelsQuery.refetch()}
                      >
                        <HugeiconsIcon
                          icon={RefreshIcon}
                          strokeWidth={2}
                          data-icon='inline-start'
                        />
                        {t('Retry')}
                      </Button>
                    </div>
                  ) : null}
                </Field>
                <Field
                  data-disabled={
                    draft.archive_scope !== 'audit_events' ||
                    groupsQuery.isLoading ||
                    groupsQuery.isError
                  }
                >
                  <FieldLabel htmlFor='archive-event-group-codes'>
                    {t('Event groups')}
                  </FieldLabel>
                  <MultiSelect
                    id='archive-event-group-codes'
                    options={includeMissingSensitiveGroupOptions(
                      groupOptions,
                      draft.event_group_codes ?? [],
                      t('Unavailable group')
                    )}
                    selected={draft.event_group_codes ?? []}
                    onChange={(groupCodes) =>
                      updateDraft({
                        event_group_codes:
                          normalizeSensitiveGroupCodes(groupCodes),
                      })
                    }
                    placeholder={t('Select groups...')}
                    emptyText={t('No groups available.')}
                    disabled={
                      draft.archive_scope !== 'audit_events' ||
                      groupsQuery.isLoading ||
                      groupsQuery.isError
                    }
                    maxVisibleChips={3}
                  />
                  {groupsQuery.isError ? (
                    <div className='text-destructive flex flex-wrap items-center gap-2 text-xs'>
                      <span>{t('Unable to load groups')}</span>
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        onClick={() => void groupsQuery.refetch()}
                      >
                        <HugeiconsIcon
                          icon={RefreshIcon}
                          strokeWidth={2}
                          data-icon='inline-start'
                        />
                        {t('Retry')}
                      </Button>
                    </div>
                  ) : null}
                </Field>
              </FieldGroup>
              <FieldDescription>
                {t(
                  'Leave a filter empty to match any value. Values within one filter use OR; different non-empty filters use AND.'
                )}
              </FieldDescription>
              <Field data-disabled={draft.archive_scope !== 'audit_events'}>
                <FieldLabel htmlFor='archive-event-sources'>
                  {t('Audit sources')}
                </FieldLabel>
                <MultiSelect
                  id='archive-event-sources'
                  options={auditSourceOptions}
                  selected={draft.event_sources ?? []}
                  onChange={(eventSources) =>
                    updateDraft({
                      event_sources: eventSources
                        .filter((source): source is RequestArchiveAuditSource =>
                          auditSourceOptions.some(
                            (option) => option.value === source
                          )
                        )
                        .sort(),
                    })
                  }
                  placeholder={t('All audit sources')}
                  emptyText={t('No audit sources available.')}
                  disabled={draft.archive_scope !== 'audit_events'}
                  maxVisibleChips={3}
                />
                <FieldDescription>
                  {draft.archive_scope === 'audit_events'
                    ? t(
                        'Leave audit sources empty to archive every source; otherwise only matching audit events trigger an archive.'
                      )
                    : t(
                        'Event filters are available only when archiving requests with audit events.'
                      )}
                </FieldDescription>
              </Field>
            </FieldSet>
            <div className='grid gap-4 md:grid-cols-3'>
              <Field>
                <FieldLabel>{t('Active storage target')}</FieldLabel>
                <Select
                  value={draft.active_target_id || null}
                  onValueChange={(value) =>
                    updateDraft({ active_target_id: value ?? '' })
                  }
                  items={draft.targets.map((target) => ({
                    value: target.id,
                    label: target.name || target.id,
                    disabled: !target.enabled,
                  }))}
                >
                  <SelectTrigger aria-label={t('Active storage target')}>
                    <SelectValue placeholder={t('Select storage target')} />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      {draft.targets.map((target) => (
                        <SelectItem
                          key={target.id}
                          value={target.id}
                          disabled={!target.enabled}
                        >
                          {target.name || target.id}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <FieldDescription>
                  {t(
                    'Only enabled targets can receive newly queued archive jobs.'
                  )}
                </FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor='archive-retention-days'>
                  {t('Retention days')}
                </FieldLabel>
                <Input
                  id='archive-retention-days'
                  type='number'
                  min={1}
                  max={3650}
                  step={1}
                  value={String(draft.retention_days)}
                  onChange={(event) =>
                    updateDraft({ retention_days: Number(event.target.value) })
                  }
                />
              </Field>
              <Field>
                <FieldLabel htmlFor='archive-worker-count'>
                  {t('Worker count')}
                </FieldLabel>
                <Input
                  id='archive-worker-count'
                  type='number'
                  min={1}
                  max={32}
                  step={1}
                  value={String(draft.worker_count)}
                  onChange={(event) =>
                    updateDraft({ worker_count: Number(event.target.value) })
                  }
                />
              </Field>
              <Field>
                <FieldLabel htmlFor='archive-queue-capacity'>
                  {t('Queue capacity')}
                </FieldLabel>
                <Input
                  id='archive-queue-capacity'
                  type='number'
                  min={1}
                  max={1048576}
                  step={1}
                  value={String(draft.queue_capacity)}
                  onChange={(event) =>
                    updateDraft({ queue_capacity: Number(event.target.value) })
                  }
                />
              </Field>
              <Field>
                <FieldLabel htmlFor='archive-max-body-bytes'>
                  {t('Maximum request body bytes')}
                </FieldLabel>
                <Input
                  id='archive-max-body-bytes'
                  type='number'
                  min={1024}
                  max={128 * 1024 * 1024}
                  step={1}
                  value={String(draft.max_body_bytes)}
                  onChange={(event) =>
                    updateDraft({ max_body_bytes: Number(event.target.value) })
                  }
                />
                <FieldDescription>
                  {t(
                    'Maximum bytes retained for one HTTP request body or Realtime client frame.'
                  )}
                </FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor='archive-queue-max-bytes'>
                  {t('Queue body byte limit')}
                </FieldLabel>
                <Input
                  id='archive-queue-max-bytes'
                  type='number'
                  min={draft.max_body_bytes}
                  max={64 * 1024 * 1024 * 1024}
                  step={1}
                  value={String(draft.queue_max_bytes)}
                  onChange={(event) =>
                    updateDraft({ queue_max_bytes: Number(event.target.value) })
                  }
                />
                <FieldDescription>
                  {t(
                    'Maximum combined HTTP request body and Realtime client frame bytes awaiting archive storage.'
                  )}
                </FieldDescription>
              </Field>
            </div>
          </FieldGroup>
        </CardContent>
      </Card>

      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div>
          <h3 className='font-medium'>{t('Archive storage targets')}</h3>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Configure local disks, S3-compatible object storage, or Cloudflare R2. S3/R2 credentials are write-only and require CRYPTO_SECRET.'
            )}
          </p>
        </div>
        <Button
          size='sm'
          onClick={() =>
            updateDraft({ targets: [...draft.targets, createTarget()] })
          }
        >
          <HugeiconsIcon
            icon={Add01Icon}
            strokeWidth={2}
            data-icon='inline-start'
          />
          {t('Add storage target')}
        </Button>
      </div>

      {draft.targets.length === 0 ? (
        <Empty className='rounded-xl border'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              <HugeiconsIcon icon={Database01Icon} strokeWidth={2} />
            </EmptyMedia>
            <EmptyTitle>{t('No archive storage targets')}</EmptyTitle>
            <EmptyDescription>
              {t(
                'Add a local, S3-compatible, or Cloudflare R2 target before enabling complete request archiving.'
              )}
            </EmptyDescription>
          </EmptyHeader>
          <EmptyContent>
            <Button
              onClick={() =>
                updateDraft({ targets: [...draft.targets, createTarget()] })
              }
            >
              <HugeiconsIcon
                icon={Add01Icon}
                strokeWidth={2}
                data-icon='inline-start'
              />
              {t('Add storage target')}
            </Button>
          </EmptyContent>
        </Empty>
      ) : (
        <div className='grid gap-4 xl:grid-cols-2'>
          {draft.targets.map((target) => (
            <ArchiveTargetCard
              key={target.id}
              target={target}
              active={target.id === draft.active_target_id}
              probing={probingId === target.id}
              onChange={(patch) => updateTarget(target.id, patch)}
              onRemove={() => setDeleteTargetId(target.id)}
              onProbe={() => void probe(target)}
            />
          ))}
        </div>
      )}

      <AlertDialog
        open={deleteTargetId !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTargetId(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} />
            </AlertDialogMedia>
            <AlertDialogTitle>
              {t('Remove archive storage target?')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'The target is removed from the draft. Previously archived objects are not deleted by this action.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction
              variant='destructive'
              onClick={() => {
                if (!deleteTargetId) return
                setDraft((current) => {
                  if (!current) return current
                  return {
                    ...current,
                    active_target_id:
                      current.active_target_id === deleteTargetId
                        ? ''
                        : current.active_target_id,
                    targets: current.targets.filter(
                      (target) => target.id !== deleteTargetId
                    ),
                  }
                })
                setDeleteTargetId(null)
              }}
            >
              {t('Remove')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
