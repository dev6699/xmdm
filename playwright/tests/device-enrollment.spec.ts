import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';

import { ensureAgentAppPublished } from '../support/apps';
import { dashboardCredentials } from '../support/auth';
import { dashboardPaths } from '../support/paths';
import { dashboardServerConfig } from '../support/server';

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

async function createPolicy(page: Page, name: string) {
  await page.goto(dashboardPaths.policies);
  await page.getByLabel('Name').fill(name);
  await page.getByRole('button', { name: 'Create policy' }).click();
  const row = page.locator('table tbody tr').filter({ hasText: name }).first();
  await expect(row).toBeVisible();
  const policyId = (await row.locator('td').nth(1).textContent() ?? '').trim();
  expect(policyId).not.toBe('');
  await row.getByRole('link', { name }).click();
  await expect(page).toHaveURL(new RegExp(`/admin/policies/${policyId}$`));
  await expect(page.getByRole('heading', { name: 'Policy Detail' })).toBeVisible();
  await expect(page.getByLabel('Enable kiosk mode')).not.toBeChecked();
  await expect(page.getByLabel('Kiosk exit passcode')).toBeVisible();
  await expect(page.getByLabel('Allow packages')).toHaveAttribute('placeholder', /One package per line/);
  const policyBody = await page.locator('main').textContent();
  expect(policyBody ?? '').toContain('Enable kiosk mode');
  expect(policyBody ?? '').not.toContain('Available policy fields:');
  return policyId;
}

async function idFromRowLink(row: ReturnType<Page['locator']>, name: string) {
  const href = await row.getByRole('link', { name }).getAttribute('href');
  const id = new URL(href ?? '', 'http://127.0.0.1').pathname.split('/').pop() ?? '';
  expect(id).not.toBe('');
  return id;
}

async function createPendingDevice(page: Page, name: string, policyId: string) {
  await page.goto(dashboardPaths.devices);
  await page.getByLabel('Display name').fill(name);
  await page.getByLabel('Policy').selectOption(policyId);
  await page.getByRole('button', { name: 'Create device' }).click();
  const row = page.locator('table tbody tr').filter({ hasText: name }).first();
  await expect(row).toBeVisible();
  return idFromRowLink(row, name);
}

async function generateDeviceEnrollmentQR(page: Page, deviceRowId: string, deviceName: string) {
  await ensureAgentAppPublished(page);

  await page.goto(dashboardPaths.deviceDetail(deviceRowId));
  await expect(page.getByRole('heading', { name: 'Device Detail' })).toBeVisible();
  await expect(page.getByRole('radiogroup', { name: 'Output format' })).toHaveCount(0);
  const currentDevice = page.getByRole('heading', { name: 'Current device' }).locator('xpath=ancestor::section[1]');
  const device = JSON.parse((await currentDevice.locator('pre').first().textContent()) ?? '{}') as { id: string; name: string };
  await page.getByRole('button', { name: 'Generate QR' }).click();

  await expect(page.getByText('QR generated')).toBeVisible();
  const qrSection = page.getByRole('heading', { name: 'QR JSON' }).locator('xpath=ancestor::section[1]');
  const payload = JSON.parse((await qrSection.locator('pre').first().textContent()) ?? '{}') as {
    ['android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE']: Record<string, string>;
    ['android.app.extra.PROVISIONING_DEVICE_ADMIN_COMPONENT_NAME']: string;
    ['android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_CHECKSUM']: string;
    ['android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_DOWNLOAD_LOCATION']: string;
  };
  const { baseURL } = dashboardServerConfig();
  expect(payload['android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE']['com.xmdm.ENROLLMENT_TOKEN']).toBeTruthy();
  expect(device.id).toBeTruthy();
  expect(device.name).toBe(deviceName);
  expect(payload['android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE']['com.xmdm.DEVICE_ID']).toBe(device.id);
  expect(payload['android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE']['com.xmdm.BASE_URL']).toBe(baseURL);
  expect(payload['android.app.extra.PROVISIONING_DEVICE_ADMIN_COMPONENT_NAME']).toBe('com.xmdm.launcher/.AdminReceiver');
  expect(payload['android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_DOWNLOAD_LOCATION']).toBe(new URL('/api/v1/enrollment/agent.apk', baseURL).toString());
  expect(payload['android.app.extra.PROVISIONING_DEVICE_ADMIN_PACKAGE_CHECKSUM']).toBeTruthy();
  await expect(page.getByRole('heading', { name: 'QR preview' })).toBeVisible();
  await expect(page.locator('img[alt="Enrollment QR preview"]')).toHaveAttribute('src', /data:image\/png;base64,/);
  return { payload };
}

test('admin can generate an enrollment qr from a pending device detail page', async ({ page }) => {
  const { username, password } = dashboardCredentials();
  const suffix = uniqueSuffix();
  const deviceName = `pending-device-${suffix}`;

  await login(page, username, password);

  const policyId = await createPolicy(page, `playwright-device-qr-policy-${suffix}`);
  const deviceId = await createPendingDevice(page, deviceName, policyId);

  await generateDeviceEnrollmentQR(page, deviceId, deviceName);
});
