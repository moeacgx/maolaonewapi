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
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Skeleton } from '@/components/ui/skeleton'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'

import {
  resolvePreviewImage,
  type ResolvedPreviewImage,
} from '../../lib/image-preview'

interface ImageDialogProps {
  imageUrl?: string
  imageUrls?: string[]
  taskId?: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

function PreviewImage(props: { src: string; alt: string; errorText: string }) {
  const [loading, setLoading] = useState(true)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    setLoading(true)
    setFailed(false)
  }, [props.src])

  return (
    <div className='bg-muted/50 relative flex min-h-[300px] items-center justify-center rounded-lg border'>
      {loading || failed ? (
        <Skeleton className='absolute inset-0 size-full rounded-lg' />
      ) : null}
      <img
        src={props.src}
        alt={props.alt}
        className={cn(
          'max-h-[550px] w-full rounded-lg object-contain transition-opacity',
          loading || failed ? 'opacity-0' : 'opacity-100'
        )}
        onLoad={() => {
          setLoading(false)
          setFailed(false)
        }}
        onError={() => {
          setLoading(false)
          setFailed(true)
        }}
        loading='lazy'
      />
      {failed ? (
        <div className='absolute inset-0 flex items-center justify-center'>
          <p className='text-muted-foreground text-sm'>{props.errorText}</p>
        </div>
      ) : null}
    </div>
  )
}

export function ImageDialog(props: ImageDialogProps) {
  const { t } = useTranslation()
  const sources = useMemo(
    () =>
      (props.imageUrls ?? (props.imageUrl ? [props.imageUrl] : [])).filter(
        (source): source is string =>
          typeof source === 'string' && source.trim() !== ''
      ),
    [props.imageUrl, props.imageUrls]
  )
  const [images, setImages] = useState<ResolvedPreviewImage[]>([])
  const [resolving, setResolving] = useState(false)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    if (!props.open || sources.length === 0) {
      setImages([])
      setResolving(false)
      setFailed(false)
      return
    }

    const controller = new AbortController()
    let active = true
    const resolved: ResolvedPreviewImage[] = []
    setResolving(true)
    setFailed(false)

    void Promise.allSettled(
      sources.map((source) =>
        resolvePreviewImage(source, controller.signal, async (url, signal) => {
          const response = await api.get<Blob>(url, {
            responseType: 'blob',
            disableDuplicate: true,
            skipErrorHandler: true,
            signal,
          })
          return response.data
        })
      )
    ).then((results) => {
      for (const result of results) {
        if (result.status === 'fulfilled') resolved.push(result.value)
      }
      if (!active) {
        resolved.forEach((image) => image.release())
        return
      }
      setImages(resolved)
      setFailed(resolved.length === 0)
      setResolving(false)
    })

    return () => {
      active = false
      controller.abort()
      resolved.forEach((image) => image.release())
    }
  }, [props.open, sources])

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('Image Preview')}
      description={
        props.taskId
          ? `${t('Task ID:')} ${props.taskId}`
          : t('View the generated image')
      }
      contentClassName='sm:max-w-3xl'
      contentHeight='min(72dvh, 640px)'
      bodyClassName='space-y-4'
    >
      {resolving ? (
        <Skeleton className='min-h-[300px] w-full rounded-lg' />
      ) : null}
      {images.map((image) => (
        <PreviewImage
          key={image.src}
          src={image.src}
          alt={t('Generated image')}
          errorText={t('Failed to load image')}
        />
      ))}
      {failed ? (
        <div className='text-muted-foreground flex min-h-[300px] items-center justify-center rounded-lg border text-sm'>
          {t('Failed to load image')}
        </div>
      ) : null}
      {sources.length > 0 ? (
        <div className='bg-muted rounded-md p-3'>
          {sources.map((source) => (
            <p
              key={source}
              className='text-muted-foreground font-mono text-xs break-all'
            >
              {source}
            </p>
          ))}
        </div>
      ) : null}
    </Dialog>
  )
}
