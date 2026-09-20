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
const { useForm } = await import('react-hook-form')
const { Form } = await import('@/components/ui/form')
const { ChannelDetectionSettings } =
  await import('../channel-detection-settings')
const { CHANNEL_FORM_DEFAULT_VALUES } = await import('../../lib/channel-form')
after(() => window.happyDOM.abort())

function SettingsFixture(props: { type: number }) {
  const form = useForm({
    defaultValues: {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      type: props.type,
      models: 'gpt-6-astra',
    },
  })
  return (
    <Form {...form}>
      <ChannelDetectionSettings form={form} />
    </Form>
  )
}

test('OpenAI channel exposes a detection switch and a per-model endpoint selector', async () => {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)
  try {
    await act(async () => {
      root.render(<SettingsFixture type={1} />)
    })
    const toggle = container.querySelector<HTMLElement>('[role="switch"]')
    assert.ok(toggle)
    assert.equal(toggle.getAttribute('aria-checked'), 'false')
    await act(async () => toggle.click())
    assert.equal(toggle.getAttribute('aria-checked'), 'true')
    assert.ok(
      container.querySelector('[aria-label="Endpoint Type: gpt-6-astra"]')
    )
  } finally {
    await act(async () => root.unmount())
    container.remove()
  }
})

test('Anthropic channel exposes detection without incompatible OpenAI endpoint choices', async () => {
  const container = document.createElement('div')
  const root = createRoot(container)
  try {
    await act(async () => {
      root.render(<SettingsFixture type={14} />)
    })
    assert.ok(container.querySelector('[role="switch"]'))
    assert.equal(
      container.querySelector('[aria-label="Endpoint Type: gpt-6-astra"]'),
      null
    )
  } finally {
    await act(async () => root.unmount())
    container.remove()
  }
})
