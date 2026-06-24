import { expect, test } from '@playwright/test';

import { managedApkPath } from '../support/apps';
import { dashboardCredentials } from '../support/auth';
import { dashboardPaths } from '../support/paths';

test('admin can create a managed app in one flow', async ({ page }) => {
  const { username, password } = dashboardCredentials();
  const suffix = `${Date.now()}-${Math.random().toString(16).slice(2, 8)}`;
  const packageName = `com.example.catalog.${suffix}`;
  const appName = `Catalog ${suffix}`;
  await page.goto(dashboardPaths.login);
  await page.getByLabel('Username').fill(username);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Login' }).click();

  await page.goto(dashboardPaths.apps);
  await expect(page.getByRole('heading', { name: 'Apps' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Create managed app' })).toBeVisible();
  await expect(page.getByLabel('Publish immediately')).toHaveCount(0);
  await expect(page.getByRole('columnheader', { name: 'Created' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Name' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Package' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'System owned' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Latest published' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Status' })).toBeVisible();

  await page.getByLabel('Package name').fill(packageName);
  await page.getByLabel('App name').fill(appName);
  await page.getByLabel('Version code').fill('100');
  await page.getByLabel('APK file').setInputFiles(managedApkPath());
  await page.getByRole('button', { name: 'Create managed app' }).click();

  await expect(page).toHaveURL(/\/admin\/apps\/[^?]+\?ok=/);
  await expect(page.getByText('managed app created')).toBeVisible();
  await expect(page.getByRole('heading', { name: 'App Detail' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Current app' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Versions' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Update app' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Retire app' })).toBeVisible();
  await expect(page.getByText(appName, { exact: true })).toBeVisible();
  await expect(page.getByText(packageName, { exact: true })).toBeVisible();
  const versionsSection = page.getByRole('heading', { name: 'Versions' }).locator('xpath=ancestor::section[1]');
  await expect(versionsSection.getByRole('cell', { name: 'v100', exact: true }).first()).toBeVisible();
  await expect(versionsSection.getByRole('cell', { name: '100', exact: true }).first()).toBeVisible();
  await expect(versionsSection.getByRole('cell', { name: 'published', exact: true }).first()).toBeVisible();
  const appId = new URL(page.url()).pathname.split('/').pop() ?? '';
  expect(appId).not.toBe('');
  await expect(page.getByText(appId, { exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Download latest APK' })).toHaveAttribute('href', `/admin/apps/${appId}/download`);
  await expect(page.locator('input[name="artifactId"]')).toHaveCount(0);
  await expect(page.locator('input[name="checksum"]')).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Create version' })).toHaveCount(0);

  await page.goto(dashboardPaths.apps);
  await expect(page.getByRole('columnheader', { name: 'Created' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Name' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Package' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'System owned' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Latest published' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Status' })).toBeVisible();
  const appRow = await page.locator('table tbody tr').filter({ hasText: appName }).first();
  await expect(appRow.getByRole('link', { name: appName, exact: true })).toHaveAttribute('href', dashboardPaths.appDetail(appId));
  await expect(appRow.getByRole('cell', { name: packageName, exact: true })).toBeVisible();
  await expect(appRow).not.toContainText('✓');
  await expect(appRow).toContainText('v100 (#100)');
  await expect(page.getByRole('button', { name: 'Update app' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Retire app' })).toHaveCount(0);
});
