const { chromium } = require('playwright');
const path = require('path');

const base = process.env.XMDM_DASHBOARD_URL || 'http://127.0.0.1:39091';
const out = path.resolve(__dirname, '../../../docs/assets');
const username = process.env.XMDM_DASHBOARD_USERNAME || 'admin';
const password = process.env.XMDM_DASHBOARD_PASSWORD || 'admin';
const executablePath = process.env.CHROME_EXECUTABLE || chromium.executablePath();

const shots = [
  ['admin-dashboard-login.png', '/admin/login', false],
  ['admin-dashboard-overview.png', '/admin', true],
  ['admin-dashboard-users.png', '/admin/users', true],
  ['admin-dashboard-user-detail.png', '/admin/users/user-admin', true],
  ['admin-dashboard-roles.png', '/admin/roles', true],
  ['admin-dashboard-role-detail.png', '/admin/roles/role-admin', true],
  ['admin-dashboard-groups.png', '/admin/groups', true],
  ['admin-dashboard-group-detail.png', '/admin/groups/group-field', true],
  ['admin-dashboard-policies.png', '/admin/policies', true],
  ['admin-dashboard-policy-detail.png', '/admin/policies/policy-baseline', true],
  ['admin-dashboard-devices.png', '/admin/devices', true],
  ['admin-dashboard-device-detail.png', '/admin/devices/device-008', true],
  ['admin-dashboard-device-remote-control.png', '/admin/devices/5dcd5e7a-0642-4718-924f-c9bf904ea194', true],
  ['admin-dashboard-device-qr.png', '/admin/devices/device-008', true],
  ['admin-dashboard-apps.png', '/admin/apps', true],
  ['admin-dashboard-app-detail.png', '/admin/apps/app-chrome', true],
  ['admin-dashboard-managed-files.png', '/admin/managed-files', true],
  ['admin-dashboard-managed-file-detail.png', '/admin/managed-files/managed-file-config', true],
  ['admin-dashboard-certificates.png', '/admin/certificates', true],
  ['admin-dashboard-certificate-detail.png', '/admin/certificates/cert-root', true],
  ['admin-dashboard-commands.png', '/admin/commands', true],
  ['admin-dashboard-command-detail.png', '/admin/commands/cmd-01', true],
  ['admin-dashboard-audit.png', '/admin/audit', true],
];

(async () => {
  const browser = await chromium.launch({
    headless: true,
    executablePath,
    args: ['--no-sandbox'],
  });
  const page = await browser.newPage({ viewport: { width: 1440, height: 1000 }, deviceScaleFactor: 1 });

  for (const [name, path, requiresAuth] of shots) {
    if (requiresAuth && !page.url().startsWith(base + '/admin')) {
      await login(page);
    }
    if (requiresAuth && page.url().endsWith('/admin/login')) {
      await login(page);
    }
    await page.goto(base + path, { waitUntil: 'networkidle' });
    if (requiresAuth && page.url().endsWith('/admin/login')) {
      await login(page);
      await page.goto(base + path, { waitUntil: 'networkidle' });
    }
    if (name === 'admin-dashboard-device-qr.png') {
      await page.getByRole('button', { name: 'Generate QR' }).click();
      await page.getByAltText('Enrollment QR preview').waitFor({ state: 'visible' });
    }
    if (name === 'admin-dashboard-device-remote-control.png') {
      await page.getByRole('link', { name: 'Remote Control' }).click();
      await page.waitForTimeout(2500);
      if ((await page.title()) === 'Remote Control Session Blocked') {
        await page.getByRole('button', { name: 'Cancel session and retry' }).click();
      }
      await page.waitForFunction(() => {
        const body = document.body.innerText;
        return body.includes('PREMIUM REMOTE CONTROL') && body.includes('Close session') && body.includes('connected');
      }, null, { timeout: 60000 });
    }
    await page.mouse.move(1300, 90);
    await page.screenshot({ path: `${out}/${name}`, fullPage: true });
  }

  await browser.close();
})();

async function login(page) {
  await page.goto(base + '/admin/login', { waitUntil: 'networkidle' });
  await page.fill('input[name="username"]', username);
  await page.fill('input[name="password"]', password);
  await Promise.all([
    page.waitForURL(base + '/admin'),
    page.click('button[type="submit"]'),
  ]);
}
