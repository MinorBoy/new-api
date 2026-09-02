import { expect, test } from '@playwright/test'

const enabled = process.env.E2E_IMAGE_PRICING_TEST === 'true'
const storageState = process.env.E2E_IMAGE_PRICING_STORAGE_STATE

test.describe('image pricing workbench', () => {
  test.use({ storageState: storageState || undefined })

  test('edits a SKU, preserves it after reload, previews routing, and remains usable on mobile', async ({
    page,
  }) => {
    test.skip(
      !enabled,
      'Set E2E_IMAGE_PRICING_TEST=true and E2E_IMAGE_PRICING_STORAGE_STATE to run authenticated acceptance.'
    )

    await page.goto('/system-settings/billing/image-pricing')
    await expect(page.getByText('gpt-image-2')).toBeVisible()

    const price = page.getByRole('textbox', {
      name: 'gpt-image-2 gen-1024x1024-medium sale price',
    })
    const original = await price.inputValue()
    await price.fill('0.035')
    await expect(page.getByText(/Unsaved changes:\s*1/)).toBeVisible()
    await expect(page.getByRole('button', { name: 'Save changes' })).toBeEnabled()
    await page.getByRole('button', { name: 'Save changes' }).click()
    await expect(price).toHaveValue('0.035')

    await page.reload()
    await expect(
      page.getByRole('textbox', {
        name: 'gpt-image-2 gen-1024x1024-medium sale price',
      })
    ).toHaveValue('0.035')

    await page.getByRole('button', { name: 'Advanced settings' }).click()
    await page.getByRole('button', { name: 'Preview image routing' }).click()
    await expect(page.getByText('Routing preview')).toBeVisible()

    await page.setViewportSize({ width: 390, height: 844 })
    const tableContainer = page.locator('div.overflow-x-auto').first()
    await expect(tableContainer).toBeVisible()
    await expect(price).toBeVisible()
    const scrollable = await tableContainer.evaluate(
      (element) => element.scrollWidth >= element.clientWidth
    )
    expect(scrollable).toBeTruthy()

    if (original !== '0.035') {
      await price.fill(original)
      await page.getByRole('button', { name: 'Save changes' }).click()
    }
  })
})
