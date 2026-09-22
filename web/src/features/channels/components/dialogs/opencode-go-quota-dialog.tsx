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
import { Loader2, RefreshCw, Timer } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import dayjs from '@/lib/dayjs'
import { formatDateTimeStr } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { OpenCodeGoQuotaResponse } from '../../api'
import { formatDurationSeconds } from '../../lib/usage-format'

export type OpenCodeGoQuotaDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelName?: string
  channelId?: number
  channelDisplayName?: string
  channelDisplayId?: string
  response: OpenCodeGoQuotaResponse | null
  onRefresh?: () => void | Promise<void>
  isRefreshing?: boolean
}

function formatIsoTimestamp(value?: string): string {
  if (typeof value !== 'string' || value.trim() === '') {
    return '-'
  }
  const parsed = dayjs(value)
  if (!parsed.isValid()) {
    return value
  }
  return formatDateTimeStr(parsed.toDate())
}

function clampPercent(value: unknown): number {
  const percent = Number(value)
  if (!Number.isFinite(percent)) {
    return 0
  }
  return Math.min(100, Math.max(0, percent))
}

function formatPercent(value: unknown): string {
  const percent = clampPercent(value)
  return `${Math.round(percent * 100) / 100}%`
}

/** The upstream names the windows rolling/weekly/monthly; the label is copy. */
function formatWindowName(name: string, t: (key: string) => string): string {
  switch (name) {
    case 'rolling':
      return t('5-hour window')
    case 'weekly':
      return t('Weekly window')
    case 'monthly':
      return t('Monthly window')
    default:
      return name
  }
}

function InfoRow(props: { label: string; value: string; mono?: boolean }) {
  return (
    <div className='flex items-baseline justify-between gap-4 text-sm'>
      <span className='text-muted-foreground'>{props.label}</span>
      <span
        className={cn(
          'text-right break-all',
          props.mono && 'font-mono tabular-nums'
        )}
      >
        {props.value}
      </span>
    </div>
  )
}

export function OpenCodeGoQuotaDialog({
  open,
  onOpenChange,
  channelName,
  channelId,
  channelDisplayName,
  channelDisplayId,
  response,
  onRefresh,
  isRefreshing,
}: OpenCodeGoQuotaDialogProps) {
  const { t } = useTranslation()

  const data = response?.data ?? null
  const errorMessage =
    response && response.success === false
      ? response.message?.trim() || t('Failed to fetch usage')
      : ''
  const windows = data?.windows ?? []
  const models = data?.models ?? []

  let channelLabel = channelDisplayName ?? channelName ?? '-'
  if (channelDisplayId != null) {
    channelLabel = `${channelLabel} (#${channelDisplayId})`
  } else if (channelId) {
    channelLabel = `${channelLabel} (#${channelId})`
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('OpenCode Go Quota')}
      description={channelLabel}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <div className='flex w-full justify-end gap-2'>
          <Button
            variant='outline'
            onClick={() => void onRefresh?.()}
            disabled={isRefreshing}
          >
            {isRefreshing ? (
              <Loader2 className='mr-2 h-4 w-4 animate-spin' />
            ) : (
              <RefreshCw className='mr-2 h-4 w-4' />
            )}
            {isRefreshing ? t('Loading...') : t('Refresh')}
          </Button>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('Close')}
          </Button>
        </div>
      }
    >
      {errorMessage ? (
        <Alert variant='destructive'>
          <AlertTitle>{t('Failed to fetch usage')}</AlertTitle>
          <AlertDescription>{errorMessage}</AlertDescription>
        </Alert>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle className='text-sm'>{t('Subscription')}</CardTitle>
          <CardDescription>
            {t('Subscription windows for this account.')}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-4'>
          <div className='space-y-2'>
            <InfoRow
              label={t('API key')}
              value={data?.key_masked || '-'}
              mono
            />
            {data?.checked_at ? (
              <InfoRow
                label={t('Last checked')}
                value={formatIsoTimestamp(data.checked_at)}
              />
            ) : null}
          </div>

          {windows.length === 0 ? (
            <div className='text-muted-foreground text-sm'>
              {t('No usage windows reported.')}
            </div>
          ) : (
            <div className='space-y-4'>
              {windows.map((entry) => {
                const used = clampPercent(entry.used_percent)
                const remaining = clampPercent(entry.remaining_percent)
                return (
                  <div key={entry.name} className='space-y-2'>
                    <div className='flex items-baseline justify-between gap-3'>
                      <span className='text-sm font-medium'>
                        {formatWindowName(entry.name, t)}
                      </span>
                      <span className='font-mono text-xs tabular-nums'>
                        {formatPercent(used)} {t('Used')}
                      </span>
                    </div>
                    <Progress
                      value={used}
                      aria-label={`${formatWindowName(entry.name, t)}: ${formatPercent(used)}`}
                    />
                    <div className='text-muted-foreground flex flex-wrap items-center gap-x-3 gap-y-1 text-xs'>
                      <span>
                        {t('Remaining')}: {formatPercent(remaining)}
                      </span>
                      {entry.reset_in_seconds ? (
                        <span className='inline-flex items-center gap-1'>
                          <Timer className='h-3 w-3' />
                          {t('Resets in')}{' '}
                          {formatDurationSeconds(entry.reset_in_seconds, t)}
                        </span>
                      ) : null}
                      {entry.reset_at ? (
                        <span>
                          {t('Resets at')} {formatIsoTimestamp(entry.reset_at)}
                        </span>
                      ) : null}
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className='text-sm'>{t('Models')}</CardTitle>
          <CardDescription>
            {t('Endpoint that serves each model.')}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-2'>
          {models.length === 0 ? (
            <div className='text-muted-foreground text-sm'>
              {t('No models configured for this channel.')}
            </div>
          ) : (
            models.map((entry) => (
              <div
                key={entry.model}
                className='flex items-center justify-between gap-3 rounded-lg border p-3'
              >
                <span className='min-w-0 text-sm font-medium break-all'>
                  {entry.model}
                </span>
                <span className='text-muted-foreground font-mono text-xs'>
                  {entry.endpoint || '-'}
                </span>
              </div>
            ))
          )}
        </CardContent>
      </Card>
    </Dialog>
  )
}
