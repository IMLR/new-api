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
import { useQuery } from '@tanstack/react-query'
import { useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { TagInput } from '@/components/tag-input'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Field,
  FieldGroup,
  FieldLabel,
  FieldDescription,
  FieldError,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'

import { previewModelMatching } from '../api'
import {
  channelModelSelectionKey,
  updateChannelModelSelection,
} from '../lib/model-matching'
import type { ChannelModelSelection, ModelMatchRule } from '../types'

interface ModelMatchingSectionProps {
  open: boolean
  rule: ModelMatchRule
  onRuleChange: (rule: ModelMatchRule) => void
  selections: ChannelModelSelection[]
  onSelectionsChange: (selections: ChannelModelSelection[]) => void
  showErrors: boolean
}

export function ModelMatchingSection(props: ModelMatchingSectionProps) {
  const { t } = useTranslation()
  const id = useId()
  const [search, setSearch] = useState('')
  const preview = useQuery({
    queryKey: ['models', 'match-preview', props.rule],
    queryFn: async ({ signal }) => {
      const response = await previewModelMatching(props.rule, signal)
      if (!response.success) {
        throw new Error(
          response.message || t('Failed to preview matching models')
        )
      }
      return response.data ?? []
    },
    enabled: props.open && props.rule.include.length > 0,
    retry: false,
    staleTime: 0,
  })
  const selectionMap = useMemo(
    () =>
      new Map(
        props.selections.map((selection) => [
          channelModelSelectionKey(selection.channel_id, selection.model),
          selection.selected,
        ])
      ),
    [props.selections]
  )
  const models = preview.data ?? []
  const enabledChannelIds = new Set<number>()
  let enabledPairs = 0
  for (const model of models) {
    for (const channel of model.channels) {
      const selected =
        selectionMap.get(channelModelSelectionKey(channel.id, model.model)) ??
        channel.selected
      if (selected && channel.enabled) {
        enabledChannelIds.add(channel.id)
        enabledPairs++
      }
    }
  }
  const searchText = search.trim().toLowerCase()
  const visibleModels = models.filter(
    (model) =>
      model.model.toLowerCase().includes(searchText) ||
      model.channels.some((channel) =>
        channel.name.toLowerCase().includes(searchText)
      )
  )
  let includeError: string | undefined
  let excludeError: string | undefined
  if (props.showErrors) {
    if (props.rule.include.length === 0) {
      includeError = t('Add at least one included text.')
    } else if (props.rule.include.length > 32) {
      includeError = t('Use at most 32 items per field.')
    } else if (
      props.rule.include.some(
        (text) => new TextEncoder().encode(text).length > 128
      )
    ) {
      includeError = t('Each text must be 128 bytes or fewer.')
    }
    if (props.rule.exclude.length > 32) {
      excludeError = t('Use at most 32 items per field.')
    } else if (
      props.rule.exclude.some(
        (text) => new TextEncoder().encode(text).length > 128
      )
    ) {
      excludeError = t('Each text must be 128 bytes or fewer.')
    }
  }

  return (
    <FieldGroup>
      <Field data-invalid={Boolean(includeError)}>
        <FieldLabel htmlFor={`${id}-include`}>
          {t('Must contain all')}
        </FieldLabel>
        <TagInput
          id={`${id}-include`}
          aria-invalid={Boolean(includeError)}
          value={props.rule.include}
          onChange={(include) => props.onRuleChange({ ...props.rule, include })}
          placeholder={t('Type text and press Enter, e.g. claude, sonnet')}
        />
        <FieldDescription>
          {t(
            'Model names must contain every item. Press Enter to add each item.'
          )}
        </FieldDescription>
        {includeError && <FieldError>{includeError}</FieldError>}
      </Field>
      <Field data-invalid={Boolean(excludeError)}>
        <FieldLabel htmlFor={`${id}-exclude`}>
          {t('Must not contain any')}
        </FieldLabel>
        <TagInput
          id={`${id}-exclude`}
          aria-invalid={Boolean(excludeError)}
          value={props.rule.exclude}
          onChange={(exclude) => props.onRuleChange({ ...props.rule, exclude })}
          placeholder={t('Optional, e.g. preview, thinking')}
        />
        <FieldDescription>
          {t('Exclude a model when its name contains any item.')}
        </FieldDescription>
        {excludeError && <FieldError>{excludeError}</FieldError>}
      </Field>
      <Field orientation='horizontal'>
        <FieldLabel htmlFor={`${id}-case`}>{t('Case sensitive')}</FieldLabel>
        <Switch
          id={`${id}-case`}
          checked={props.rule.case_sensitive}
          onCheckedChange={(case_sensitive) =>
            props.onRuleChange({ ...props.rule, case_sensitive })
          }
        />
      </Field>
      <div
        className='space-y-3 rounded-lg border p-3'
        aria-busy={preview.isFetching}
      >
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <h4 className='text-sm font-medium'>
            {t('Matching models and channels')}
          </h4>
          {props.rule.include.length > 0 && preview.data && (
            <span
              className='text-muted-foreground text-xs tabular-nums'
              aria-live='polite'
            >
              {t('{{models}} models · {{channels}} channels available', {
                models: models.length,
                channels: enabledChannelIds.size,
              })}
            </span>
          )}
        </div>
        <p className='text-muted-foreground text-xs leading-relaxed'>
          {t(
            'Uncheck to stop using this model on this channel in all groups. Save to apply; check again to restore.'
          )}
        </p>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Routing also depends on the group, endpoint and channel priority.'
          )}
        </p>
        {props.rule.include.length === 0 && (
          <p className='text-muted-foreground py-4 text-center text-sm'>
            {t('Add included text to preview matching models.')}
          </p>
        )}
        {props.rule.include.length > 0 && preview.isPending && (
          <p
            className='text-muted-foreground py-4 text-center text-sm'
            role='status'
          >
            {t('Loading...')}
          </p>
        )}
        {props.rule.include.length > 0 && preview.isError && (
          <div className='space-y-2' role='alert'>
            <p className='text-destructive text-sm'>
              {preview.error.message || t('Failed to preview matching models')}
            </p>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => preview.refetch()}
            >
              {t('Retry')}
            </Button>
          </div>
        )}
        {props.rule.include.length > 0 &&
          !preview.isPending &&
          !preview.isError &&
          models.length === 0 && (
            <p className='text-muted-foreground py-4 text-center text-sm'>
              {t('No channel models match these filters.')}
            </p>
          )}
        {props.rule.include.length > 0 && models.length > 0 && (
          <>
            <Input
              aria-label={t('Search matching models or channels')}
              placeholder={t('Search matching models or channels')}
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
            <div className='max-h-80 space-y-3 overflow-y-auto overscroll-contain pr-1'>
              {visibleModels.map((model) => (
                <div key={model.model} className='space-y-2'>
                  <p className='font-mono text-xs font-medium break-all'>
                    {model.model}
                  </p>
                  <div className='space-y-1'>
                    {model.channels.map((channel) => {
                      const key = channelModelSelectionKey(
                        channel.id,
                        model.model
                      )
                      const selected = selectionMap.get(key) ?? channel.selected
                      const checkboxId = `${id}-${channel.id}-${encodeURIComponent(model.model)}`
                      return (
                        <label
                          key={channel.id}
                          htmlFor={checkboxId}
                          className={cn(
                            'hover:bg-muted/50 flex cursor-pointer items-start gap-3 rounded-md border px-3 py-2 transition-colors',
                            selected && channel.enabled
                              ? 'border-border'
                              : 'border-dashed opacity-70'
                          )}
                        >
                          <Checkbox
                            id={checkboxId}
                            className='mt-0.5'
                            checked={selected}
                            aria-labelledby={`${checkboxId}-label`}
                            onCheckedChange={(checked) =>
                              props.onSelectionsChange(
                                updateChannelModelSelection(
                                  props.selections,
                                  {
                                    channel_id: channel.id,
                                    model: model.model,
                                    selected: checked,
                                  },
                                  channel.selected
                                )
                              )
                            }
                          />
                          <span id={`${checkboxId}-label`} className='sr-only'>
                            {t('Use {{model}} on {{channel}}', {
                              model: model.model,
                              channel: channel.name,
                            })}
                          </span>
                          <div className='min-w-0 flex-1'>
                            <div className='flex flex-wrap items-center gap-2'>
                              <span className='text-sm break-all'>
                                {channel.name}
                              </span>
                              <span className='text-muted-foreground text-xs'>
                                #{channel.id}
                              </span>
                              {!channel.enabled && (
                                <Badge variant='secondary'>
                                  {t('Channel disabled')}
                                </Badge>
                              )}
                              {!selected && (
                                <Badge variant='outline'>{t('Excluded')}</Badge>
                              )}
                            </div>
                            <p className='text-muted-foreground mt-0.5 text-xs break-all'>
                              {channel.groups.join(', ')}
                            </p>
                          </div>
                        </label>
                      )
                    })}
                  </div>
                </div>
              ))}
              {visibleModels.length === 0 && (
                <p className='text-muted-foreground text-sm'>
                  {t('No matching results')}
                </p>
              )}
            </div>
            {enabledPairs === 0 && (
              <p className='text-destructive text-xs'>
                {t(
                  'No channels selected. These models cannot be routed after saving.'
                )}
              </p>
            )}
          </>
        )}
        {props.selections.length > 0 && (
          <p className='text-muted-foreground text-xs' aria-live='polite'>
            {t('{{count}} channel model changes will be applied on save.', {
              count: props.selections.length,
            })}
          </p>
        )}
      </div>
    </FieldGroup>
  )
}
