import { expect, test } from '@playwright/test';

import { dashboardCredentials } from '../support/auth';
import { dashboardPaths } from '../support/paths';

test('admin can log in and reach the overview page', async ({ page }) => {
  const { username, password } = dashboardCredentials();

  await page.goto(dashboardPaths.login);
  await expect(page).toHaveTitle(/Login - XMDM/);

  await page.getByLabel('Username').fill(username);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Login' }).click();

  await expect(page).toHaveURL(new RegExp(`${dashboardPaths.overview}$`));
  await expect(page.getByRole('button', { name: 'Sign out' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible();
});
