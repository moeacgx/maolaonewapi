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
import { ArrowUpRight, Brush } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { GroupSelector } from '@/components/model-group-selector'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { getUserGroupsWithDefault } from '@/features/playground/api'
import { useStatus } from '@/hooks/use-status'
import { getCanvasSettingsFromSidebarModules } from '@/lib/canvas-settings'
import { getCustomNavIcon } from '@/lib/custom-nav'

import { buildCanvasLaunchUrl, resolveCanvasDefaultGroup } from './lib'

export function CanvasLauncher() {
  const { t } = useTranslation()
  const { status } = useStatus()
  const [selectedGroup, setSelectedGroup] = useState('')
  const canvasSettings = useMemo(
    () =>
      getCanvasSettingsFromSidebarModules(
        (status as Record<string, unknown> | null)?.SidebarModulesAdmin
      ),
    [status]
  )
  const CanvasIcon = getCustomNavIcon(canvasSettings.canvasIcon) ?? Brush

  const { data: groupsData, isLoading } = useQuery({
    queryKey: ['canvas-groups'],
    queryFn: async () => {
      try {
        return await getUserGroupsWithDefault()
      } catch (error) {
        toast.error(
          error instanceof Error
            ? error.message
            : t('Failed to load playground groups')
        )
        return { groups: [], defaultGroup: '' }
      }
    },
  })
  const groups = groupsData?.groups ?? []

  useEffect(() => {
    if (selectedGroup || groups.length === 0) return
    setSelectedGroup(
      resolveCanvasDefaultGroup(groups, groupsData?.defaultGroup ?? '')
    )
  }, [groups, groupsData?.defaultGroup, selectedGroup])

  const launchUrl = useMemo(() => {
    if (!selectedGroup || typeof window === 'undefined') return ''
    return buildCanvasLaunchUrl({
      canvasOrigin: canvasSettings.canvasOrigin,
      newApiOrigin: window.location.origin,
      group: selectedGroup,
    })
  }, [canvasSettings.canvasOrigin, selectedGroup])

  const handleOpenCanvas = () => {
    if (!launchUrl) return
    window.open(launchUrl, '_blank', 'noopener')
  }

  return (
    <div className='flex min-h-full items-center justify-center p-4 sm:p-8'>
      <Card className='w-full max-w-xl'>
        <CardHeader>
          <div className='bg-muted mb-2 flex h-10 w-10 items-center justify-center rounded-lg'>
            <CanvasIcon className='h-5 w-5' aria-hidden='true' />
          </div>
          <CardTitle>{t('Infinite Canvas')}</CardTitle>
          <CardDescription>
            {t(
              'Choose a group and open Infinite Canvas. The canvas uses your current New API login session to call models.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-4'>
          <div className='space-y-2'>
            <p className='text-sm font-medium'>{t('Model Group')}</p>
            <GroupSelector
              selectedGroup={selectedGroup}
              groups={groups}
              onGroupChange={setSelectedGroup}
              disabled={isLoading || groups.length === 0}
              className='h-9 w-full justify-between px-3 sm:w-full'
            />
          </div>

          <Button
            className='w-full'
            onClick={handleOpenCanvas}
            disabled={!launchUrl || isLoading}
          >
            <ArrowUpRight className='h-4 w-4' aria-hidden='true' />
            {t('Open in new tab')}
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}
