import { expect, test } from '@playwright/test';
import type { Page } from '@playwright/test';

import { ensureAgentAppPublished } from '../support/apps';
import { dashboardCredentials } from '../support/auth';
import { dashboardPaths } from '../support/paths';
import { dashboardServerConfig } from '../support/server';

function uniqueSuffix() {
  return `${Date.now()}-${Math.random().toString(16).slice(2, 8)}`;
}

function formatDateTimeLocal(date: Date) {
  const pad = (value: number) => String(value).padStart(2, '0');
  return [
    date.getFullYear(),
    '-',
    pad(date.getMonth() + 1),
    '-',
    pad(date.getDate()),
    'T',
    pad(date.getHours()),
    ':',
    pad(date.getMinutes()),
  ].join('');
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

async function createPolicy(page: Page, name: string) {
  await page.goto(dashboardPaths.policies);
  await page.getByLabel('Name').fill(name);
  await page.getByRole('button', { name: 'Create policy' }).click();
  const row = await findRowByText(page, name);
  const policyId = (await row.locator('td').nth(1).textContent() ?? '').trim();
  expect(policyId).not.toBe('');
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

async function issueDeviceEnrollmentQR(page: Page, deviceName: string) {
  await ensureAgentAppPublished(page);

  await page.goto(dashboardPaths.devices);
  const row = await findRowByText(page, deviceName);
  const deviceRecordId = (await row.locator('td').nth(1).textContent() ?? '').trim();
  await row.getByRole('link', { name: deviceName }).click();
  await expect(page).toHaveURL(new RegExp(`/admin/devices/${deviceRecordId}$`));
  await expect(page.getByRole('heading', { name: 'Current device' })).toBeVisible();

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
  await expect(page.locator('img[alt="Enrollment QR preview"]')).toHaveAttribute('src', /data:image\/png;base64,/);
  return { deviceId: device.id, token, deviceRecordId };
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

async function createEnrolledDevice(page: Page, policyId: string, deviceName: string, groupName: string) {
  await page.goto(dashboardPaths.devices);
  await page.getByLabel('Display name').fill(deviceName);
  await page.getByLabel('Policy').selectOption(policyId);
  await page.getByLabel(groupName).check();
  await page.getByRole('button', { name: 'Create device' }).click();
  await expect(page).toHaveURL(new RegExp(`${dashboardPaths.devices}(\\?.*)?$`));

  const { deviceId, token, deviceRecordId } = await issueDeviceEnrollmentQR(page, deviceName);
  const enrollment = await enrollDevice(page, deviceId, token, {
    model: 'Pixel 8 Pro',
    serialNumber: `SN-${deviceName}`,
    manufacture: 'Google',
  });
  expect(enrollment.deviceId).toBe(deviceId);
  expect(enrollment.status).toBe('enrolled');
  expect(enrollment.deviceSecret).toBeTruthy();
  return { deviceId, deviceSecret: enrollment.deviceSecret, deviceRecordId };
}

async function pollDeviceCommands(page: Page, deviceId: string, deviceSecret: string) {
  const { baseURL } = dashboardServerConfig();
  const response = await page.request.get(new URL(`/api/v1/devices/${deviceId}/commands`, baseURL).toString(), {
    headers: {
      'X-XMDM-Device-Secret': deviceSecret,
    },
  });
  if (!response.ok()) {
    throw new Error(`device command fetch failed with ${response.status()}: ${await response.text()}`);
  }
  return response.json() as Promise<{ commands: Array<{ id: string; type: string; status: string }> }>;
}

async function acknowledgeDeviceCommand(page: Page, deviceId: string, deviceSecret: string, commandId: string, status: 'acked' | 'failed', message: string) {
  const { baseURL } = dashboardServerConfig();
  const response = await page.request.post(new URL(`/api/v1/devices/${deviceId}/commands/${commandId}/ack`, baseURL).toString(), {
    headers: {
      'X-XMDM-Device-Secret': deviceSecret,
    },
    data: {
      status,
      message,
    },
  });
  if (!response.ok()) {
    throw new Error(`device command ack failed with ${response.status}: ${await response.text()}`);
  }
  return response.json() as Promise<{ id: string; status: string }>;
}

async function createCommand(page: Page, options: {
  command: string;
  targetType: 'device' | 'group';
  targetDeviceId?: string;
  targetGroupId?: string;
  payload?: Record<string, unknown>;
  expiresAt?: Date;
}) {
  const setSelectValue = async (selector: string, value: string) => {
    await page.locator(selector).evaluate((element, selectedValue) => {
      const select = element as HTMLSelectElement;
      select.value = String(selectedValue);
      select.dispatchEvent(new Event('input', { bubbles: true }));
      select.dispatchEvent(new Event('change', { bubbles: true }));
    }, value);
  };
  await page.goto(dashboardPaths.commands);
  const commandSelect = page.getByLabel('Command');
  await expect(commandSelect).toHaveValue('ping');
  await expect(commandSelect.locator('option')).toHaveText(['ping', 'reboot', 'sync_config', 'exit_kiosk', 'launch_companion_app']);
  await expect(page.getByLabel('Target type').locator('option')).toHaveText(['Device', 'Group']);

  await setSelectValue('select[name="type"]', options.command);
  await setSelectValue('select[name="targetType"]', options.targetType);
  if (options.targetDeviceId) {
    await setSelectValue('select[name="targetDeviceId"]', options.targetDeviceId);
  }
  if (options.targetGroupId) {
    await setSelectValue('select[name="targetGroupId"]', options.targetGroupId);
  }
  await page.getByLabel('Payload JSON').fill(JSON.stringify(options.payload ?? {}));
  if (options.expiresAt) {
    await page.getByLabel('Expires at').fill(formatDateTimeLocal(options.expiresAt));
  }

  await page.getByRole('button', { name: 'Send command' }).click();
  await expect(page).toHaveURL(new RegExp(`${dashboardPaths.commands}(\\?.*)?$`));
}

test('admin can create commands for every target type and observe device ack and error states', async ({ page }) => {
  test.setTimeout(60_000);
  const { username, password } = dashboardCredentials();
  const suffix = uniqueSuffix();
  const policyName = `playwright-command-policy-${suffix}`;
  const groupName = `playwright-command-group-${suffix}`;
  const deviceOneName = `playwright-command-device-a-${suffix}`;
  const deviceTwoName = `playwright-command-device-b-${suffix}`;

  await login(page, username, password);

  const policyId = await createPolicy(page, policyName);
  const groupId = await createGroup(page, groupName);
  const deviceOne = await createEnrolledDevice(page, policyId, deviceOneName, groupName);
  const deviceTwo = await createEnrolledDevice(page, policyId, deviceTwoName, groupName);

  await page.goto(dashboardPaths.commands);
  await expect(page.getByRole('heading', { name: 'Commands' })).toBeVisible();
  await expect(page.getByLabel('Command').locator('option')).toHaveText(['ping', 'reboot', 'sync_config', 'exit_kiosk', 'launch_companion_app']);
  await expect(page.getByLabel('Target type').locator('option')).toHaveText(['Device', 'Group']);
  const deviceOptions = await page.getByLabel('Device').locator('option').evaluateAll((options) => options.map((option) => (option as HTMLOptionElement).value));
  expect(deviceOptions).toContain(deviceOne.deviceId);
  expect(deviceOptions).toContain(deviceTwo.deviceId);
  const groupOptions = await page.getByLabel('Group').locator('option').evaluateAll((options) => options.map((option) => (option as HTMLOptionElement).value));
  expect(groupOptions).toContain(groupId);

  const expiresAt = new Date(Date.now() + 60 * 60 * 1000);

  await createCommand(page, {
    command: 'ping',
    targetType: 'device',
    targetDeviceId: deviceOne.deviceId,
    payload: { reason: 'device-target' },
    expiresAt,
  });
  const deviceQueued = await pollDeviceCommands(page, deviceOne.deviceId, deviceOne.deviceSecret);
  expect(deviceQueued.commands).toHaveLength(1);
  expect(deviceQueued.commands[0].type).toBe('ping');
  const deviceCommandId = deviceQueued.commands[0].id;
  await page.getByRole('link', { name: deviceCommandId }).click();
  await expect(page).toHaveURL(new RegExp(`/admin/commands/${deviceCommandId}$`));
  await expect(page.getByRole('heading', { name: 'Current command' })).toBeVisible();
  await expect(page.getByText('Payload', { exact: true })).toBeVisible();
  await expect(page.getByText('device-target', { exact: true })).toBeVisible();
  await acknowledgeDeviceCommand(page, deviceOne.deviceId, deviceOne.deviceSecret, deviceCommandId, 'acked', 'pong');
  await page.goto(dashboardPaths.commands);
  await expect(await findRowByText(page, deviceCommandId)).toContainText('acked');

  await createCommand(page, {
    command: 'ping',
    targetType: 'group',
    targetGroupId: groupId,
    payload: { reason: 'group-target' },
  });
  const groupQueuedOne = await pollDeviceCommands(page, deviceOne.deviceId, deviceOne.deviceSecret);
  const groupQueuedTwo = await pollDeviceCommands(page, deviceTwo.deviceId, deviceTwo.deviceSecret);
  expect(groupQueuedOne.commands).toHaveLength(1);
  expect(groupQueuedTwo.commands).toHaveLength(1);
  const groupCommandIdOne = groupQueuedOne.commands[0].id;
  const groupCommandIdTwo = groupQueuedTwo.commands[0].id;
  await acknowledgeDeviceCommand(page, deviceOne.deviceId, deviceOne.deviceSecret, groupCommandIdOne, 'acked', 'pong');
  await acknowledgeDeviceCommand(page, deviceTwo.deviceId, deviceTwo.deviceSecret, groupCommandIdTwo, 'acked', 'pong');
  await page.goto(dashboardPaths.commands);
  await expect(await findRowByText(page, groupCommandIdOne)).toContainText('acked');
  await expect(await findRowByText(page, groupCommandIdTwo)).toContainText('acked');

  await createCommand(page, {
    command: 'reboot',
    targetType: 'device',
    targetDeviceId: deviceOne.deviceId,
    payload: { reason: 'error-target' },
  });
  const errorQueued = await pollDeviceCommands(page, deviceOne.deviceId, deviceOne.deviceSecret);
  expect(errorQueued.commands).toHaveLength(1);
  expect(errorQueued.commands[0].type).toBe('reboot');
  const errorCommandId = errorQueued.commands[0].id;
  await acknowledgeDeviceCommand(page, deviceOne.deviceId, deviceOne.deviceSecret, errorCommandId, 'failed', 'reboot unavailable');
  await page.goto(dashboardPaths.commands);
  await expect(await findRowByText(page, errorCommandId)).toContainText('failed');
});
