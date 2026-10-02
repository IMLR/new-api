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
import { test } from 'node:test'

import { updateChannelModelSelection } from '../model-matching'

test('unchecking one channel/model preserves selections for other models and channels', () => {
  const selections = [
    { channel_id: 2, model: 'claude-sonnet', selected: false },
  ]
  const next = updateChannelModelSelection(
    selections,
    { channel_id: 1, model: 'claude-sonnet', selected: false },
    true
  )
  assert.deepEqual(next, [
    { channel_id: 2, model: 'claude-sonnet', selected: false },
    { channel_id: 1, model: 'claude-sonnet', selected: false },
  ])
  assert.deepEqual(selections, [
    { channel_id: 2, model: 'claude-sonnet', selected: false },
  ])
})

test('checking a saved exclusion adds a restore operation and undoing it removes the draft', () => {
  const restored = updateChannelModelSelection(
    [],
    { channel_id: 1, model: 'claude-sonnet', selected: true },
    false
  )
  assert.deepEqual(restored, [
    { channel_id: 1, model: 'claude-sonnet', selected: true },
  ])
  const undone = updateChannelModelSelection(
    restored,
    { channel_id: 1, model: 'claude-sonnet', selected: false },
    false
  )
  assert.deepEqual(undone, [])
})
