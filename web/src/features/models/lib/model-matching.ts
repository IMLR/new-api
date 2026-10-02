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
import { z } from 'zod'

import type { ChannelModelSelection, Model } from '../types'

const matchTextSchema = z
  .string()
  .min(1)
  .refine((text) => new TextEncoder().encode(text).length <= 128)

export const modelMatchRuleSchema = z.object({
  include: z.array(matchTextSchema).min(1).max(32),
  exclude: z.array(matchTextSchema).max(32),
  case_sensitive: z.boolean(),
})

export const channelModelSelectionSchema = z.object({
  channel_id: z.number().int().positive(),
  model: z.string().min(1),
  selected: z.boolean(),
})

export function getModelMatchRule(
  model?: Pick<Model, 'model_name' | 'match_rule'> | null
) {
  return (
    model?.match_rule ?? {
      include: model?.model_name ? [model.model_name] : [],
      exclude: [],
      case_sensitive: false,
    }
  )
}

export function channelModelSelectionKey(channelId: number, model: string) {
  return `${channelId}:${model}`
}

export function updateChannelModelSelection(
  selections: ChannelModelSelection[],
  selection: ChannelModelSelection,
  savedSelected: boolean
) {
  const next = selections.filter(
    (item) =>
      item.channel_id !== selection.channel_id || item.model !== selection.model
  )
  if (selection.selected !== savedSelected) next.push(selection)
  return next
}
