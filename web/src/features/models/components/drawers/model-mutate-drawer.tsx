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
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, Loader2 } from 'lucide-react'
import { useEffect, useState, useCallback, useMemo, useRef } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

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
  useSystemOptions,
  getOptionValue,
} from '@/features/system-settings/hooks/use-system-options'
import { useUpdateOption } from '@/features/system-settings/hooks/use-update-option'
import { normalizeJsonString } from '@/features/system-settings/models/utils'
import type { ModelSettings } from '@/features/system-settings/types'
import { safeJsonParse } from '@/features/system-settings/utils/json-parser'

import { createModel, updateModel, getModel, getVendors } from '../../api'
import { getNameRuleOptions, ENDPOINT_TEMPLATES } from '../../constants'
import {
  modelsQueryKeys,
  vendorsQueryKeys,
  parseModelTags,
  buildPricingRatios,
  type PricingPriceInputs,
} from '../../lib'
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
  createCacheRatio: z.string().optional(),
  completionRatio: z.string().optional(),
})

type ExtendedModelFormValues = z.infer<typeof extendedModelFormSchema>

type PricingMode = 'per-token' | 'per-request'

type PricingFields = Pick<
  ExtendedModelFormValues,
  'price' | 'ratio' | 'cacheRatio' | 'createCacheRatio' | 'completionRatio'
>

// Form state describing the pricing currently configured for one model name.
// The interface shows prices in USD per 1M tokens; the ratio fields carry the
// values the backend stores, derived from those prices on every edit.
type PricingConfig = {
  mode: PricingMode
  fields: PricingFields
  promptPrice: string
  completionPrice: string
  cachePrice: string
  createCachePrice: string
  advancedOpen: boolean
}

const EMPTY_PRICING_FIELDS: PricingFields = {
  price: '',
  ratio: '',
  cacheRatio: '',
  createCacheRatio: '',
  completionRatio: '',
}

const EMPTY_PRICING_CONFIG: PricingConfig = {
  mode: 'per-token',
  fields: EMPTY_PRICING_FIELDS,
  promptPrice: '',
  completionPrice: '',
  cachePrice: '',
  createCachePrice: '',
  advancedOpen: false,
}

function lookupModelRatio(
  rawMap: string,
  modelName: string
): number | undefined {
  return safeJsonParse<Record<string, number>>(rawMap, {
    fallback: {},
    silent: true,
  })[modelName]
}

// Prices are shown in USD per 1M tokens and stored as ratios, so the values
// read back out of the option maps need their float drift trimmed before they
// land in an input field.
function formatPriceInput(value: number): string {
  return Number(value.toFixed(10)).toString()
}

// Pricing is not stored on the model row: it lives in system options as
// model-name keyed JSON maps, so it has to be read back out of those maps to
// populate the form. Both create and edit rely on this, because submit rebuilds
// the maps from the form and would otherwise drop pricing it never loaded.
function readPricingConfig(
  settings: ModelSettings | null,
  modelName: string
): PricingConfig {
  if (!settings || !modelName) return EMPTY_PRICING_CONFIG

  const price = lookupModelRatio(settings.ModelPrice, modelName)
  const ratio = lookupModelRatio(settings.ModelRatio, modelName)
  const cacheRatio = lookupModelRatio(settings.CacheRatio, modelName)
  const createCacheRatio = lookupModelRatio(settings.CreateCacheRatio, modelName)
  const completionRatio = lookupModelRatio(settings.CompletionRatio, modelName)

  // A fixed per-request price wins outright at billing time (see
  // GetModelRatioOrPrice), so a name that has one is shown, and saved back, as
  // price-only: the ratios alongside it are dead weight.
  if (price !== undefined && price !== null) {
    return {
      ...EMPTY_PRICING_CONFIG,
      mode: 'per-request',
      fields: { ...EMPTY_PRICING_FIELDS, price: price.toString() },
    }
  }

  let promptPrice = ''
  let completionPrice = ''
  let cachePrice = ''
  let createCachePrice = ''
  if (ratio !== undefined && ratio !== null) {
    const inputPrice = ratio * 2
    promptPrice = formatPriceInput(inputPrice)
    if (completionRatio !== undefined && completionRatio !== null) {
      completionPrice = formatPriceInput(inputPrice * completionRatio)
    }
    if (cacheRatio !== undefined && cacheRatio !== null) {
      cachePrice = formatPriceInput(inputPrice * cacheRatio)
    }
    if (createCacheRatio !== undefined && createCacheRatio !== null) {
      createCachePrice = formatPriceInput(inputPrice * createCacheRatio)
    }
  }

  return {
    mode: 'per-token',
    fields: {
      price: '',
      ratio: ratio?.toString() || '',
      cacheRatio: cacheRatio?.toString() || '',
      createCacheRatio: createCacheRatio?.toString() || '',
      completionRatio: completionRatio?.toString() || '',
    },
    promptPrice,
    completionPrice,
    cachePrice,
    createCachePrice,
    // Configured is not the same as non-zero: a 0 ratio (free cache reads, for
    // instance) still has to be visible rather than hidden behind the collapse.
    advancedOpen: [cacheRatio, createCacheRatio].some(
      (value) => value !== undefined && value !== null
    ),
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
  const currentModelId = currentRow?.id
  const isEditing = Boolean(currentModelId)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [pricingMode, setPricingMode] = useState<PricingMode>('per-token')
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [promptPrice, setPromptPrice] = useState('')
  const [completionPrice, setCompletionPrice] = useState('')
  const [cachePrice, setCachePrice] = useState('')
  const [createCachePrice, setCreateCachePrice] = useState('')
  const [oldModelName, setOldModelName] = useState<string>('')
  // Model name whose pricing was read into the form when the drawer opened.
  // Submit may only rewrite pricing for this name, or for a name the user
  // explicitly priced; anything else it never saw and must leave alone.
  const [loadedPricingName, setLoadedPricingName] = useState<string>('')
  // Keep a ref so the load effect can read the latest modelSettings without
  // depending on it: modelSettings is a fresh object on every system-options
  // refetch, and including it in the deps would reset the form under the user.
  const modelSettingsRef = useRef<ModelSettings | null>(null)

  // Fetch vendors for dropdown
  const { data: vendorsData } = useQuery({
    queryKey: vendorsQueryKeys.list(),
    queryFn: () => getVendors({ page_size: 1000 }),
    enabled: open,
  })

  const vendors = vendorsData?.data?.items || []

  // Fetch model detail if editing
  const { data: modelData } = useQuery({
    queryKey: modelsQueryKeys.detail(currentModelId || 0),
    queryFn: () => {
      if (!currentModelId) {
        throw new Error('Model ID is required')
      }
      return getModel(currentModelId)
    },
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
      RetryTimes: 0,
      ChannelDisableThreshold: '',
      AutomaticDisableChannelEnabled: false,
      AutomaticEnableChannelEnabled: false,
      AutomaticDisableKeywords: '',
      AutomaticDisableStatusCodes: '401',
      AutomaticRetryStatusCodes:
        '100-199,300-399,401-407,409-499,500-503,505-523,525-599',
      'monitor_setting.auto_test_channel_enabled': false,
      'monitor_setting.auto_test_channel_minutes': 10,
      'monitor_setting.channel_test_mode': 'scheduled_all',
      'channel_affinity_setting.enabled': false,
      'channel_affinity_setting.switch_on_success': true,
      'channel_affinity_setting.keep_on_channel_disabled': false,
      'channel_affinity_setting.max_entries': 100000,
      'channel_affinity_setting.default_ttl_seconds': 3600,
      'channel_affinity_setting.rules': '[]',
      'model_deployment.ionet.api_key': '',
      'model_deployment.ionet.enabled': false,
    }
    return getOptionValue(systemOptionsData.data, defaultModelSettings)
  }, [systemOptionsData])

  // The load effect keys off this boolean, not the object: it re-runs once
  // when the settings first arrive (so a drawer opened before that still gets
  // its pricing prefilled), while later refetches only produce a new object
  // reference and must not reset a form the user may be editing.
  const hasModelSettings = modelSettings !== null
  useEffect(() => {
    modelSettingsRef.current = modelSettings
  })

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
      createCacheRatio: '',
      completionRatio: '',
    },
  })

  const validateNumber = (value: string) => {
    if (value === '') return true
    return !Number.isNaN(Number.parseFloat(value))
  }

  // Every price input shares one conversion: the prices shown in the form are
  // turned into the ratios the backend stores.
  const syncPricingFields = (next: PricingPriceInputs) => {
    const fields = buildPricingRatios(next)
    form.setValue('ratio', fields.ratio)
    form.setValue('completionRatio', fields.completionRatio)
    form.setValue('cacheRatio', fields.cacheRatio)
    form.setValue('createCacheRatio', fields.createCacheRatio)
  }

  const handlePromptPriceChange = (value: string) => {
    if (!validateNumber(value)) return
    setPromptPrice(value)
    syncPricingFields({
      prompt: value,
      completion: completionPrice,
      cache: cachePrice,
      createCache: createCachePrice,
    })
  }

  const handleCompletionPriceChange = (value: string) => {
    if (!validateNumber(value)) return
    setCompletionPrice(value)
    syncPricingFields({
      prompt: promptPrice,
      completion: value,
      cache: cachePrice,
      createCache: createCachePrice,
    })
  }

  const handleCachePriceChange = (value: string) => {
    if (!validateNumber(value)) return
    setCachePrice(value)
    syncPricingFields({
      prompt: promptPrice,
      completion: completionPrice,
      cache: value,
      createCache: createCachePrice,
    })
  }

  const handleCreateCachePriceChange = (value: string) => {
    if (!validateNumber(value)) return
    setCreateCachePrice(value)
    syncPricingFields({
      prompt: promptPrice,
      completion: completionPrice,
      cache: cachePrice,
      createCache: value,
    })
  }

  // Load model data for editing and ratio configuration
  useEffect(() => {
    if (open && isEditing && modelData?.data) {
      const model = modelData.data
      setOldModelName(model.model_name)

      const pricing = readPricingConfig(
        modelSettingsRef.current,
        model.model_name
      )
      setLoadedPricingName(model.model_name)
      setPricingMode(pricing.mode)
      setPromptPrice(pricing.promptPrice)
      setCompletionPrice(pricing.completionPrice)
      setCachePrice(pricing.cachePrice)
      setCreateCachePrice(pricing.createCachePrice)
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
    } else if (open && !isEditing) {
      // Pre-fill model name if passed from missing models, along with any
      // pricing that name already has, so the user edits it instead of being
      // shown an empty form that hides existing configuration.
      const modelName = currentRow?.model_name || ''
      const pricing = readPricingConfig(modelSettingsRef.current, modelName)
      setOldModelName('')
      setLoadedPricingName(modelName)
      setPricingMode(pricing.mode)
      setPromptPrice(pricing.promptPrice)
      setCompletionPrice(pricing.completionPrice)
      setCachePrice(pricing.cachePrice)
      setCreateCachePrice(pricing.createCachePrice)
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
    }
  }, [open, isEditing, modelData, currentRow, form, hasModelSettings])

  const onSubmit = useCallback(
    async (values: ExtendedModelFormValues): Promise<void> => {
      setIsSubmitting(true)
      try {
        const submitData = {
          ...values,
          id: isEditing ? currentModelId : undefined,
          tags: Array.isArray(values.tags) ? values.tags.join(',') : '',
          status: values.status ? 1 : 0,
          sync_official: values.sync_official ? 1 : 0,
        }

        // Remove ratio fields from model data (they're stored in system settings)
        const {
          price,
          ratio,
          cacheRatio,
          createCacheRatio,
          completionRatio,
          ...modelData
        } = submitData

        const response =
          isEditing && currentModelId
            ? await updateModel({ ...modelData, id: currentModelId })
            : await createModel(modelData)

        if (response.success) {
          // Handle ratio configuration updates in system settings
          const finalModelName = values.model_name
          const hasRatioConfig =
            (pricingMode === 'per-request' &&
              values.price &&
              values.price !== '') ||
            (pricingMode === 'per-token' &&
              (values.ratio ||
                values.cacheRatio ||
                values.createCacheRatio ||
                values.completionRatio))

          // Always process system settings updates if we have modelSettings
          // This ensures we can remove stale entries even when clearing all pricing fields
          if (modelSettings) {
            // Read existing configurations
            const priceMap = safeJsonParse<Record<string, number>>(
              modelSettings.ModelPrice,
              { fallback: {}, silent: true }
            )
            const ratioMap = safeJsonParse<Record<string, number>>(
              modelSettings.ModelRatio,
              { fallback: {}, silent: true }
            )
            const cacheMap = safeJsonParse<Record<string, number>>(
              modelSettings.CacheRatio,
              { fallback: {}, silent: true }
            )
            const createCacheMap = safeJsonParse<Record<string, number>>(
              modelSettings.CreateCacheRatio,
              { fallback: {}, silent: true }
            )
            const completionMap = safeJsonParse<Record<string, number>>(
              modelSettings.CompletionRatio,
              { fallback: {}, silent: true }
            )

            // Remove old model name entries if model name changed (always, even if no new config)
            if (isEditing && oldModelName && oldModelName !== finalModelName) {
              delete priceMap[oldModelName]
              delete ratioMap[oldModelName]
              delete cacheMap[oldModelName]
              delete createCacheMap[oldModelName]
              delete completionMap[oldModelName]
            }

            // Rebuild this model name's entries from the form, but only when
            // the form speaks for that name: it loaded the name's pricing when
            // the drawer opened, so clearing every field means "remove
            // pricing", or the user typed pricing in, which then wins outright
            // (this is also what replaces the old entries across a mode
            // switch). A name the form never loaded may still have pricing
            // configured elsewhere, and an untouched pricing section must not
            // wipe it -- that covers creating a model over an existing name,
            // and renaming onto one.
            if (hasRatioConfig || finalModelName === loadedPricingName) {
              delete priceMap[finalModelName]
              delete ratioMap[finalModelName]
              // Lane ratios can only be derived from a positive input price.
              // A free model (ratio 0) keeps whatever lanes it already had,
              // because the form cannot express them.
              const replaceLanes =
                pricingMode === 'per-request' ||
                ((values.ratio ?? '') !== '' &&
                  Number.parseFloat(values.ratio ?? '') > 0)
              if (replaceLanes) {
                delete cacheMap[finalModelName]
                delete createCacheMap[finalModelName]
                delete completionMap[finalModelName]
              }
            }

            // Only add new entries if user provided new configuration
            if (hasRatioConfig) {
              if (
                pricingMode === 'per-request' &&
                values.price &&
                values.price !== ''
              ) {
                priceMap[finalModelName] = Number.parseFloat(values.price)
              } else if (pricingMode === 'per-token') {
                if (values.ratio && values.ratio !== '') {
                  ratioMap[finalModelName] = Number.parseFloat(values.ratio)
                }
                if (values.cacheRatio && values.cacheRatio !== '') {
                  cacheMap[finalModelName] = Number.parseFloat(
                    values.cacheRatio
                  )
                }
                if (values.createCacheRatio && values.createCacheRatio !== '') {
                  createCacheMap[finalModelName] = Number.parseFloat(
                    values.createCacheRatio
                  )
                }
                if (values.completionRatio && values.completionRatio !== '') {
                  completionMap[finalModelName] = Number.parseFloat(
                    values.completionRatio
                  )
                }
              }
            }

            // Update system options if there are changes
            const updates: Array<{ key: string; value: string }> = []

            const newModelPrice = normalizeJsonString(JSON.stringify(priceMap))
            if (
              newModelPrice !== normalizeJsonString(modelSettings.ModelPrice)
            ) {
              updates.push({ key: 'ModelPrice', value: newModelPrice })
            }

            const newModelRatio = normalizeJsonString(JSON.stringify(ratioMap))
            if (
              newModelRatio !== normalizeJsonString(modelSettings.ModelRatio)
            ) {
              updates.push({ key: 'ModelRatio', value: newModelRatio })
            }

            const newCacheRatio = normalizeJsonString(JSON.stringify(cacheMap))
            if (
              newCacheRatio !== normalizeJsonString(modelSettings.CacheRatio)
            ) {
              updates.push({ key: 'CacheRatio', value: newCacheRatio })
            }

            const newCompletionRatio = normalizeJsonString(
              JSON.stringify(completionMap)
            )
            if (
              newCompletionRatio !==
              normalizeJsonString(modelSettings.CompletionRatio)
            ) {
              updates.push({
                key: 'CompletionRatio',
                value: newCompletionRatio,
              })
            }

            const newCreateCacheRatio = normalizeJsonString(
              JSON.stringify(createCacheMap)
            )
            if (
              newCreateCacheRatio !==
              normalizeJsonString(modelSettings.CreateCacheRatio)
            ) {
              updates.push({
                key: 'CreateCacheRatio',
                value: newCreateCacheRatio,
              })
            }

            // Apply all updates (including deletions when clearing fields)
            for (const update of updates) {
              await updateOption.mutateAsync(update)
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
      currentModelId,
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
                      items={vendors.map((vendor) => ({
                        value: String(vendor.id),
                        label: vendor.name,
                      }))}
                      onValueChange={(value) =>
                        field.onChange(
                          value ? Number.parseInt(value) : undefined
                        )
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
                          field.onChange(Number.parseInt(value))
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
                  items={Object.keys(ENDPOINT_TEMPLATES).map((key) => ({
                    value: key,
                    label: key,
                  }))}
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
                      {t('Define API endpoints for this model (JSON format)')}
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
                      {t('Per-token billing (USD per 1M tokens)')}
                    </Label>
                  </div>
                  <div className='flex items-center space-x-2'>
                    <RadioGroupItem value='per-request' id='per-request' />
                    <Label htmlFor='per-request' className='font-normal'>
                      {t('Per-request billing (fixed price)')}
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
                  <div className='space-y-2'>
                    <Label htmlFor='prompt-price'>
                      {t('Input price ($/1M tokens)')}
                    </Label>
                    <Input
                      id='prompt-price'
                      type='text'
                      inputMode='decimal'
                      value={promptPrice}
                      onChange={(e) => handlePromptPriceChange(e.target.value)}
                    />
                    <p className='text-muted-foreground text-sm'>
                      {t('USD price per 1M input tokens.')}
                    </p>
                  </div>

                  <div className='space-y-2'>
                    <Label htmlFor='completion-price'>
                      {t('Output price ($/1M tokens)')}
                    </Label>
                    <Input
                      id='completion-price'
                      type='text'
                      inputMode='decimal'
                      value={completionPrice}
                      onChange={(e) =>
                        handleCompletionPriceChange(e.target.value)
                      }
                    />
                  </div>

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
                      <div className='space-y-2'>
                        <Label htmlFor='cache-price'>
                          {t('Cache read price')}
                        </Label>
                        <Input
                          id='cache-price'
                          type='text'
                          inputMode='decimal'
                          value={cachePrice}
                          onChange={(e) =>
                            handleCachePriceChange(e.target.value)
                          }
                        />
                        <p className='text-muted-foreground text-sm'>
                          {t('Token price for cache reads.')}
                        </p>
                      </div>

                      <div className='space-y-2'>
                        <Label htmlFor='create-cache-price'>
                          {t('Cache write price')}
                        </Label>
                        <Input
                          id='create-cache-price'
                          type='text'
                          inputMode='decimal'
                          value={createCachePrice}
                          onChange={(e) =>
                            handleCreateCachePriceChange(e.target.value)
                          }
                        />
                        <p className='text-muted-foreground text-sm'>
                          {t('Token price for creating cache entries.')}
                        </p>
                      </div>
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
