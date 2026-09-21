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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import dayjs from '@/lib/dayjs'
import { formatDateTimeStr } from '@/lib/format'
import { cn } from '@/lib/utils'

import {
  probeClineQuota,
  type ClineQuotaModel,
  type ClineQuotaResponse,
} from '../../api'
import { formatTimeLeftUntil } from '../../lib/usage-format'

export type ClineQuotaDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelName?: string
  channelId?: number
  channelDisplayName?: string
  channelDisplayId?: string
  response: ClineQuotaResponse | null
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

export function ClineQuotaDialog({
  open,
  onOpenChange,
  channelName,
  channelId,
  channelDisplayName,
  channelDisplayId,
  response,
  onRefresh,
  isRefreshing,
}: ClineQuotaDialogProps) {
  const { t } = useTranslation()
  const [probingModel, setProbingModel] = useState('')
  const [probedModels, setProbedModels] = useState<
    Record<string, ClineQuotaModel>
  >({})

  const data = response?.data ?? null
  const errorMessage =
    response && response.success === false
      ? response.message?.trim() || t('Failed to fetch usage')
      : ''
  const models = (data?.models ?? []).map((entry) => {
    const probed = entry.model ? probedModels[entry.model] : undefined
    return probed ? { ...entry, ...probed } : entry
  })
  const planActive = Boolean(data?.plan?.active)
  const planName =
    data?.plan?.display_name?.trim() ||
    data?.plan?.name?.trim() ||
    data?.plan?.type?.trim() ||
    (planActive ? t('ClinePass subscription') : t('No subscription'))
  const credentialExpiresAt = data?.credential?.expires_at
  const checkedAt = data?.checked_at

  let channelLabel = channelDisplayName ?? channelName ?? '-'
  if (channelDisplayId != null) {
    channelLabel = `${channelLabel} (#${channelDisplayId})`
  } else if (channelId) {
    channelLabel = `${channelLabel} (#${channelId})`
  }

  const handleProbe = async (model: string) => {
    if (!channelId || probingModel) {
      return
    }
    setProbingModel(model)
    try {
      const res = await probeClineQuota(channelId, model)
      if (!res.success || !res.data?.model) {
        throw new Error(res.message || t('Quota check failed'))
      }
      const probed = res.data.model
      setProbedModels((previous) => ({ ...previous, [model]: probed }))
      if (probed.cooling_down) {
        toast.warning(
          `${model}: ${t('Resets in')} ${formatTimeLeftUntil(probed.reset_at, t)}`
        )
      } else {
        toast.success(`${model}: ${t('Available')}`)
      }
      await onRefresh?.()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Quota check failed')
      )
    } finally {
      setProbingModel('')
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Cline Quota')}
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

      {data?.account_error ? (
        <Alert>
          <AlertTitle>{t('Account details unavailable')}</AlertTitle>
          <AlertDescription>{data.account_error}</AlertDescription>
        </Alert>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle className='text-sm'>{t('Account')}</CardTitle>
          <CardDescription>
            {t('Free quota windows for this account.')}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-2'>
          <InfoRow label={t('Email')} value={data?.account?.email || '-'} />
          <InfoRow label={t('Plan')} value={planName} />
          {data?.plan?.current_period_end ? (
            <InfoRow
              label={t('Valid until')}
              value={formatIsoTimestamp(data.plan.current_period_end)}
            />
          ) : null}
          <InfoRow
            label={t('Credential expiry')}
            value={
              credentialExpiresAt
                ? `${formatIsoTimestamp(credentialExpiresAt)} · ${formatTimeLeftUntil(credentialExpiresAt, t)}`
                : '-'
            }
          />
          <InfoRow
            label={t('Renews automatically')}
            value={data?.credential?.auto_refresh ? t('Yes') : t('No')}
          />
          {checkedAt ? (
            <InfoRow
              label={t('Last checked')}
              value={formatIsoTimestamp(checkedAt)}
            />
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className='text-sm'>{t('Model quota')}</CardTitle>
          <CardDescription>
            {t('Checking quota spends one free request for this model.')}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-2'>
          {models.length === 0 ? (
            <div className='text-muted-foreground text-sm'>
              {t('No models configured for this channel.')}
            </div>
          ) : (
            models.map((entry) => {
              const cooling = Boolean(entry.cooling_down)
              const probing = probingModel === entry.model
              return (
                <div
                  key={entry.model}
                  className='flex items-start justify-between gap-3 rounded-lg border p-3'
                >
                  <div className='min-w-0 space-y-1'>
                    <div className='flex flex-wrap items-center gap-2'>
                      <span className='text-sm font-medium'>{entry.model}</span>
                      <StatusBadge
                        label={cooling ? t('Cooling down') : t('Available')}
                        variant={cooling ? 'warning' : 'success'}
                        size='sm'
                        copyable={false}
                      />
                    </div>
                    {entry.upstream_model &&
                    entry.upstream_model !== entry.model ? (
                      <div className='text-muted-foreground truncate font-mono text-xs'>
                        {entry.upstream_model}
                      </div>
                    ) : null}
                    {cooling ? (
                      <div className='text-muted-foreground flex items-center gap-1 text-xs'>
                        <Timer className='h-3 w-3' />
                        {t('Resets at')} {formatIsoTimestamp(entry.reset_at)}
                        {' · '}
                        {t('Resets in')} {formatTimeLeftUntil(entry.reset_at, t)}
                      </div>
                    ) : null}
                    {entry.reason ? (
                      <div className='text-muted-foreground line-clamp-2 text-xs'>
                        {entry.reason}
                      </div>
                    ) : null}
                  </div>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={() => void handleProbe(entry.model)}
                    disabled={probingModel !== ''}
                  >
                    {probing ? (
                      <Loader2 className='mr-1 h-3.5 w-3.5 animate-spin' />
                    ) : null}
                    {probing ? t('Checking...') : t('Check quota')}
                  </Button>
                </div>
              )
            })
          )}
        </CardContent>
      </Card>
    </Dialog>
  )
}
