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
import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
  type PaginationState,
  type RowSelectionState,
} from '@tanstack/react-table'
import {
  Copy01Icon,
  Database01Icon,
  Delete02Icon,
  FilterIcon,
  RefreshIcon,
  Search01Icon,
  ViewIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
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
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { DataTablePage } from '@/components/data-table'
import {
  batchDeleteSecurityAuditEvents,
  deleteSecurityAuditEvent,
  deleteSecurityAuditEventsByFilter,
  getSecurityAuditEvent,
  getSecurityAuditEvents,
  hasSecurityAuditEventFilter,
  previewSecurityAuditDelete,
} from './api'
import {
  DecisionBadge,
  formatAuditInteger,
  formatAuditTime,
  type SensitiveActionRunner,
} from './shared'
import type {
  SecurityAuditDeletePreview,
  SecurityAuditEndpointDraft,
  SecurityAuditEvent,
  SecurityAuditEventDetail,
  SecurityAuditEventFilter,
} from './types'

const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

function DetailItem({
  label,
  value,
}: {
  label: string
  value: React.ReactNode
}) {
  return (
    <div className='flex flex-col gap-1'>
      <span className='text-muted-foreground text-xs'>{label}</span>
      <span className='text-sm break-all'>{value || '-'}</span>
    </div>
  )
}

export function SecurityAuditEventsView({
  endpoints,
  runSensitive,
}: {
  endpoints: SecurityAuditEndpointDraft[]
  runSensitive: SensitiveActionRunner
}) {
  const { t } = useTranslation()
  const [draftFilter, setDraftFilter] = useState<SecurityAuditEventFilter>({})
  const [filter, setFilter] = useState<SecurityAuditEventFilter>({})
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: 20,
  })
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({})
  const [detail, setDetail] = useState<SecurityAuditEventDetail | null>(null)
  const [detailLoading, setDetailLoading] = useState<number | null>(null)
  const [singleDelete, setSingleDelete] = useState<SecurityAuditEvent | null>(
    null
  )
  const [batchDeleteOpen, setBatchDeleteOpen] = useState(false)
  const [deletePreview, setDeletePreview] =
    useState<SecurityAuditDeletePreview | null>(null)
  const [deletePreviewFilter, setDeletePreviewFilter] =
    useState<SecurityAuditEventFilter | null>(null)
  const [deleting, setDeleting] = useState(false)
  const hasActiveFilter = hasSecurityAuditEventFilter(filter)

  const eventsQuery = useQuery({
    queryKey: [
      'security-audit',
      'events',
      filter,
      pagination.pageIndex,
      pagination.pageSize,
    ],
    queryFn: () =>
      getSecurityAuditEvents(
        filter,
        pagination.pageIndex + 1,
        pagination.pageSize
      ),
    placeholderData: (previous) => previous,
  })

  const openDetail = async (event: SecurityAuditEvent) => {
    await runSensitive(
      async () => {
        setDetailLoading(event.id)
        try {
          const result = await getSecurityAuditEvent(event.id)
          setDetail(result)
          return result
        } finally {
          setDetailLoading(null)
        }
      },
      {
        title: t('Verify prompt access'),
        description: t(
          'The full prompt is encrypted at rest. Confirm your identity before temporarily decrypting it.'
        ),
      }
    )
  }

  const columns = useMemo<ColumnDef<SecurityAuditEvent>[]>(
    () => [
      {
        id: 'select',
        header: ({ table }) => (
          <Checkbox
            checked={table.getIsAllPageRowsSelected()}
            indeterminate={table.getIsSomePageRowsSelected()}
            onCheckedChange={(checked) =>
              table.toggleAllPageRowsSelected(checked === true)
            }
            aria-label={t('Select all rows')}
          />
        ),
        cell: ({ row }) => (
          <Checkbox
            checked={row.getIsSelected()}
            onCheckedChange={(checked) => row.toggleSelected(checked === true)}
            aria-label={t('Select row')}
          />
        ),
        enableSorting: false,
      },
      {
        accessorKey: 'created_at',
        header: t('Time'),
        cell: ({ row }) => (
          <span className='whitespace-nowrap'>
            {formatAuditTime(row.original.created_at)}
          </span>
        ),
      },
      {
        accessorKey: 'decision',
        header: t('Decision'),
        cell: ({ row }) => (
          <DecisionBadge decision={row.original.decision} t={t} />
        ),
      },
      {
        id: 'identity',
        header: t('User'),
        cell: ({ row }) => (
          <div className='flex min-w-28 flex-col gap-0.5'>
            <span className='truncate font-medium'>
              {row.original.username || `#${row.original.user_id}`}
            </span>
            <span className='text-muted-foreground truncate text-xs'>
              {row.original.group_name || `ID ${row.original.group_id}`}
            </span>
          </div>
        ),
      },
      {
        accessorKey: 'redacted_preview',
        header: t('Prompt preview'),
        cell: ({ row }) => (
          <div className='max-w-md'>
            <p className='line-clamp-2 break-all'>
              {row.original.redacted_preview || t('No preview')}
            </p>
            <p className='text-muted-foreground mt-1 font-mono text-xs'>
              {row.original.prompt_hash.slice(0, 16)}…
            </p>
          </div>
        ),
      },
      {
        id: 'source',
        header: t('Source'),
        cell: ({ row }) => (
          <div className='flex min-w-32 flex-col gap-0.5'>
            <span className='truncate'>{row.original.model || '-'}</span>
            <span className='text-muted-foreground truncate text-xs'>
              {row.original.endpoint || row.original.protocol}
            </span>
          </div>
        ),
      },
      {
        id: 'risk',
        header: t('Risk'),
        cell: ({ row }) => (
          <div className='flex max-w-48 flex-wrap gap-1'>
            {(row.original.categories ?? []).length > 0 ? (
              row.original.categories.slice(0, 2).map((category) => (
                <Badge key={category} variant='outline'>
                  {category}
                </Badge>
              ))
            ) : (
              <span className='text-muted-foreground'>-</span>
            )}
          </div>
        ),
      },
      {
        accessorKey: 'latency_ms',
        header: t('Latency'),
        cell: ({ row }) => `${row.original.latency_ms} ms`,
      },
      {
        id: 'actions',
        header: t('Actions'),
        cell: ({ row }) => (
          <div className='flex items-center gap-1'>
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={() => void openDetail(row.original)}
              disabled={detailLoading === row.original.id}
              aria-label={t('View full prompt')}
            >
              {detailLoading === row.original.id ? (
                <Spinner />
              ) : (
                <HugeiconsIcon icon={ViewIcon} strokeWidth={2} />
              )}
            </Button>
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={() => setSingleDelete(row.original)}
              aria-label={t('Delete event')}
            >
              <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} />
            </Button>
          </div>
        ),
      },
    ],
    [detailLoading, t]
  )

  const table = useReactTable({
    data: eventsQuery.data?.items ?? [],
    columns,
    state: { pagination, rowSelection },
    onPaginationChange: setPagination,
    onRowSelectionChange: setRowSelection,
    getCoreRowModel: getCoreRowModel(),
    getRowId: (row) => String(row.id),
    enableRowSelection: true,
    manualPagination: true,
    pageCount: Math.max(
      1,
      Math.ceil((eventsQuery.data?.total ?? 0) / pagination.pageSize)
    ),
  })

  const selectedIds = table
    .getSelectedRowModel()
    .rows.map((row) => row.original.id)

  const refreshAfterDelete = async () => {
    setRowSelection({})
    await eventsQuery.refetch()
  }

  const deleteOne = async () => {
    if (!singleDelete) return
    await runSensitive(
      async () => {
        setDeleting(true)
        try {
          const result = await deleteSecurityAuditEvent(singleDelete.id)
          toast.success(
            t('{{count}} audit event deleted', {
              count: result.deleted_events,
            })
          )
          setSingleDelete(null)
          await refreshAfterDelete()
          return result
        } finally {
          setDeleting(false)
        }
      },
      {
        title: t('Verify audit event deletion'),
        description: t('Deleted encrypted audit data cannot be recovered.'),
      }
    )
  }

  const deleteSelected = async () => {
    if (selectedIds.length === 0) return
    await runSensitive(
      async () => {
        setDeleting(true)
        try {
          const result = await batchDeleteSecurityAuditEvents(selectedIds)
          toast.success(
            t('{{count}} audit events deleted', {
              count: result.deleted_events,
            })
          )
          setBatchDeleteOpen(false)
          await refreshAfterDelete()
          return result
        } finally {
          setDeleting(false)
        }
      },
      {
        title: t('Verify audit event deletion'),
        description: t('Deleted encrypted audit data cannot be recovered.'),
      }
    )
  }

  const previewFilteredDelete = async () => {
    await runSensitive(
      async () => {
        setDeleting(true)
        try {
          const result = await previewSecurityAuditDelete(filter)
          if (result.matched_count === 0) {
            toast.info(t('No audit events match the current filter.'))
          } else {
            setDeletePreview(result)
            setDeletePreviewFilter({ ...filter })
          }
          return result
        } finally {
          setDeleting(false)
        }
      },
      {
        title: t('Verify filtered deletion preview'),
        description: t(
          'Confirm your identity before creating a five-minute deletion preview.'
        ),
      }
    )
  }

  const deleteByFilter = async () => {
    if (!deletePreview || !deletePreviewFilter) return
    await runSensitive(
      async () => {
        setDeleting(true)
        try {
          const result = await deleteSecurityAuditEventsByFilter(
            deletePreviewFilter,
            deletePreview
          )
          toast.success(
            t('{{count}} audit events deleted', {
              count: result.deleted_events,
            })
          )
          setDeletePreview(null)
          setDeletePreviewFilter(null)
          await refreshAfterDelete()
          return result
        } finally {
          setDeleting(false)
        }
      },
      {
        title: t('Verify filtered audit event deletion'),
        description: t('Deleted encrypted audit data cannot be recovered.'),
      }
    )
  }

  const applyFilter = () => {
    setPagination((current) => ({ ...current, pageIndex: 0 }))
    setRowSelection({})
    setFilter(draftFilter)
  }

  const resetFilter = () => {
    setDraftFilter({})
    setFilter({})
    setPagination((current) => ({ ...current, pageIndex: 0 }))
    setRowSelection({})
  }

  return (
    <>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={eventsQuery.isLoading}
        isFetching={eventsQuery.isFetching}
        emptyTitle={t('No audit events')}
        emptyDescription={t(
          'Risk events and optionally Safe events will appear here after audit is enabled.'
        )}
        emptyIcon={<HugeiconsIcon icon={Database01Icon} strokeWidth={2} />}
        paginationInFooter={false}
        toolbar={
          <Card>
            <CardHeader>
              <CardTitle className='flex items-center gap-2'>
                <HugeiconsIcon icon={FilterIcon} strokeWidth={2} />
                {t('Event filters')}
              </CardTitle>
              <CardDescription>
                {t('Filters and pagination are evaluated by the server.')}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <FieldGroup>
                <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-4'>
                  <Field>
                    <FieldLabel htmlFor='audit-event-keyword'>
                      {t('Preview keyword')}
                    </FieldLabel>
                    <Input
                      id='audit-event-keyword'
                      value={draftFilter.keyword ?? ''}
                      onChange={(event) =>
                        setDraftFilter((current) => ({
                          ...current,
                          keyword: event.target.value,
                        }))
                      }
                      onKeyDown={(event) => {
                        if (event.key === 'Enter') applyFilter()
                      }}
                    />
                  </Field>
                  <Field>
                    <FieldLabel>{t('Decision')}</FieldLabel>
                    <Select
                      items={[
                        { value: 'all', label: t('All decisions') },
                        { value: 'pass', label: t('Allowed') },
                        { value: 'flag', label: t('Flagged') },
                        { value: 'critical', label: t('Blocked') },
                        { value: 'error', label: t('Error') },
                      ]}
                      value={draftFilter.decision || 'all'}
                      onValueChange={(value) =>
                        setDraftFilter((current) => ({
                          ...current,
                          decision:
                            value === 'all' || value === null ? '' : value,
                        }))
                      }
                    >
                      <SelectTrigger aria-label={t('Decision')}>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          <SelectItem value='all'>
                            {t('All decisions')}
                          </SelectItem>
                          <SelectItem value='pass'>{t('Allowed')}</SelectItem>
                          <SelectItem value='flag'>{t('Flagged')}</SelectItem>
                          <SelectItem value='critical'>
                            {t('Blocked')}
                          </SelectItem>
                          <SelectItem value='error'>{t('Error')}</SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </Field>
                  <Field>
                    <FieldLabel>{t('Guard node')}</FieldLabel>
                    <Select
                      items={[
                        { value: 'all', label: t('All Guard nodes') },
                        ...endpoints.map((endpoint) => ({
                          value: endpoint.id,
                          label: endpoint.name || endpoint.id,
                        })),
                      ]}
                      value={draftFilter.endpoint || 'all'}
                      onValueChange={(value) =>
                        setDraftFilter((current) => ({
                          ...current,
                          endpoint:
                            value === 'all' || value === null ? '' : value,
                        }))
                      }
                    >
                      <SelectTrigger aria-label={t('Guard node')}>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          <SelectItem value='all'>
                            {t('All Guard nodes')}
                          </SelectItem>
                          {endpoints.map((endpoint) => (
                            <SelectItem key={endpoint.id} value={endpoint.id}>
                              {endpoint.name || endpoint.id}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor='audit-event-request-id'>
                      {t('Request ID')}
                    </FieldLabel>
                    <Input
                      id='audit-event-request-id'
                      value={draftFilter.request_id ?? ''}
                      onChange={(event) =>
                        setDraftFilter((current) => ({
                          ...current,
                          request_id: event.target.value,
                        }))
                      }
                    />
                  </Field>
                </div>
                <div className='flex flex-wrap items-center gap-2'>
                  <Button size='sm' onClick={applyFilter}>
                    <HugeiconsIcon
                      icon={Search01Icon}
                      strokeWidth={2}
                      data-icon='inline-start'
                    />
                    {t('Apply filters')}
                  </Button>
                  <Button variant='outline' size='sm' onClick={resetFilter}>
                    {t('Reset')}
                  </Button>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={() => void eventsQuery.refetch()}
                    disabled={eventsQuery.isFetching}
                  >
                    {eventsQuery.isFetching ? (
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
                  {selectedIds.length > 0 && (
                    <Button
                      variant='destructive'
                      size='sm'
                      onClick={() => setBatchDeleteOpen(true)}
                    >
                      <HugeiconsIcon
                        icon={Delete02Icon}
                        strokeWidth={2}
                        data-icon='inline-start'
                      />
                      {t('Delete selected ({{count}})', {
                        count: selectedIds.length,
                      })}
                    </Button>
                  )}
                  <Button
                    variant='destructive'
                    size='sm'
                    onClick={() => void previewFilteredDelete()}
                    disabled={deleting || !hasActiveFilter}
                  >
                    <HugeiconsIcon
                      icon={Delete02Icon}
                      strokeWidth={2}
                      data-icon='inline-start'
                    />
                    {t('Delete matching events')}
                  </Button>
                </div>
              </FieldGroup>
            </CardContent>
          </Card>
        }
        afterTable={
          <div className='flex flex-wrap items-center justify-between gap-3 text-sm'>
            <span className='text-muted-foreground'>
              {t('{{count}} events', {
                count: formatAuditInteger(eventsQuery.data?.total),
              })}
            </span>
            <Field orientation='horizontal' className='w-auto'>
              <FieldLabel htmlFor='audit-page-size'>
                {t('Rows per page')}
              </FieldLabel>
              <Select
                items={PAGE_SIZE_OPTIONS.map((value) => ({
                  value: String(value),
                  label: String(value),
                }))}
                value={String(pagination.pageSize)}
                onValueChange={(value) => {
                  if (!value) return
                  setPagination({ pageIndex: 0, pageSize: Number(value) })
                }}
              >
                <SelectTrigger id='audit-page-size' className='w-20'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    {PAGE_SIZE_OPTIONS.map((value) => (
                      <SelectItem key={value} value={String(value)}>
                        {value}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
          </div>
        }
      />

      <Dialog
        open={detail !== null}
        onOpenChange={(open) => !open && setDetail(null)}
      >
        <DialogContent className='max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-4xl'>
          <DialogHeader>
            <DialogTitle>{t('Audit event details')}</DialogTitle>
            <DialogDescription>
              {t(
                'This response is not cached. Close the dialog when you finish reviewing the decrypted prompt.'
              )}
            </DialogDescription>
          </DialogHeader>
          {detail && (
            <div className='flex flex-col gap-4'>
              <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
                <DetailItem label={t('Event ID')} value={detail.id} />
                <DetailItem
                  label={t('Time')}
                  value={formatAuditTime(detail.created_at)}
                />
                <DetailItem
                  label={t('Decision')}
                  value={<DecisionBadge decision={detail.decision} t={t} />}
                />
                <DetailItem label={t('Risk level')} value={detail.risk_level} />
                <DetailItem
                  label={t('User')}
                  value={detail.username || detail.user_id}
                />
                <DetailItem
                  label={t('API key')}
                  value={detail.api_key_name || detail.api_key_id}
                />
                <DetailItem
                  label={t('Group')}
                  value={detail.group_name || detail.group_id}
                />
                <DetailItem label={t('Model')} value={detail.model} />
                <DetailItem label={t('Request ID')} value={detail.request_id} />
                <DetailItem
                  label={t('Prompt hash')}
                  value={detail.prompt_hash}
                />
                <DetailItem
                  label={t('Guard node')}
                  value={detail.guard_endpoint_id}
                />
                <DetailItem
                  label={t('Latency')}
                  value={`${detail.latency_ms} ms`}
                />
              </div>
              <div className='flex flex-col gap-2'>
                <div className='flex flex-wrap items-center justify-between gap-2'>
                  <h4 className='font-medium'>{t('Decrypted prompt')}</h4>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={async () => {
                      await navigator.clipboard.writeText(detail.full_prompt)
                      toast.success(t('Copied'))
                    }}
                  >
                    <HugeiconsIcon
                      icon={Copy01Icon}
                      strokeWidth={2}
                      data-icon='inline-start'
                    />
                    {t('Copy')}
                  </Button>
                </div>
                <pre className='bg-muted max-h-[45vh] overflow-auto rounded-lg p-4 text-xs break-words whitespace-pre-wrap'>
                  {detail.full_prompt}
                </pre>
                {detail.prompt_truncated && (
                  <Badge variant='outline'>
                    {t('Stored prompt was truncated')}
                  </Badge>
                )}
              </div>
            </div>
          )}
          <DialogFooter>
            <Button variant='outline' onClick={() => setDetail(null)}>
              {t('Close')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={singleDelete !== null}
        onOpenChange={(open) => !open && setSingleDelete(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} />
            </AlertDialogMedia>
            <AlertDialogTitle>{t('Delete this audit event?')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'The encrypted prompt and associated finished task cannot be recovered.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction
              variant='destructive'
              disabled={deleting}
              onClick={() => void deleteOne()}
            >
              {deleting && <Spinner data-icon='inline-start' />}
              {t('Delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog
        open={batchDeleteOpen}
        onOpenChange={(open) => setBatchDeleteOpen(open)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} />
            </AlertDialogMedia>
            <AlertDialogTitle>
              {t('Delete selected audit events?')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t('{{count}} selected events will be permanently deleted.', {
                count: selectedIds.length,
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction
              variant='destructive'
              disabled={deleting}
              onClick={() => void deleteSelected()}
            >
              {deleting && <Spinner data-icon='inline-start' />}
              {t('Delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog
        open={deletePreview !== null}
        onOpenChange={(open) => {
          if (!open) {
            setDeletePreview(null)
            setDeletePreviewFilter(null)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} />
            </AlertDialogMedia>
            <AlertDialogTitle>
              {t('Confirm filtered deletion')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                '{{count}} events up to snapshot ID {{id}} match. Events created after this preview are protected.',
                {
                  count: deletePreview?.matched_count ?? 0,
                  id: deletePreview?.snapshot_max_id ?? 0,
                }
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction
              variant='destructive'
              disabled={deleting}
              onClick={() => void deleteByFilter()}
            >
              {deleting && <Spinner data-icon='inline-start' />}
              {t('Delete matching events')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
