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

import { buildModelPriceIndex, buildPricingRatios } from './model-price'

describe('model price index', () => {
  test('reads ratios back as prices per 1M tokens', () => {
    const index = buildModelPriceIndex([
      { key: 'ModelRatio', value: '{"claude-sonnet-5":1.5}' },
      { key: 'CompletionRatio', value: '{"claude-sonnet-5":5}' },
    ])

    assert.deepEqual(index.get('claude-sonnet-5'), {
      kind: 'per-token',
      input: 3,
      output: 15,
    })
  })

  test('leaves the output price empty when no completion ratio is set', () => {
    const index = buildModelPriceIndex([
      { key: 'ModelRatio', value: '{"glm-5.3-flash":0.1}' },
    ])

    assert.deepEqual(index.get('glm-5.3-flash'), {
      kind: 'per-token',
      input: 0.2,
      output: null,
    })
  })

  test('a fixed per-request price wins over ratio entries', () => {
    const index = buildModelPriceIndex([
      { key: 'ModelRatio', value: '{"mj-video":1}' },
      { key: 'ModelPrice', value: '{"mj-video":0.2}' },
    ])

    assert.deepEqual(index.get('mj-video'), {
      kind: 'per-request',
      price: 0.2,
    })
  })

  test('ignores malformed option values', () => {
    const index = buildModelPriceIndex([
      { key: 'ModelRatio', value: 'not json' },
      { key: 'ModelPrice', value: '' },
    ])

    assert.equal(index.size, 0)
  })
})

describe('model pricing ratios', () => {
  test('converts USD prices into the stored ratios', () => {
    assert.deepEqual(
      buildPricingRatios({
        prompt: '3',
        completion: '15',
        cache: '0.3',
        createCache: '3.75',
      }),
      {
        ratio: '1.5',
        completionRatio: '5',
        cacheRatio: '0.1',
        createCacheRatio: '1.25',
      }
    )
  })

  test('keeps a free model at ratio zero', () => {
    assert.deepEqual(
      buildPricingRatios({
        prompt: '0',
        completion: '',
        cache: '',
        createCache: '',
      }),
      {
        ratio: '0',
        completionRatio: '',
        cacheRatio: '',
        createCacheRatio: '',
      }
    )
  })

  test('clears every lane when the input price is emptied', () => {
    assert.deepEqual(
      buildPricingRatios({
        prompt: '',
        completion: '15',
        cache: '0.3',
        createCache: '3.75',
      }),
      {
        ratio: '',
        completionRatio: '',
        cacheRatio: '',
        createCacheRatio: '',
      }
    )
  })

  test('only clears the lane that was emptied', () => {
    assert.deepEqual(
      buildPricingRatios({
        prompt: '2',
        completion: '4',
        cache: '',
        createCache: '',
      }),
      {
        ratio: '1',
        completionRatio: '2',
        cacheRatio: '',
        createCacheRatio: '',
      }
    )
  })
})

