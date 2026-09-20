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
import { after, test } from 'node:test'

import { Window } from 'happy-dom'

const window = new Window()
for (const key of [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLInputElement',
  'HTMLButtonElement',
  'getComputedStyle',
  'Node',
  'Element',
  'Event',
  'MutationObserver',
] as const) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: key === 'window' ? window : window[key],
  })
}
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const i18next = (await import('i18next')).default
const { initReactI18next } = await import('react-i18next')
await i18next.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
  fallbackLng: 'en',
})
const { QueryClient, QueryClientProvider, notifyManager } =
  await import('@tanstack/react-query')
const { api } = await import('@/lib/api')
const { ChannelFingerprintButton } =
  await import('../dialogs/channel-fingerprint-button')
after(() => window.happyDOM.abort())

test('fingerprint action disables duplicate clicks and renders returned candidates', async () => {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  const previous = api.defaults.adapter
  let finish: (() => void) | undefined
  const requested: string[] = []
  api.defaults.adapter = (config) =>
    new Promise((resolve) => {
      requested.push(config.url || '')
      finish = () =>
        resolve({
          config,
          status: 200,
          statusText: 'OK',
          headers: {},
          data: {
            success: true,
            data: {
              model: 'gpt-6-astra',
              reference: 'GPT reference',
              candidates: [{ model: 'gpt-5.6-luna', score: 1.8 }],
              samples: [],
            },
          },
        })
    })
  notifyManager.setScheduler((callback) => callback())
  try {
    await act(async () => {
      root.render(
        <QueryClientProvider client={client}>
          <ChannelFingerprintButton channelId={11} model='gpt-6-astra' />
        </QueryClientProvider>
      )
    })
    const button = container.querySelector<HTMLButtonElement>(
      '[aria-label="Test fingerprint"]'
    )
    assert.ok(button)
    await act(async () => button.click())
    assert.equal(button.disabled, true)
    assert.deepEqual(requested, ['/api/channel/fingerprint/11'])
    assert.ok(finish)
    const completed = new Promise<void>((resolve) => {
      const unsubscribe = client.getMutationCache().subscribe((event) => {
        if (event.mutation?.state.status === 'success') {
          unsubscribe()
          resolve()
        }
      })
    })
    await act(async () => {
      if (finish) {
        finish()
      }
      await completed
    })
    assert.equal(button.disabled, false)
    assert.ok(document.body.textContent?.includes('gpt-5.6-luna'))
    assert.ok(document.body.textContent?.includes('1.800'))
  } finally {
    await act(async () => root.unmount())
    container.remove()
    client.clear()
    api.defaults.adapter = previous
    notifyManager.setScheduler((callback) => setTimeout(callback, 0))
  }
})
