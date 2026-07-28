import { readFile } from 'node:fs/promises'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

import { expect, test } from '@playwright/test'

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const converterURL = pathToFileURL(
  path.join(webRoot, 'dist', 'channel-config-converter', 'index.html')
).href
const v1Fixture = path.join(
  webRoot,
  'src',
  'channel-config-converter',
  '__fixtures__',
  'channel-config-v1-corrected.xlsx'
)

test.describe('offline channel configuration converter', () => {
  test.beforeEach(async ({ page }) => {
    const requestedURLs: string[] = []
    page.on('request', (request) => {
      const url = request.url()
      if (
        !url.startsWith('file:') &&
        !url.startsWith('data:') &&
        !url.startsWith('blob:')
      ) {
        requestedURLs.push(url)
      }
    })
    await page.goto(converterURL)
    await page.locator('#workbook-file').setInputFiles(v1Fixture)
    await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()
    expect(requestedURLs).toEqual([])
  })

  test('exports a selected secure line locally without persistence or network', async ({
    page,
  }) => {
    await expect(page.getByText('channel_lines', { exact: true })).toBeVisible()
    await page.getByRole('tab', { name: 'Selection' }).click()
    await page.getByRole('checkbox', { name: 'secure-enterprise' }).check()
    await expect(
      page.getByText('Selected lines').locator('xpath=following-sibling::dd')
    ).toHaveText('1')
    await expect(
      page.getByRole('button', { name: 'Export selected JSON' })
    ).toBeEnabled()
    await expect(page.getByRole('tab', { name: 'Issues' })).toBeVisible()
    await expect(page.getByRole('tab', { name: 'JSON' })).toBeVisible()

    const downloadPromise = page.waitForEvent('download')
    await page.getByRole('button', { name: 'Export selected JSON' }).click()
    const download = await downloadPromise
    const downloadPath = await download.path()
    expect(downloadPath).not.toBeNull()
    if (downloadPath === null) {
      throw new Error(
        'Expected the converter JSON download to have a local path'
      )
    }
    const document = JSON.parse(await readFile(downloadPath, 'utf8')) as {
      entities: {
        channel_lines: Array<{ line_ref: string }>
        route_blueprints: Array<{
          targets: Array<{ line_ref: string }>
        }>
      }
      kind: string
      manifest: { payload_sha256: string }
    }
    expect(document.kind).toBe('new-api.channel-config-import')
    expect(document.manifest.payload_sha256).toMatch(/^[a-f0-9]{64}$/)
    expect(
      document.entities.channel_lines.map((line) => line.line_ref)
    ).toEqual(['secure-enterprise'])
    expect(
      document.entities.route_blueprints.every((route) =>
        route.targets.every((target) => target.line_ref === 'secure-enterprise')
      )
    ).toBeTruthy()

    const storage = await page.evaluate(async () => ({
      localStorage: window.localStorage.length,
      indexedDB: await indexedDB
        .databases()
        .then((databases) => databases.length),
    }))
    expect(storage).toEqual({ localStorage: 0, indexedDB: 0 })

    await page.reload()
    await expect(page.getByRole('tab', { name: 'Selection' })).toHaveCount(0)
  })

  test('keeps content inside the viewport and all controls accessible', async ({
    page,
  }) => {
    for (const tab of [
      'Selection',
      'Channels and lines',
      'Model SKUs',
      'Sale pricing',
      'Channel costs',
      'Model mappings and routing',
      'Issues',
      'JSON',
    ]) {
      await page.getByRole('tab', { name: tab }).click()
      await expect(page.getByRole('tab', { name: tab })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      expect(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth
        )
      ).toBeTruthy()
    }

    await page.getByRole('tab', { name: 'Issues' }).click()
    await expect(page.getByLabel('Issue severity')).toBeVisible()
    await expect(
      page.getByRole('button', { name: 'Download issue report' })
    ).toBeVisible()
  })
})
