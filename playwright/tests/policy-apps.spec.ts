import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';

import { ensureAgentAppPublished, managedApkPath } from '../support/apps';
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

async function findRowByText(page: Page, text: string) {
  const row = page.locator('table tbody tr').filter({ hasText: text }).first();
  await expect(row).toBeVisible();
  return row;
}

async function idFromRowLink(row: ReturnType<Page['locator']>, name: string) {
  const href = await row.getByRole('link', { name }).getAttribute('href');
  const id = new URL(href ?? '', 'http://127.0.0.1').pathname.split('/').pop() ?? '';
  expect(id).not.toBe('');
  return id;
}

async function createManagedApp(page: Page, name: string, packageName: string, versionCode: string) {
  await page.goto(dashboardPaths.apps);
  await page.getByLabel('Package name').fill(packageName);
  await page.getByLabel('App name').fill(name);
  await page.getByLabel('Version code').fill(versionCode);
  await page.getByLabel('APK file').setInputFiles(managedApkPath());
  await page.getByRole('button', { name: 'Create managed app' }).click();

  await expect(page).toHaveURL(/\/admin\/apps\/[^?]+\?ok=/);
  const appId = new URL(page.url()).pathname.split('/').pop() ?? '';
  expect(appId).not.toBe('');
  await expect(page.getByRole('heading', { name: 'App Detail' })).toBeVisible();
  await expect(page.getByText(name, { exact: true })).toBeVisible();
  await expect(page.getByText(packageName, { exact: true })).toBeVisible();
  return { appId, name, packageName };
}

async function createCertificate(page: Page, name: string, fileContent: string) {
  await page.goto(dashboardPaths.certificates);
  await page.getByLabel('Name').fill(name);
  await page.getByLabel('File').setInputFiles({
    name: `${name}.pem`,
    mimeType: 'application/x-pem-file',
    buffer: Buffer.from(fileContent),
  });
  await page.getByRole('button', { name: 'Upload certificate' }).click();

  const row = await findRowByText(page, name);
  const certificateId = (await row.locator('td').nth(1).textContent() ?? '').trim();
  expect(certificateId).not.toBe('');
  return { certificateId, name };
}

async function createPolicy(page: Page, name: string) {
  await page.goto(dashboardPaths.policies);
  await page.getByLabel('Name').fill(name);
  await page.getByRole('button', { name: 'Create policy' }).click();
  const row = await findRowByText(page, name);
  const policyId = (await row.locator('td').nth(1).textContent() ?? '').trim();
  expect(policyId).not.toBe('');
  await row.getByRole('link', { name }).click();
  await expect(page).toHaveURL(new RegExp(`${dashboardPaths.policyDetail(policyId)}$`));
  await expect(page.getByRole('heading', { name: 'Policy Detail' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Managed apps' })).toBeVisible();
  return policyId;
}

async function createPendingDevice(page: Page, name: string, policyId: string) {
  await page.goto(dashboardPaths.devices);
  await page.getByLabel('Display name').fill(name);
  await page.getByLabel('Policy').selectOption(policyId);
  await page.getByRole('button', { name: 'Create device' }).click();
  const row = await findRowByText(page, name);
  return idFromRowLink(row, name);
}

async function issueEnrollmentToken(page: Page, deviceRowId: string, deviceName: string) {
  await ensureAgentAppPublished(page);

  await page.goto(dashboardPaths.deviceDetail(deviceRowId));
  await expect(page.getByRole('heading', { name: 'Device Detail' })).toBeVisible();
  const currentDevice = page.getByRole('heading', { name: 'Current device' }).locator('xpath=ancestor::section[1]');
  const device = JSON.parse((await currentDevice.locator('pre').first().textContent()) ?? '{}') as {
    id: string;
    name: string;
  };
  expect(device.id).toBeTruthy();
  expect(device.name).toBe(deviceName);

  await page.getByRole('button', { name: 'Generate QR' }).click();
  await expect(page.getByText('QR generated')).toBeVisible();

  const qrSection = page.getByRole('heading', { name: 'QR JSON' }).locator('xpath=ancestor::section[1]');
  const payload = JSON.parse((await qrSection.locator('pre').first().textContent()) ?? '{}') as {
    ['android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE']: Record<string, string>;
  };
  const token = payload['android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE']['com.xmdm.ENROLLMENT_TOKEN'];
  expect(token).toBeTruthy();
  expect(payload['android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE']['com.xmdm.DEVICE_ID']).toBe(device.id);
  return { deviceId: device.id, token };
}

async function enrollDevice(page: Page, deviceId: string, token: string) {
  const { baseURL } = dashboardServerConfig();
  const response = await page.request.post(new URL('/api/v1/enrollment', baseURL).toString(), {
    data: {
      enrollmentToken: token,
      deviceIdentityPolicy: {
        deviceId,
      },
    },
  });
  if (!response.ok()) {
    throw new Error(`enrollment failed with ${response.status()}: ${await response.text()}`);
  }
  return response.json() as Promise<{ deviceId: string; deviceSecret: string; status: string }>;
}

async function fetchDeviceConfig(page: Page, deviceId: string, deviceSecret: string) {
  const { baseURL } = dashboardServerConfig();
  const response = await page.request.get(new URL(`/api/v1/devices/${deviceId}/config`, baseURL).toString(), {
    headers: {
      'X-XMDM-Device-Secret': deviceSecret,
    },
  });
  if (!response.ok()) {
    throw new Error(`config fetch failed with ${response.status()}: ${await response.text()}`);
  }
  return response.json() as Promise<{
    apps: Array<{
      appId: string;
      packageName: string;
      name?: string;
      versionId: string;
      versionName: string;
      versionCode: number;
      downloadPath: string;
    }>;
    certificates: Array<{
      id: string;
      name: string;
      artifactId: string;
      checksum: string;
      downloadPath: string;
    }>;
  }>;
}

test('admin can toggle managed apps and certificates on a policy', async ({ page }) => {
  const { username, password } = dashboardCredentials();
  const suffix = uniqueSuffix();
  const policyName = `playwright-policy-apps-${suffix}`;
  const assignedAppName = `playwright-assigned-app-${suffix}`;
  const unassignedAppName = `playwright-unassigned-app-${suffix}`;
  const assignedCertName = `playwright-assigned-cert-${suffix}`;
  const unassignedCertName = `playwright-unassigned-cert-${suffix}`;
  const deviceName = `playwright-policy-device-${suffix}`;

  await login(page, username, password);

  const assignedApp = await createManagedApp(
    page,
    assignedAppName,
    `com.example.assigned.${suffix}`,
    '101',
  );
  const unassignedApp = await createManagedApp(
    page,
    unassignedAppName,
    `com.example.unassigned.${suffix}`,
    '102',
  );
  const assignedCert = await createCertificate(
    page,
    assignedCertName,
    `-----BEGIN CERTIFICATE-----\nassigned-${suffix}\n-----END CERTIFICATE-----\n`,
  );
  const unassignedCert = await createCertificate(
    page,
    unassignedCertName,
    `-----BEGIN CERTIFICATE-----\nunassigned-${suffix}\n-----END CERTIFICATE-----\n`,
  );

  const policyId = await createPolicy(page, policyName);
  await page.goto(dashboardPaths.policyDetail(policyId));
  await expect(page.getByRole('heading', { name: 'Managed certificates' })).toBeVisible();
  const assignedRow = await findRowByText(page, assignedAppName);
  const unassignedRow = await findRowByText(page, unassignedAppName);
  const assignedCertRow = await findRowByText(page, assignedCertName);
  const unassignedCertRow = await findRowByText(page, unassignedCertName);
  await expect(assignedRow).toContainText('disabled');
  await expect(unassignedRow).toContainText('disabled');
  await expect(assignedCertRow).toContainText('disabled');
  await expect(unassignedCertRow).toContainText('disabled');
  await expect(assignedRow.getByRole('button', { name: 'Enable' })).toBeVisible();
  await expect(unassignedRow.getByRole('button', { name: 'Enable' })).toBeVisible();
  await expect(assignedCertRow.getByRole('button', { name: 'Enable' })).toBeVisible();
  await expect(unassignedCertRow.getByRole('button', { name: 'Enable' })).toBeVisible();

  await assignedRow.getByRole('button', { name: 'Enable' }).click();
  await expect(page).toHaveURL(new RegExp(`${dashboardPaths.policyDetail(policyId)}\\?ok=`));
  await expect(page.getByText('app enabled')).toBeVisible();
  const enabledAssignedRow = await findRowByText(page, assignedAppName);
  await expect(enabledAssignedRow).toContainText('enabled');
  await expect(enabledAssignedRow.getByRole('button', { name: 'Disable' })).toBeVisible();

  await unassignedRow.getByRole('button', { name: 'Enable' }).click();
  await expect(page).toHaveURL(new RegExp(`${dashboardPaths.policyDetail(policyId)}\\?ok=`));
  await expect(page.getByText('app enabled')).toBeVisible();
  const enabledUnassignedRow = await findRowByText(page, unassignedAppName);
  await expect(enabledUnassignedRow).toContainText('enabled');

  await assignedCertRow.getByRole('button', { name: 'Enable' }).click();
  await expect(page).toHaveURL(new RegExp(`${dashboardPaths.policyDetail(policyId)}\\?ok=`));
  await expect(page.getByText('certificate enabled')).toBeVisible();
  const enabledAssignedCertRow = await findRowByText(page, assignedCertName);
  await expect(enabledAssignedCertRow).toContainText('enabled');
  await expect(enabledAssignedCertRow.getByRole('button', { name: 'Disable' })).toBeVisible();

  const enabledAssignedRowAgain = await findRowByText(page, assignedAppName);
  await enabledAssignedRowAgain.getByRole('button', { name: 'Disable' }).click();
  await expect(page).toHaveURL(new RegExp(`${dashboardPaths.policyDetail(policyId)}\\?ok=`));
  await expect(page.getByText('app disabled')).toBeVisible();
  const disabledAssignedRow = await findRowByText(page, assignedAppName);
  await expect(disabledAssignedRow).toContainText('disabled');
  await expect(disabledAssignedRow.getByRole('button', { name: 'Enable' })).toBeVisible();

  const deviceRowId = await createPendingDevice(page, deviceName, policyId);
  const { deviceId, token } = await issueEnrollmentToken(page, deviceRowId, deviceName);
  const enrollment = await enrollDevice(page, deviceId, token);
  expect(enrollment.deviceId).toBe(deviceId);
  expect(enrollment.status).toBe('enrolled');

  const config = await fetchDeviceConfig(page, deviceId, enrollment.deviceSecret);
  expect(config.apps).toHaveLength(1);
  expect(config.apps[0].appId).toBe(unassignedApp.appId);
  expect(config.apps[0].packageName).toBe(unassignedApp.packageName);
  expect(config.apps[0].name).toBe(unassignedAppName);
  expect(config.apps[0].downloadPath).toContain(`/api/v1/devices/${deviceId}/apps/${unassignedApp.appId}/versions/`);
  expect(config.certificates).toHaveLength(1);
  expect(config.certificates[0].id).toBe(assignedCert.certificateId);
  expect(config.certificates[0].name).toBe(assignedCertName);
  expect(config.certificates[0].downloadPath).toBe(`/api/v1/devices/${deviceId}/certificates/${assignedCert.certificateId}/artifact`);
});
