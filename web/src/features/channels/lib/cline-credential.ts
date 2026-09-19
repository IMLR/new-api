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
/** Read a Cline export without retaining unrelated Desktop provider credentials. */
export function normalizeClineCredential(value: string): string {
  let data = JSON.parse(value)
  if (!data || typeof data !== 'object' || Array.isArray(data)) {
    throw new Error('Invalid Cline credential')
  }
  if (data.providers) data = data.providers?.cline?.settings?.auth
  else if (data.auth) data = data.auth
  const refreshToken = data?.refreshToken ?? data?.refresh_token
  const accessToken = data?.accessToken ?? data?.access_token ?? ''
  if (
    typeof refreshToken !== 'string' ||
    !refreshToken.trim() ||
    typeof accessToken !== 'string'
  ) {
    throw new Error('Invalid Cline credential')
  }
  return JSON.stringify(
    {
      accessToken: accessToken.trim(),
      refreshToken: refreshToken.trim(),
      expiresAt: data?.expiresAt ?? data?.expires_at,
    },
    null,
    2
  )
}
