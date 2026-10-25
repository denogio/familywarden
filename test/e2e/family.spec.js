const { test, expect } = require('@playwright/test');

// Requests page: the shared administrator UI (A20) mounted in LuCI.
const BASE = process.env.FW_BASE_URL || 'http://localhost:8080';
const URL = `${BASE}/cgi-bin/luci/admin/services/familywarden/family`;
test.use({ locale: 'en-GB', timezoneId: 'UTC' });

async function login(page) {
  await page.goto(`${BASE}/cgi-bin/luci/admin/login`);
  await page.fill('[name="luci_username"]', 'root');
  await page.fill('[name="luci_password"]', process.env.FW_PASSWORD ?? 'ulysses');
  await page.getByRole('button', { name: 'Log in' }).click();
  await page.waitForLoadState('networkidle');
}

const pageErrors = new WeakMap();
test.beforeEach(async ({ page }) => {
  const errors = [];
  pageErrors.set(page, errors);
  await login(page);
  page.on('pageerror', (error) => errors.push(error.message));
});
test.afterEach(async ({ page }) => {
  await page.unrouteAll({ behavior: 'wait' });
  expect(pageErrors.get(page)).toEqual([]);
});

test('Requests page talks to the real core through rpcd and LuCI ACLs', async ({ page }) => {
  await page.goto(URL);
  await expect(page.getByRole('heading', { name: 'Requests and screen time' })).toBeVisible();
  await expect(page.getByText('No requests waiting.')).toBeVisible();
  await expect(page.getByText(/^Updated /)).toBeVisible();
  await expect(page.getByText(/Could not reach/)).toHaveCount(0);
});

test('approving sends exactly the chosen minutes and nothing for another request', async ({ page }) => {
  const now = Math.floor(Date.now() / 1000);
  const answers = [];
  await page.route(/\/ubus(?:\/|$)/, async (route) => {
    const body = route.request().postDataJSON();
    const calls = Array.isArray(body) ? body : [body];
    const ours = calls.filter((c) => c.params?.[1] === 'familywarden' && ['requests', 'usage', 'request_answer', 'new_apps', 'app_decision'].includes(c.params?.[2]));
    if (!ours.length) return route.continue();
    const results = calls.map((c) => {
      const method = c.params?.[2];
      if (method === 'request_answer' || method === 'app_decision') answers.push(c.params[3]);
      const reply = method === 'requests' ? { requests: [
        { request_id: 'request-e2e-0001', profile: 'alex', profile_name: 'Alex', target: 'com.example.video', label: 'Video',
          minutes: 30, received: now - 60, expires: now + 3540, state: 'pending' },
        { request_id: 'request-e2e-0002', profile: 'sam', profile_name: 'Sam', target: 'total', minutes: 15,
          received: now - 30, expires: now + 3570, state: 'pending' }] }
        : method === 'usage' ? { day: '2027-01-15', profiles: [{ id: 'alex', name: 'Alex', total: 4920,
          devices: [{ name: "Alex's phone", total: 4920, incomplete: false }], apps: [], week: [] }] }
        : method === 'new_apps' ? { profiles: [{ id: 'sam', name: 'Sam', pending: [{ package: 'com.example.draw', label: 'Draw', seen: now }], approved: [] }] }
        : { status: 'ok', state: 'approved', granted: 20 };
      return { jsonrpc: '2.0', id: c.id, result: [0, reply] };
    });
    await route.fulfill({ json: Array.isArray(body) ? results : results[0] });
  });
  await page.goto(URL);
  const card = page.locator('.fwx-card', { hasText: 'Video' });
  await expect(card).toBeVisible();
  await expect(page.getByText('82 min').first()).toBeVisible();
  // Out of range is not sent at all.
  await card.getByLabel('Minutes').fill('31');
  await card.getByRole('button', { name: 'Approve' }).click();
  await page.waitForTimeout(300);
  expect(answers).toEqual([]);
  await card.getByLabel('Minutes').fill('20');
  await card.getByRole('button', { name: 'Approve' }).click();
  await expect.poll(() => answers.length).toBe(1);
  expect(answers[0]).toEqual({ request_id: 'request-e2e-0001', approve: true, minutes: 20 });
});

test('allowing a new app sends exactly that profile and package', async ({ page }) => {
  const calls = [];
  await page.route(/\/ubus(?:\/|$)/, async (route) => {
    const body = route.request().postDataJSON();
    const list = Array.isArray(body) ? body : [body];
    if (!list.some((c) => c.params?.[1] === 'familywarden' && ['requests', 'usage', 'new_apps', 'app_decision'].includes(c.params?.[2]))) return route.continue();
    const results = list.map((c) => {
      const m = c.params?.[2];
      if (m === 'app_decision') calls.push(c.params[3]);
      const reply = m === 'requests' ? { requests: [] } : m === 'usage' ? { day: '2027-01-15', profiles: [] }
        : m === 'new_apps' ? { profiles: [{ id: 'sam', name: 'Sam', pending: [{ package: 'com.example.draw', label: 'Draw', seen: 1 }], approved: [] }] }
        : { status: 'ok', state: 'approved' };
      return { jsonrpc: '2.0', id: c.id, result: [0, reply] };
    });
    await route.fulfill({ json: Array.isArray(body) ? results : results[0] });
  });
  await page.goto(URL);
  const card = page.locator('.fwx-card', { hasText: 'Draw' });
  await card.getByRole('button', { name: 'Allow' }).click();
  await expect.poll(() => calls.length).toBe(1);
  expect(calls[0]).toEqual({ profile: 'sam', package: 'com.example.draw', action: 'approve' });
});
