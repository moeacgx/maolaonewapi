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
import { useEffect, useState, useCallback, useMemo, useRef } from 'react'
import * as z from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
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
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
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
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  SideDrawerSection,
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
  sideDrawerSwitchItemClassName,
} from '@/components/drawer-layout'
import { JsonEditor } from '@/components/json-editor'
import { TagInput } from '@/components/tag-input'
import { getSystemOptions } from '@/features/system-settings/api'
import {
  useSystemOptions,
  getOptionValue,
} from '@/features/system-settings/hooks/use-system-options'
import { useUpdateOption } from '@/features/system-settings/hooks/use-update-option'
import { normalizeJsonString } from '@/features/system-settings/models/utils'
import type { ModelSettings } from '@/features/system-settings/types'
import { safeJsonParse } from '@/features/system-settings/utils/json-parser'
import { createModel, updateModel, getModel, getVendors } from '../../api'
import { getNameRuleOptions, ENDPOINT_TEMPLATES } from '../../constants'
import { modelsQueryKeys, vendorsQueryKeys, parseModelTags } from '../../lib'
import type { Model } from '../../types'

// Extended schema for ratio configuration (internal form state only)
const extendedModelFormSchema = z.object({
  id: z.number().optional(),
  model_name: z.string().min(1, 'Model name is required'),
  description: z.string(),
  icon: z.string(),
  tags: z.array(z.string()),
  vendor_id: z.number().optional(),
  endpoints: z.string(),
  name_rule: z.number(),
  status: z.boolean(),
  sync_official: z.boolean(),
  price: z.string().optional(),
  ratio: z.string().optional(),
  cacheRatio: z.string().optional(),
  completionRatio: z.string().optional(),
  imageRatio: z.string().optional(),
  audioRatio: z.string().optional(),
  audioCompletionRatio: z.string().optional(),
})

type ExtendedModelFormValues = z.infer<typeof extendedModelFormSchema>

type PricingMode = 'per-token' | 'per-request'
type PricingSubMode = 'ratio' | 'price'

type PricingFields = Pick<
  ExtendedModelFormValues,
  | 'price'
  | 'ratio'
  | 'cacheRatio'
  | 'completionRatio'
  | 'imageRatio'
  | 'audioRatio'
  | 'audioCompletionRatio'
>

type PricingConfig = {
  mode: PricingMode
  fields: PricingFields
  promptPrice: string
  completionPrice: string
  advancedOpen: boolean
}

const PRICING_FIELD_KEYS: Array<keyof PricingFields> = [
  'price',
  'ratio',
  'cacheRatio',
  'completionRatio',
  'imageRatio',
  'audioRatio',
  'audioCompletionRatio',
]

function createEmptyPricingConfig(): PricingConfig {
  return {
    mode: 'per-token',
    fields: {
      price: '',
      ratio: '',
      cacheRatio: '',
      completionRatio: '',
      imageRatio: '',
      audioRatio: '',
      audioCompletionRatio: '',
    },
    promptPrice: '',
    completionPrice: '',
    advancedOpen: false,
  }
}

function lookupPricingValue(
  rawMap: string,
  modelName: string
): number | undefined {
  return safeJsonParse<Record<string, number>>(rawMap, {
    fallback: {},
    silent: true,
  })[modelName]
}

function hasPricingMapEntry(rawMap: string, modelName: string): boolean {
  const values = safeJsonParse<Record<string, unknown>>(rawMap, {
    fallback: {},
    silent: true,
  })
  return Object.prototype.hasOwnProperty.call(values, modelName)
}

function hasPricingOutsideDrawer(
  settings: ModelSettings,
  modelName: string
): boolean {
  return [
    settings.ModelPriceUnit,
    settings.ModelPriceVariants,
    settings.ModelRoutePriceVariants,
    settings.CreateCacheRatio,
    settings['billing_setting.billing_mode'],
    settings['billing_setting.billing_expr'],
  ].some((rawMap) => hasPricingMapEntry(rawMap, modelName))
}

function pricingConfigChanged(
  loaded: PricingConfig | null,
  mode: PricingMode,
  fields: PricingFields
): boolean {
  if (!loaded) {
    return PRICING_FIELD_KEYS.some((key) => (fields[key] ?? '') !== '')
  }
  return (
    loaded.mode !== mode ||
    PRICING_FIELD_KEYS.some(
      (key) => (loaded.fields[key] ?? '') !== (fields[key] ?? '')
    )
  )
}

// 模型定价存储在按名称索引的系统选项中，而不是模型元数据行里。
// 创建和编辑都必须先读回现有定价，避免提交空表单时误删已有配置。
function readPricingConfig(
  settings: ModelSettings | null,
  modelName: string
): PricingConfig {
  if (!settings || !modelName) return createEmptyPricingConfig()

  const price = lookupPricingValue(settings.ModelPrice, modelName)
  const ratio = lookupPricingValue(settings.ModelRatio, modelName)
  const cacheRatio = lookupPricingValue(settings.CacheRatio, modelName)
  const completionRatio = lookupPricingValue(
    settings.CompletionRatio,
    modelName
  )
  const imageRatio = lookupPricingValue(settings.ImageRatio, modelName)
  const audioRatio = lookupPricingValue(settings.AudioRatio, modelName)
  const audioCompletionRatio = lookupPricingValue(
    settings.AudioCompletionRatio,
    modelName
  )

  if (price !== undefined && price !== null) {
    const emptyPricing = createEmptyPricingConfig()
    return {
      ...emptyPricing,
      mode: 'per-request',
      fields: { ...emptyPricing.fields, price: price.toString() },
    }
  }

  let promptPrice = ''
  let completionPrice = ''
  if (ratio !== undefined && ratio !== null) {
    const tokenPrice = ratio * 2
    promptPrice = tokenPrice.toString()
    if (completionRatio !== undefined && completionRatio !== null) {
      completionPrice = (tokenPrice * completionRatio).toString()
    }
  }

  return {
    mode: 'per-token',
    fields: {
      price: '',
      ratio: ratio?.toString() || '',
      cacheRatio: cacheRatio?.toString() || '',
      completionRatio: completionRatio?.toString() || '',
      imageRatio: imageRatio?.toString() || '',
      audioRatio: audioRatio?.toString() || '',
      audioCompletionRatio: audioCompletionRatio?.toString() || '',
    },
    promptPrice,
    completionPrice,
    advancedOpen: [
      cacheRatio,
      imageRatio,
      audioRatio,
      audioCompletionRatio,
    ].some((value) => value !== undefined && value !== null),
  }
}

type ModelMutateDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: Model | null
}

export function ModelMutateDrawer({
  open,
  onOpenChange,
  currentRow,
}: ModelMutateDrawerProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isEditing = Boolean(currentRow?.id)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [pricingMode, setPricingMode] = useState<PricingMode>('per-token')
  const [pricingSubMode, setPricingSubMode] = useState<PricingSubMode>('ratio')
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [promptPrice, setPromptPrice] = useState('')
  const [completionPrice, setCompletionPrice] = useState('')
  const [oldModelName, setOldModelName] = useState<string>('')
  // 仅允许重写抽屉打开时实际读取过的名称，或用户明确填写了价格的名称。
  const [loadedPricingName, setLoadedPricingName] = useState<string>('')
  const loadedPricingRef = useRef<PricingConfig | null>(null)
  const initializedFormKeyRef = useRef('')

  // Fetch vendors for dropdown
  const { data: vendorsData } = useQuery({
    queryKey: vendorsQueryKeys.list(),
    queryFn: () => getVendors({ page_size: 1000 }),
    enabled: open,
  })

  const vendors = vendorsData?.data?.items || []

  // Fetch model detail if editing
  const { data: modelData } = useQuery({
    queryKey: modelsQueryKeys.detail(currentRow?.id || 0),
    queryFn: () => getModel(currentRow!.id),
    enabled: open && isEditing,
  })

  // Fetch system options for ratio configuration
  const { data: systemOptionsData } = useSystemOptions()

  const updateOption = useUpdateOption()

  // Get model settings from system options
  const modelSettings = useMemo(() => {
    if (!systemOptionsData?.data) return null
    const defaultModelSettings: ModelSettings = {
      'global.pass_through_request_enabled': false,
      'global.thinking_model_blacklist': '[]',
      'global.chat_completions_to_responses_policy': '{}',
      'general_setting.ping_interval_enabled': false,
      'general_setting.ping_interval_seconds': 60,
      'gemini.safety_settings': '',
      'gemini.version_settings': '',
      'gemini.supported_imagine_models': '',
      'gemini.thinking_adapter_enabled': false,
      'gemini.thinking_adapter_budget_tokens_percentage': 0.6,
      'gemini.function_call_thought_signature_enabled': false,
      'gemini.remove_function_response_id_enabled': true,
      'claude.model_headers_settings': '',
      'claude.default_max_tokens': '',
      'claude.thinking_adapter_enabled': true,
      'claude.thinking_adapter_budget_tokens_percentage': 0.8,
      ModelPrice: '',
      ModelPriceUnit: '{}',
      ModelPriceVariants: '{}',
      ModelRoutePriceVariants: '{}',
      ModelRatio: '',
      CacheRatio: '',
      CompletionRatio: '',
      ImageRatio: '',
      AudioRatio: '',
      AudioCompletionRatio: '',
      ExposeRatioEnabled: false,
      'billing_setting.billing_mode': '{}',
      'billing_setting.billing_expr': '{}',
      'tool_price_setting.prices': '{}',
      TopupGroupRatio: '',
      GroupRatio: '',
      UserUsableGroups: '',
      GroupGroupRatio: '',
      AutoGroups: '',
      DefaultUseAutoGroup: false,
      CreateCacheRatio: '',
      'group_ratio_setting.group_special_usable_group': '{}',
      'grok.violation_deduction_enabled': false,
      'grok.violation_deduction_amount': 0,
      'channel_affinity_setting.enabled': false,
      'channel_affinity_setting.switch_on_success': true,
      'channel_affinity_setting.max_entries': 100000,
      'channel_affinity_setting.default_ttl_seconds': 3600,
      'channel_affinity_setting.rules': '[]',
      'model_deployment.ionet.api_key': '',
      'model_deployment.ionet.enabled': false,
    }
    return getOptionValue(systemOptionsData.data, defaultModelSettings)
  }, [systemOptionsData])

  const form = useForm<ExtendedModelFormValues>({
    resolver: zodResolver(extendedModelFormSchema),
    defaultValues: {
      model_name: '',
      description: '',
      icon: '',
      tags: [],
      vendor_id: undefined,
      endpoints: '',
      name_rule: 0,
      status: true,
      sync_official: true,
      price: '',
      ratio: '',
      cacheRatio: '',
      completionRatio: '',
      imageRatio: '',
      audioRatio: '',
      audioCompletionRatio: '',
    },
  })

  const validateNumber = (value: string) => {
    if (value === '') return true
    return !isNaN(parseFloat(value))
  }

  const handlePromptPriceChange = (value: string) => {
    setPromptPrice(value)
    if (value && !isNaN(parseFloat(value))) {
      const ratio = parseFloat(value) / 2
      form.setValue('ratio', ratio.toString())
    } else {
      form.setValue('ratio', '')
    }
  }

  const handleCompletionPriceChange = (value: string) => {
    setCompletionPrice(value)
    if (
      value &&
      !isNaN(parseFloat(value)) &&
      promptPrice &&
      !isNaN(parseFloat(promptPrice)) &&
      parseFloat(promptPrice) > 0
    ) {
      const completionRatio = parseFloat(value) / parseFloat(promptPrice)
      form.setValue('completionRatio', completionRatio.toString())
    } else {
      form.setValue('completionRatio', '')
    }
  }

  // Load model data for editing and ratio configuration
  useEffect(() => {
    if (!open) {
      initializedFormKeyRef.current = ''
      loadedPricingRef.current = null
      setLoadedPricingName('')
      return
    }

    // 定价必须与系统选项一起初始化；同一次打开只初始化一次，避免查询
    // 后台刷新时覆盖管理员尚未提交的表单输入。
    if (!modelSettings) return

    const formKey = isEditing
      ? `edit:${currentRow?.id || 0}`
      : `create:${currentRow?.model_name || ''}`
    if (initializedFormKeyRef.current === formKey) return

    if (open && isEditing && modelData?.data) {
      const model = modelData.data
      setOldModelName(model.model_name)

      const pricing = readPricingConfig(modelSettings, model.model_name)
      setLoadedPricingName(model.model_name)
      loadedPricingRef.current = pricing
      setPricingSubMode('ratio')
      setPricingMode(pricing.mode)
      setPromptPrice(pricing.promptPrice)
      setCompletionPrice(pricing.completionPrice)
      setAdvancedOpen(pricing.advancedOpen)
      form.reset({
        id: model.id,
        model_name: model.model_name,
        description: model.description || '',
        icon: model.icon || '',
        tags: parseModelTags(model.tags),
        vendor_id: model.vendor_id,
        endpoints: model.endpoints || '',
        name_rule: model.name_rule || 0,
        status: model.status === 1,
        sync_official: model.sync_official === 1,
        ...pricing.fields,
      })
      initializedFormKeyRef.current = formKey
    } else if (open && !isEditing) {
      // 缺失模型入口可能预填一个已经配置过价格的名称，创建表单也要读回价格。
      const modelName = currentRow?.model_name || ''
      const pricing = readPricingConfig(modelSettings, modelName)
      setOldModelName('')
      setLoadedPricingName(modelName)
      loadedPricingRef.current = pricing
      setPricingSubMode('ratio')
      setPricingMode(pricing.mode)
      setPromptPrice(pricing.promptPrice)
      setCompletionPrice(pricing.completionPrice)
      setAdvancedOpen(pricing.advancedOpen)
      form.reset({
        model_name: modelName,
        description: '',
        icon: '',
        tags: [],
        vendor_id: undefined,
        endpoints: '',
        name_rule: 0,
        status: true,
        sync_official: true,
        ...pricing.fields,
      })
      initializedFormKeyRef.current = formKey
    }
  }, [open, isEditing, modelData, currentRow, form, modelSettings])

  const onSubmit = useCallback(
    async (values: ExtendedModelFormValues): Promise<void> => {
      setIsSubmitting(true)
      try {
        if (!modelSettings) {
          throw new Error('Model pricing settings are still loading')
        }

        // 定价选项是整张按模型名索引的映射。提交前必须重新读取，避免用
        // React Query 的旧快照覆盖其他页面或其他管理员刚保存的价格。
        const latestOptions = await getSystemOptions()
        if (!latestOptions.success || !latestOptions.data) {
          throw new Error(
            latestOptions.message || 'Failed to load current model pricing'
          )
        }
        const latestModelSettings = getOptionValue(
          latestOptions.data,
          modelSettings
        )

        if (
          isEditing &&
          oldModelName &&
          oldModelName !== values.model_name &&
          hasPricingOutsideDrawer(latestModelSettings, oldModelName)
        ) {
          throw new Error(
            'Move advanced or dynamic pricing to the new model name before renaming it'
          )
        }

        const submitData = {
          ...values,
          id: isEditing ? currentRow!.id : undefined,
          tags: Array.isArray(values.tags) ? values.tags.join(',') : '',
          status: values.status ? 1 : 0,
          sync_official: values.sync_official ? 1 : 0,
        }

        // Remove ratio fields from model data (they're stored in system settings)
        const {
          price,
          ratio,
          cacheRatio,
          completionRatio,
          imageRatio,
          audioRatio,
          audioCompletionRatio,
          ...modelData
        } = submitData

        const response = isEditing
          ? await updateModel({ ...modelData, id: currentRow!.id })
          : await createModel(modelData)

        if (response.success) {
          // Handle ratio configuration updates in system settings
          const finalModelName = values.model_name
          const hasRatioConfig = Boolean(
            (pricingMode === 'per-request' &&
              values.price &&
              values.price !== '') ||
            (pricingMode === 'per-token' &&
              (values.ratio ||
                values.cacheRatio ||
                values.completionRatio ||
                values.imageRatio ||
                values.audioRatio ||
                values.audioCompletionRatio))
          )
          const pricingWasEdited = pricingConfigChanged(
            loadedPricingRef.current,
            pricingMode,
            {
              price: values.price,
              ratio: values.ratio,
              cacheRatio: values.cacheRatio,
              completionRatio: values.completionRatio,
              imageRatio: values.imageRatio,
              audioRatio: values.audioRatio,
              audioCompletionRatio: values.audioCompletionRatio,
            }
          )

          // 即使所有输入都被清空也要处理，以便显式移除旧定价。
          {
            // Read existing configurations
            const priceMap = safeJsonParse<Record<string, number>>(
              latestModelSettings.ModelPrice,
              { fallback: {}, silent: true }
            )
            const ratioMap = safeJsonParse<Record<string, number>>(
              latestModelSettings.ModelRatio,
              { fallback: {}, silent: true }
            )
            const cacheMap = safeJsonParse<Record<string, number>>(
              latestModelSettings.CacheRatio,
              { fallback: {}, silent: true }
            )
            const completionMap = safeJsonParse<Record<string, number>>(
              latestModelSettings.CompletionRatio,
              { fallback: {}, silent: true }
            )
            const imageMap = safeJsonParse<Record<string, number>>(
              latestModelSettings.ImageRatio,
              { fallback: {}, silent: true }
            )
            const audioMap = safeJsonParse<Record<string, number>>(
              latestModelSettings.AudioRatio,
              { fallback: {}, silent: true }
            )
            const audioCompletionMap = safeJsonParse<Record<string, number>>(
              latestModelSettings.AudioCompletionRatio,
              { fallback: {}, silent: true }
            )

            const targetHasStoredPricing =
              [
                priceMap,
                ratioMap,
                cacheMap,
                completionMap,
                imageMap,
                audioMap,
                audioCompletionMap,
              ].some((values) =>
                Object.prototype.hasOwnProperty.call(values, finalModelName)
              ) || hasPricingOutsideDrawer(latestModelSettings, finalModelName)
            const isRenamingLoadedPricing =
              isEditing &&
              oldModelName !== finalModelName &&
              loadedPricingName === oldModelName
            const shouldTransferPricing =
              isRenamingLoadedPricing &&
              hasRatioConfig &&
              !targetHasStoredPricing
            const shouldRewritePricing =
              (pricingWasEdited &&
                ((loadedPricingName !== '' &&
                  finalModelName === loadedPricingName) ||
                  hasRatioConfig)) ||
              shouldTransferPricing

            // 重命名到已有定价的名称时保留目标价格；目标尚无定价时才把
            // 当前表单中已加载的价格随名称迁移过去。
            if (
              isEditing &&
              oldModelName &&
              oldModelName !== finalModelName &&
              (shouldTransferPricing || targetHasStoredPricing)
            ) {
              delete priceMap[oldModelName]
              delete ratioMap[oldModelName]
              delete cacheMap[oldModelName]
              delete completionMap[oldModelName]
              delete imageMap[oldModelName]
              delete audioMap[oldModelName]
              delete audioCompletionMap[oldModelName]
            }

            // 只有表单实际加载过该名称的定价，或用户明确填写了新定价时，
            // 才能重建对应映射。工具栏“新建模型”允许手输已有名称，空白
            // 定价区不能因此删除管理员原有配置。
            if (shouldRewritePricing) {
              delete priceMap[finalModelName]
              delete ratioMap[finalModelName]
              delete cacheMap[finalModelName]
              delete completionMap[finalModelName]
              delete imageMap[finalModelName]
              delete audioMap[finalModelName]
              delete audioCompletionMap[finalModelName]
            }

            // 动态计费表达式不属于此抽屉的可编辑字段，因此这里不读取或
            // 删除 billing_mode / billing_expr 映射，元数据操作不会误伤它们。

            // 仅在本次确实重建目标定价时写入。重命名到已有定价的名称且
            // 用户没有编辑价格时，目标名称必须保留提交前重新读取到的配置。
            if (hasRatioConfig && shouldRewritePricing) {
              if (
                pricingMode === 'per-request' &&
                values.price &&
                values.price !== ''
              ) {
                priceMap[finalModelName] = parseFloat(values.price)
              } else if (pricingMode === 'per-token') {
                if (values.ratio && values.ratio !== '') {
                  ratioMap[finalModelName] = parseFloat(values.ratio)
                }
                if (values.cacheRatio && values.cacheRatio !== '') {
                  cacheMap[finalModelName] = parseFloat(values.cacheRatio)
                }
                if (values.completionRatio && values.completionRatio !== '') {
                  completionMap[finalModelName] = parseFloat(
                    values.completionRatio
                  )
                }
                if (values.imageRatio && values.imageRatio !== '') {
                  imageMap[finalModelName] = parseFloat(values.imageRatio)
                }
                if (values.audioRatio && values.audioRatio !== '') {
                  audioMap[finalModelName] = parseFloat(values.audioRatio)
                }
                if (
                  values.audioCompletionRatio &&
                  values.audioCompletionRatio !== ''
                ) {
                  audioCompletionMap[finalModelName] = parseFloat(
                    values.audioCompletionRatio
                  )
                }
              }
            }

            // Update system options if there are changes
            const updates: Array<{ key: string; value: string }> = []

            const newModelPrice = normalizeJsonString(JSON.stringify(priceMap))
            if (
              newModelPrice !==
              normalizeJsonString(latestModelSettings.ModelPrice)
            ) {
              updates.push({ key: 'ModelPrice', value: newModelPrice })
            }

            const newModelRatio = normalizeJsonString(JSON.stringify(ratioMap))
            if (
              newModelRatio !==
              normalizeJsonString(latestModelSettings.ModelRatio)
            ) {
              updates.push({ key: 'ModelRatio', value: newModelRatio })
            }

            const newCacheRatio = normalizeJsonString(JSON.stringify(cacheMap))
            if (
              newCacheRatio !==
              normalizeJsonString(latestModelSettings.CacheRatio)
            ) {
              updates.push({ key: 'CacheRatio', value: newCacheRatio })
            }

            const newCompletionRatio = normalizeJsonString(
              JSON.stringify(completionMap)
            )
            if (
              newCompletionRatio !==
              normalizeJsonString(latestModelSettings.CompletionRatio)
            ) {
              updates.push({
                key: 'CompletionRatio',
                value: newCompletionRatio,
              })
            }

            const newImageRatio = normalizeJsonString(JSON.stringify(imageMap))
            if (
              newImageRatio !==
              normalizeJsonString(latestModelSettings.ImageRatio)
            ) {
              updates.push({ key: 'ImageRatio', value: newImageRatio })
            }

            const newAudioRatio = normalizeJsonString(JSON.stringify(audioMap))
            if (
              newAudioRatio !==
              normalizeJsonString(latestModelSettings.AudioRatio)
            ) {
              updates.push({ key: 'AudioRatio', value: newAudioRatio })
            }

            const newAudioCompletionRatio = normalizeJsonString(
              JSON.stringify(audioCompletionMap)
            )
            if (
              newAudioCompletionRatio !==
              normalizeJsonString(latestModelSettings.AudioCompletionRatio)
            ) {
              updates.push({
                key: 'AudioCompletionRatio',
                value: newAudioCompletionRatio,
              })
            }

            // 从固定价切换到倍率时先写入新倍率，最后再删除旧固定价；中途
            // 失败仍会保留旧的有效价格，不会落到未知模型默认倍率。
            const orderedUpdates =
              pricingMode === 'per-token'
                ? [
                    ...updates.filter(({ key }) => key !== 'ModelPrice'),
                    ...updates.filter(({ key }) => key === 'ModelPrice'),
                  ]
                : updates

            for (const update of orderedUpdates) {
              const result = await updateOption.mutateAsync(update)
              if (!result.success) {
                throw new Error(result.message || 'Failed to update pricing')
              }
            }
          }

          toast.success(
            isEditing
              ? 'Model updated successfully'
              : 'Model created successfully'
          )
          queryClient.invalidateQueries({ queryKey: modelsQueryKeys.lists() })
          queryClient.invalidateQueries({ queryKey: ['system-options'] })
          onOpenChange(false)
        } else {
          toast.error(response.message || 'Operation failed')
        }
      } catch (error: unknown) {
        toast.error((error as Error)?.message || 'Operation failed')
      } finally {
        setIsSubmitting(false)
      }
    },
    [
      isEditing,
      currentRow,
      queryClient,
      onOpenChange,
      pricingMode,
      oldModelName,
      loadedPricingName,
      modelSettings,
      updateOption,
    ]
  )

  const handleFillEndpointTemplate = (templateKey: string) => {
    const template = ENDPOINT_TEMPLATES[templateKey]
    if (template) {
      const templateJson = JSON.stringify({ [templateKey]: template }, null, 2)
      form.setValue('endpoints', templateJson)
    }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-2xl')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>
            {isEditing ? t('Edit Model') : t('Create Model')}
          </SheetTitle>
          <SheetDescription>
            {isEditing
              ? t("Update model configuration and click save when you're done.")
              : t(
                  'Add a new model to the system by providing the necessary information.'
                )}
          </SheetDescription>
        </SheetHeader>

        <Form {...form}>
          <form
            id='model-form'
            onSubmit={form.handleSubmit(
              onSubmit as Parameters<typeof form.handleSubmit>[0]
            )}
            className={sideDrawerFormClassName()}
          >
            {/* Basic Information */}
            <SideDrawerSection>
              <h3 className='text-sm font-semibold'>
                {t('Basic Information')}
              </h3>

              <FormField
                control={form.control}
                name='model_name'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Model Name *')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('gpt-4, claude-3-opus, etc.')}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('The unique identifier for this model')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='description'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Description')}</FormLabel>
                    <FormControl>
                      <Textarea
                        placeholder={t('Describe this model...')}
                        rows={3}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='icon'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Icon')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('OpenAI, Anthropic, etc.')}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription className='text-xs'>
                      {t('@lobehub/icons key')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='vendor_id'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Vendor')}</FormLabel>
                    <Select
                      items={[
                        ...vendors.map((vendor) => ({
                          value: String(vendor.id),
                          label: vendor.name,
                        })),
                      ]}
                      onValueChange={(value) =>
                        field.onChange(value ? parseInt(value) : undefined)
                      }
                      value={field.value ? String(field.value) : undefined}
                    >
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue placeholder={t('Select vendor')} />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          {vendors.map((vendor) => (
                            <SelectItem
                              key={vendor.id}
                              value={String(vendor.id)}
                            >
                              {vendor.name}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='tags'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Tags')}</FormLabel>
                    <FormControl>
                      <TagInput
                        value={field.value || []}
                        onChange={field.onChange}
                        placeholder={t('Add tags...')}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Press Enter or comma to add tags')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </SideDrawerSection>

            {/* Matching Configuration */}
            <SideDrawerSection>
              <h3 className='text-sm font-semibold'>{t('Matching Rules')}</h3>

              <FormField
                control={form.control}
                name='name_rule'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Name Rule')}</FormLabel>
                    <FormControl>
                      <RadioGroup
                        onValueChange={(value) =>
                          field.onChange(parseInt(value))
                        }
                        value={String(field.value)}
                        className='grid grid-cols-2 gap-4'
                      >
                        {getNameRuleOptions(t).map((option) => (
                          <div
                            key={option.value}
                            className='flex items-center space-x-2'
                          >
                            <RadioGroupItem
                              value={String(option.value)}
                              id={`rule-${option.value}`}
                            />
                            <Label
                              htmlFor={`rule-${option.value}`}
                              className='cursor-pointer font-normal'
                            >
                              {option.label}
                            </Label>
                          </div>
                        ))}
                      </RadioGroup>
                    </FormControl>
                    <FormDescription>
                      {t('How this model name should match requests')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </SideDrawerSection>

            {/* Endpoints Configuration */}
            <SideDrawerSection>
              <div className='flex items-center justify-between'>
                <h3 className='text-sm font-semibold'>{t('Endpoints')}</h3>
                <Select<string>
                  items={[
                    ...Object.keys(ENDPOINT_TEMPLATES).map((key) => ({
                      value: key,
                      label: key,
                    })),
                  ]}
                  onValueChange={(v) =>
                    v !== null && handleFillEndpointTemplate(v)
                  }
                >
                  <SelectTrigger size='sm' className='w-[200px]'>
                    <SelectValue placeholder={t('Load template...')} />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      {Object.keys(ENDPOINT_TEMPLATES).map((key) => (
                        <SelectItem key={key} value={key}>
                          {key}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </div>

              <FormField
                control={form.control}
                name='endpoints'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Endpoint Configuration')}</FormLabel>
                    <FormControl>
                      <JsonEditor
                        value={field.value || ''}
                        onChange={field.onChange}
                        keyPlaceholder='endpoint_type'
                        valuePlaceholder='{"path": "/v1/...", "method": "POST"}'
                        keyLabel='Endpoint Type'
                        valueLabel='Configuration'
                        valueType='any'
                        emptyMessage={t(
                          'No endpoints configured. Switch to JSON mode or add rows to define endpoints.'
                        )}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        "Model information is mainly used for Model Square display. Endpoint configuration also affects image auto-routing: built-in image models are detected automatically, while custom models are eligible only when image-generation is their sole endpoint. Codex channels, openai-compact models, and custom multi-endpoint models keep the client's original path. Configure channels and model mappings in Channel Management."
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </SideDrawerSection>

            {/* Pricing Configuration */}
            <SideDrawerSection>
              <h3 className='text-sm font-semibold'>
                {t('Pricing Configuration')}
              </h3>

              <div className='space-y-4'>
                <Label>{t('Pricing mode')}</Label>
                <RadioGroup
                  value={pricingMode}
                  onValueChange={(value) =>
                    setPricingMode(value as PricingMode)
                  }
                >
                  <div className='flex items-center space-x-2'>
                    <RadioGroupItem value='per-token' id='per-token' />
                    <Label htmlFor='per-token' className='font-normal'>
                      {t('Per-token (ratio based)')}
                    </Label>
                  </div>
                  <div className='flex items-center space-x-2'>
                    <RadioGroupItem value='per-request' id='per-request' />
                    <Label htmlFor='per-request' className='font-normal'>
                      {t('Per-request (fixed price)')}
                    </Label>
                  </div>
                </RadioGroup>
              </div>

              {pricingMode === 'per-request' ? (
                <FormField
                  control={form.control}
                  name='price'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Fixed price (USD)')}</FormLabel>
                      <FormControl>
                        <Input
                          type='text'
                          placeholder='0.01'
                          {...field}
                          onChange={(e) => {
                            const value = e.target.value
                            if (validateNumber(value)) {
                              field.onChange(value)
                            }
                          }}
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Cost in USD per request, regardless of tokens used.'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              ) : (
                <>
                  <div className='space-y-4'>
                    <Label>{t('Input mode')}</Label>
                    <RadioGroup
                      value={pricingSubMode}
                      onValueChange={(value) =>
                        setPricingSubMode(value as PricingSubMode)
                      }
                    >
                      <div className='flex items-center space-x-2'>
                        <RadioGroupItem value='ratio' id='ratio' />
                        <Label htmlFor='ratio' className='font-normal'>
                          {t('Ratio mode')}
                        </Label>
                      </div>
                      <div className='flex items-center space-x-2'>
                        <RadioGroupItem value='price' id='price' />
                        <Label htmlFor='price' className='font-normal'>
                          {t('Price mode (USD per 1M tokens)')}
                        </Label>
                      </div>
                    </RadioGroup>
                  </div>

                  {pricingSubMode === 'ratio' ? (
                    <>
                      <FormField
                        control={form.control}
                        name='ratio'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Model ratio')}</FormLabel>
                            <FormControl>
                              <Input
                                type='text'
                                placeholder='1.0'
                                {...field}
                                onChange={(e) => {
                                  const value = e.target.value
                                  if (validateNumber(value)) {
                                    field.onChange(value)
                                    if (value) {
                                      setPromptPrice(
                                        (parseFloat(value) * 2).toString()
                                      )
                                    } else {
                                      setPromptPrice('')
                                    }
                                  }
                                }}
                              />
                            </FormControl>
                            <FormDescription>
                              {field.value && !isNaN(parseFloat(field.value))
                                ? `Calculated price: $${(parseFloat(field.value) * 2).toFixed(4)} per 1M tokens`
                                : t('Multiplier for prompt tokens.')}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />

                      <FormField
                        control={form.control}
                        name='completionRatio'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Completion ratio')}</FormLabel>
                            <FormControl>
                              <Input
                                type='text'
                                placeholder='1.0'
                                {...field}
                                onChange={(e) => {
                                  const value = e.target.value
                                  if (validateNumber(value)) {
                                    field.onChange(value)
                                    const ratio = form.getValues('ratio')
                                    if (value && ratio) {
                                      const compPrice =
                                        parseFloat(ratio) *
                                        2 *
                                        parseFloat(value)
                                      setCompletionPrice(compPrice.toString())
                                    } else {
                                      setCompletionPrice('')
                                    }
                                  }
                                }}
                              />
                            </FormControl>
                            <FormDescription>
                              {field.value &&
                              !isNaN(parseFloat(field.value)) &&
                              promptPrice &&
                              !isNaN(parseFloat(promptPrice))
                                ? `Calculated price: $${(parseFloat(promptPrice) * parseFloat(field.value)).toFixed(4)} per 1M tokens`
                                : t('Multiplier for completion tokens.')}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    </>
                  ) : (
                    <>
                      <div className='space-y-4'>
                        <div className='space-y-2'>
                          <Label>{t('Prompt price ($/1M tokens)')}</Label>
                          <Input
                            type='text'
                            placeholder='2.0'
                            value={promptPrice}
                            onChange={(e) =>
                              handlePromptPriceChange(e.target.value)
                            }
                          />
                          <p className='text-muted-foreground text-sm'>
                            {promptPrice && !isNaN(parseFloat(promptPrice))
                              ? `Calculated ratio: ${(parseFloat(promptPrice) / 2).toFixed(4)}`
                              : t('Enter Input price to calculate ratio')}
                          </p>
                        </div>

                        <div className='space-y-2'>
                          <Label>{t('Completion price ($/1M tokens)')}</Label>
                          <Input
                            type='text'
                            placeholder='4.0'
                            value={completionPrice}
                            onChange={(e) =>
                              handleCompletionPriceChange(e.target.value)
                            }
                          />
                          <p className='text-muted-foreground text-sm'>
                            {completionPrice &&
                            !isNaN(parseFloat(completionPrice)) &&
                            promptPrice &&
                            !isNaN(parseFloat(promptPrice)) &&
                            parseFloat(promptPrice) > 0
                              ? `Calculated ratio: ${(parseFloat(completionPrice) / parseFloat(promptPrice)).toFixed(4)}`
                              : t('Enter Completion price to calculate ratio')}
                          </p>
                        </div>
                      </div>
                    </>
                  )}

                  <Collapsible
                    open={advancedOpen}
                    onOpenChange={setAdvancedOpen}
                  >
                    <CollapsibleTrigger
                      render={
                        <Button
                          type='button'
                          variant='outline'
                          className='flex w-full items-center justify-between'
                        />
                      }
                    >
                      {t('Advanced options')}
                      <ChevronDown
                        className={`h-4 w-4 transition-transform duration-200 ${
                          advancedOpen ? 'rotate-180' : ''
                        }`}
                      />
                    </CollapsibleTrigger>
                    <CollapsibleContent className='flex flex-col gap-4 pt-4'>
                      <FormField
                        control={form.control}
                        name='cacheRatio'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Cache ratio')}</FormLabel>
                            <FormControl>
                              <Input
                                type='text'
                                placeholder='0.1'
                                {...field}
                                onChange={(e) => {
                                  const value = e.target.value
                                  if (validateNumber(value)) {
                                    field.onChange(value)
                                  }
                                }}
                              />
                            </FormControl>
                            <FormDescription>
                              {t('Discount ratio for cache hits.')}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />

                      <FormField
                        control={form.control}
                        name='imageRatio'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Image ratio')}</FormLabel>
                            <FormControl>
                              <Input
                                type='text'
                                placeholder='1.0'
                                {...field}
                                onChange={(e) => {
                                  const value = e.target.value
                                  if (validateNumber(value)) {
                                    field.onChange(value)
                                  }
                                }}
                              />
                            </FormControl>
                            <FormDescription>
                              {t('Multiplier for image processing.')}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />

                      <FormField
                        control={form.control}
                        name='audioRatio'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Audio ratio')}</FormLabel>
                            <FormControl>
                              <Input
                                type='text'
                                placeholder='1.0'
                                {...field}
                                onChange={(e) => {
                                  const value = e.target.value
                                  if (validateNumber(value)) {
                                    field.onChange(value)
                                  }
                                }}
                              />
                            </FormControl>
                            <FormDescription>
                              {t('Multiplier for audio inputs.')}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />

                      <FormField
                        control={form.control}
                        name='audioCompletionRatio'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Audio completion ratio')}</FormLabel>
                            <FormControl>
                              <Input
                                type='text'
                                placeholder='1.0'
                                {...field}
                                onChange={(e) => {
                                  const value = e.target.value
                                  if (validateNumber(value)) {
                                    field.onChange(value)
                                  }
                                }}
                              />
                            </FormControl>
                            <FormDescription>
                              {t('Multiplier for audio outputs.')}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    </CollapsibleContent>
                  </Collapsible>
                </>
              )}
            </SideDrawerSection>

            {/* Status & Sync */}
            <SideDrawerSection>
              <h3 className='text-sm font-semibold'>{t('Status & Sync')}</h3>

              <FormField
                control={form.control}
                name='status'
                render={({ field }) => (
                  <FormItem className={sideDrawerSwitchItemClassName()}>
                    <div className='flex flex-col gap-0.5'>
                      <FormLabel className='text-base'>
                        {t('Enabled')}
                      </FormLabel>
                      <FormDescription>
                        {t('Enable or disable this model')}
                      </FormDescription>
                    </div>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='sync_official'
                render={({ field }) => (
                  <FormItem className={sideDrawerSwitchItemClassName()}>
                    <div className='flex flex-col gap-0.5'>
                      <FormLabel className='text-base'>
                        {t('Official Sync')}
                      </FormLabel>
                      <FormDescription>
                        {t('Sync this model with official upstream')}
                      </FormDescription>
                    </div>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
            </SideDrawerSection>
          </form>
        </Form>

        <SheetFooter className={sideDrawerFooterClassName()}>
          <SheetClose
            render={<Button variant='outline' disabled={isSubmitting} />}
          >
            {t('Cancel')}
          </SheetClose>
          <Button form='model-form' type='submit' disabled={isSubmitting}>
            {isSubmitting && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {isEditing ? t('Update Model') : t('Save changes')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
