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
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { formatLogQuota } from '@/lib/format'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import { StatusBadge } from '@/components/status-badge'
import { getChannel } from '@/features/channels/api'
import {
  CHANNEL_STATUS_CONFIG,
  CHANNEL_TYPES,
} from '@/features/channels/constants'

function ChannelInfoRow(props: {
  label: React.ReactNode
  value: React.ReactNode
  mono?: boolean
}) {
  return (
    <div className='grid min-w-0 grid-cols-[6rem_minmax(0,1fr)] gap-3 text-sm'>
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

function ChannelModels({ models }: { models?: string }) {
  const values = (models || '')
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
    .slice(0, 12)

  if (values.length === 0) {
    return <span className='text-muted-foreground'>-</span>
  }

  return (
    <div className='flex flex-wrap gap-1'>
      {values.map((model) => (
        <StatusBadge
          key={model}
          label={model}
          autoColor={model}
          size='sm'
          className='max-w-56'
        />
      ))}
    </div>
  )
}

export function ChannelInfoDialog({
  channelId,
  open,
  onOpenChange,
}: {
  channelId: number | null
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['usage-logs', 'channel-info', channelId],
    queryFn: () => getChannel(channelId ?? 0),
    enabled: open && !!channelId,
  })

  const channel = query.data?.data
  const status = channel
    ? CHANNEL_STATUS_CONFIG[
        channel.status as keyof typeof CHANNEL_STATUS_CONFIG
      ] || CHANNEL_STATUS_CONFIG[0]
    : null
  const channelType = channel
    ? CHANNEL_TYPES[channel.type as keyof typeof CHANNEL_TYPES] || 'Unknown'
    : 'Unknown'

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>
            {t('Channel')} {channelId ? `#${channelId}` : ''}
          </DialogTitle>
        </DialogHeader>
        {query.isLoading ? (
          <div className='text-muted-foreground py-8 text-center text-sm'>
            {t('Loading')}
          </div>
        ) : !channel ? (
          <div className='text-muted-foreground py-8 text-center text-sm'>
            {t('No data')}
          </div>
        ) : (
          <ScrollArea className='max-h-[70vh] pr-3'>
            <div className='space-y-4'>
              <section className='bg-muted/20 space-y-2 rounded-md border p-3'>
                <Label className='text-xs font-semibold'>{t('Channel')}</Label>
                <ChannelInfoRow
                  label={t('Channel ID')}
                  value={`#${channel.id}`}
                  mono
                />
                <ChannelInfoRow
                  label={t('Channel name')}
                  value={channel.name || '-'}
                />
                <ChannelInfoRow
                  label={t('Channel type')}
                  value={t(channelType)}
                />
                <ChannelInfoRow
                  label={t('Status')}
                  value={
                    status ? (
                      <StatusBadge
                        label={t(status.label)}
                        variant={status.variant}
                        copyable={false}
                      />
                    ) : (
                      '-'
                    )
                  }
                />
              </section>
              <section className='bg-muted/20 space-y-2 rounded-md border p-3'>
                <Label className='text-xs font-semibold'>{t('Usage')}</Label>
                <ChannelInfoRow
                  label={t('Used Quota')}
                  value={formatLogQuota(channel.used_quota)}
                />
                <ChannelInfoRow
                  label={t('Balance')}
                  value={`$${Number(channel.balance || 0).toFixed(4)}`}
                  mono
                />
                <ChannelInfoRow
                  label={t('Priority')}
                  value={String(channel.priority ?? 0)}
                />
              </section>
              <section className='bg-muted/20 space-y-2 rounded-md border p-3'>
                <Label className='text-xs font-semibold'>
                  {t('Models & Groups')}
                </Label>
                <ChannelInfoRow
                  label={t('Group')}
                  value={channel.group || '-'}
                  mono
                />
                <ChannelInfoRow
                  label={t('Models')}
                  value={<ChannelModels models={channel.models} />}
                />
                <ChannelInfoRow
                  label={t('Remark')}
                  value={channel.remark || '-'}
                />
              </section>
            </div>
          </ScrollArea>
        )}
      </DialogContent>
    </Dialog>
  )
}
