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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { normalizeClineCredential } from '../cline-credential'

describe('Cline credential import', () => {
  test('Desktop export imports only Cline and removes unrelated credentials', () => {
    const value = normalizeClineCredential(
      JSON.stringify({
        providers: {
          cline: {
            settings: {
              auth: {
                accessToken: 'a',
                refreshToken: 'r',
                expiresAt: 1800000000000,
              },
            },
          },
          other: { key: 'private' },
        },
      })
    )
    assert.deepEqual(JSON.parse(value), {
      accessToken: 'a',
      refreshToken: 'r',
      expiresAt: 1800000000000,
    })
  })
  test('snake case import preserves refresh-only credentials for automatic renewal', () => {
    assert.deepEqual(
      JSON.parse(normalizeClineCredential('{"refresh_token":"r"}')),
      { accessToken: '', refreshToken: 'r' }
    )
  })
  test('missing or invalid refresh credentials are rejected', () => {
    for (const input of [
      '',
      'null',
      '[]',
      '{"accessToken":"a"}',
      '{"refreshToken":42}',
      '{"providers":{}}',
    ]) {
      assert.throws(() => normalizeClineCredential(input))
    }
  })
})
