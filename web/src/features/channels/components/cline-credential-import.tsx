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
import { useTranslation } from 'react-i18next'

import { Input } from '@/components/ui/input'

import { normalizeClineCredential } from '../lib/cline-credential'

export function ClineCredentialImport({
  onImport,
  disabled,
}: {
  onImport: (value: string) => void
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const [error, setError] = useState(false)
  return (
    <div className='space-y-2'>
      <Input
        type='file'
        accept='.json,application/json'
        aria-label={t('Import Cline credential')}
        aria-invalid={error}
        disabled={disabled}
        onChange={async (event) => {
          const file = event.currentTarget.files?.[0]
          event.currentTarget.value = ''
          if (!file) return
          try {
            if (file.size > 1024 * 1024) throw new Error('File too large')
            onImport(normalizeClineCredential(await file.text()))
            setError(false)
          } catch {
            setError(true)
          }
        }}
      />
      {error && (
        <p role='alert' className='text-destructive text-xs'>
          {t('Invalid Cline credential')}
        </p>
      )}
      <p className='text-muted-foreground text-xs'>
        {t(
          'Import a Cline credential or Desktop providers.json. Tokens renew automatically. Model discovery includes free models and active ClinePass models.'
        )}
      </p>
    </div>
  )
}
