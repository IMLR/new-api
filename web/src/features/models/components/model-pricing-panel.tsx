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
import {
  getOptionValue,
  useSystemOptions,
} from '@/features/system-settings/hooks/use-system-options'
import { RatioSettingsCard } from '@/features/system-settings/models/ratio-settings-card'

const pricingDefaults = {
  ModelPrice: '',
  ModelRatio: '',
  CacheRatio: '',
  CreateCacheRatio: '',
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
  'group_ratio_setting.group_special_usable_group': '{}',
}

export function ModelPricingPanel() {
  const { data, isLoading } = useSystemOptions()

  const settings = getOptionValue(data?.data, pricingDefaults) as {
    ModelPrice: string
    ModelRatio: string
    CacheRatio: string
    CreateCacheRatio: string
    CompletionRatio: string
    ImageRatio: string
    AudioRatio: string
    AudioCompletionRatio: string
    ExposeRatioEnabled: boolean
    'billing_setting.billing_mode': string
    'billing_setting.billing_expr': string
    'tool_price_setting.prices': string
    TopupGroupRatio: string
    GroupRatio: string
    UserUsableGroups: string
    GroupGroupRatio: string
    AutoGroups: string
    DefaultUseAutoGroup: boolean
    'group_ratio_setting.group_special_usable_group': string
  }

  if (isLoading) {
    return null
  }

  return (
    <RatioSettingsCard
      titleKey='Model Pricing'
      modelDefaults={{
        ModelPrice: settings.ModelPrice,
        ModelRatio: settings.ModelRatio,
        CacheRatio: settings.CacheRatio,
        CreateCacheRatio: settings.CreateCacheRatio,
        CompletionRatio: settings.CompletionRatio,
        ImageRatio: settings.ImageRatio,
        AudioRatio: settings.AudioRatio,
        AudioCompletionRatio: settings.AudioCompletionRatio,
        ExposeRatioEnabled: settings.ExposeRatioEnabled,
        BillingMode: settings['billing_setting.billing_mode'],
        BillingExpr: settings['billing_setting.billing_expr'],
      }}
      groupDefaults={{
        TopupGroupRatio: settings.TopupGroupRatio,
        GroupRatio: settings.GroupRatio,
        UserUsableGroups: settings.UserUsableGroups,
        GroupGroupRatio: settings.GroupGroupRatio,
        AutoGroups: settings.AutoGroups,
        DefaultUseAutoGroup: settings.DefaultUseAutoGroup,
        GroupSpecialUsableGroup:
          settings['group_ratio_setting.group_special_usable_group'],
      }}
      toolPricesDefault={settings['tool_price_setting.prices']}
      visibleTabs={['models', 'unset-models', 'tool-prices', 'upstream-sync']}
    />
  )
}
