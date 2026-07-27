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
import { useState } from 'react'
import {
  Add01Icon,
  ArrowDown01Icon,
  ArrowUp01Icon,
  Delete02Icon,
  Edit02Icon,
  Key01Icon,
  TestTube01Icon,
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
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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
  FieldTitle,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { probeSecurityAuditEndpoint } from './api'
import type { SensitiveActionRunner } from './shared'
import type {
  SecurityAuditEndpointDraft,
  SecurityAuditTokenAction,
} from './types'

interface EditingEndpoint {
  index: number | null
  value: SecurityAuditEndpointDraft
}

function createEndpoint(): SecurityAuditEndpointDraft {
  const suffix =
    typeof crypto !== 'undefined' && 'randomUUID' in crypto
      ? crypto.randomUUID().slice(0, 8)
      : Date.now().toString(36)
  return {
    id: `guard-${suffix}`,
    name: '',
    protocol: 'openai_compatible',
    base_url: '',
    model: 'sileader/qwen3guard:0.6b',
    timeout_ms: 3000,
    input_limit: 4000,
    enabled: true,
    has_token: false,
    token_status: 'missing',
    token_action: 'replace',
    token: '',
  }
}

function EndpointEditorDialog({
  editing,
  onOpenChange,
  onEditingChange,
  onCommit,
}: {
  editing: EditingEndpoint | null
  onOpenChange: (open: boolean) => void
  onEditingChange: (editing: EditingEndpoint) => void
  onCommit: (editing: EditingEndpoint) => void
}) {
  const { t } = useTranslation()
  if (!editing) return null
  const endpoint = editing.value
  const update = (patch: Partial<SecurityAuditEndpointDraft>) =>
    onEditingChange({ ...editing, value: { ...endpoint, ...patch } })

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className='max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>
            {editing.index === null
              ? t('Add Guard node')
              : t('Edit Guard node')}
          </DialogTitle>
          <DialogDescription>
            {t(
              'Guard nodes use the OpenAI-compatible chat completions protocol and are tried in priority order.'
            )}
          </DialogDescription>
        </DialogHeader>
        <FieldGroup>
          <div className='grid gap-4 sm:grid-cols-2'>
            <Field>
              <FieldLabel htmlFor='guard-node-id'>{t('Node ID')}</FieldLabel>
              <Input
                id='guard-node-id'
                value={endpoint.id}
                disabled={editing.index !== null}
                onChange={(event) => update({ id: event.target.value })}
              />
              <FieldDescription>
                {t('Stable identifier used in events and health metrics')}
              </FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor='guard-node-name'>{t('Name')}</FieldLabel>
              <Input
                id='guard-node-name'
                value={endpoint.name}
                onChange={(event) => update({ name: event.target.value })}
              />
            </Field>
          </div>
          <Field>
            <FieldLabel htmlFor='guard-node-url'>{t('Base URL')}</FieldLabel>
            <Input
              id='guard-node-url'
              type='url'
              value={endpoint.base_url}
              placeholder='https://guard.example.com/v1'
              onChange={(event) => update({ base_url: event.target.value })}
            />
            <FieldDescription>
              {t(
                'URLs with credentials, query parameters, fragments, link-local, or cloud metadata addresses are rejected.'
              )}
            </FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor='guard-node-model'>{t('Model')}</FieldLabel>
            <Input
              id='guard-node-model'
              value={endpoint.model}
              onChange={(event) => update({ model: event.target.value })}
            />
          </Field>
          <Field>
            <FieldLabel id='guard-token-action'>{t('Node token')}</FieldLabel>
            <ToggleGroup
              aria-labelledby='guard-token-action'
              value={[endpoint.token_action]}
              onValueChange={(values) => {
                const value = values[0] as SecurityAuditTokenAction | undefined
                if (value) update({ token_action: value, token: '' })
              }}
              spacing={2}
              className='flex-wrap justify-start'
            >
              <ToggleGroupItem
                value='keep'
                disabled={!endpoint.has_token && editing.index === null}
              >
                {t('Keep')}
              </ToggleGroupItem>
              <ToggleGroupItem value='replace'>{t('Replace')}</ToggleGroupItem>
              <ToggleGroupItem value='clear'>{t('Clear')}</ToggleGroupItem>
            </ToggleGroup>
            <FieldDescription>
              {endpoint.has_token
                ? t(
                    'A token is configured. Its value is never returned by the API.'
                  )
                : t('No token is currently configured for this node.')}
            </FieldDescription>
          </Field>
          {endpoint.token_action === 'replace' && (
            <Field>
              <FieldLabel htmlFor='guard-node-token'>
                {t('New token')}
              </FieldLabel>
              <Input
                id='guard-node-token'
                type='password'
                autoComplete='new-password'
                value={endpoint.token}
                onChange={(event) => update({ token: event.target.value })}
              />
            </Field>
          )}
          <Field orientation='horizontal'>
            <FieldContent>
              <FieldTitle>{t('Enabled')}</FieldTitle>
              <FieldDescription>
                {t(
                  'Disabled nodes remain configured but receive no audit traffic.'
                )}
              </FieldDescription>
            </FieldContent>
            <Switch
              checked={endpoint.enabled}
              onCheckedChange={(enabled) => update({ enabled })}
              aria-label={t('Enabled')}
            />
          </Field>
        </FieldGroup>
        <DialogFooter>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button
            onClick={() => onCommit(editing)}
            disabled={
              !endpoint.id.trim() ||
              !endpoint.name.trim() ||
              !endpoint.base_url.trim() ||
              !endpoint.model.trim() ||
              (endpoint.token_action === 'replace' && !endpoint.token.trim())
            }
          >
            {t('Apply')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function SecurityAuditEndpointsView({
  endpoints,
  onChange,
  runSensitive,
}: {
  endpoints: SecurityAuditEndpointDraft[]
  onChange: (endpoints: SecurityAuditEndpointDraft[]) => void
  runSensitive: SensitiveActionRunner
}) {
  const { t } = useTranslation()
  const [editing, setEditing] = useState<EditingEndpoint | null>(null)
  const [deleteIndex, setDeleteIndex] = useState<number | null>(null)
  const [probingId, setProbingId] = useState<string | null>(null)

  const move = (index: number, offset: -1 | 1) => {
    const target = index + offset
    if (target < 0 || target >= endpoints.length) return
    const next = [...endpoints]
    const [endpoint] = next.splice(index, 1)
    if (!endpoint) return
    next.splice(target, 0, endpoint)
    onChange(next)
  }

  const probe = async (endpoint: SecurityAuditEndpointDraft) => {
    await runSensitive(
      async () => {
        setProbingId(endpoint.id)
        try {
          const result = await probeSecurityAuditEndpoint(endpoint)
          if (result.healthy) {
            toast.success(
              t('Guard node responded in {{latency}} ms', {
                latency: result.latency_ms,
              })
            )
          } else {
            toast.error(result.message || t('Guard node is unavailable'))
          }
          return result
        } finally {
          setProbingId(null)
        }
      },
      {
        title: t('Verify Guard node probe'),
        description: t(
          'Confirm your identity before sending a connectivity probe to this Guard node.'
        ),
      }
    )
  }

  return (
    <div className='flex flex-col gap-4'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div>
          <h3 className='font-medium'>{t('Guard nodes')}</h3>
          <p className='text-muted-foreground text-sm'>
            {t('Nodes are tried from top to bottom with fault failover.')}
          </p>
        </div>
        <Button
          size='sm'
          onClick={() => setEditing({ index: null, value: createEndpoint() })}
        >
          <HugeiconsIcon
            icon={Add01Icon}
            strokeWidth={2}
            data-icon='inline-start'
          />
          {t('Add Guard node')}
        </Button>
      </div>

      {endpoints.length === 0 ? (
        <Empty className='rounded-xl border'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              <HugeiconsIcon icon={Key01Icon} strokeWidth={2} />
            </EmptyMedia>
            <EmptyTitle>{t('No Guard nodes')}</EmptyTitle>
            <EmptyDescription>
              {t(
                'Add at least one enabled node before turning on security audit.'
              )}
            </EmptyDescription>
          </EmptyHeader>
          <EmptyContent>
            <Button
              onClick={() =>
                setEditing({ index: null, value: createEndpoint() })
              }
            >
              <HugeiconsIcon
                icon={Add01Icon}
                strokeWidth={2}
                data-icon='inline-start'
              />
              {t('Add Guard node')}
            </Button>
          </EmptyContent>
        </Empty>
      ) : (
        <div className='grid gap-4 xl:grid-cols-2'>
          {endpoints.map((endpoint, index) => (
            <Card key={endpoint.id}>
              <CardHeader>
                <CardTitle className='flex flex-wrap items-center gap-2'>
                  <span className='truncate'>
                    {endpoint.name || endpoint.id}
                  </span>
                  <Badge variant={endpoint.enabled ? 'default' : 'secondary'}>
                    {endpoint.enabled ? t('Enabled') : t('Disabled')}
                  </Badge>
                  <Badge variant='outline'>#{index + 1}</Badge>
                </CardTitle>
                <CardDescription className='truncate'>
                  {endpoint.base_url || t('Base URL not configured')}
                </CardDescription>
                <CardAction className='flex gap-1'>
                  <Button
                    variant='ghost'
                    size='icon-sm'
                    onClick={() => move(index, -1)}
                    disabled={index === 0}
                    aria-label={t('Move up')}
                  >
                    <HugeiconsIcon icon={ArrowUp01Icon} strokeWidth={2} />
                  </Button>
                  <Button
                    variant='ghost'
                    size='icon-sm'
                    onClick={() => move(index, 1)}
                    disabled={index === endpoints.length - 1}
                    aria-label={t('Move down')}
                  >
                    <HugeiconsIcon icon={ArrowDown01Icon} strokeWidth={2} />
                  </Button>
                </CardAction>
              </CardHeader>
              <CardContent className='grid gap-3 text-sm sm:grid-cols-2'>
                <div className='flex flex-col gap-1'>
                  <span className='text-muted-foreground text-xs'>
                    {t('Model')}
                  </span>
                  <span className='truncate'>{endpoint.model}</span>
                </div>
                <div className='flex flex-col gap-1'>
                  <span className='text-muted-foreground text-xs'>
                    {t('Token')}
                  </span>
                  <span>
                    {endpoint.has_token && endpoint.token_action !== 'clear'
                      ? t('Configured')
                      : endpoint.token_action === 'replace' && endpoint.token
                        ? t('Pending replacement')
                        : t('Not configured')}
                  </span>
                </div>
                <div className='flex flex-col gap-1'>
                  <span className='text-muted-foreground text-xs'>
                    {t('Timeout')}
                  </span>
                  <span>
                    {t('{{value}} ms', { value: endpoint.timeout_ms })}
                  </span>
                </div>
                <div className='flex flex-col gap-1'>
                  <span className='text-muted-foreground text-xs'>
                    {t('Chunk size')}
                  </span>
                  <span>{endpoint.input_limit}</span>
                </div>
              </CardContent>
              <CardFooter className='flex flex-wrap gap-2'>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => void probe(endpoint)}
                  disabled={probingId === endpoint.id}
                >
                  {probingId === endpoint.id ? (
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
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() =>
                    setEditing({ index, value: { ...endpoint, token: '' } })
                  }
                >
                  <HugeiconsIcon
                    icon={Edit02Icon}
                    strokeWidth={2}
                    data-icon='inline-start'
                  />
                  {t('Edit')}
                </Button>
                <Button
                  variant='destructive'
                  size='sm'
                  onClick={() => setDeleteIndex(index)}
                >
                  <HugeiconsIcon
                    icon={Delete02Icon}
                    strokeWidth={2}
                    data-icon='inline-start'
                  />
                  {t('Remove')}
                </Button>
              </CardFooter>
            </Card>
          ))}
        </div>
      )}

      <EndpointEditorDialog
        editing={editing}
        onOpenChange={(open) => {
          if (!open) setEditing(null)
        }}
        onEditingChange={setEditing}
        onCommit={(next) => {
          const endpointsNext = [...endpoints]
          if (next.index === null) {
            endpointsNext.push(next.value)
          } else {
            endpointsNext[next.index] = next.value
          }
          onChange(endpointsNext)
          setEditing(null)
        }}
      />

      <AlertDialog
        open={deleteIndex !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteIndex(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} />
            </AlertDialogMedia>
            <AlertDialogTitle>{t('Remove Guard node?')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'The node is removed from the draft. The change takes effect only after you save the configuration.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction
              variant='destructive'
              onClick={() => {
                if (deleteIndex !== null) {
                  onChange(
                    endpoints.filter((_, index) => index !== deleteIndex)
                  )
                }
                setDeleteIndex(null)
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
