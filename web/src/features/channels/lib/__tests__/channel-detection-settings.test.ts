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

import type { Channel } from '../../types'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  transformFormDataToCreatePayload,
  transformChannelToFormDefaults,
} from '../channel-form'

test('saved channel detection and endpoint preferences survive reopening', () => {
  const values = {
    ...CHANNEL_FORM_DEFAULT_VALUES,
    type: 1,
    models: 'gpt-6-astra,gpt-5.6-sol',
    relay_detection: true,
    model_endpoints: { 'gpt-6-astra': 'openai-response' as const },
  }
  const payload = transformFormDataToCreatePayload(values).channel
  const restored = transformChannelToFormDefaults({
    id: 11,
    ...payload,
  } as Channel)
  assert.equal(restored.relay_detection, true)
  assert.deepEqual(restored.model_endpoints, values.model_endpoints)
})
test('removing a model drops its endpoint and changing provider clears incompatible endpoints', () => {
  const values = {
    ...CHANNEL_FORM_DEFAULT_VALUES,
    type: 1,
    models: 'remaining',
    model_endpoints: { removed: 'openai-response' as const },
  }
  assert.deepEqual(
    JSON.parse(transformFormDataToCreatePayload(values).channel.setting || '{}')
      .model_endpoints,
    {}
  )
  assert.deepEqual(
    JSON.parse(
      transformFormDataToCreatePayload({
        ...values,
        type: 14,
        models: 'removed',
      }).channel.setting || '{}'
    ).model_endpoints,
    {}
  )
})
