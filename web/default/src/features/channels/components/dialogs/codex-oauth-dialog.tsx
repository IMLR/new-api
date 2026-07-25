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
  KeyRound,
  Loader2,
  Radio,
  ShieldCheck,
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
import { Textarea } from '@/components/ui/textarea'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'

import {
  completeCodexOAuth,
  startCodexOAuth,
  type CodexOAuthCompleteResponse,
} from '../../api'

type CodexOAuthDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelId?: number
  proxy?: string
  onAuthorized: (
    result: NonNullable<CodexOAuthCompleteResponse['data']>
  ) => void
}

export function CodexOAuthDialog({
  open,
  onOpenChange,
  channelId,
  proxy,
  onAuthorized,
}: CodexOAuthDialogProps) {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })
  const [flowId, setFlowId] = useState('')
  const [authorizeUrl, setAuthorizeUrl] = useState('')
  const [expiresAt, setExpiresAt] = useState('')
  const [callbackUrl, setCallbackUrl] = useState('')
  const [isStarting, setIsStarting] = useState(false)
  const [isCompleting, setIsCompleting] = useState(false)

  useEffect(() => {
    if (!open) {
      setFlowId('')
      setAuthorizeUrl('')
      setExpiresAt('')
      setCallbackUrl('')
      setIsStarting(false)
      setIsCompleting(false)
    }
  }, [open])

  const handleStart = async () => {
    setIsStarting(true)
    try {
      const response = await startCodexOAuth(channelId, proxy?.trim())
      if (
        !response.success ||
        !response.data?.flow_id ||
        !response.data.authorize_url
      ) {
        throw new Error(
          response.message || t('Failed to start Codex authorization')
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
          : t('Failed to start Codex authorization')
      )
    } finally {
      setIsStarting(false)
    }
  }

  const handleComplete = async () => {
    if (!flowId || !callbackUrl.trim()) return
    setIsCompleting(true)
    try {
      const response = await completeCodexOAuth(
        flowId,
        callbackUrl.trim(),
        channelId
      )
      if (!response.success || !response.data) {
        throw new Error(
          response.message || t('Failed to complete Codex authorization')
        )
      }
      onAuthorized(response.data)
      toast.success(t('Codex authorization completed'))
      onOpenChange(false)
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to complete Codex authorization')
      )
    } finally {
      setIsCompleting(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        <span className='flex items-center gap-2 text-slate-100'>
          <Radio className='size-4 text-cyan-400' />
          {t('Authorize Codex with ChatGPT')}
        </span>
      }
      description={t(
        'Sign in independently for this channel. No local Codex credential is read or shared.'
      )}
      contentClassName='border-cyan-400/20 bg-[#05070a] text-slate-100 shadow-[0_0_80px_rgba(34,211,238,0.08)] sm:max-w-xl'
      descriptionClassName='text-slate-400'
      bodyClassName='py-1'
      footerClassName='border-cyan-400/10 bg-[#080b10]'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={isCompleting}
            className='border-slate-700 bg-transparent text-slate-300 hover:bg-slate-900 hover:text-white'
          >
            {t('Cancel')}
          </Button>
          <Button
            type='button'
            onClick={handleComplete}
            disabled={!flowId || !callbackUrl.trim() || isCompleting}
            className='bg-cyan-400 text-slate-950 hover:bg-cyan-300'
          >
            {isCompleting ? (
              <Loader2 className='size-4 animate-spin' />
            ) : (
              <ShieldCheck className='size-4' />
            )}
            {isCompleting
              ? t('Completing authorization...')
              : t('Complete authorization')}
          </Button>
        </>
      }
    >
      <FieldGroup>
        <Alert className='border-cyan-400/20 bg-cyan-400/[0.04] text-slate-300'>
          <KeyRound className='text-cyan-400' />
          <AlertDescription className='text-slate-400'>
            {t(
              'The authorization link expires after 10 minutes and can be used only once.'
            )}
          </AlertDescription>
        </Alert>

        <Field>
          <div className='flex items-center justify-between gap-3'>
            <FieldLabel className='text-slate-200'>
              <span className='font-mono text-cyan-400'>01</span>
              {t('Create authorization link')}
            </FieldLabel>
            <Button
              type='button'
              size='sm'
              onClick={handleStart}
              disabled={isStarting || isCompleting}
              className='bg-cyan-400 text-slate-950 hover:bg-cyan-300'
            >
              {isStarting ? (
                <Loader2 className='size-4 animate-spin' />
              ) : (
                <KeyRound className='size-4' />
              )}
              {flowId ? t('Create a new link') : t('Start authorization')}
            </Button>
          </div>
          <FieldDescription className='text-slate-500'>
            {t(
              'Use the ChatGPT account intended for this Codex channel. Starting again invalidates the previous link after it is submitted.'
            )}
          </FieldDescription>
        </Field>

        {authorizeUrl && (
          <Field>
            <FieldLabel className='text-slate-200'>
              <span className='font-mono text-cyan-400'>02</span>
              {t('Open the ChatGPT sign-in page')}
            </FieldLabel>
            <div className='flex flex-wrap gap-2'>
              <Button
                type='button'
                variant='outline'
                onClick={() =>
                  window.open(authorizeUrl, '_blank', 'noopener,noreferrer')
                }
                className='border-cyan-400/30 bg-cyan-400/[0.04] text-cyan-300 hover:bg-cyan-400/10 hover:text-cyan-200'
              >
                <ExternalLink className='size-4' />
                {t('Open authorization page')}
              </Button>
              <Button
                type='button'
                variant='outline'
                onClick={() => copyToClipboard(authorizeUrl)}
                className='border-slate-700 bg-transparent text-slate-300 hover:bg-slate-900 hover:text-white'
              >
                {copiedText === authorizeUrl ? (
                  <Check className='size-4 text-emerald-400' />
                ) : (
                  <Copy className='size-4' />
                )}
                {copiedText === authorizeUrl ? t('Copied') : t('Copy link')}
              </Button>
            </div>
            {expiresAt && (
              <FieldDescription className='font-mono text-xs text-slate-600'>
                {t('Expires at {{time}}', {
                  time: new Date(expiresAt).toLocaleString(),
                })}
              </FieldDescription>
            )}
          </Field>
        )}

        <Field>
          <FieldLabel htmlFor='codex-oauth-callback' className='text-slate-200'>
            <span className='font-mono text-cyan-400'>03</span>
            {t('Paste the localhost callback URL')}
          </FieldLabel>
          <Textarea
            id='codex-oauth-callback'
            value={callbackUrl}
            onChange={(event) => setCallbackUrl(event.target.value)}
            disabled={!flowId || isCompleting}
            rows={4}
            spellCheck={false}
            autoComplete='off'
            placeholder='http://localhost:1455/auth/callback?code=...&state=...'
            className='min-h-24 resize-y border-slate-800 bg-black/60 font-mono text-xs text-cyan-100 placeholder:text-slate-700 focus-visible:border-cyan-400/60 focus-visible:ring-cyan-400/20'
          />
          <FieldDescription className='text-slate-500'>
            {t(
              'After sign-in, the browser may show that localhost cannot be reached. Copy the entire URL from the address bar and paste it here.'
            )}
          </FieldDescription>
        </Field>
      </FieldGroup>
    </Dialog>
  )
}
