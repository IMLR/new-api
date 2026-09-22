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
  Check,
  Copy,
  ExternalLink,
  Link2,
  Loader2,
  RefreshCw,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'

import {
  completeWorkBuddyOAuth,
  startWorkBuddyOAuth,
  type WorkBuddyOAuthCompleteResponse,
} from '../../api'

type WorkBuddyOAuthDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  proxy?: string
  onAuthorized: (
    credential: string,
    account: NonNullable<
      NonNullable<WorkBuddyOAuthCompleteResponse['data']>['account']
    >
  ) => void
}

const POLL_ATTEMPTS = 6
const POLL_INTERVAL_MS = 2000

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => {
    window.setTimeout(resolve, ms)
  })
}

export function WorkBuddyOAuthDialog({
  open,
  onOpenChange,
  proxy,
  onAuthorized,
}: WorkBuddyOAuthDialogProps) {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })
  const [realm, setRealm] = useState<'cn' | 'global'>('cn')
  const [flowId, setFlowId] = useState('')
  const [authorizeUrl, setAuthorizeUrl] = useState('')
  const [expiresAt, setExpiresAt] = useState('')
  const [isStarting, setIsStarting] = useState(false)
  const [isConfirming, setIsConfirming] = useState(false)
  const [hint, setHint] = useState('')

  useEffect(() => {
    if (!open) {
      setRealm('cn')
      setFlowId('')
      setAuthorizeUrl('')
      setExpiresAt('')
      setIsStarting(false)
      setIsConfirming(false)
      setHint('')
    }
  }, [open])

  const handleStart = async () => {
    setIsStarting(true)
    setHint('')
    try {
      const response = await startWorkBuddyOAuth(realm, proxy?.trim())
      if (
        !response.success ||
        !response.data?.flow_id ||
        !response.data.authorize_url
      ) {
        throw new Error(
          response.message || t('Failed to start WorkBuddy sign-in')
        )
      }
      setFlowId(response.data.flow_id)
      setAuthorizeUrl(response.data.authorize_url)
      setExpiresAt(response.data.expires_at)
      window.open(response.data.authorize_url, '_blank', 'noopener,noreferrer')
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to start WorkBuddy sign-in')
      )
    } finally {
      setIsStarting(false)
    }
  }

  const handleConfirm = async () => {
    if (!flowId) return
    setIsConfirming(true)
    setHint('')
    try {
      for (let attempt = 0; attempt < POLL_ATTEMPTS; attempt += 1) {
        const response = await completeWorkBuddyOAuth(flowId)
        if (!response.success) {
          throw new Error(
            response.message || t('Failed to start WorkBuddy sign-in')
          )
        }
        if (response.data?.status === 'ready' && response.data.credential) {
          toast.success(
            t('Signed in as {{account}}', {
              account:
                response.data.account?.nickname ||
                response.data.account?.uid ||
                t('WorkBuddy'),
            })
          )
          onAuthorized(response.data.credential, response.data.account ?? {})
          onOpenChange(false)
          return
        }
        if (attempt < POLL_ATTEMPTS - 1) {
          await delay(POLL_INTERVAL_MS)
        }
      }
      setHint(
        t(
          'Sign-in is not finished yet. Complete it in the browser and confirm again.'
        )
      )
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to start WorkBuddy sign-in')
      )
    } finally {
      setIsConfirming(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('WorkBuddy sign-in')}
      description={t('Sign in with your CodeBuddy account.')}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <div className='flex w-full justify-end gap-2'>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('Close')}
          </Button>
        </div>
      }
    >
      <FieldGroup>
        <Field>
          <FieldLabel>{t('Region')}</FieldLabel>
          <div className='flex flex-wrap gap-2'>
            <Button
              type='button'
              size='sm'
              variant={realm === 'cn' ? 'default' : 'outline'}
              disabled={isStarting || Boolean(flowId)}
              onClick={() => setRealm('cn')}
            >
              {t('China')}
            </Button>
            <Button
              type='button'
              size='sm'
              variant={realm === 'global' ? 'default' : 'outline'}
              disabled={isStarting || Boolean(flowId)}
              onClick={() => setRealm('global')}
            >
              {t('International')}
            </Button>
          </div>
          <FieldDescription>
            {t('Pick the deployment of the account you sign in with.')}
          </FieldDescription>
        </Field>

        {!flowId ? (
          <Button
            type='button'
            onClick={() => void handleStart()}
            disabled={isStarting}
          >
            {isStarting ? (
              <Loader2 className='mr-2 h-4 w-4 animate-spin' />
            ) : (
              <Link2 className='mr-2 h-4 w-4' />
            )}
            {t('Get sign-in link')}
          </Button>
        ) : (
          <div className='space-y-3'>
            <Field>
              <FieldLabel>{t('Sign-in link')}</FieldLabel>
              <div className='flex items-center gap-2'>
                <Input readOnly value={authorizeUrl} className='font-mono' />
                <Button
                  type='button'
                  variant='outline'
                  size='icon'
                  onClick={() => void copyToClipboard(authorizeUrl)}
                >
                  {copiedText === authorizeUrl ? (
                    <Check className='h-4 w-4' />
                  ) : (
                    <Copy className='h-4 w-4' />
                  )}
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  size='icon'
                  onClick={() =>
                    window.open(authorizeUrl, '_blank', 'noopener,noreferrer')
                  }
                >
                  <ExternalLink className='h-4 w-4' />
                </Button>
              </div>
              <FieldDescription>
                {expiresAt
                  ? t('Link expires at {{time}}', {
                      time: new Date(expiresAt).toLocaleTimeString(),
                    })
                  : t('Open the link, sign in, then confirm here.')}
              </FieldDescription>
            </Field>

            <div className='flex flex-wrap items-center gap-2'>
              <Button
                type='button'
                onClick={() => void handleConfirm()}
                disabled={isConfirming}
              >
                {isConfirming ? (
                  <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                ) : (
                  <Check className='mr-2 h-4 w-4' />
                )}
                {isConfirming
                  ? t('Waiting for sign-in...')
                  : t('I have finished signing in')}
              </Button>
              <Button
                type='button'
                variant='outline'
                onClick={() => void handleStart()}
                disabled={isStarting || isConfirming}
              >
                <RefreshCw className='mr-2 h-4 w-4' />
                {t('Get sign-in link')}
              </Button>
            </div>
          </div>
        )}
      </FieldGroup>

      {hint ? (
        <Alert>
          <AlertDescription>{hint}</AlertDescription>
        </Alert>
      ) : null}
    </Dialog>
  )
}
