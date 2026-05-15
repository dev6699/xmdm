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

async function findRowByText(page: Page, text: string) {
  const row = page.locator('table tbody tr').filter({ hasText: text }).first();
  await expect(row).toBeVisible();
  return row;
}

test('admin can manage files and certificates from scan-first list pages', async ({ page }) => {
  const { username, password } = dashboardCredentials();
  const suffix = uniqueSuffix();

  const fileContent = `managed file ${suffix}`;
  const fileName = `playwright-file-${suffix}.txt`;
  const managedFilePath = `/sdcard/xmdm/${fileName}`;

  const certName = `playwright-cert-${suffix}`;
  const certContent = `-----BEGIN CERTIFICATE-----\nplaywright-${suffix}\n-----END CERTIFICATE-----\n`;

  await login(page, username, password);

  await page.goto(dashboardPaths.managedFiles);
  await expect(page.getByRole('columnheader', { name: 'Created' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'ID' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Path' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'File' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Template' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Status' })).toBeVisible();

  await page.getByLabel('Device path').fill(managedFilePath);
  await page.getByLabel('Replace variables').check();
  await page.getByLabel('File').setInputFiles({
    name: fileName,
    mimeType: 'text/plain',
    buffer: Buffer.from(fileContent),
  });
  await page.getByRole('button', { name: 'Upload managed file' }).click();

  const managedFileRow = await findRowByText(page, managedFilePath);
  const managedFileId = (await managedFileRow.locator('td').nth(1).textContent() ?? '').trim();
  expect(managedFileId).not.toBe('');
  await expect(managedFileRow).toContainText('enabled');
  await expect(managedFileRow.getByRole('link', { name: managedFilePath })).toHaveAttribute('href', dashboardPaths.managedFileDetail(managedFileId));

  await managedFileRow.getByRole('link', { name: managedFilePath }).click();
  await expect(page).toHaveURL(new RegExp(`${dashboardPaths.managedFileDetail(managedFileId)}$`));
  const managedFileDetail = page.getByRole('heading', { name: 'Current managed file' }).locator('xpath=ancestor::section[1]');
  await expect(managedFileDetail).toContainText(managedFilePath);
  await expect(managedFileDetail).toContainText(fileName);
  await expect(page.getByRole('button', { name: 'Retire managed file' })).toBeVisible();

  await page.goto(dashboardPaths.managedFiles);
  await page.getByLabel('Device path').fill(managedFilePath);
  await page.getByLabel('File').setInputFiles({
    name: fileName,
    mimeType: 'text/plain',
    buffer: Buffer.from(`${fileContent} updated`),
  });
  await page.getByRole('button', { name: 'Upload managed file' }).click();

  const replacedManagedFileRow = await findRowByText(page, managedFilePath);
  const replacedManagedFileId = (await replacedManagedFileRow.locator('td').nth(1).textContent() ?? '').trim();
  expect(replacedManagedFileId).toBe(managedFileId);
  await expect(replacedManagedFileRow).toContainText('enabled');
  await replacedManagedFileRow.getByRole('link', { name: managedFilePath }).click();
  await expect(page).toHaveURL(new RegExp(`${dashboardPaths.managedFileDetail(managedFileId)}$`));
  const replacedManagedFileDetail = page.getByRole('heading', { name: 'Current managed file' }).locator('xpath=ancestor::section[1]');
  await expect(replacedManagedFileDetail).toContainText(managedFilePath);

  await page.goto(dashboardPaths.certificates);
  await expect(page.getByRole('columnheader', { name: 'Created' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'ID' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Name' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Artifact' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Status' })).toBeVisible();

  await expect(page.getByLabel('Storage key')).toHaveCount(0);
  await expect(page.getByLabel('Checksum')).toHaveCount(0);
  await expect(page.getByLabel('Size bytes')).toHaveCount(0);
  await expect(page.getByLabel('MIME type')).toHaveCount(0);

  await page.getByLabel('Name').fill(certName);
  await page.getByLabel('File').setInputFiles({
    name: `${certName}.pem`,
    mimeType: 'application/x-pem-file',
    buffer: Buffer.from(certContent),
  });
  await page.getByRole('button', { name: 'Upload certificate' }).click();

  const certRow = await findRowByText(page, certName);
  const certId = (await certRow.locator('td').nth(1).textContent() ?? '').trim();
  expect(certId).not.toBe('');
  await expect(certRow.getByRole('link', { name: certName })).toHaveAttribute('href', dashboardPaths.certificateDetail(certId));

  await certRow.getByRole('link', { name: certName }).click();
  await expect(page).toHaveURL(new RegExp(`${dashboardPaths.certificateDetail(certId)}$`));
  const certificateDetail = page.getByRole('heading', { name: 'Current certificate' }).locator('xpath=ancestor::section[1]');
  await expect(certificateDetail).toContainText(certName);
  await expect(certificateDetail).toContainText('artifacts/certificates/');
  await expect(page.getByRole('button', { name: 'Retire certificate' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Download certificate' })).toHaveAttribute('href', `/admin/certificates/${certId}/download`);
});
