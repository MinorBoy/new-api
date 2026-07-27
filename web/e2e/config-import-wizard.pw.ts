import { expect, test } from '@playwright/test'

test.describe('config import access guard', () => {
  test('requires an authenticated administrator before showing the wizard', async ({
    page,
  }) => {
    await page.goto('http://127.0.0.1:4173/config-import')
    await expect(page).toHaveURL(/\/sign-in/)
    await expect(page.getByRole('heading', { name: /sign in/i })).toBeVisible()
  })
})
