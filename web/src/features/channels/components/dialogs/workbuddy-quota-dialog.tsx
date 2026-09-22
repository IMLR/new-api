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
import {
  CheckCircle2,
  CircleDashed,
  Loader2,
  RefreshCw,
  Timer,
  XCircle,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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
import { Switch } from '@/components/ui/switch'
import dayjs from '@/lib/dayjs'
import { formatDateTimeStr } from '@/lib/format'
import { cn } from '@/lib/utils'

import {
  setWorkBuddyTask,
  type WorkBuddyQuotaResponse,
  type WorkBuddyTask,
} from '../../api'

export type WorkBuddyQuotaDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelName?: string
  channelId?: number
  channelDisplayName?: string
  channelDisplayId?: string
  response: WorkBuddyQuotaResponse | null
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

function formatAmount(value: unknown): string {
  const amount = Number(value)
  if (!Number.isFinite(amount)) {
    return '0'
  }
  return Math.round(amount).toLocaleString()
}

function usedPercent(used: number, size: number): number {
  if (!Number.isFinite(size) || size <= 0) {
    return 0
  }
  return Math.min(100, Math.max(0, (used / size) * 100))
}

function TaskStatusIcon(props: { status: WorkBuddyTask['status'] }) {
  switch (props.status) {
    case 'done':
      return <CheckCircle2 className='h-4 w-4 text-emerald-500' />
    case 'failed':
      return <XCircle className='h-4 w-4 text-red-500' />
    case 'skipped':
      return <CircleDashed className='h-4 w-4 text-amber-500' />
    default:
      return <CircleDashed className='text-muted-foreground h-4 w-4' />
  }
}

function taskStatusLabel(
  status: WorkBuddyTask['status'],
  t: (key: string) => string
): string {
  switch (status) {
    case 'done':
      return t('Done')
    case 'failed':
      return t('Failed')
    case 'skipped':
      return t('Skipped')
    default:
      return t('Not started')
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

export function WorkBuddyQuotaDialog({
  open,
  onOpenChange,
  channelName,
  channelId,
  channelDisplayName,
  channelDisplayId,
  response,
  onRefresh,
  isRefreshing,
}: WorkBuddyQuotaDialogProps) {
  const { t } = useTranslation()
  const [busyTask, setBusyTask] = useState('')

  const data = response?.data ?? null
  const errorMessage =
    response && response.success === false
      ? response.message?.trim() || t('Failed to fetch usage')
      : ''
  const packages = data?.packages ?? []
  const tasks = data?.tasks ?? []
  const remain = Number(data?.remain ?? 0)
  const used = Number(data?.used ?? 0)
  const size = Number(data?.size ?? 0)

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
      title={t('WorkBuddy Credits')}
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
          <CardTitle className='text-sm'>{t('Credits')}</CardTitle>
          <CardDescription>
            {t('Credit balance of this account.')}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-4'>
          <div className='space-y-2'>
            <InfoRow
              label={t('Account')}
              value={data?.account || data?.channel_name || '-'}
              mono
            />
            {data?.realm ? (
              <InfoRow
                label={t('Region')}
                value={
                  data.realm === 'global' ? t('International') : t('China')
                }
              />
            ) : null}
            {data?.checked_at ? (
              <InfoRow
                label={t('Last checked')}
                value={formatIsoTimestamp(data.checked_at)}
              />
            ) : null}
          </div>

          <div className='space-y-2'>
            <div className='flex items-baseline justify-between gap-3'>
              <span className='text-sm font-medium'>{t('Remaining')}</span>
              <span className='font-mono text-xs tabular-nums'>
                {formatAmount(remain)} / {formatAmount(size)}
              </span>
            </div>
            <Progress
              value={usedPercent(used, size)}
              aria-label={`${t('Credits')}: ${formatAmount(used)} / ${formatAmount(size)}`}
            />
            <div className='text-muted-foreground flex flex-wrap items-center gap-x-3 gap-y-1 text-xs'>
              <span>
                {t('Used')}: {formatAmount(used)}
              </span>
              <span>
                {t('Remaining')}: {formatAmount(remain)}
              </span>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className='text-sm'>{t('Account tasks')}</CardTitle>
          <CardDescription>
            {t(
              'Tasks that run on the account. Turn one off if it keeps failing.'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-2'>
          {tasks.length === 0 ? (
            <div className='text-muted-foreground text-sm'>
              {t('No tasks reported.')}
            </div>
          ) : (
            tasks.map((task) => (
              <div
                key={task.id}
                className='flex items-start justify-between gap-3 rounded-lg border p-3'
              >
                <div className='min-w-0 space-y-1'>
                  <div className='flex items-center gap-2'>
                    <TaskStatusIcon status={task.status} />
                    <span className='text-sm font-medium break-all'>
                      {task.title}
                    </span>
                    <span className='text-muted-foreground text-xs'>
                      {task.applicable === false
                        ? t('N/A')
                        : taskStatusLabel(task.status, t)}
                    </span>
                  </div>
                  {task.description ? (
                    <div className='text-muted-foreground text-xs'>
                      {task.description}
                    </div>
                  ) : null}
                  {task.message ? (
                    <div className='text-muted-foreground text-xs break-all'>
                      {task.message}
                    </div>
                  ) : null}
                  {task.hours?.length ? (
                    <div className='text-muted-foreground text-xs'>
                      {t('Runs at')}{' '}
                      {task.hours
                        .map((hour) => `${String(hour).padStart(2, '0')}:00`)
                        .join(', ')}
                    </div>
                  ) : null}
                </div>
                <Switch
                  checked={task.enabled}
                  disabled={
                    task.applicable === false ||
                    busyTask === task.id ||
                    !channelId
                  }
                  onCheckedChange={async (checked) => {
                    if (!channelId) return
                    setBusyTask(task.id)
                    try {
                      const res = await setWorkBuddyTask(
                        channelId,
                        task.id,
                        checked
                      )
                      if (!res.success) {
                        throw new Error(
                          res.message || t('Failed to update task')
                        )
                      }
                      toast.success(t('Task updated'))
                      await onRefresh?.()
                    } catch (error) {
                      toast.error(
                        error instanceof Error
                          ? error.message
                          : t('Failed to update task')
                      )
                    } finally {
                      setBusyTask('')
                    }
                  }}
                />
              </div>
            ))
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className='text-sm'>{t('Credit packages')}</CardTitle>
          <CardDescription>
            {t('Packages that make up the balance.')}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-2'>
          {packages.length === 0 ? (
            <div className='text-muted-foreground text-sm'>
              {t('No credit packages reported.')}
            </div>
          ) : (
            packages.map((entry) => (
              <div
                key={`${entry.name}-${entry.expires_at ?? ''}`}
                className='space-y-1 rounded-lg border p-3'
              >
                <div className='flex items-center justify-between gap-3'>
                  <span className='min-w-0 text-sm font-medium break-all'>
                    {entry.name || '-'}
                  </span>
                  <span className='font-mono text-xs tabular-nums'>
                    {formatAmount(entry.remain)} / {formatAmount(entry.size)}
                  </span>
                </div>
                {entry.expires_at ? (
                  <div className='text-muted-foreground flex items-center gap-1 text-xs'>
                    <Timer className='h-3 w-3' />
                    {entry.expires_soon
                      ? t('Expires soon')
                      : t('Expires at')}{' '}
                    {formatIsoTimestamp(entry.expires_at)}
                  </div>
                ) : null}
              </div>
            ))
          )}
        </CardContent>
      </Card>
    </Dialog>
  )
}
