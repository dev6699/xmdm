import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';

import { dashboardCredentials } from '../support/auth';
import { dashboardPaths } from '../support/paths';

function uniqueSuffix() {
  return `${Date.now()}-${Math.random().toString(16).slice(2, 8)}`;
}

async function login(page: Page, username: string, password: string) {
  await page.goto(dashboardPaths.login);
  await page.getByLabel('Username').fill(username);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Login' }).click();
  await expect(page).toHaveURL(new RegExp(`${dashboardPaths.overview}$`));
}

test('admin can inspect audit events from the dashboard', async ({ page }) => {
  const { username, password } = dashboardCredentials();
  const suffix = uniqueSuffix();
  const roleName = `playwright-audit-role-${suffix}`;

  await login(page, username, password);

  await page.goto(dashboardPaths.roles);
  await page.getByLabel('Name').fill(roleName);
  await page.getByLabel('admin.read').check();
  await page.getByRole('button', { name: 'Create role' }).click();

  const roleRow = page.locator('table tbody tr').filter({ hasText: roleName }).first();
  await expect(roleRow).toBeVisible();
  const roleId = (await roleRow.locator('td').nth(1).textContent() ?? '').trim();
  expect(roleId).not.toBe('');

  await page.goto(dashboardPaths.audit);
  await expect(page.getByRole('heading', { name: 'Audit' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Created' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Actor' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Action' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Resource' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Details' })).toBeVisible();

  const auditRow = page.locator('table tbody tr').filter({ hasText: roleId }).first();
  await expect(auditRow).toBeVisible();
  await expect(auditRow).toContainText('create');
  await expect(auditRow).toContainText(`roles/${roleId}`);
  await expect(auditRow).toContainText(roleName);
});
