import path from 'node:path';

import { expect, type Page } from '@playwright/test';

import { dashboardPaths } from './paths';

export const AGENT_APP_PACKAGE = 'com.xmdm.launcher';
export const AGENT_APP_NAME = 'XMDM Agent';
export const AGENT_APP_VERSION_CODE = '999999';

export function agentApkPath() {
  return path.resolve(process.cwd(), '..', 'artifacts', 'chrome.apk');
}

export async function ensureAgentAppPublished(page: Page) {
  await page.goto(dashboardPaths.apps);
  await expect(page.getByRole('heading', { name: 'Apps' })).toBeVisible();

  await page.getByLabel('Package name').fill(AGENT_APP_PACKAGE);
  await page.getByLabel('App name').fill(AGENT_APP_NAME);
  await page.getByLabel('Version code').fill(AGENT_APP_VERSION_CODE);
  await page.getByLabel('APK file').setInputFiles(agentApkPath());
  await page.getByRole('button', { name: 'Create managed app' }).click();

  await expect(page).toHaveURL(/\/admin\/apps\/[^?]+\?ok=/);
  await expect(page.getByRole('heading', { name: 'App Detail' })).toBeVisible();
  await expect(page.getByText(AGENT_APP_PACKAGE, { exact: true })).toBeVisible();
}
