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
import { useTranslation } from 'react-i18next'

import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  getOptionValue,
  useSystemOptions,
} from '@/features/system-settings/hooks/use-system-options'
import { PasskeySection } from '@/features/system-settings/passkey-section'

const passkeyDefaults = {
  'passkey.enabled': false,
  'passkey.rp_display_name': 'New API',
  'passkey.rp_id': '',
  'passkey.origins': '',
  'passkey.allow_insecure_origin': false,
  'passkey.user_verification': 'preferred',
  'passkey.attachment_preference': 'none',
}

export function PasskeySettingsCard() {
  const { t } = useTranslation()
  const { data, isLoading } = useSystemOptions()

  const defaults = getOptionValue(data?.data, passkeyDefaults) as {
    'passkey.enabled': boolean
    'passkey.rp_display_name': string
    'passkey.rp_id': string
    'passkey.origins': string
    'passkey.allow_insecure_origin': boolean
    'passkey.user_verification': string
    'passkey.attachment_preference': '' | 'platform' | 'cross-platform'
  }

  return (
    <Card data-card-hover='false' className='gap-0 overflow-hidden py-0'>
      <CardHeader className='border-b p-3 !pb-3 sm:p-5 sm:!pb-5'>
        <span className='text-foreground text-sm font-medium'>
          {t('Passkey Authentication')}
        </span>
      </CardHeader>
      <CardContent className='p-3 sm:p-5'>
        {isLoading ? (
          <Skeleton className='h-40 w-full' />
        ) : (
          <PasskeySection
            defaultValues={{
              'passkey.enabled': defaults['passkey.enabled'],
              'passkey.rp_display_name':
                defaults['passkey.rp_display_name'],
              'passkey.rp_id': defaults['passkey.rp_id'],
              'passkey.origins': defaults['passkey.origins'],
              'passkey.allow_insecure_origin':
                defaults['passkey.allow_insecure_origin'],
              'passkey.user_verification': (defaults[
                'passkey.user_verification'
              ] as 'required' | 'preferred' | 'discouraged') ?? 'preferred',
              'passkey.attachment_preference':
                defaults['passkey.attachment_preference'],
            }}
          />
        )}
      </CardContent>
    </Card>
  )
}
