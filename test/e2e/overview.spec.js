const { test, expect } = require('@playwright/test');

const BASE = process.env.FW_BASE_URL || 'http://localhost:8080';
const URL = `${BASE}/cgi-bin/luci/admin/services/familywarden`;
test.use({ locale: 'en-GB', timezoneId: 'UTC' });

async function login(page) {
  await page.goto(`${BASE}/cgi-bin/luci/admin/login`);
  await page.fill('[name="luci_username"]', 'root');
  await page.fill('[name="luci_password"]', process.env.FW_PASSWORD ?? 'ulysses');
  await page.getByRole('button', { name: 'Log in' }).click();
  await page.waitForLoadState('networkidle');
}

// Only replace one FamilyWarden reply; the rest of LuCI and its CSS are real.
async function mockProfiles(page, reply) { return mockFamilyWarden(page, 'list', reply); }
async function mockFamilyWarden(page, method, reply) {
  await page.route(/\/ubus(?:\/|$)/, async route => {
    const body = route.request().postDataJSON();
    const calls = Array.isArray(body) ? body : [body];
    if (!calls.some(c => c.params?.[1] === 'familywarden' && c.params?.[2] === method))
      return route.continue();
    const response = await route.fetch();
    const json = await response.json();
    const results = Array.isArray(json) ? json : [json];
    for (const call of calls) {
      if (call.params?.[1] === 'familywarden' && call.params?.[2] === method) {
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
  await expect(alex.getByRole('button', { name: 'Add 10 minutes', exact: true })).toBeVisible();
  const captain = root.locator('[data-profile="morgan"]');
  await expect(captain.getByRole('button', { name: 'Block', exact: true })).toBeVisible();
  await expect(captain.getByRole('button', { name: 'Allow', exact: true })).toHaveCount(0);
  await expect(captain.locator('.fw-ticket')).toHaveCount(0);
  await expect(page.locator('.cbi-page-actions')).toHaveCount(0);
});

test('Phones distinguishes incompatible clients from waiting and acknowledgements', async ({ page }) => {
  const base = { profile: 'alex', administrator: '0', fingerprint: 'a'.repeat(64), seen: 0, frozen: false };
  await mockFamilyWarden(page, 'devices', () => ({ pending: [], devices: [
    { ...base, id: 'fw-one', name: 'Alex phone', apps_rev: '0123456789abcdef', apps_applied: '0123456789abcdef' },
    { ...base, id: 'fw-two', name: 'Sam phone', apps_rev: '0123456789abcdef', apps_applied: '' },
    { ...base, id: 'fw-three', name: 'Robin phone', apps_rev: '0123456789abcdef', apps_applied: '', apps_error: 'unsupported_capabilities' },
    { ...base, id: 'fw-four', name: 'Kim phone' },
    { ...base, id: 'fw-five', name: 'Alex tablet', apps_rev: '' },
    { ...base, id: 'fw-six', name: 'Sam tablet', apps_rev: null, apps_applied: null }
  ] }));
  await page.goto(`${URL}/devices`);
  const row = name => page.locator('tbody tr').filter({ has: page.getByText(name, { exact: true }) });
  await expect(row('Alex phone')).toContainText('Current revision acknowledged');
  await expect(row('Sam phone')).toContainText('Not applied yet');
  await expect(row('Robin phone')).toContainText('Update FamilyWarden on this phone');
  await expect(row('Robin phone')).not.toContainText('Current revision acknowledged');
  await expect(row('Kim phone')).toContainText('Not reported by this router');
  await expect(row('Alex tablet')).toContainText('Not configured');
  await expect(row('Sam tablet')).toContainText('Not reported by this router');
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
  await expect(alex.locator('.fw-ticket .fw-act-hint')).toHaveCount(0);
  await alex.getByRole('button', { name: 'Add 10 minutes', exact: true }).click();
  await expect.poll(async () => (await call('status', { profile: 'alex' })).override_expires)
    .toBe(initial.override_expires + 600);
  await expect(alex.getByRole('button', { name: 'Cancel ticket', exact: true })).toBeEnabled();
  await alex.getByRole('button', { name: 'Cancel ticket', exact: true }).click();
  await expect.poll(async () => (await call('status', { profile: 'alex' })).override).toBe('none');
  await expect(alex.locator('.fw-ticket')).toHaveCount(0);
});

test('refined cards use semantic SVG icons and grouped schedule days', async ({ page }) => {
  await mockProfiles(page, () => ({ profiles: {
    alex: { ...fixtures.alex, policy: 'block' },
    living: { ...fixtures.alex, name: 'Living room', policy: 'allow', override: 'block' },
    scheduled: { ...fixtures.alex, name: 'Scheduled' },
    daily: { ...fixtures.alex, name: 'Daily', status: 'allowed', schedules: [
      { days: 'sun mon tue wed thu fri sat mon', start: '00:00', stop: '23:59' },
      { days: 'mon wed fri', start: '07:00', stop: '08:00' }
    ] }
  } }));
  await page.goto(URL);
  const alex = page.locator('[data-profile="alex"]');
  await expect(alex.locator('.fw-state svg[data-icon="ban"]')).toBeVisible();
  await expect(alex.locator('.fw-state-reason')).toHaveText('Policy is Blocked. Allow or a ticket opens it.');
  await expect(page.locator('[data-profile="living"] .fw-state svg[data-icon="ban"]')).toBeVisible();
  await expect(page.locator('[data-profile="living"] .fw-state-reason')).toHaveText('Manual block until reset.');
  await expect(page.locator('[data-profile="living"] .fw-tag-override')).toHaveText('Override');
  await expect(page.locator('[data-profile="living"] .fw-tag-override svg[data-icon="hand"]')).toBeVisible();
  await expect(page.locator('[data-profile="living"] .fw-btn-reset')).toHaveCSS('font-weight', '600');
  await expect(alex.locator('.fw-tag-override')).toHaveCount(0);
  const scheduled = page.locator('[data-profile="scheduled"]');
  await expect(scheduled.locator('.fw-state svg[data-icon="moon"]')).toBeVisible();
  await expect(scheduled.locator('.fw-schedules')).toContainText('Mon–Fri');
  const daily = page.locator('[data-profile="daily"]');
  await expect(daily.locator('.fw-state svg[data-icon="check"]')).toBeVisible();
  await expect(daily.locator('.fw-schedules li').first()).toHaveText('Every dayAll day');
  await expect(daily.locator('.fw-hours').first()).toHaveAttribute('title', '00:00–23:59');
  await expect(daily.locator('.fw-schedules li').nth(1)).toContainText('Mon, Wed, Fri');
  const gap = await scheduled.locator('.fw-schedules li').evaluate(el =>
    el.lastElementChild.getBoundingClientRect().left - el.firstElementChild.getBoundingClientRect().right);
  expect(gap).toBeLessThanOrEqual(10);
});

test('ticket status combines minutes and short expiry with cancellation beside additions', async ({ page }) => {
  await page.clock.setFixedTime(new Date('2026-10-20T21:24:00Z'));
  await mockProfiles(page, () => ({ profiles: {
    sam: { ...fixtures.alex, name: 'Sam', status: 'allowed', override: 'allow',
      override_expires: Date.parse('2026-10-20T21:41:00Z') / 1000 },
    tomorrow: { ...fixtures.alex, name: 'Tomorrow', status: 'allowed', override: 'allow',
      override_expires: Date.parse('2026-10-21T00:05:00Z') / 1000 }
  } }));
  await page.goto(URL);
  const sam = page.locator('[data-profile="sam"]');
  await expect(sam.locator('.fw-state-reason')).toHaveText('Internet ticket · 17 min left, ends 21:41');
  await expect(sam.locator('.fw-facts dt').filter({ hasText: 'Internet ticket' })).toHaveCount(0);
  await expect(sam.locator('.fw-tag-override')).toHaveText('Timed override');
  const cancel = sam.locator('.fw-ticket .fw-act-row').getByRole('button', { name: 'Cancel ticket', exact: true });
  await expect(cancel).toBeVisible();
  await expect(cancel).toHaveClass(/cbi-button/);
  await expect(cancel).toHaveClass(/fw-btn-revoke/);
  await expect(sam.locator('.fw-seg .fw-btn-grant')).toHaveCount(3);
  await expect(sam.locator('.fw-seg .fw-btn-grant').first()).toHaveText('+5');
  await expect(cancel).toHaveCSS('text-decoration-line', 'none');
  await expect(cancel).toHaveCSS('border-top-style', 'solid');
  await expect(page.locator('[data-profile="tomorrow"] .fw-state-reason')).toContainText('ends 21 Oct 00:05');
});

test('cards fit content, actions align at the top, and the captain gets a Pact zone', async ({ page }) => {
  await page.goto(URL);
  await expect(page.locator('#fw-dashboard')).toBeVisible();
  const captainId = await page.evaluate(async () => (await L.require('uci')).get('familywarden', 'settings', 'ulysses_profile'));
  await mockProfiles(page, () => ({ profiles: {
    short: { ...fixtures.alex, name: 'Short', policy: 'block', devices: [], schedules: [] },
    [captainId]: { ...fixtures.morgan, pact_type: 'until', pact_until: Math.floor(Date.now() / 1000) + 86400 }
  } }));
  await page.reload();
  const short = page.locator('[data-profile="short"]');
  const captain = page.locator(`[data-profile="${captainId}"]`);
  await expect(short).toBeVisible();
  await expect(page.locator('.fw-grid')).toHaveCSS('align-items', 'start');
  await expect(short.locator('.fw-card-foot')).toHaveCSS('margin-top', '0px');
  await expect(short.locator('.fw-card-foot')).toHaveCSS('align-items', 'flex-start');
  // Polling replaces the cards: measure both in one DOM snapshot, not across awaits.
  await expect.poll(() => page.locator('.fw-grid').evaluate((grid, id) => {
    const short = grid.querySelector('[data-profile="short"]');
    const captain = Array.from(grid.querySelectorAll('[data-profile]'))
      .find(el => el.dataset.profile === id);
    return !!short && !!captain && short.getBoundingClientRect().height > 0 &&
      short.getBoundingClientRect().height < captain.getBoundingClientRect().height;
  }, captainId)).toBe(true);
  const labels = await short.locator('.fw-act-zone > .fw-actions-label').evaluateAll(els => els.map(e => e.getBoundingClientRect().top));
  expect(Math.abs(labels[0] - labels[1])).toBeLessThanOrEqual(1);
  await expect(captain.locator('.fw-ulysses svg[data-icon="mast"]')).toBeVisible();
  await expect(captain.locator('.fw-ulysses')).not.toContainText('⚓');
  await expect(captain.locator('.fw-pact-actions').getByRole('button', { name: 'Extend pact', exact: true })).toBeVisible();
  await expect(captain.locator('.fw-card-foot > .fw-held:first-child')).toContainText('Allow and internet tickets are unavailable');
  await expect(captain.locator('.fw-override').getByRole('button', { name: 'Block', exact: true })).toBeVisible();
});

test('MAC addresses stack based on card width and Profiles help stays below its heading', async ({ page }) => {
  await mockProfiles(page, () => ({ profiles: { alex: fixtures.alex } }));
  await page.goto(URL);
  const card = page.locator('[data-profile="alex"]');
  await expect(card).toBeVisible();
  // overview.css is injected as a <link> after the cards render.
  await expect(card).toHaveCSS('container-type', 'inline-size');
  // Resize and measure in one step: the dashboard's refresh can replace the card in
  // between, and a fresh card has its normal width (flaky in CI runs 39792461019 ff.).
  const positions = await card.evaluate(el => {
    el.style.width = '440px';
    const li = el.querySelector('.fw-devices li');
    return {
      name: li.firstElementChild.getBoundingClientRect().toJSON(),
      mac: li.lastElementChild.getBoundingClientRect().toJSON()
    };
  });
  expect(positions.mac.top).toBeGreaterThanOrEqual(positions.name.bottom);
  expect(positions.mac.left).toBe(positions.name.left);
  const help = await page.locator('.fw-section-head').evaluate(el => ({
    heading: el.firstElementChild.getBoundingClientRect().toJSON(),
    text: el.lastElementChild.getBoundingClientRect().toJSON()
  }));
  expect(help.text.top).toBeGreaterThanOrEqual(help.heading.bottom);
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
    await expect(page.locator('.fw-btn-allow')).toHaveCSS('background-color', dark ? 'rgb(22, 48, 47)' : 'rgb(228, 242, 239)');
    await expect(page.locator('.fw-btn-allow')).toHaveCSS('color', dark ? 'rgb(124, 199, 189)' : 'rgb(31, 106, 100)');
    await expect(page.locator('.fw-btn-block')).toHaveCSS('background-color', dark ? 'rgb(24, 36, 49)' : 'rgb(235, 240, 245)');
    await expect(page.locator('.fw-btn-grant').first()).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)');
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
  // Every FamilyWarden page is framed, but only while its root exists: once the
  // FamilyWarden root is gone, the stylesheet must leave LuCI's own chrome alone.
  await page.evaluate(() => { document.body.style.zoom = ''; });
  await page.goto(`${URL}/info`);
  await expect(page.locator('#fw-info')).toBeVisible();
  const framed = await page.locator('#maincontent').evaluate(el => getComputedStyle(el).backgroundColor);
  await page.evaluate(() => document.getElementById('fw-info').remove());
  await expect(page.locator('#maincontent')).not.toHaveCSS('background-color', framed);
  await expect(page.locator('#fw-dashboard')).toHaveCount(0);
});
