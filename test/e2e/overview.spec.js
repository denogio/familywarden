const { test, expect } = require('@playwright/test');

const BASE = process.env.FW_BASE_URL || 'http://localhost:8080';
const URL = `${BASE}/cgi-bin/luci/admin/services/familywarden`;

async function login(page) {
  await page.goto(`${BASE}/cgi-bin/luci/admin/login`);
  await page.fill('[name="luci_username"]', 'root');
  await page.fill('[name="luci_password"]', process.env.FW_PASSWORD ?? 'ulysses');
  await page.getByRole('button', { name: 'Log in' }).click();
  await page.waitForLoadState('networkidle');
}

// Only replace the profile-list reply; the rest of LuCI and its CSS are real.
async function mockProfiles(page, reply) {
  await page.route(/\/ubus(?:\/|$)/, async route => {
    const body = route.request().postDataJSON();
    const calls = Array.isArray(body) ? body : [body];
    if (!calls.some(c => c.params?.[1] === 'familywarden' && c.params?.[2] === 'list'))
      return route.continue();
    const response = await route.fetch();
    const json = await response.json();
    const results = Array.isArray(json) ? json : [json];
    for (const call of calls) {
      if (call.params?.[1] === 'familywarden' && call.params?.[2] === 'list') {
        const result = results.find(r => r.id === call.id);
        result.result = [0, reply(result.result[1])];
      }
    }
    await route.fulfill({ response, json: Array.isArray(json) ? results : results[0] });
  });
}

const fixtures = {
  alex: { name: 'Alex', policy: 'scheduled', status: 'blocked', override: 'none',
    devices: ['02:00:00:00:00:01'], schedules: [{ days: 'mon tue wed thu fri', start: '07:00', stop: '20:00' }] },
  morgan: { name: 'Morgan', policy: 'scheduled', status: 'allowed', override: 'none',
    captain: true, pact_type: 'ongoing', pact_window: 'sun 10:00-11:00',
    devices: [], schedules: [] }
};

const pageErrors = new WeakMap();
test.beforeEach(async ({ page }) => {
  const errors = [];
  pageErrors.set(page, errors);
  await login(page);
  // LuCI probes UCI while unauthenticated on the login page; only assert errors
  // from the authenticated view under test, not that expected Access denied probe.
  page.on('pageerror', error => errors.push(error.message));
});
test.afterEach(async ({ page }) => {
  await page.unrouteAll({ behavior: 'wait' });
  expect(pageErrors.get(page)).toEqual([]);
});

test('Overview uses truthful access labels and preserves pact tightening', async ({ page }) => {
  await mockProfiles(page, () => ({ profiles: fixtures }));
  await page.goto(URL);
  const root = page.locator('#fw-dashboard');
  await expect(root.getByRole('heading', { name: 'Overview', exact: true })).toBeVisible();
  await expect(root.locator('.fw-summary')).toContainText('Internet allowed');
  await expect(root.locator('.fw-summary')).not.toContainText('Online');
  const alex = root.locator('[data-profile="alex"]');
  await expect(alex.locator('.fw-status')).toContainText('Internet blocked');
  await expect(alex.getByRole('button', { name: '+10 min', exact: true })).toBeVisible();
  const captain = root.locator('[data-profile="morgan"]');
  await expect(captain.getByRole('button', { name: 'Block', exact: true })).toBeVisible();
  await expect(captain.getByRole('button', { name: 'Allow', exact: true })).toHaveCount(0);
  await expect(captain.locator('.fw-ticket')).toHaveCount(0);
  await expect(page.locator('.cbi-page-actions')).toHaveCount(0);
});

test('native actions block and reset a real profile', async ({ page }) => {
  await page.goto(URL);
  const alex = page.locator('[data-profile="alex"]');
  await expect(alex).toBeVisible();
  if (await alex.getByRole('button', { name: 'Reset override', exact: true }).count())
    await alex.getByRole('button', { name: 'Reset override', exact: true }).click();
  await expect(alex.locator('.fw-status')).toContainText('Internet allowed');
  await alex.getByRole('button', { name: 'Block', exact: true }).click();
  await expect(alex.locator('.fw-status')).toContainText('Internet blocked');
  await alex.getByRole('button', { name: 'Reset override', exact: true }).click();
  await expect(alex.locator('.fw-status')).toContainText('Internet allowed');
});

test('running tickets extend through the UI and can be cancelled separately', async ({ page }) => {
  await page.goto(URL);
  await expect(page.locator('#fw-dashboard')).toBeVisible();
  const session = await page.evaluate(() => L.env.sessionid);
  async function call(method, args) {
    const response = await page.request.post(`${BASE}/ubus/`, { data: {
      jsonrpc: '2.0', id: 1, method: 'call', params: [session, 'familywarden', method, args]
    } });
    const body = await response.json();
    expect(body.result[0]).toBe(0);
    return body.result[1];
  }
  await call('reset', { profile: 'alex' });
  await call('ticket', { profile: 'alex', minutes: 5 });
  const initial = await call('status', { profile: 'alex' });
  await page.reload();
  const alex = page.locator('[data-profile="alex"]');
  await expect(alex.locator('.fw-override')).toContainText('Override');
  await expect(alex.locator('.fw-ticket')).toContainText('Adds time to the running ticket.');
  await alex.getByRole('button', { name: '+10 min', exact: true }).click();
  await expect.poll(async () => (await call('status', { profile: 'alex' })).override_expires)
    .toBe(initial.override_expires + 600);
  await expect(alex.getByRole('button', { name: 'Cancel ticket', exact: true })).toBeEnabled();
  await alex.getByRole('button', { name: 'Cancel ticket', exact: true }).click();
  await expect.poll(async () => (await call('status', { profile: 'alex' })).override).toBe('none');
  await expect(alex.locator('.fw-ticket')).toHaveCount(0);
});

test('unknown status is not counted as allowed and offers no access actions', async ({ page }) => {
  await mockProfiles(page, () => ({ profiles: { alex: { ...fixtures.alex, status: null } } }));
  await page.goto(URL);
  const root = page.locator('#fw-dashboard');
  await expect(root.locator('.fw-status')).toContainText('Status unknown');
  await expect(root.locator('.fw-card button')).toHaveCount(0);
  await expect(root.locator('.fw-summary .fw-fact').filter({ hasText: 'Internet allowed' }).locator('dd')).toHaveText('0');
});

test('failed refresh keeps explicit stale data; valid empty household clears cards', async ({ page }) => {
  test.setTimeout(45000);
  let reply = { profiles: fixtures };
  await mockProfiles(page, () => reply);
  await page.goto(URL);
  await expect(page.locator('.fw-card')).toHaveCount(2);
  reply = {};
  await expect(page.locator('.fw-refresh-status')).toContainText('Showing the last successful update', { timeout: 15000 });
  await expect(page.locator('.fw-card')).toHaveCount(2);
  await expect(page.locator('[data-profile="alex"] button').first()).toBeDisabled();
  reply = { profiles: {} };
  await page.getByRole('button', { name: 'Retry', exact: true }).click();
  await expect(page.locator('.fw-empty')).toContainText('No profiles configured');
  await expect(page.locator('.fw-card')).toHaveCount(0);
  await expect(page.locator('.fw-refresh-status')).toBeEmpty();
});

test('initial RPC failure is not presented as an empty household', async ({ page }) => {
  await mockProfiles(page, () => ({}));
  await page.goto(URL);
  await expect(page.locator('.fw-refresh-status')).toContainText('No profile status has been received');
  await expect(page.locator('.fw-empty')).toHaveCount(0);
  await expect(page.locator('.fw-summary')).toHaveCount(0);
});

test('polling preserves keyboard focus', async ({ page }) => {
  await mockProfiles(page, () => ({ profiles: fixtures }));
  await page.goto(URL);
  const button = page.locator('[data-focus="alex:ticket-10"]');
  await expect(button).toBeVisible();
  await button.focus();
  const updated = await page.locator('.fw-updated').innerText();
  await expect(page.locator('.fw-updated')).not.toHaveText(updated, { timeout: 15000 });
  await expect(button).toBeFocused();
});

test('responsive scoped styles follow LuCI theme and do not restyle other pages', async ({ page }) => {
  await mockProfiles(page, () => ({ profiles: fixtures }));
  await page.goto(URL);
  await expect(page.locator('.fw-card')).toHaveCount(2);
  for (const dark of [false, true]) {
    await page.evaluate(d => document.documentElement.setAttribute('data-darkmode', String(d)), dark);
    await expect(page.locator('.fw-card').first()).toHaveCSS('background-color', dark ? 'rgb(18, 28, 39)' : 'rgb(255, 255, 255)');
    await expect(page.locator('#maincontent')).toHaveCSS('background-color', dark ? 'rgb(20, 23, 27)' : 'rgb(234, 240, 246)');
    await expect(page.locator('#tabmenu .tabs > .active')).toHaveCSS('background-color', dark ? 'rgb(20, 23, 27)' : 'rgb(255, 255, 255)');
    await expect(page.locator('#tabmenu .tabs > .active > a')).toHaveCSS('color', dark ? 'rgb(230, 232, 235)' : 'rgb(15, 27, 39)');
    await expect(page.locator('#tabmenu .tabs > li:not(.active) > a').first()).toHaveCSS('color', dark ? 'rgb(169, 177, 186)' : 'rgb(74, 93, 112)');
    for (const width of [1440, 720, 390, 320]) {
      await page.setViewportSize({ width, height: 900 });
      expect(await page.locator('#fw-dashboard').evaluate(el => el.scrollWidth <= el.clientWidth + 1)).toBe(true);
      expect(await page.locator('#maincontent').evaluate(el => el.getBoundingClientRect().right <= innerWidth + 1)).toBe(true);
    }
  }
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.evaluate(() => { document.body.style.zoom = '2'; });
  expect(await page.locator('#fw-dashboard').evaluate(el => el.scrollWidth <= el.clientWidth + 1)).toBe(true);
  // Simulate keeping the stylesheet loaded when leaving the Overview.
  await page.evaluate(() => { document.body.style.zoom = ''; });
  await page.goto(`${URL}/profiles`);
  const originalTabColor = await page.locator('#tabmenu .tabs > .active > a').evaluate(el => getComputedStyle(el).color);
  const originalBackground = await page.locator('#maincontent').evaluate(el => getComputedStyle(el).backgroundColor);
  await page.addStyleTag({ url: `${BASE}/luci-static/resources/familywarden/overview.css` });
  await expect(page.locator('#maincontent')).toHaveCSS('background-color', originalBackground);
  await expect(page.locator('#tabmenu .tabs > .active > a')).toHaveCSS('color', originalTabColor);
  await expect(page.locator('[data-name="policy"] select')).toBeVisible();
  await expect(page.locator('#fw-dashboard')).toHaveCount(0);
});
