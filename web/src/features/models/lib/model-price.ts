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
/**
 * Pricing for one model name, read back from the system option maps.
 *
 * The backend stores pricing as ratios keyed by model name; a ratio of 1
 * equals 2 USD per 1M tokens for input, which is what the interface shows.
 */
export type ModelPriceInfo =
  | { kind: 'per-request'; price: number }
  | { kind: 'per-token'; input: number; output: number | null }

export type ModelPriceIndex = Map<string, ModelPriceInfo>

type RawOption = { key: string; value: string }

function readNumberMap(options: Map<string, string>, key: string) {
  const raw = options.get(key) ?? ''
  if (!raw.trim()) return {} as Record<string, number>
  try {
    const parsed = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return {} as Record<string, number>
    }
    return parsed as Record<string, number>
  } catch {
    return {} as Record<string, number>
  }
}

// Ratios are written back into the option maps, so float drift from the price
// division has to be trimmed before it lands in storage.
function trimFloat(value: number): string {
  return Number(value.toFixed(10)).toString()
}

export function buildModelPriceIndex(
  options: RawOption[] | undefined
): ModelPriceIndex {
  const index: ModelPriceIndex = new Map()
  if (!options) return index

  const raw = new Map(options.map((option) => [option.key, option.value]))
  const priceMap = readNumberMap(raw, 'ModelPrice')
  const ratioMap = readNumberMap(raw, 'ModelRatio')
  const completionMap = readNumberMap(raw, 'CompletionRatio')

  Object.entries(ratioMap).forEach(([name, ratio]) => {
    if (!Number.isFinite(ratio)) return
    const input = ratio * 2
    const completion = completionMap[name]
    index.set(name, {
      kind: 'per-token',
      input,
      output: Number.isFinite(completion) ? input * completion : null,
    })
  })

  // A fixed per-request price wins outright at billing time, so it replaces
  // any ratio entries that also exist for the same name.
  Object.entries(priceMap).forEach(([name, price]) => {
    if (!Number.isFinite(price)) return
    index.set(name, { kind: 'per-request', price })
  })

  return index
}

export function formatPriceUsd(value: number): string {
  const rounded = Number(value.toFixed(6))
  return `$${rounded}`
}

export type PricingPriceInputs = {
  prompt: string
  completion: string
  cache: string
  createCache: string
}

export type PricingRatioFields = {
  ratio: string
  completionRatio: string
  cacheRatio: string
  createCacheRatio: string
}

/**
 * Convert the USD prices shown in the model editor into the ratios the backend
 * stores. A ratio of 1 equals 2 USD per 1M input tokens, and every other lane
 * is a multiplier relative to the input price. A zero input price is a valid
 * free model: its ratio is written as 0 while the other lanes stay empty
 * because they cannot be derived by division.
 */
export function buildPricingRatios(
  inputs: PricingPriceInputs
): PricingRatioFields {
  const inputPrice = Number.parseFloat(inputs.prompt)
  const hasInputPrice =
    inputs.prompt !== '' && !Number.isNaN(inputPrice) && inputPrice >= 0

  const laneRatio = (value: string) => {
    if (!hasInputPrice || inputPrice === 0 || value === '') return ''
    const price = Number.parseFloat(value)
    if (Number.isNaN(price)) return ''
    return trimFloat(price / inputPrice)
  }

  return {
    ratio: hasInputPrice ? trimFloat(inputPrice / 2) : '',
    completionRatio: laneRatio(inputs.completion),
    cacheRatio: laneRatio(inputs.cache),
    createCacheRatio: laneRatio(inputs.createCache),
  }
}
