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

import { AxiosError, AxiosHeaders } from 'axios'
import i18next from 'i18next'

import { getServerErrorMessage } from './handle-server-error'

await i18next.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  fallbackLng: 'en',
})

test('API message is shown when the response has no title', () => {
  const error = new AxiosError('Request failed with status code 409')
  error.response = {
    data: { message: 'A fingerprint test is already running on this channel' },
    status: 409,
    statusText: 'Conflict',
    headers: {},
    config: { headers: new AxiosHeaders() },
  }
  assert.equal(
    getServerErrorMessage(error),
    'A fingerprint test is already running on this channel'
  )
  error.response.data = { title: 'Legacy error' }
  assert.equal(getServerErrorMessage(error), 'Legacy error')
  error.response.data = { message: ' ', title: {} }
  assert.equal(
    getServerErrorMessage(error),
    'Request failed with status code 409'
  )
})

test('empty errors always have a nonempty fallback', () => {
  assert.equal(
    getServerErrorMessage(new AxiosError('')),
    'Something went wrong!'
  )
  assert.equal(getServerErrorMessage(null), 'Something went wrong!')
})
