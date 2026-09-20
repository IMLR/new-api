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
import { useMutation } from '@tanstack/react-query'
import { Fingerprint, Loader2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'

import { testChannelFingerprint } from '../../api'
import type { FingerprintCandidate } from '../../api'

function CandidateRanking(props: { candidates: FingerprintCandidate[] }) {
  const { t } = useTranslation()
  if (!props.candidates.length)
    return <p>{t('No valid fingerprint samples')}</p>
  return (
    <ol className='space-y-1'>
      {props.candidates.map((candidate, index) => (
        <li
          key={candidate.model}
          className='flex justify-between gap-4 text-sm'
        >
          <span className='break-all'>
            {index + 1}. {candidate.model}
          </span>
          <span className='font-mono tabular-nums'>
            {candidate.score.toFixed(3)}
          </span>
        </li>
      ))}
    </ol>
  )
}

export function ChannelFingerprintButton(props: {
  channelId: number
  model: string
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const mutation = useMutation({
    mutationFn: () =>
      testChannelFingerprint(
        props.channelId,
        props.model,
        props.model.toLowerCase().startsWith('gpt') ? 'gpt' : 'claude'
      ),
  })
  return (
    <>
      <Button
        variant='ghost'
        size='icon-sm'
        disabled={props.disabled || mutation.isPending}
        aria-label={t('Test fingerprint')}
        title={t('Test fingerprint')}
        onClick={() => {
          setOpen(true)
          mutation.mutate()
        }}
      >
        {mutation.isPending ? (
          <Loader2 className='size-4 animate-spin' />
        ) : (
          <Fingerprint className='size-4' />
        )}
      </Button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={`${t('Test fingerprint')}: ${props.model}`}
        description={t(
          'Relative ranking within the reference library, not identity probabilities. Each test sends three paid requests.'
        )}
        bodyClassName='space-y-4 overflow-y-auto'
      >
        {mutation.isPending && (
          <p role='status'>{t('Collecting three fingerprint samples...')}</p>
        )}
        {mutation.isError && <p role='alert'>{t('Fingerprint test failed')}</p>}
        {mutation.data && !mutation.isPending && (
          <>
            <p className='text-muted-foreground text-xs'>
              {mutation.data.reference}
            </p>
            <CandidateRanking candidates={mutation.data.candidates} />
            {mutation.data.samples.map((sample, index) => (
              <details key={index} className='rounded-md border p-3'>
                <summary>
                  {t('Sample {{number}}', { number: index + 1 })} ·{' '}
                  {sample.count} {t('integers')}
                </summary>
                {sample.error ? (
                  <p>{t('No valid fingerprint samples')}</p>
                ) : (
                  <CandidateRanking candidates={sample.candidates || []} />
                )}
                <pre className='mt-2 max-h-48 overflow-auto text-xs break-all whitespace-pre-wrap'>
                  {sample.text}
                </pre>
              </details>
            ))}
          </>
        )}
      </Dialog>
    </>
  )
}
