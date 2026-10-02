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
import { test } from 'node:test'

const baseURL = process.env.MODEL_MATCHING_E2E_URL
const username = process.env.MODEL_MATCHING_E2E_USER
const password = process.env.MODEL_MATCHING_E2E_PASSWORD

// Run against a disposable, initialized gateway; no upstream requests are made.
test(
  'literal filters, case selection and channel/model exclusions persist after save and reload',
  {
    skip: !baseURL || !username || !password,
  },
  async () => {
    const { chromium, expect } = await import('@playwright/test')
    const browser = await chromium.launch({ headless: true })
    const context = await browser.newContext({
      viewport: { width: 1440, height: 1100 },
      locale: 'en-US',
    })
    const page = await context.newPage()
    const pageErrors = []
    page.on('pageerror', (error) => pageErrors.push(error.message))
    await page.addInitScript(() => localStorage.setItem('i18nextLng', 'en'))
    const createdChannels = []
    let createdModelId
    let headers = {}
    try {
      const login = await context.request.post(`${baseURL}/api/user/login`, {
        data: { username, password },
      })
      const bundle = await login.json()
      assert.equal(bundle.success, true)
      headers = { Authorization: `Bearer ${bundle.data.access_token}` }
      const resourceName = `matching-${Date.now()}`
      const primaryName = `Primary ${resourceName}`
      const backupName =
        `Backup ${resourceName} ${'long-name '.repeat(12)}`.trim()
      for (const name of [primaryName, backupName]) {
        const response = await context.request.post(`${baseURL}/api/channel/`, {
          headers,
          data: {
            mode: 'single',
            channel: {
              type: 1,
              name,
              key: 'sk-local-test',
              models:
                'claude-sonnet-4,CLAUDE-SONNET-4,claude-sonnet-preview,claude-opus-4',
              group: 'default',
              status: 1,
            },
          },
        })
        assert.equal((await response.json()).success, true)
      }
      const inventory = await context.request.post(
        `${baseURL}/api/models/match_preview`,
        {
          headers,
          data: {
            match_rule: {
              include: ['claude', 'sonnet'],
              exclude: [],
              case_sensitive: false,
            },
          },
        }
      )
      const inventoryBody = await inventory.json()
      assert.equal(inventoryBody.success, true)
      for (const channel of inventoryBody.data.flatMap(
        (model) => model.channels
      )) {
        if (
          (channel.name === primaryName || channel.name === backupName) &&
          !createdChannels.includes(channel.id)
        ) {
          createdChannels.push(channel.id)
        }
      }
      assert.equal(createdChannels.length, 2)

      // The API login established the real refresh cookie used by the UI bootstrap.
      await page.goto(`${baseURL}/models`)
      await page.getByRole('button', { name: 'Add Model', exact: true }).click()
      const dialog = page.getByRole('dialog')
      await dialog
        .getByLabel('Model Name *', { exact: true })
        .fill(resourceName)
      const include = dialog.getByLabel('Must contain all', { exact: true })
      await dialog
        .getByRole('button', { name: 'Save changes', exact: true })
        .click()
      await expect(include).toHaveAttribute('aria-invalid', 'true')
      await expect(
        dialog.getByText('Add at least one included text.', { exact: true })
      ).toBeVisible()
      await include.fill('claude')
      await include.press('Enter')
      await include.fill('sonnet')
      await include.press('Enter')
      const previewName = `Use claude-sonnet-preview on ${primaryName}`
      await expect(
        dialog.getByRole('checkbox', { name: previewName, exact: true })
      ).toBeVisible()
      assert.equal(
        await dialog
          .getByRole('checkbox', {
            name: `Use claude-opus-4 on ${primaryName}`,
            exact: true,
          })
          .count(),
        0
      )
      const exclude = dialog.getByLabel('Must not contain any', { exact: true })
      await exclude.fill('preview')
      await exclude.press('Enter')
      await expect(
        dialog.getByRole('checkbox', { name: previewName, exact: true })
      ).toHaveCount(0)
      const upperName = `Use CLAUDE-SONNET-4 on ${primaryName}`
      await expect(
        dialog.getByRole('checkbox', { name: upperName, exact: true })
      ).toBeChecked()
      await expect(
        dialog.getByRole('checkbox', { name: previewName, exact: true })
      ).toHaveCount(0)
      const caseSwitch = dialog.getByRole('switch', {
        name: 'Case sensitive',
        exact: true,
      })
      await caseSwitch.click()
      await expect(
        dialog.getByRole('checkbox', {
          name: `Use claude-sonnet-4 on ${primaryName}`,
          exact: true,
        })
      ).toBeChecked()
      await expect(
        dialog.getByRole('checkbox', { name: upperName, exact: true })
      ).toHaveCount(0)
      await caseSwitch.click()
      await expect(
        dialog.getByRole('checkbox', { name: upperName, exact: true })
      ).toBeChecked()
      await expect(page.getByText('canceled', { exact: true })).toHaveCount(0)

      const primary = dialog.getByRole('checkbox', {
        name: `Use claude-sonnet-4 on ${primaryName}`,
        exact: true,
      })
      await primary.click()
      await expect(primary).not.toBeChecked()
      await primary.press('Space')
      await expect(primary).toBeChecked()
      await primary.press('Space')
      await expect(primary).not.toBeChecked()
      await expect(
        dialog.getByRole('checkbox', {
          name: `Use claude-sonnet-4 on ${backupName}`,
          exact: true,
        })
      ).toBeChecked()
      await expect(
        dialog.getByRole('checkbox', { name: upperName, exact: true })
      ).toBeChecked()
      const savedResponse = page.waitForResponse(
        (response) =>
          response.url().endsWith('/api/models/') &&
          response.request().method() === 'POST'
      )
      await dialog
        .getByRole('button', { name: 'Save changes', exact: true })
        .click()
      const saved = await (await savedResponse).json()
      assert.equal(saved.success, true)
      createdModelId = saved.data.id
      await expect(dialog).not.toBeVisible()
      await page.reload()
      const row = page.getByRole('row').filter({ hasText: resourceName })
      await row.getByRole('button', { name: 'Edit', exact: true }).click()
      await expect(primary).not.toBeChecked()
      await expect(
        dialog.getByRole('checkbox', { name: upperName, exact: true })
      ).toBeChecked()

      await page.setViewportSize({ width: 390, height: 844 })
      const longText = `vendor-${'x'.repeat(121)}`
      await include.fill(longText)
      await include.press('Enter')
      await expect(
        dialog.getByText('No channel models match these filters.', {
          exact: true,
        })
      ).toBeVisible()
      assert.equal(
        await dialog.evaluate(
          (element) => element.scrollWidth <= element.clientWidth
        ),
        true,
        'long filter text must fit the mobile drawer'
      )
      const tagBounds = await dialog
        .getByText(longText, { exact: true })
        .boundingBox()
      const dialogBounds = await dialog.boundingBox()
      assert.ok(
        tagBounds && dialogBounds && tagBounds.width <= dialogBounds.width
      )
      await dialog
        .getByRole('button', { name: `Remove ${longText}`, exact: true })
        .click()
      await expect(primary).not.toBeChecked()
      await primary.scrollIntoViewIfNeeded()
      assert.equal(
        await dialog.evaluate(
          (element) => element.scrollWidth <= element.clientWidth
        ),
        true
      )
      await primary.focus()
      await primary.press('Space')
      await expect(primary).toBeChecked()
      const updatedResponse = page.waitForResponse(
        (response) =>
          response.url().endsWith('/api/models/') &&
          response.request().method() === 'PUT'
      )
      await dialog
        .getByRole('button', { name: 'Update Model', exact: true })
        .click()
      assert.equal((await (await updatedResponse).json()).success, true)
      await expect(dialog).not.toBeVisible()
      await page.reload()
      await page
        .getByPlaceholder('Filter by model name...', { exact: true })
        .fill(resourceName)
      await page.getByRole('button', { name: 'Edit', exact: true }).click()
      await expect(primary).toBeChecked()
      assert.deepEqual(pageErrors, [])
    } finally {
      if (createdModelId) {
        await context.request.delete(
          `${baseURL}/api/models/${createdModelId}`,
          { headers }
        )
      }
      for (const channelId of createdChannels) {
        await context.request.delete(`${baseURL}/api/channel/${channelId}`, {
          headers,
        })
      }
      await browser.close()
    }
  }
)
