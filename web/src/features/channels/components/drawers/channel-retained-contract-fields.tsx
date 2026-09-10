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
import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import { CLAUDE_FIELD_PASSTHROUGH_TYPES } from '../../constants'
import type { ChannelFormValues } from '../../lib'

type OverrideFieldName =
  | 'monitor_enabled'
  | 'monitor_auto_disable_enabled'
  | 'monitor_auto_enable_enabled'

function BooleanOverrideField(props: {
  name: OverrideFieldName
  label: string
  description: string
}) {
  const { t } = useTranslation()
  const form = useFormContext<ChannelFormValues>()

  return (
    <FormField
      control={form.control}
      name={props.name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{props.label}</FormLabel>
          <Select
            value={field.value || 'inherit'}
            onValueChange={field.onChange}
          >
            <FormControl>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
            </FormControl>
            <SelectContent>
              <SelectGroup>
                <SelectItem value='inherit'>{t('Inherit')}</SelectItem>
                <SelectItem value='enabled'>{t('Enabled')}</SelectItem>
                <SelectItem value='disabled'>{t('Disabled')}</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
          <FormDescription>{props.description}</FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

function NumericOverrideField(props: {
  name:
    | 'monitor_test_interval_minutes'
    | 'monitor_response_time_threshold_seconds'
    | 'monitor_disable_threshold'
    | 'monitor_enable_threshold'
  label: string
  description: string
  integer?: boolean
}) {
  const form = useFormContext<ChannelFormValues>()

  return (
    <FormField
      control={form.control}
      name={props.name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{props.label}</FormLabel>
          <FormControl>
            <Input
              type='number'
              min={1}
              step={props.integer ? 1 : 'any'}
              value={field.value || ''}
              onChange={field.onChange}
            />
          </FormControl>
          <FormDescription>{props.description}</FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

export function ChannelRetainedContractFields(props: { disabled?: boolean }) {
  const { t } = useTranslation()
  const form = useFormContext<ChannelFormValues>()
  const channelType = form.watch('type')
  const lastTokensProAllowed = form.watch('tokenspro_overview_last_allowed')
  const lastTokensProSuccessTime = form.watch(
    'tokenspro_overview_last_success_time'
  )
  const lastTokensProError = form.watch('tokenspro_overview_last_error')
  const showClaudeFingerprint = CLAUDE_FIELD_PASSTHROUGH_TYPES.has(channelType)

  return (
    <fieldset
      disabled={props.disabled}
      className='space-y-5 border-t pt-4 disabled:opacity-60'
    >
      <div>
        <h4 className='text-sm font-medium'>{t('Operational Overrides')}</h4>
        <p className='text-muted-foreground mt-1 text-xs'>
          {t('Configure concurrency and monitor behavior.')}
        </p>
      </div>

      <div className='grid gap-4 sm:grid-cols-2'>
        <FormField
          control={form.control}
          name='concurrency_limit'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Concurrency Limit')}</FormLabel>
              <FormControl>
                <Input
                  type='number'
                  min={0}
                  step={1}
                  value={field.value ?? 0}
                  onChange={(event) =>
                    field.onChange(Number(event.target.value))
                  }
                />
              </FormControl>
              <FormDescription>
                {t('Zero keeps the channel concurrency unlimited.')}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='tokenspro_overview_sync_enabled'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Align TokensPro concurrency')}</FormLabel>
              <FormControl>
                <Switch
                  checked={field.value === true}
                  onCheckedChange={field.onChange}
                />
              </FormControl>
              <FormDescription>
                {t(
                  'Poll TokensPro overview with this channel key and write concurrency.allowed. Failures leave concurrency and status unchanged. allowed=0 auto-disables; recovery only re-enables channels disabled by this sync.'
                )}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='tokenspro_overview_sync_interval_seconds'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('TokensPro sync interval (seconds)')}</FormLabel>
              <FormControl>
                <Input
                  type='number'
                  min={60}
                  step={1}
                  placeholder='60'
                  value={field.value || ''}
                  onChange={field.onChange}
                />
              </FormControl>
              <FormDescription>
                {t('Minimum 60 seconds. Unsuccessful channels retry every minute.')}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormItem>
          <FormLabel>{t('Last TokensPro allowed')}</FormLabel>
          <Input
            disabled
            value={
              lastTokensProAllowed == null ? '' : String(lastTokensProAllowed)
            }
          />
        </FormItem>
        <FormItem>
          <FormLabel>{t('Last TokensPro sync time')}</FormLabel>
          <Input
            disabled
            value={
              lastTokensProSuccessTime
                ? new Date(lastTokensProSuccessTime * 1000).toLocaleString()
                : t('Not synced yet')
            }
          />
        </FormItem>
        <FormItem>
          <FormLabel>{t('Last TokensPro sync error')}</FormLabel>
          <Input disabled value={lastTokensProError || ''} />
        </FormItem>
      </div>

      <div className='grid gap-4 sm:grid-cols-2'>
        <BooleanOverrideField
          name='monitor_enabled'
          label={t('Monitoring')}
          description={t(
            'Controls whether scheduled monitoring tests include this channel'
          )}
        />
        <NumericOverrideField
          name='monitor_test_interval_minutes'
          label={t('Monitor interval (minutes)')}
          description={t('Override the scheduled monitor interval.')}
        />
        <NumericOverrideField
          name='monitor_response_time_threshold_seconds'
          label={t('Response threshold (seconds)')}
          description={t('Override the monitor response-time threshold.')}
        />
        <BooleanOverrideField
          name='monitor_auto_disable_enabled'
          label={t('Disable on monitor failure')}
          description={t(
            'Controls automatic disabling for this channel during monitoring'
          )}
        />
        <NumericOverrideField
          name='monitor_disable_threshold'
          label={t('Failure threshold')}
          description={t(
            'Disable after this many failed monitoring tests in a row'
          )}
          integer
        />
        <BooleanOverrideField
          name='monitor_auto_enable_enabled'
          label={t('Enable on monitor success')}
          description={t(
            'Controls automatic re-enabling for this channel during monitoring'
          )}
        />
        <NumericOverrideField
          name='monitor_enable_threshold'
          label={t('Recovery threshold')}
          description={t(
            'Enable after this many successful monitoring tests in a row'
          )}
          integer
        />
      </div>

      {showClaudeFingerprint && (
        <div className='space-y-4 border-t pt-4'>
          <div>
            <h4 className='text-sm font-medium'>
              {t('Claude Code Fingerprint')}
            </h4>
            <p className='text-muted-foreground mt-1 text-xs'>
              {t('Override Claude Code client and transport fingerprints.')}
            </p>
          </div>
          <div className='divide-border divide-y rounded-lg border'>
            <FormField
              control={form.control}
              name='claude_code_fingerprint_enabled'
              render={({ field }) => (
                <FormItem className='flex items-center justify-between gap-3 px-4 py-3'>
                  <FormLabel>{t('Claude Code fingerprint')}</FormLabel>
                  <FormControl>
                    <Switch
                      checked={field.value === true}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='claude_code_transport_fingerprint_enabled'
              render={({ field }) => (
                <FormItem className='flex items-center justify-between gap-3 px-4 py-3'>
                  <FormLabel>{t('Transport fingerprint')}</FormLabel>
                  <FormControl>
                    <Switch
                      checked={field.value === true}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
          </div>
          <div className='grid gap-4 sm:grid-cols-2'>
            <FormField
              control={form.control}
              name='claude_code_version'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Claude Code version')}</FormLabel>
                  <FormControl>
                    <Input {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='claude_code_entrypoint'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Claude Code entrypoint')}</FormLabel>
                  <FormControl>
                    <Input {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
        </div>
      )}
    </fieldset>
  )
}
