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

async function findRowByText(page: Page, text: string) {
  const row = page.locator('table tbody tr').filter({ hasText: text }).first();
  await expect(row).toBeVisible();
  return row;
}

async function issueDeviceEnrollmentQR(page: Page, deviceName: string) {
  await ensureAgentAppPublished(page);

  await page.goto(dashboardPaths.devices);
  const row = await findRowByText(page, deviceName);
  const deviceId = (await row.locator('td').nth(1).textContent() ?? '').trim();
  await row.getByRole('link', { name: deviceName }).click();
  await expect(page).toHaveURL(new RegExp(`/admin/devices/${deviceId}$`));
  await expect(page.getByRole('heading', { name: 'Device Detail' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Active policy' })).toBeVisible();
  await expect(page.getByRole('radiogroup', { name: 'Output format' })).toHaveCount(0);
  const currentDevice = page.getByRole('heading', { name: 'Current device' }).locator('xpath=ancestor::section[1]');
  const device = JSON.parse((await currentDevice.locator('pre').first().textContent()) ?? '{}') as { id: string; name: string };

  await page.getByRole('button', { name: 'Generate QR' }).click();

  await expect(page.getByText('QR generated')).toBeVisible();
  const qrSection = page.getByRole('heading', { name: 'QR JSON' }).locator('xpath=ancestor::section[1]');
  const payload = JSON.parse((await qrSection.locator('pre').first().textContent()) ?? '{}') as {
    ['android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE']: Record<string, string>;
  };
  const token = payload['android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE']['com.xmdm.ENROLLMENT_TOKEN'];
  expect(token).toBeTruthy();
  expect(device.id).toBeTruthy();
  expect(device.name).toBe(deviceName);
  expect(payload['android.app.extra.PROVISIONING_ADMIN_EXTRAS_BUNDLE']['com.xmdm.DEVICE_ID']).toBe(device.id);

  await expect(page.getByRole('heading', { name: 'QR preview' })).toBeVisible();
  await expect(page.locator('img[alt="Enrollment QR preview"]')).toHaveAttribute('src', /data:image\/png;base64,/);

  return { deviceId: device.id, token };
}

async function enrollDevice(page: Page, deviceId: string, token: string, bootstrapExtras: Record<string, unknown>) {
  const { baseURL } = dashboardServerConfig();
  const response = await page.request.post(new URL('/api/v1/enrollment', baseURL).toString(), {
    data: {
      enrollmentToken: token,
      deviceIdentityPolicy: {
        deviceId,
      },
      bootstrapExtras,
    },
  });
  if (!response.ok()) {
    throw new Error(`enrollment failed with ${response.status()}: ${await response.text()}`);
  }
  return response.json() as Promise<{ deviceId: string; deviceSecret: string; status: string }>;
}

async function createPolicy(page: Page, name: string, options?: { kioskMode?: boolean }) {
  await page.goto(dashboardPaths.policies);
  await page.getByLabel('Name').fill(name);
  if (options?.kioskMode) {
    await page.getByLabel('Enable kiosk mode').check();
    await page.getByLabel('Kiosk app package').fill('com.android.chrome');
    await page.getByLabel('Kiosk exit passcode').fill('1234');
    await page.getByLabel('Allow packages').fill('com.android.chrome');
  }
  await page.getByRole('button', { name: 'Create policy' }).click();
  const policyRow = await findRowByText(page, name);
  const policyId = (await policyRow.locator('td').nth(1).textContent() ?? '').trim();
  expect(policyId).not.toBe('');
  await policyRow.getByRole('link', { name }).click();
  await expect(page).toHaveURL(new RegExp(`${dashboardPaths.policyDetail(policyId)}$`));
  await expect(page.getByRole('heading', { name: 'Policy Detail' })).toBeVisible();
  if (options?.kioskMode) {
    await expect(page.getByLabel('Enable kiosk mode')).toBeChecked();
  } else {
    await expect(page.getByLabel('Enable kiosk mode')).not.toBeChecked();
  }
  await expect(page.getByLabel('Kiosk exit passcode')).toBeVisible();
  await expect(page.getByLabel('Allow packages')).toHaveAttribute('placeholder', /One package per line/);
  const kioskModeOffset = await page.getByLabel('Enable kiosk mode').evaluate((node) => (node as HTMLElement).getBoundingClientRect().top);
  const passcodeOffset = await page.getByLabel('Kiosk exit passcode').evaluate((node) => (node as HTMLElement).getBoundingClientRect().top);
  expect(passcodeOffset).toBeGreaterThan(kioskModeOffset);
  const policyBody = await page.locator('main').textContent();
  expect(policyBody ?? '').not.toContain('Available policy fields:');
  return policyId;
}

async function createGroup(page: Page, name: string) {
  await page.goto(dashboardPaths.groups);
  await page.getByLabel('Name').fill(name);
  await page.getByRole('button', { name: 'Create group' }).click();
  const row = await findRowByText(page, name);
  const groupId = (await row.locator('td').nth(1).textContent() ?? '').trim();
  expect(groupId).not.toBe('');
  return groupId;
}

async function uploadDeviceInfo(page: Page, deviceId: string, deviceSecret: string, payload: Record<string, unknown>) {
  const { baseURL } = dashboardServerConfig();
  const response = await page.request.post(new URL(`/api/v1/devices/${deviceId}/info`, baseURL).toString(), {
    headers: {
      'X-XMDM-Device-Secret': deviceSecret,
    },
      data: {
        payload,
      },
  });
  if (!response.ok()) {
    throw new Error(`device info upload failed with ${response.status()}: ${await response.text()}`);
  }
  return response.json() as Promise<{ deviceInfo: Array<{ id: string; deviceId: string; payload: Record<string, unknown> }> }>;
}

async function fetchDeviceConfig(page: Page, deviceId: string, deviceSecret: string) {
  const { baseURL } = dashboardServerConfig();
  const response = await page.request.get(new URL(`/api/v1/devices/${deviceId}/config`, baseURL).toString(), {
    headers: {
      'X-XMDM-Device-Secret': deviceSecret,
    },
  });
  if (!response.ok()) {
    throw new Error(`device config fetch failed with ${response.status()}: ${await response.text()}`);
  }
  return response.json() as Promise<Record<string, unknown>>;
}

test('admin can edit and retire a device from the detail page', async ({ page }) => {
  const { username, password } = dashboardCredentials();
  const suffix = uniqueSuffix();
  const policyName = `playwright-device-policy-${suffix}`;
  const deviceName = `playwright-device-${suffix}`;
  const deviceUpdatedName = `${deviceName}-updated`;

  let policyId = '';
  let deviceId = '';

  await login(page, username, password);

  try {
    policyId = await createPolicy(page, policyName, { kioskMode: true });
    await createGroup(page, `${deviceName}-field`);
    await createGroup(page, `${deviceName}-ops`);

    await page.goto(dashboardPaths.devices);
    await expect(page.getByRole('heading', { name: 'Devices' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Created' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'ID' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Name' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Policy' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Status' })).toBeVisible();
    await expect(page.getByRole('group', { name: 'Groups' })).toBeVisible();
    await page.getByLabel('Display name').fill(deviceName);
    await page.getByLabel('Policy').selectOption(policyId);
    await page.getByLabel(`${deviceName}-field`).check();
    await page.getByLabel(`${deviceName}-ops`).check();
    await page.getByRole('button', { name: 'Create device' }).click();
    await expect(page).toHaveURL(new RegExp(`${dashboardPaths.devices}(\\?.*)?$`));
    const deviceRow = await findRowByText(page, deviceName);
    deviceId = (await deviceRow.locator('td').nth(1).textContent() ?? '').trim();
    expect(deviceId).not.toBe('');

    await deviceRow.getByRole('link', { name: deviceName }).click();
    await expect(page).toHaveURL(new RegExp(`/admin/devices/${deviceId}$`));
    await expect(page.getByRole('heading', { name: 'Device Detail' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Update device' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Retire device' })).toBeVisible();
    await expect(page.getByText('Current device')).toBeVisible();
    await expect(page.getByLabel('Device secret')).toHaveCount(0);
    await expect(page.getByRole('group', { name: 'Groups' })).toBeVisible();
    await expect(page.getByLabel(`${deviceName}-field`)).toBeChecked();
    await expect(page.getByLabel(`${deviceName}-ops`)).toBeChecked();

    await page.getByLabel('Display name').fill(deviceUpdatedName);
    await page.getByLabel('Policy').selectOption(policyId);
    await page.getByLabel(`${deviceName}-ops`).uncheck();
    await page.getByRole('button', { name: 'Update device' }).click();
    await expect(page).toHaveURL(new RegExp(`/admin/devices/${deviceId}(\\?.*)?$`));

    await page.goto(dashboardPaths.devices);
    await page.getByRole('link', { name: deviceUpdatedName }).click();
    await expect(page).toHaveURL(new RegExp(`/admin/devices/${deviceId}$`));
    await expect(page.getByLabel(`${deviceName}-field`)).toBeChecked();
    await expect(page.getByLabel(`${deviceName}-ops`)).not.toBeChecked();
    await page.getByRole('button', { name: 'Retire device' }).click();
    await expect(page).toHaveURL(new RegExp(`${dashboardPaths.devices}(\\?.*)?$`));

    await page.goto(dashboardPaths.deviceDetail(deviceId));
    await expect(page.getByRole('heading', { name: 'Device Detail' })).toBeVisible();
    const retiredDevice = JSON.parse((await page.getByRole('heading', { name: 'Current device' }).locator('xpath=ancestor::section[1]').locator('pre').first().textContent()) ?? '{}') as { status: string };
    expect(retiredDevice.status).toBe('retired');
    await expect(page.getByRole('button', { name: 'Update device' })).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Retire device' })).toHaveCount(0);
  } finally {
    if (deviceId) {
      await page.goto(dashboardPaths.deviceDetail(deviceId)).catch(() => undefined);
    }
  }
});

test('admin can simulate device enrollment and inspect device info', async ({ page }) => {
  const { username, password } = dashboardCredentials();
  const suffix = uniqueSuffix();
  const policyName = `playwright-enrollment-policy-${suffix}`;
  const deviceName = `playwright-enrollment-device-${suffix}`;
  const deviceModel = 'Pixel 8 Pro';
  const serialNumber = `SN-${suffix}`;

  let policyId = '';
  let deviceRowId = '';

  await login(page, username, password);

  try {
    policyId = await createPolicy(page, policyName);
    await createGroup(page, `${deviceName}-field`);

    await page.goto(dashboardPaths.devices);
    await page.getByLabel('Display name').fill(deviceName);
    await page.getByLabel('Policy').selectOption(policyId);
    await page.getByLabel(`${deviceName}-field`).check();
    await page.getByRole('button', { name: 'Create device' }).click();
    const deviceRow = await findRowByText(page, deviceName);
    deviceRowId = (await deviceRow.locator('td').nth(1).textContent() ?? '').trim();
    expect(deviceRowId).not.toBe('');

    const { deviceId, token } = await issueDeviceEnrollmentQR(page, deviceName);

    const enrollment = await enrollDevice(page, deviceId, token, {
      model: deviceModel,
      serialNumber,
      manufacture: 'Google',
    });
    expect(enrollment.deviceId).toBe(deviceId);
    expect(enrollment.status).toBe('enrolled');
    expect(enrollment.deviceSecret).toBeTruthy();

    const config = await fetchDeviceConfig(page, deviceId, enrollment.deviceSecret);

    const deviceInfo = await uploadDeviceInfo(page, deviceId, enrollment.deviceSecret, {
      model: deviceModel,
      serialNumber,
      batteryLevel: 87,
      network: 'wifi',
    });
    expect(deviceInfo.deviceInfo).toHaveLength(1);

    await page.goto(dashboardPaths.devices);
    const enrolledRow = await findRowByText(page, deviceName);
    await expect(enrolledRow).toContainText('active');
    await expect(enrolledRow).toContainText(policyName);

    await enrolledRow.getByRole('link', { name: deviceName }).click();
    await expect(page).toHaveURL(new RegExp(`/admin/devices/${deviceRowId}$`));
    const currentDevice = page.getByRole('heading', { name: 'Current device' }).locator('xpath=ancestor::section[1]');
    const activePolicy = page.getByRole('heading', { name: 'Active policy' }).locator('xpath=ancestor::section[1]');
    const recentInfo = page.getByRole('heading', { name: 'Recent device info' }).locator('xpath=ancestor::section[1]');

    await expect(currentDevice).toContainText(`"id": "${deviceRowId}"`);
    await expect(currentDevice).toContainText(`"status": "active"`);
    await expect(currentDevice).toContainText(policyId);
    await expect(activePolicy).toContainText(policyName);
    await expect(activePolicy.getByRole('link', { name: policyName })).toHaveAttribute('href', dashboardPaths.policyDetail(policyId));
    await expect(page.getByRole('group', { name: 'Groups' })).toBeVisible();
    await expect(page.getByLabel(`${deviceName}-field`)).toBeChecked();
    const previewConfig = { ...config } as Record<string, unknown>;
    delete previewConfig.signature;
    const configPreview = page.getByRole('heading', { name: 'Config preview' }).locator('xpath=ancestor::section[1]');
    await expect(configPreview.locator('pre')).toHaveText(JSON.stringify(previewConfig, null, 2));
    await expect(recentInfo).toContainText(`"model": "${deviceModel}"`);
    await expect(recentInfo).toContainText(`"serialNumber": "${serialNumber}"`);
    await expect(recentInfo).toContainText('"batteryLevel": 87');

    const { baseURL } = dashboardServerConfig();
    const response = await page.request.get(new URL(`/api/v1/device-info?deviceId=${deviceId}&limit=50`, baseURL).toString());
    expect(response.ok()).toBeTruthy();
    const body = await response.json() as { deviceInfo: Array<{ deviceId: string; payload: Record<string, unknown> }> };
    expect(body.deviceInfo).toHaveLength(1);
    expect(body.deviceInfo[0].deviceId).toBe(deviceId);
    expect(body.deviceInfo[0].payload).toMatchObject({
      model: deviceModel,
      serialNumber,
      batteryLevel: 87,
    });
  } finally {
    if (deviceRowId) {
      await page.goto(dashboardPaths.deviceDetail(deviceRowId)).catch(() => undefined);
    }
  }
});

test('admin can inspect group detail and member devices', async ({ page }) => {
  const { username, password } = dashboardCredentials();
  const suffix = uniqueSuffix();
  const policyName = `playwright-group-policy-${suffix}`;
  const groupName = `playwright-group-${suffix}`;
  const deviceName = `playwright-group-device-${suffix}`;

  let policyId = '';
  let groupId = '';
  let deviceId = '';

  await login(page, username, password);

  try {
    policyId = await createPolicy(page, policyName);
    groupId = await createGroup(page, groupName);

    await page.goto(dashboardPaths.devices);
    await page.getByLabel('Display name').fill(deviceName);
    await page.getByLabel('Policy').selectOption(policyId);
    await page.getByLabel(groupName).check();
    await page.getByRole('button', { name: 'Create device' }).click();
    const deviceRow = await findRowByText(page, deviceName);
    deviceId = (await deviceRow.locator('td').nth(1).textContent() ?? '').trim();
    expect(deviceId).not.toBe('');

    await page.goto(dashboardPaths.groups);
    await expect(page.getByRole('columnheader', { name: 'Created' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'ID' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Name' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Status' })).toBeVisible();
    const groupRow = await findRowByText(page, groupName);
    await expect(groupRow).toContainText(groupId);
    await expect(groupRow).toContainText('active');
    await groupRow.getByRole('link', { name: groupName }).click();
    await expect(page).toHaveURL(new RegExp(dashboardPaths.groupDetail(groupId).replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '$'));
    await expect(page.getByRole('heading', { name: 'Group Detail' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Update group' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Retire group' })).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Member devices' })).toBeVisible();
    await expect(page.getByRole('link', { name: deviceName })).toHaveAttribute('href', dashboardPaths.deviceDetail(deviceId));
    await expect(page.getByLabel('Name')).toHaveValue(groupName);

    await page.getByLabel('Name').fill(`${groupName}-updated`);
    await page.getByRole('button', { name: 'Update group' }).click();
    await expect(page).toHaveURL(new RegExp(dashboardPaths.groupDetail(groupId).replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '(\\?.*)?$'));

    await page.goto(dashboardPaths.groups);
    await page.getByRole('link', { name: `${groupName}-updated` }).click();
    await expect(page).toHaveURL(new RegExp(`${dashboardPaths.groupDetail(groupId)}$`));
    await page.getByRole('button', { name: 'Retire group' }).click();
    await expect(page).toHaveURL(new RegExp(`${dashboardPaths.groups}(\\?.*)?$`));
  } finally {
    if (groupId) {
      await page.goto(dashboardPaths.groupDetail(groupId)).catch(() => undefined);
    }
  }
});
