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

async function submitForm(page: Page, path: string, body: string) {
  return page.evaluate(async ({ path, body }) => {
    const response = await fetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body,
      credentials: 'same-origin',
    });
    return {
      status: response.status,
      text: await response.text(),
    };
  }, { path, body });
}

async function findRowByText(page: Page, text: string) {
  const row = page.locator('table tbody tr').filter({ hasText: text }).first();
  await expect(row).toBeVisible();
  return row;
}

async function retireIfActive(page: Page, detailPath: string, retireButtonName: string) {
  await page.goto(detailPath);
  const snapshot = await page.locator('pre').first().textContent().catch(() => '');
  if (snapshot?.includes('"status": "retired"')) {
    return;
  }
  await page.getByRole('button', { name: retireButtonName }).click();
}

test('admin can manage users and roles', async ({ page }) => {
  const { username, password } = dashboardCredentials();
  const suffix = uniqueSuffix();
  const managerRoleName = `playwright-manager-${suffix}`;
  const managerRoleUpdatedName = `${managerRoleName}-updated`;
  const managerEmail = `manager-${suffix}@example.com`;
  const managerUpdatedEmail = `manager-updated-${suffix}@example.com`;

  let managerRoleId = '';
  let managerUserId = '';

  await login(page, username, password);

  try {
    await page.goto(dashboardPaths.users);
    await expect(page.getByRole('heading', { name: 'Users' })).toBeVisible();
    await expect(page.getByText('Manage operator accounts and role bindings.')).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Created' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'ID' })).toBeVisible();

    await page.goto(dashboardPaths.roles);
    await expect(page.getByRole('heading', { name: 'Roles' })).toBeVisible();
    await expect(page.getByText('Define the permission bundles available to operators.')).toBeVisible();
    await expect(page.getByRole('group', { name: 'Permissions' })).toBeVisible();
    await expect(page.locator('input[name="permissions"]')).toHaveCount(4);
    await expect(page.getByRole('columnheader', { name: 'Created' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'ID' })).toBeVisible();

    await page.goto(dashboardPaths.roles);
    await page.getByLabel('Name').fill(managerRoleName);
    await page.getByLabel('admin.read').check();
    await page.getByLabel('admin.write').check();
    await page.getByRole('button', { name: 'Create role' }).click();
    await expect(page).toHaveURL(new RegExp(`${dashboardPaths.roles}(\\?.*)?$`));
    const managerRoleRow = await findRowByText(page, managerRoleName);
    managerRoleId = (await managerRoleRow.locator('td').nth(1).textContent() ?? '').trim();
    expect(managerRoleId).not.toBe('');
    await managerRoleRow.getByRole('link', { name: managerRoleName }).click();
    await expect(page).toHaveURL(new RegExp(`/admin/roles/${managerRoleId}$`));
    await expect(page.getByRole('heading', { name: 'Role Detail' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Update role' })).toBeVisible();
    await page.getByLabel('Name').fill(managerRoleUpdatedName);
    await page.getByRole('button', { name: 'Update role' }).click();
    await expect(page).toHaveURL(new RegExp(`/admin/roles/${managerRoleId}(\\?.*)?$`));
    await page.goto(dashboardPaths.roles);
    await page.getByRole('link', { name: managerRoleUpdatedName }).click();
    await expect(page).toHaveURL(new RegExp(`/admin/roles/${managerRoleId}$`));

    await page.goto(dashboardPaths.users);
    await page.getByLabel('Email').fill(managerEmail);
    await page.getByLabel('Password').fill('manager-secret-123');
    await page.getByLabel('Role').selectOption(managerRoleId);
    await page.getByRole('button', { name: 'Create user' }).click();
    await expect(page).toHaveURL(new RegExp(`${dashboardPaths.users}(\\?.*)?$`));
    const managerUserRow = await findRowByText(page, managerEmail);
    managerUserId = (await managerUserRow.locator('td').nth(1).textContent() ?? '').trim();
    expect(managerUserId).not.toBe('');
    await managerUserRow.getByRole('link', { name: managerEmail }).click();
    await expect(page).toHaveURL(new RegExp(`/admin/users/${managerUserId}$`));
    await expect(page.getByRole('heading', { name: 'User Detail' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Update user' })).toBeVisible();
    await page.getByLabel('Email').fill(managerUpdatedEmail);
    await page.getByLabel('Role').selectOption(managerRoleId);
    await page.getByRole('button', { name: 'Update user' }).click();
    await expect(page).toHaveURL(new RegExp(`/admin/users/${managerUserId}(\\?.*)?$`));
    await page.goto(dashboardPaths.users);
    await page.getByRole('link', { name: managerUpdatedEmail }).click();
    await expect(page).toHaveURL(new RegExp(`/admin/users/${managerUserId}$`));

    await page.goto(dashboardPaths.users);
    const csrfFailure = await submitForm(
      page,
      dashboardPaths.users + '/create',
      new URLSearchParams({
        email: `csrf-${suffix}@example.com`,
        password: 'csrf-secret',
        roleId: managerRoleId,
      }).toString(),
    );
    expect(csrfFailure.status).toBe(200);
    expect(csrfFailure.text).toContain('forbidden');

    await page.getByRole('button', { name: 'Sign out' }).click();
    await expect(page).toHaveURL(new RegExp(`${dashboardPaths.login}$`));
    await login(page, managerUpdatedEmail, 'manager-secret-123');
    await page.goto(dashboardPaths.users);
    await expect(page.getByRole('heading', { name: 'Users' })).toBeVisible();

    await retireIfActive(page, `${dashboardPaths.users}/${managerUserId}`, 'Retire user');
    await retireIfActive(page, `${dashboardPaths.roles}/${managerRoleId}`, 'Retire role');

  } finally {
    if (managerUserId) {
      await retireIfActive(page, `${dashboardPaths.users}/${managerUserId}`, 'Retire user').catch(() => undefined);
    }
    if (managerRoleId) {
      await retireIfActive(page, `${dashboardPaths.roles}/${managerRoleId}`, 'Retire role').catch(() => undefined);
    }
  }
});
