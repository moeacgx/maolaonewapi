import { useQuery } from '@tanstack/react-query'
import {
  BarChart3,
  Ban,
  FilePenLine,
  Pause,
  Play,
  Plus,
  RefreshCw,
  Square,
  Ticket,
  Trash2,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  DataTableRowActionMenu,
  StaticDataTable,
} from '@/components/data-table'
import {
  sideDrawerContentClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { formatQuota } from '@/lib/format'

import {
  createAdminBenefitActivity,
  deleteAdminBenefitActivities,
  getBenefitGroupOptions,
  getAdminBenefitActivities,
  publishAdminBenefitActivity,
  terminateAdminBenefitActivity,
  transitionAdminBenefitActivity,
  updateAdminBenefitActivity,
} from '../api'
import { activityStatusLabel } from '../lib/labels'
import type { BenefitActivity, BenefitActivityStatus } from '../types'
import { BenefitActivityForm } from './benefit-activity-form'
import { BenefitActivityReport } from './benefit-activity-report'
import { BenefitTerminateDialog } from './benefit-terminate-dialog'
import { BenefitVouchersSheet } from './benefit-vouchers-sheet'

const DELETABLE_STATUSES = new Set<BenefitActivityStatus>([
  'draft',
  'ended',
  'terminated',
])

const ALL_ACTIVITY_STATUSES: BenefitActivityStatus[] = [
  'draft',
  'published',
  'paused',
  'ended',
  'terminated',
]

const EMPTY_ACTIVITIES: BenefitActivity[] = []

function isDeletableStatus(status: BenefitActivityStatus): boolean {
  return DELETABLE_STATUSES.has(status)
}

export function BenefitActivitiesPanel() {
  const { t } = useTranslation()
  const [formTarget, setFormTarget] = useState<
    'create' | BenefitActivity | null
  >(null)
  const [reportActivity, setReportActivity] = useState<BenefitActivity | null>(
    null
  )
  const [vouchersActivity, setVouchersActivity] =
    useState<BenefitActivity | null>(null)
  const [terminateID, setTerminateID] = useState<number | null>(null)
  const [selectedIds, setSelectedIds] = useState<ReadonlySet<number>>(new Set())
  const [keyword, setKeyword] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [deleting, setDeleting] = useState(false)

  const query = useQuery({
    queryKey: ['benefit', 'admin', 'activities'],
    queryFn: getAdminBenefitActivities,
  })
  const groupsQuery = useQuery({
    queryKey: ['benefit', 'groups'],
    queryFn: getBenefitGroupOptions,
  })

  const activities = query.data ?? EMPTY_ACTIVITIES
  const filteredActivities = useMemo(() => {
    const normalizedKeyword = keyword.trim().toLowerCase()
    return activities.filter((activity) => {
      if (statusFilter && activity.status !== statusFilter) return false
      if (!normalizedKeyword) return true
      return (
        activity.name.toLowerCase().includes(normalizedKeyword) ||
        activity.group_name_snapshot.toLowerCase().includes(normalizedKeyword)
      )
    })
  }, [activities, keyword, statusFilter])
  const deletableFilteredIds = filteredActivities
    .filter((activity) => isDeletableStatus(activity.status))
    .map((activity) => activity.id)
  const allDeletableSelected =
    deletableFilteredIds.length > 0 &&
    deletableFilteredIds.every((id) => selectedIds.has(id))

  const refresh = async () => {
    await query.refetch()
  }

  const toggleSelected = (id: number, checked: boolean) => {
    setSelectedIds((current) => {
      const next = new Set(current)
      if (checked) next.add(id)
      else next.delete(id)
      return next
    })
  }

  const toggleSelectAllDeletable = (checked: boolean) => {
    setSelectedIds(checked ? new Set(deletableFilteredIds) : new Set())
  }

  const create = async (
    input: Parameters<typeof createAdminBenefitActivity>[0]
  ) => {
    const response = await createAdminBenefitActivity(input)
    if (!response.success) {
      toast.error(response.message ?? t('Failed to save benefit activity'))
      return
    }
    toast.success(t('Benefit activity draft created'))
    setFormTarget(null)
    await refresh()
  }

  const save = async (
    input: Parameters<typeof updateAdminBenefitActivity>[0]
  ) => {
    const response = await updateAdminBenefitActivity(input)
    if (!response.success) {
      toast.error(response.message ?? t('Failed to save benefit activity'))
      return
    }
    toast.success(t('Benefit activity saved'))
    setFormTarget(null)
    await refresh()
  }

  const runAction = async (
    action: () => Promise<{ success: boolean; message?: string }>
  ) => {
    try {
      const response = await action()
      if (!response.success) {
        toast.error(response.message ?? t('Benefit activity operation failed'))
        return
      }
      await refresh()
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Benefit activity operation failed')
      )
    }
  }

  const deleteSelected = async () => {
    setDeleting(true)
    try {
      const ids = [...selectedIds]
      const response = await deleteAdminBenefitActivities(ids)
      if (!response.success) {
        toast.error(response.message ?? t('Failed to delete activities'))
        return
      }
      const deletedCount = response.data?.deleted ?? 0
      const skippedCount = response.data?.skipped ?? 0
      if (skippedCount > 0) {
        toast.warning(
          t(
            'Deleted {{deleted}} activities; {{skipped}} were skipped because they are still active or not eligible for deletion',
            { deleted: deletedCount, skipped: skippedCount }
          )
        )
      } else {
        toast.success(
          t('Deleted {{count}} activities', { count: deletedCount })
        )
      }
      setSelectedIds(new Set())
      setConfirmDelete(false)
      await refresh()
    } finally {
      setDeleting(false)
    }
  }

  return (
    <div className='grid gap-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div className='flex flex-wrap items-center gap-2'>
          <Input
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            placeholder={t('Search by name or group')}
            className='w-56'
            aria-label={t('Search by name or group')}
          />
          <Select
            items={[
              { value: '', label: t('All statuses') },
              ...ALL_ACTIVITY_STATUSES.map((status) => ({
                value: status,
                label: activityStatusLabel(status, t),
              })),
            ]}
            value={statusFilter}
            onValueChange={(value) => setStatusFilter(value ?? '')}
          >
            <SelectTrigger className='w-40'>
              <SelectValue placeholder={t('All statuses')} />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                <SelectItem value=''>{t('All statuses')}</SelectItem>
                {ALL_ACTIVITY_STATUSES.map((status) => (
                  <SelectItem key={status} value={status}>
                    {activityStatusLabel(status, t)}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>
        <div className='flex gap-2'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => void refresh()}
          >
            <RefreshCw />
            {t('Refresh')}
          </Button>
          <Button
            type='button'
            size='sm'
            onClick={() => setFormTarget('create')}
          >
            <Plus />
            {t('Create activity')}
          </Button>
        </div>
      </div>

      {selectedIds.size > 0 ? (
        <div className='bg-muted/40 border-border flex items-center justify-between gap-2 rounded-md border px-3 py-2 text-sm'>
          <span>
            {t('{{count}} activities selected', { count: selectedIds.size })}
          </span>
          <div className='flex gap-2'>
            <Button
              type='button'
              size='sm'
              variant='outline'
              onClick={() => setSelectedIds(new Set())}
            >
              {t('Clear selection')}
            </Button>
            <Button
              type='button'
              size='sm'
              variant='destructive'
              onClick={() => setConfirmDelete(true)}
            >
              <Trash2 />
              {t('Delete selected')}
            </Button>
          </div>
        </div>
      ) : null}

      {query.isError ? (
        <p className='text-destructive text-sm'>
          {query.error instanceof Error
            ? query.error.message
            : t('Benefit activity operation failed')}
        </p>
      ) : null}

      <div className='border-border bg-card overflow-hidden rounded-xl border'>
        <StaticDataTable
          data={filteredActivities}
          getRowKey={(activity) => activity.id}
          emptyClassName={
            query.isLoading ? 'py-8' : 'text-muted-foreground py-8'
          }
          emptyContent={
            query.isLoading ? t('Loading...') : t('No benefit activities')
          }
          columns={[
            {
              id: 'select',
              header: (
                <Checkbox
                  checked={allDeletableSelected}
                  onCheckedChange={(checked) =>
                    toggleSelectAllDeletable(checked === true)
                  }
                  disabled={deletableFilteredIds.length === 0}
                  aria-label={t('Select all deletable activities')}
                />
              ),
              cell: (activity) =>
                isDeletableStatus(activity.status) ? (
                  <Checkbox
                    checked={selectedIds.has(activity.id)}
                    onCheckedChange={(checked) =>
                      toggleSelected(activity.id, checked === true)
                    }
                    aria-label={t('Select {{name}}', { name: activity.name })}
                  />
                ) : (
                  <Checkbox
                    checked={false}
                    disabled
                    title={t(
                      'End or terminate the activity before deleting it'
                    )}
                    aria-label={t('Select {{name}}', { name: activity.name })}
                  />
                ),
            },
            {
              id: 'id',
              header: t('ID'),
              cell: (activity) => <TableId value={activity.id} />,
            },
            {
              id: 'name',
              header: t('Name'),
              cell: (activity) => (
                <div>
                  <div className='font-medium'>{activity.name}</div>
                  <div className='text-muted-foreground text-xs'>
                    {activity.group_name_snapshot}
                  </div>
                </div>
              ),
            },
            {
              id: 'budget',
              header: t('Total budget'),
              cell: (activity) => formatQuota(activity.total_quota),
            },
            {
              id: 'count',
              header: t('Total count'),
              cell: (activity) => activity.total_count,
            },
            {
              id: 'status',
              header: t('Status'),
              cell: (activity) => (
                <StatusBadge
                  label={activityStatusLabel(activity.status, t)}
                  variant={
                    activity.status === 'published' ? 'success' : 'neutral'
                  }
                  copyable={false}
                />
              ),
            },
            {
              id: 'actions',
              header: t('Actions'),
              className: 'text-right',
              cellClassName: 'text-right',
              cell: (activity) => (
                <DataTableRowActionMenu ariaLabel={t('Actions')}>
                  <DropdownMenuItem onClick={() => setFormTarget(activity)}>
                    {t('Edit')}
                    <DropdownMenuShortcut>
                      <FilePenLine size={16} />
                    </DropdownMenuShortcut>
                  </DropdownMenuItem>
                  {activity.status === 'draft' ? (
                    <DropdownMenuItem
                      onClick={() =>
                        void runAction(() =>
                          publishAdminBenefitActivity(activity.id)
                        )
                      }
                    >
                      {t('Publish')}
                      <DropdownMenuShortcut>
                        <Play size={16} />
                      </DropdownMenuShortcut>
                    </DropdownMenuItem>
                  ) : null}
                  {activity.status === 'published' ? (
                    <DropdownMenuItem
                      onClick={() =>
                        void runAction(() =>
                          transitionAdminBenefitActivity(activity.id, 'pause')
                        )
                      }
                    >
                      {t('Pause')}
                      <DropdownMenuShortcut>
                        <Pause size={16} />
                      </DropdownMenuShortcut>
                    </DropdownMenuItem>
                  ) : null}
                  {activity.status === 'paused' ? (
                    <DropdownMenuItem
                      onClick={() =>
                        void runAction(() =>
                          transitionAdminBenefitActivity(activity.id, 'resume')
                        )
                      }
                    >
                      {t('Resume')}
                      <DropdownMenuShortcut>
                        <Play size={16} />
                      </DropdownMenuShortcut>
                    </DropdownMenuItem>
                  ) : null}
                  {activity.status === 'published' ||
                  activity.status === 'paused' ? (
                    <DropdownMenuItem
                      onClick={() =>
                        void runAction(() =>
                          transitionAdminBenefitActivity(activity.id, 'end')
                        )
                      }
                    >
                      {t('End now')}
                      <DropdownMenuShortcut>
                        <Square size={16} />
                      </DropdownMenuShortcut>
                    </DropdownMenuItem>
                  ) : null}
                  <DropdownMenuSeparator />
                  <DropdownMenuItem onClick={() => setReportActivity(activity)}>
                    {t('Report')}
                    <DropdownMenuShortcut>
                      <BarChart3 size={16} />
                    </DropdownMenuShortcut>
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={() => setVouchersActivity(activity)}
                  >
                    {t('Vouchers')}
                    <DropdownMenuShortcut>
                      <Ticket size={16} />
                    </DropdownMenuShortcut>
                  </DropdownMenuItem>
                  {activity.status === 'published' ||
                  activity.status === 'paused' ? (
                    <>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        variant='destructive'
                        onClick={() => setTerminateID(activity.id)}
                      >
                        {t('Terminate')}
                        <DropdownMenuShortcut>
                          <Ban size={16} />
                        </DropdownMenuShortcut>
                      </DropdownMenuItem>
                    </>
                  ) : null}
                </DataTableRowActionMenu>
              ),
            },
          ]}
        />
      </div>

      <Sheet
        open={formTarget !== null}
        onOpenChange={(open) => {
          if (!open) setFormTarget(null)
        }}
      >
        <SheetContent className={sideDrawerContentClassName('sm:max-w-2xl')}>
          <SheetHeader className={sideDrawerHeaderClassName()}>
            <SheetTitle>
              {formTarget !== 'create' && formTarget
                ? t('Edit benefit activity')
                : t('Create benefit activity')}
            </SheetTitle>
          </SheetHeader>
          <div className={sideDrawerFormClassName()}>
            {formTarget !== null ? (
              <BenefitActivityForm
                key={formTarget === 'create' ? 'create' : formTarget.id}
                initial={formTarget === 'create' ? undefined : formTarget}
                groupOptions={groupsQuery.data}
                onCancel={() => setFormTarget(null)}
                onSubmit={
                  formTarget === 'create'
                    ? create
                    : (input) => save({ ...input, id: formTarget.id })
                }
              />
            ) : null}
          </div>
        </SheetContent>
      </Sheet>

      <BenefitActivityReport
        activity={reportActivity}
        open={reportActivity !== null}
        onOpenChange={(open) => {
          if (!open) setReportActivity(null)
        }}
      />

      <BenefitVouchersSheet
        activity={vouchersActivity}
        open={vouchersActivity !== null}
        onOpenChange={(open) => {
          if (!open) setVouchersActivity(null)
        }}
      />

      {terminateID ? (
        <BenefitTerminateDialog
          onCancel={() => setTerminateID(null)}
          onConfirm={async (mode, reason) => {
            const response = await terminateAdminBenefitActivity(
              terminateID,
              mode,
              reason
            )
            if (!response.success) {
              toast.error(response.message ?? t('Failed to terminate activity'))
              return
            }
            setTerminateID(null)
            await refresh()
          }}
        />
      ) : null}

      <ConfirmDialog
        destructive
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        handleConfirm={() => void deleteSelected()}
        isLoading={deleting}
        className='max-w-md'
        title={t('Delete {{count}} activities?', { count: selectedIds.size })}
        desc={t(
          'Deleted activities disappear from this list, but their vouchers and ledger stay available for audit. This action cannot be undone.'
        )}
        confirmText={t('Delete')}
      />
    </div>
  )
}
