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
import { AxiosError } from 'axios'
import i18next from 'i18next'
import { toast } from 'sonner'

import { getServerErrorMessageKey } from '@/lib/server-error-message'

export function getServerErrorMessage(error: unknown): string {
  const messageKey = getServerErrorMessageKey(error)
  if (messageKey) return i18next.t(messageKey)

  if (
    error &&
    typeof error === 'object' &&
    'status' in error &&
    Number(error.status) === 204
  ) {
    return i18next.t('Content not found.')
  }

  if (error instanceof AxiosError) {
    const payload = error.response?.data
    for (const value of [payload?.message, payload?.title, error.message]) {
      if (typeof value === 'string' && value.trim()) return value
    }
  } else if (error instanceof Error && error.message.trim()) {
    return error.message
  }
  return i18next.t('Something went wrong!')
}

export function handleServerError(error: unknown) {
  toast.error(getServerErrorMessage(error))
}
