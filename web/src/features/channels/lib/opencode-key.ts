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
const KEY_FIELDS = [
  'key',
  'apiKey',
  'api_key',
  'OPENCODE_API_KEY',
  'opencodeApiKey',
]

function readKeyField(value: unknown): string {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return ''
  }
  for (const field of KEY_FIELDS) {
    const candidate = (value as Record<string, unknown>)[field]
    if (typeof candidate === 'string' && candidate.trim()) {
      return candidate.trim()
    }
  }
  return ''
}

/**
 * Normalize one OpenCode Go key. A plain key passes through; the opencode-go
 * entry of the OpenCode credential file is unwrapped so it can be pasted as is.
 */
export function normalizeOpenCodeGoKey(value: string): string {
  const trimmed = value.trim()
  if (!trimmed) {
    throw new Error('OpenCode Go API key is empty')
  }
  if (!trimmed.startsWith('{')) {
    return trimmed
  }
  let data: unknown
  try {
    data = JSON.parse(trimmed)
  } catch {
    throw new Error('Invalid OpenCode Go key JSON')
  }
  if (!data || typeof data !== 'object' || Array.isArray(data)) {
    throw new Error('Invalid OpenCode Go key JSON')
  }
  const record = data as Record<string, unknown>
  const nested = record['opencode-go']
  const nestedKey = readKeyField(nested)
  if (nestedKey) {
    return nestedKey
  }
  const key = readKeyField(record)
  if (key) {
    return key
  }
  if ('opencode' in record) {
    throw new Error('OpenCode Zen credential is not an OpenCode Go key')
  }
  throw new Error('OpenCode Go key JSON must contain key or apiKey')
}
