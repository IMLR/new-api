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
import type { UseFormReturn } from 'react-hook-form'
import { useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
} from '@/components/ui/form'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import type { ChannelFormValues } from '../lib/channel-form'

export function ChannelDetectionSettings(props: {
  form: UseFormReturn<ChannelFormValues>
}) {
  const { t } = useTranslation()
  const type = useWatch({ control: props.form.control, name: 'type' })
  const models = useWatch({ control: props.form.control, name: 'models' })
  if (type !== 1 && type !== 14) return null
  const names = [
    ...new Set(
      (models || '')
        .split(',')
        .map((value) => value.trim())
        .filter(Boolean)
    ),
  ]
  return (
    <div className='mb-5 space-y-4 rounded-lg border p-4'>
      <FormField
        control={props.form.control}
        name='relay_detection'
        render={({ field }) => (
          <FormItem className='flex items-center justify-between gap-4'>
            <div>
              <FormLabel>{t('Relay detection')}</FormLabel>
              <FormDescription>
                {t(
                  'Show fingerprint tests for this channel. Each test sends three paid requests.'
                )}
              </FormDescription>
            </div>
            <FormControl>
              <Switch
                checked={field.value || false}
                onCheckedChange={field.onChange}
              />
            </FormControl>
          </FormItem>
        )}
      />
      {type === 1 && (
        <FormField
          control={props.form.control}
          name='model_endpoints'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Model endpoints')}</FormLabel>
              <FormDescription>
                {t(
                  'Apply to this channel only. Responses converts chat requests automatically. Leave automatic to use existing routing.'
                )}
              </FormDescription>
              <div className='max-h-64 space-y-3 overflow-y-auto'>
                {names.map((model) => (
                  <div
                    key={model}
                    className='flex flex-col gap-2 sm:flex-row sm:items-center'
                  >
                    <span className='min-w-0 flex-1 break-all'>{model}</span>
                    <Select
                      value={field.value?.[model] || 'auto'}
                      onValueChange={(value) => {
                        if (!value) return
                        const endpoints = { ...field.value }
                        if (value === 'auto') delete endpoints[model]
                        else if (
                          value === 'openai' ||
                          value === 'openai-response'
                        )
                          endpoints[model] = value
                        field.onChange(endpoints)
                      }}
                    >
                      <SelectTrigger
                        className='w-full sm:w-60'
                        aria-label={`${t('Endpoint Type')}: ${model}`}
                      >
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value='auto'>
                          {t('Auto-detect (Default)')}
                        </SelectItem>
                        <SelectItem value='openai'>Chat Completions</SelectItem>
                        <SelectItem value='openai-response'>
                          Responses
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                ))}
              </div>
            </FormItem>
          )}
        />
      )}
    </div>
  )
}
