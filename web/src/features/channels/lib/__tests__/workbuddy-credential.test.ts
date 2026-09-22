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

import { normalizeWorkBuddyCredential } from '../workbuddy-credential'

describe('WorkBuddy credential import', () => {
  test('nested plugin export is flattened', () => {
    const value = normalizeWorkBuddyCredential(
      JSON.stringify({
        auth: {
          accessToken: 'at-1',
          refreshToken: 'rt-1',
          expiresAt: 1790000000,
          domain: 'www.workbuddy.ai',
          realm: 'global',
        },
        account: { uid: 'user-1', enterpriseId: 'ent-1', nickname: 'Alice' },
        device_token: 'dev-1',
      })
    )
    const parsed = JSON.parse(value)
    assert.equal(parsed.accessToken, 'at-1')
    assert.equal(parsed.refreshToken, 'rt-1')
    assert.equal(parsed.uid, 'user-1')
    assert.equal(parsed.enterpriseId, 'ent-1')
    assert.equal(parsed.device_token, 'dev-1')
  })

  test('flat credential keeps its fields', () => {
    const value = normalizeWorkBuddyCredential(
      JSON.stringify({
        accessToken: 'at-2',
        refreshToken: 'rt-2',
        uid: 'user-2',
        realm: 'cn',
      })
    )
    const parsed = JSON.parse(value)
    assert.equal(parsed.refreshToken, 'rt-2')
    assert.equal(parsed.realm, 'cn')
    assert.equal(parsed.domain, undefined)
  })

  test('credential without refresh token is rejected', () => {
    assert.throws(
      () =>
        normalizeWorkBuddyCredential(JSON.stringify({ accessToken: 'at-3' })),
      /refreshToken/
    )
  })

  test('non JSON input is rejected', () => {
    assert.throws(() => normalizeWorkBuddyCredential('sk-plain-key'))
  })
})
