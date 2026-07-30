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
import { useEffect, useRef, useState } from 'react'
import * as z from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import {
  isValidCCSwitchAddress,
  normalizeCCSwitchAddress,
} from '@/features/keys/lib/cc-switch'
import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import { ChatSettingsVisualEditor } from './chat-settings-visual-editor'
import { formatJsonForEditor, normalizeJsonString } from './utils'

const createChatSchema = (t: (key: string) => string) =>
  z.object({
    CCSwitchAPIAddress: z.string().refine(isValidCCSwitchAddress, {
      message: t(
        'Enter a complete HTTP or HTTPS address without credentials, query parameters, or fragments.'
      ),
    }),
    Chats: z.string().superRefine((value, ctx) => {
      try {
        const parsed = JSON.parse(value || '[]')
        if (!Array.isArray(parsed)) {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            message: t('Expected a JSON array.'),
          })
          return
        }
        for (const item of parsed) {
          if (
            item === null ||
            typeof item !== 'object' ||
            Array.isArray(item)
          ) {
            ctx.addIssue({
              code: z.ZodIssueCode.custom,
              message: t(
                'Each item must be an object with a single key-value pair.'
              ),
            })
            return
          }
          const entries = Object.entries(item)
          if (entries.length !== 1) {
            ctx.addIssue({
              code: z.ZodIssueCode.custom,
              message: t('Each item must have exactly one key-value pair.'),
            })
            return
          }
        }
      } catch {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: t('Invalid JSON string.'),
        })
      }
    }),
  })

type ChatSettingsFormValues = z.infer<ReturnType<typeof createChatSchema>>

type ChatSettingsSectionProps = {
  defaultValue: string
  ccSwitchApiAddress: string
}

export function ChatSettingsSection({
  defaultValue,
  ccSwitchApiAddress,
}: ChatSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [editMode, setEditMode] = useState<'visual' | 'json'>('visual')

  const chatSchema = createChatSchema(t)
  const formatted = formatJsonForEditor(defaultValue, '[]')
  const form = useForm<ChatSettingsFormValues>({
    resolver: zodResolver(chatSchema),
    mode: 'onChange', // Enable real-time validation
    defaultValues: {
      CCSwitchAPIAddress: ccSwitchApiAddress,
      Chats: formatted,
    },
  })

  const initialValuesRef = useRef({
    CCSwitchAPIAddress: normalizeCCSwitchAddress(ccSwitchApiAddress),
    Chats: normalizeJsonString(defaultValue, '[]'),
  })

  useEffect(() => {
    form.reset({
      CCSwitchAPIAddress: ccSwitchApiAddress,
      Chats: formatJsonForEditor(defaultValue, '[]'),
    })
    initialValuesRef.current = {
      CCSwitchAPIAddress: normalizeCCSwitchAddress(ccSwitchApiAddress),
      Chats: normalizeJsonString(defaultValue, '[]'),
    }
  }, [ccSwitchApiAddress, defaultValue, form])

  const onSubmit = async (values: ChatSettingsFormValues) => {
    const normalizedValues = {
      CCSwitchAPIAddress: normalizeCCSwitchAddress(values.CCSwitchAPIAddress),
      Chats: normalizeJsonString(values.Chats, '[]'),
    }
    const updates = Object.entries(normalizedValues).filter(
      ([key, value]) =>
        value !==
        initialValuesRef.current[key as keyof typeof initialValuesRef.current]
    )

    for (const [key, value] of updates) {
      const result = await updateOption.mutateAsync({ key, value })
      if (!result.success) return
      initialValuesRef.current = {
        ...initialValuesRef.current,
        [key]: value,
      }
    }

    form.reset({
      CCSwitchAPIAddress: normalizedValues.CCSwitchAPIAddress,
      Chats: formatJsonForEditor(normalizedValues.Chats, '[]'),
    })
  }

  return (
    <SettingsSection title={t('Chat Presets')}>
      <Form {...form}>
        {/* eslint-disable-next-line react-hooks/refs */}
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save chat settings'
          />
          <FormField
            control={form.control}
            name='CCSwitchAPIAddress'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('CC Switch API address')}</FormLabel>
                <FormControl>
                  <Input placeholder='https://api.example.com' {...field} />
                </FormControl>
                <FormDescription>
                  {t(
                    'Used for CC Switch one-click import. Enter the API root address without /v1; leave blank to use the website server address.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <Tabs
            value={editMode}
            onValueChange={(value) => setEditMode(value as 'visual' | 'json')}
          >
            <TabsList className='grid w-full grid-cols-2'>
              <TabsTrigger value='visual'>{t('Visual')}</TabsTrigger>
              <TabsTrigger value='json'>{t('JSON')}</TabsTrigger>
            </TabsList>

            <TabsContent value='visual' className='mt-6'>
              <FormField
                control={form.control}
                name='Chats'
                render={({ field }) => (
                  <FormItem>
                    <FormControl>
                      <ChatSettingsVisualEditor
                        value={field.value}
                        onChange={field.onChange}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </TabsContent>

            <TabsContent value='json' className='mt-6'>
              <FormField
                control={form.control}
                name='Chats'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Chat configuration JSON')}</FormLabel>
                    <FormControl>
                      <Textarea
                        rows={12}
                        placeholder={t(
                          '[{"ChatGPT":"https://chat.openai.com"},{"Lobe Chat":"https://chat-preview.lobehub.com/?settings={...}"}]'
                        )}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Array of chat client presets. Each item is an object with one key-value pair: client name and its URL.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </TabsContent>
          </Tabs>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
