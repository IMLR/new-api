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
Object.assign(globalThis, {
  IS_REACT_ACT_ENVIRONMENT: true,
  requestAnimationFrame: window.requestAnimationFrame.bind(window),
  cancelAnimationFrame: window.cancelAnimationFrame.bind(window),
})
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

test('fingerprint runs in background and cached results open without a paid request', async () => {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const previous = api.defaults.adapter
  const queryKey = ['channel-fingerprint', 11, 'gpt-6-astra']
  const completed = {
    status: 'completed' as const,
    model: 'gpt-6-astra',
    reference: 'GPT reference',
    candidates: [{ model: 'gpt-5.6-luna', score: 1.8 }],
    samples: [],
  }
  let current:
    | typeof completed
    | { status: 'running'; candidates: []; samples: [] }
    | null = null
  let posts = 0
  api.defaults.adapter = async (config) => {
    if (config.method === 'post') {
      posts++
      current = { status: 'running', candidates: [], samples: [] }
    }
    return {
      config,
      status: 200,
      statusText: 'OK',
      headers: {},
      data: { success: true, data: current },
    }
  }
  notifyManager.setScheduler((callback) => callback())
  const render = () =>
    root.render(
      <QueryClientProvider client={client}>
        <ChannelFingerprintButton channelId={11} model='gpt-6-astra' />
      </QueryClientProvider>
    )
  try {
    await client.fetchQuery({ queryKey, queryFn: async () => null })
    await act(async () => render())
    const button = container.querySelector<HTMLButtonElement>(
      '[aria-label="Test fingerprint"]'
    )
    assert.ok(button)
    await act(async () => button.click())
    assert.equal(posts, 1)
    assert.equal(button.disabled, true)
    assert.equal(document.querySelector('[role="dialog"]'), null)
    // Closing the containing channel modal unmounts this component.
    await act(async () => root.render(null))
    current = completed
    await client.invalidateQueries({ queryKey })
    await act(async () => render())
    await act(async () => {
      await client.refetchQueries({ queryKey })
    })
    const candidate = Array.from(container.querySelectorAll('button')).find(
      (item) => item.textContent?.includes('gpt-5.6-luna')
    )
    assert.ok(candidate)
    assert.ok(candidate.textContent?.includes('1.800'))
    await act(async () => candidate.click())
    assert.ok(document.querySelector('[role="dialog"]'))
    assert.ok(document.body.textContent?.includes('GPT reference'))
    assert.equal(posts, 1)
  } finally {
    await act(async () => root.unmount())
    container.remove()
    client.clear()
    api.defaults.adapter = previous
    notifyManager.setScheduler((callback) => setTimeout(callback, 0))
  }
})
