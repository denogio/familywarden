const { test, expect } = require('@playwright/test');

// Requests: the shared administrator UI (A20) mounted in the single LuCI page.
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

// The single FamilyWarden page hosts the shared workspace; Requests is a tab in it.
async function openRequests(page) {
  await page.goto(URL);
  await page.locator('.fw-nav').getByRole('button', { name: 'Requests', exact: true }).click();
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

// Same production module in a neutral host: no LuCI rendering/session helpers.
async function neutral(page, options = {}) {
  await page.setContent('<main id="shared-fixture"></main>');
  await page.evaluate(async ({ lang = 'en', interval = 100000, bad = false, defer = false }) => {
    const module = await import('/luci-static/resources/familywarden/shared/family.js');
    const pending = { request_id: 'request-neutral-001', profile: 'alex', profile_name: 'Alex',
      target: 'com.example.video', label: 'Video', minutes: 30,
      received: Math.floor(Date.now() / 1000), expires: Math.floor(Date.now() / 1000) + 3600, state: 'pending' };
    const fixture = window.sharedFixture = { reads: 0, mutations: [], replies: {
      requests: bad ? {} : { requests: [pending] }, usage: { day: '2027-01-15', profiles: [] }, new_apps: { profiles: [] }
    } };
    fixture.control = module.mount(document.getElementById('shared-fixture'), { lang, interval,
      call: (method, params) => {
        if (method === 'request_answer' || method === 'app_decision') {
          fixture.mutations.push({ method, params });
          return new Promise(resolve => { fixture.finishAction = resolve; });
        }
        fixture.reads++;
        if (defer && method === 'requests') return new Promise(resolve => { fixture.finishRead = resolve; });
        return Promise.resolve(fixture.replies[method]);
      }
    });
    fixture.pending = pending;
  }, options);
  return page.locator('#shared-fixture');
}

test('neutral shared host keeps refusals visible and serializes outgoing actions', async ({ page }) => {
  const root = await neutral(page);
  await expect(root).toHaveClass(/fwx/);
  const approve = root.getByRole('button', { name: 'Approve', exact: true });
  await expect(approve).toBeEnabled();
  await root.getByLabel('Minutes').fill('20');
  await approve.click();
  await expect(approve).toBeDisabled();
  await expect(root.getByRole('button', { name: 'Decline', exact: true })).toBeDisabled();
  await expect(root.getByLabel('Minutes')).toBeDisabled();
  await approve.dispatchEvent('click');
  expect(await page.evaluate(() => sharedFixture.mutations)).toEqual([
    { method: 'request_answer', params: { request_id: 'request-neutral-001', approve: true, minutes: 20 } }
  ]);
  await page.evaluate(() => sharedFixture.finishAction({ error: 'not_authorized' }));
  await expect(root.getByRole('alert')).toHaveText('The router refused: not_authorized');
  await page.evaluate(() => sharedFixture.control.refresh());
  await expect(root.getByRole('alert')).toHaveText('The router refused: not_authorized');
  await expect(root.getByLabel('Minutes')).toHaveValue('20');
});

test('neutral shared host retains stale data but disables actions until a valid refresh', async ({ page }) => {
  const root = await neutral(page);
  const approve = root.getByRole('button', { name: 'Approve', exact: true });
  await expect(approve).toBeEnabled();
  await page.evaluate(async () => { sharedFixture.replies.requests = {}; await sharedFixture.control.refresh(); });
  await expect(root).toContainText('Invalid reply from FamilyWarden.');
  await expect(root).toContainText('Showing previously received data.');
  await expect(root).toContainText('Video');
  await expect(root.getByText('No requests waiting.', { exact: true })).toHaveCount(0);
  await expect(approve).toBeDisabled();
  await approve.dispatchEvent('click');
  expect(await page.evaluate(() => sharedFixture.mutations)).toEqual([]);
  await page.evaluate(async () => {
    sharedFixture.replies.requests = { requests: [] };
    await sharedFixture.control.refresh();
  });
  await expect(root.getByText('No requests waiting.', { exact: true })).toBeVisible();
  await expect(root).not.toContainText('Invalid reply from FamilyWarden.');
});

test('initial malformed shared data is not an empty household, in Danish too', async ({ page }) => {
  const root = await neutral(page, { lang: 'da', bad: true });
  await expect(root).toContainText('Ugyldigt svar fra FamilyWarden.');
  await expect(root).not.toContainText('Ingen ventende anmodninger.');
  await expect(root.locator('button')).toHaveCount(0);
});

test('shared polling preserves an active amount and its draft after blur', async ({ page }) => {
  const root = await neutral(page, { interval: 40 });
  const input = root.getByLabel('Minutes');
  await expect(input).toBeEnabled();
  await input.fill('21');
  const reads = await page.evaluate(() => sharedFixture.reads);
  await page.waitForTimeout(160); // Several poll intervals: no repaint may interrupt typing.
  await expect(input).toBeFocused();
  await expect(input).toHaveValue('21');
  expect(await page.evaluate(() => sharedFixture.reads)).toBe(reads);
  await root.getByRole('heading').first().click();
  await expect.poll(() => page.evaluate(() => sharedFixture.reads)).toBeGreaterThan(reads);
  await expect(input).toHaveValue('21');
});

test('stopping the shared mount ignores a late reply and does not restart polling', async ({ page }) => {
  const root = await neutral(page, { interval: 40, defer: true });
  await page.evaluate(() => {
    sharedFixture.control.stop();
    sharedFixture.finishRead({ requests: [sharedFixture.pending] });
  });
  await expect(root).toHaveAttribute('aria-busy', 'false');
  await page.waitForTimeout(160);
  await expect(root.locator('.fwx-card')).toHaveCount(0);
  expect(await page.evaluate(() => sharedFixture.reads)).toBe(3);
});

for (const [lang, labels] of [
  ['en', { week: 'Last 7 days', missing: 'No usage report for this day', unknown: 'Reporting coverage unavailable', zero: '0 min' }],
  ['da', { week: 'Seneste 7 dage', missing: 'Ingen forbrugsrapport for denne dag', unknown: 'Rapportdækning er ukendt', zero: '0 min.' }]
]) test(`weekly usage distinguishes gaps, measured zero and legacy coverage (${lang})`, async ({ page }) => {
  const root = await neutral(page, { lang });
  await expect(root.getByRole('button', { name: lang === 'en' ? 'Approve' : 'Godkend', exact: true })).toBeEnabled();
  await page.evaluate(async () => {
    sharedFixture.replies.requests = { requests: [] };
    sharedFixture.replies.usage = { profiles: [
      { id: 'alex', name: 'Alex', total: 0, devices: [], apps: [], week: [
        { day: '2027-01-12', total: 0, reported: false },
        { day: '2027-01-13', total: 0, reported: true },
        { day: '2027-01-14', total: 0 },
        { day: '2027-01-15', total: 1800 }
      ] },
      { id: 'sam', name: 'Sam', total: 0, devices: [{ name: 'Sam phone', total: 0 }], apps: [], week: [] }
    ] };
    await sharedFixture.control.refresh();
  });
  const alex = root.locator('.fwx-card', { has: page.getByText('Alex', { exact: true }) });
  const sam = root.locator('.fwx-card', { has: page.getByText('Sam', { exact: true }) });
  await expect(alex.locator('.fwx-big')).toHaveText('—');
  await expect(sam.locator('.fwx-big')).toHaveText(labels.zero);
  const chart = alex.getByRole('group', { name: labels.week });
  await expect(chart.getByRole('img', { name: `2027-01-12: ${labels.missing}`, exact: true })).toHaveClass('fwx-gap');
  await expect(chart.getByRole('img', { name: `2027-01-13: ${labels.zero}`, exact: true })).not.toHaveClass(/fwx-gap/);
  await expect(chart.getByRole('img', { name: `2027-01-14: ${labels.unknown}`, exact: true })).toHaveClass('fwx-gap');
  await expect(chart.getByRole('img', { name: new RegExp(`2027-01-15:.*30.*${labels.unknown}`) })).toHaveClass('fwx-coverage-unknown');
});

test('Requests tab talks to the real core through rpcd and LuCI ACLs', async ({ page }) => {
  await openRequests(page);
  await expect(page.getByRole('heading', { name: 'Requests and screen time' })).toBeVisible();
  await expect(page.getByText('No requests waiting.')).toBeVisible();
  await expect(page.locator('.fwx').getByText(/^Updated /)).toBeVisible();
  await expect(page.getByText(/Could not reach/)).toHaveCount(0);
});

test('real HTTP approval persists the chosen grant through the LuCI session shim', async ({ page }) => {
  const container = process.env.FW_E2E_CONTAINER;
  test.skip(!container?.startsWith('familywarden-e2e-'), 'Disposable container fixture only');
  const { execFileSync } = require('node:child_process');
  const fixture = (action) => execFileSync('docker', ['exec', '-e', 'FW_E2E_FIXTURE=1', container,
    'sh', '/src/scripts/e2e-request-fixture.sh', action], { timeout: 20000 });
  try {
    fixture('create');
    await openRequests(page);
    const card = page.locator('.fwx-card', { hasText: 'Alex HTTP test' });
    await card.getByLabel('Minutes').fill('20');
    const answer = page.waitForResponse((r) => r.url().includes('/ubus') &&
      r.request().postData()?.includes('request_answer'));
    await card.getByRole('button', { name: 'Approve', exact: true }).click();
    const response = await (await answer).json();
    const replies = Array.isArray(response) ? response : [response];
    expect(replies.some((r) => r.result?.[1]?.state === 'approved' && r.result[1].granted === 20)).toBe(true);
    // Read through the same authenticated HTTP path, not a fixture or a mock.
    await page.reload();
    await expect(card.getByRole('button', { name: 'Approve', exact: true })).toHaveCount(0);
    const stored = JSON.parse(execFileSync('docker', ['exec', container, 'sh', '-c',
      "echo '{}' | /usr/libexec/rpcd/familywarden call requests"], { encoding: 'utf8', timeout: 10000 }));
    const request = stored.requests.find((r) => r.request_id === 'http-request-0001');
    expect(request.state).toBe('approved');
    expect(request.granted).toBe(20);
    const reportDay = fixture('report-zero').toString().trim();
    const usage = await page.evaluate(async () => {
      const rpc = await L.require('rpc');
      return rpc.declare({ object: 'familywarden', method: 'usage', expect: { '': {} } })();
    });
    const profile = usage.profiles.find(p => p.id === 'http_test');
    expect(profile.week.find(d => d.day === reportDay)).toMatchObject({ total: 0, reported: true });
    expect(profile.week.filter(d => d.day !== reportDay).every(d => d.reported === false && d.total === 0)).toBe(true);
  } finally {
    fixture('cleanup');
  }
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
  await openRequests(page);
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
  await openRequests(page);
  const card = page.locator('.fwx-card', { hasText: 'Draw' });
  await card.getByRole('button', { name: 'Allow' }).click();
  await expect.poll(() => calls.length).toBe(1);
  expect(calls[0]).toEqual({ profile: 'sam', package: 'com.example.draw', action: 'approve' });
});
