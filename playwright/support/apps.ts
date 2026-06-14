import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';

import { expect, type Page } from '@playwright/test';

import { dashboardPaths } from './paths';

export const AGENT_APP_PACKAGE = 'com.xmdm.launcher';
export const AGENT_APP_NAME = 'XMDM Agent';
export const AGENT_APP_VERSION_CODE = '999999';

let fakeApkPath: string | null = null;

function ensureFakeApkPath() {
  if (fakeApkPath) {
    return fakeApkPath;
  }

  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'xmdm-playwright-apk-'));
  fakeApkPath = path.join(dir, 'fixture.apk');
  fs.writeFileSync(fakeApkPath, Buffer.from('xmdm-playwright-fake-apk\n'));
  return fakeApkPath;
}

export function agentApkPath() {
  return ensureFakeApkPath();
}

export function managedApkPath() {
  return ensureFakeApkPath();
}

async function findAgentAppRow(page: Page) {
  await page.goto(dashboardPaths.apps);
  await expect(page.getByRole('heading', { name: 'Apps' })).toBeVisible();

  for (;;) {
    const row = page.locator('table tbody tr').filter({ hasText: AGENT_APP_PACKAGE }).first();
    if (await row.count() > 0) {
      await expect(row).toBeVisible();
      return row;
    }

    const next = page.getByRole('link', { name: 'Next' });
    if (await next.count() === 0) {
      break;
    }

    await next.click();
    await expect(page.getByRole('heading', { name: 'Apps' })).toBeVisible();
  }

  return null;
}

export async function ensureAgentAppPublished(page: Page) {
  const existing = await findAgentAppRow(page);
  if (existing) {
    return;
  }

  await page.getByLabel('Package name').fill(AGENT_APP_PACKAGE);
  await page.getByLabel('App name').fill(AGENT_APP_NAME);
  await page.getByLabel('Version code').fill(AGENT_APP_VERSION_CODE);
  await page.getByLabel('APK file').setInputFiles(agentApkPath());
  await page.getByRole('button', { name: 'Create managed app' }).click();

  if (page.url().includes('error=conflict')) {
    const row = await findAgentAppRow(page);
    if (!row) {
      throw new Error(`agent app ${AGENT_APP_PACKAGE} was created but could not be found in the apps list`);
    }
    await row.getByRole('link', { name: AGENT_APP_NAME }).click();
  } else {
    await expect(page).toHaveURL(/\/admin\/apps\/[^?]+\?ok=/);
  }
  await expect(page.getByRole('heading', { name: 'App Detail' })).toBeVisible();
  await expect(page.getByText(AGENT_APP_PACKAGE, { exact: true })).toBeVisible();
}
