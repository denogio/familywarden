const { test, expect } = require('@playwright/test');

// Phase 2: the core owns the configuration in a FRESH disposable container
// (scripts/test-e2e.sh runs this file after the legacy suite, with
// FW_E2E_ALLOW_PACT=1 and FW_E2E_CONTAINER set by that runner only). It never
// imports against a user-configured router. Browser edits must use core RPCs;
// only the fixture prepares synthetic UCI data before the explicit import.
const container = process.env.FW_E2E_CONTAINER;
test.skip(process.env.FW_E2E_OWNED_PHASE !== '1' ||
  !container?.startsWith('familywarden-e2e-') || process.env.FW_E2E_ALLOW_PACT !== '1',
  'core-owned cutover: disposable e2e container only');
test.describe.configure({ mode: 'serial', timeout: 90000 });

const BASE_URL = process.env.FW_BASE_URL || 'http://localhost:8080';
const PASSWORD = process.env.FW_PASSWORD ?? 'ulysses';
const FW_URL = `${BASE_URL}/cgi-bin/luci/admin/services/familywarden`;

const { execFileSync } = require('node:child_process');
const fixture = (action, timeout = 30000) => execFileSync(
  'docker', ['exec', '-e', 'FW_E2E_FIXTURE=1', container, 'sh', '/src/scripts/e2e-owned-fixture.sh', action],
  { encoding: 'utf8', timeout });
const call = (method, params = {}) => JSON.parse(execFileSync('docker', ['exec', container, 'sh', '-c',
  `printf '%s' '${JSON.stringify(params)}' | /usr/libexec/rpcd/familywarden call ${method}`],
  { encoding: 'utf8', timeout: 15000 }));
const shell = (cmd, timeout = 15000) => execFileSync('docker', ['exec', container, 'sh', '-c', cmd],
  { encoding: 'utf8', timeout });

async function login(page) {
  await page.goto(`${BASE_URL}/cgi-bin/luci/admin/login`);
  await page.fill('input[name="luci_username"]', 'root');
  await page.fill('input[name="luci_password"]', PASSWORD);
  await page.click('button:has-text("Log in")');
  await page.waitForLoadState('networkidle', { timeout: 15000 });
}

test.beforeEach(async ({ page }) => {
  await login(page);
});

test('import cuts the configuration over to the core and materializes its projection', async () => {
  expect(call('configuration_status').owner).toBe('legacy');
  fixture('import');
  expect(call('configuration_status').owner).toBe('core');
  expect(shell('test -s /etc/familywarden/core/configuration.json')).toBe('');
  // The adapter materializes the core document in OpenWrt's UCI format.
  expect(shell('uci -q get familywarden.settings.dns_domain')).toBe('dns.example.net\n');
  // The document carries the imported settings, including the resolver URL.
  const exported = call('configuration_export');
  expect(exported.owner).toBe('core');
  expect(exported.configuration.current.settings.dns_domain).toBe('dns.example.net');
  expect(exported.configuration.current.settings.resolver_url).toBe('http://192.0.2.1:3000');
});

test('an unrelated profile edit flows through apply, confirm and reload', async ({ page }) => {
  // The CAS flow is exercised through the real root CLI, the same path the
  // rpcd shim and the shared UI both reach; the browser verifies the result.
  const alex = (doc) => doc.current.profiles.find((p) => p.id === 'alex');
  let doc = call('configuration_export').configuration;
  expect(call('configuration_status').owner).toBe('core');
  const original = JSON.parse(JSON.stringify(doc));
  alex(doc).name = 'Alex (verified)';
  const applied = call('configuration_apply', {
    revision: doc.revision, current: doc.current });
  expect(applied.token).toBeTruthy();
  // Unconfirmed: the candidate stays pending, not committed.
  const pending = call('configuration_export');
  expect(pending.pending).toBe(true);
  expect(alex(pending.configuration).name).toBe('Alex (verified)');
  expect(pending.configuration.revision).not.toBe(original.revision);
  const confirmed = call('configuration_confirm', {
    revision: applied.revision, token: applied.token });
  expect(confirmed.status).toBe('ok');
  expect(confirmed.revision).not.toBe(doc.revision);
  await shell('familywarden reload >/dev/null');
  doc = call('configuration_export').configuration;
  expect(call('configuration_export').pending).toBe(false);
  expect(alex(doc).name).toBe('Alex (verified)');
  // The single shared UI shows the confirmed name in the family view.
  await page.goto(FW_URL);
  await expect(page.locator('.fw-kid h3', { hasText: 'Alex (verified)' })).toBeVisible({ timeout: 15000 });
});

test('lost confirmation restores the document and leaves the browser edit unsaved', async ({ page }) => {
  const before = call('configuration_export').configuration;
  const name = 'Alex (not confirmed)';
  const operations = [];
  let interrupted = false;
  await page.route(/\/ubus(?:\/|$)/, async (route) => {
    const body = route.request().postDataJSON();
    const requests = Array.isArray(body) ? body : [body];
    const methods = requests.filter((c) => c.params?.[1] === 'familywarden').map((c) => c.params[2]);
    operations.push(...methods);
    if (!methods.includes('configuration_apply')) return route.continue();
    // Forward the real RPC, but interrupt between apply and browser confirmation.
    // No API response is fabricated: restarting the daemon rolls the candidate back.
    const response = await route.fetch();
    const pending = call('configuration_export');
    expect(pending.pending).toBe(true);
    expect(pending.configuration.current.profiles.find((p) => p.id === 'alex').name).toBe(name);
    fixture('restart');
    interrupted = true;
    await route.fulfill({ response });
  });
  await page.goto(FW_URL);
  const alexCard = page.locator('.fw-kid', { has: page.locator('h3', { hasText: 'Alex' }) });
  await alexCard.getByRole('button', { name: 'Edit rules', exact: true }).click();
  const input = page.getByLabel('Name', { exact: true }).first();
  await input.fill(name);
  await page.getByRole('button', { name: 'Save rules', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('The unconfirmed change was rolled back or superseded.', { timeout: 20000 });
  await expect(page.getByRole('button', { name: 'Confirm connection', exact: true })).toHaveCount(0);
  expect(interrupted).toBe(true);
  expect(operations.filter((m) => m === 'configuration_apply')).toHaveLength(1);
  expect(operations.filter((m) => m === 'configuration_confirm')).toHaveLength(0);
  // The refreshed core state detects the rollback without offering a stale confirmation.
  // The unsaved draft survives that recovery.
  await expect(input).toHaveValue(name);
  const recovered = call('configuration_export');
  expect(recovered.pending).toBe(false);
  expect(recovered.configuration).toEqual(before);
  const confirmedName = before.current.profiles.find((p) => p.id === 'alex').name;
  expect(shell('uci -q get familywarden.alex.name').trim()).toBe(confirmedName);
  await page.reload();
  await expect(page.locator('.fw-kid h3', { hasText: confirmedName })).toBeVisible({ timeout: 15000 });
});

test('captain DNS during a pact is a cumulative, explicit transaction', async ({ page }) => {
  const calls = [];
  page.on('request', (r) => {
    if (!r.url().includes('/ubus')) return;
    try {
      const body = r.postDataJSON();
      for (const c of Array.isArray(body) ? body : [body])
        if (c.params?.[1] === 'familywarden') calls.push(c.params[2]);
        else if (c.params?.[1] === 'uci') calls.push(`uci/${c.params[2]}`);
    } catch { /* not JSON */ }
  });

  // Make the ongoing pact through the shared Ulysses view (ubus pact method).
  await page.goto(FW_URL);
  page.on('dialog', (d) => d.accept());
  await page.locator('.fw-nav').getByRole('button', { name: 'Ulysses', exact: true }).click();
  await page.getByLabel('Pact', { exact: true }).selectOption('ongoing');
  await page.getByRole('button', { name: 'Make a pact', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Until you sail home', exact: true })).toBeVisible({ timeout: 10000 });

  // The core document is authoritative; UCI is only the adapter's projection.
  const doc = () => call('configuration_export').configuration;
  const dennis = (c) => c.current.profiles.find((p) => p.id === 'dennis');
  expect(doc().current.settings.dns_domain).toBe('dns.example.net');
  expect(dennis(doc()).client_id).toBe('');

  // The captain's DNS stays editable on the frozen profile; the explicit
  // dns_update transaction applies it (the normal CAS path is for other profiles).
  await page.locator('.fw-nav').getByRole('button', { name: 'Family', exact: true }).click();
  const dennisCard = page.locator('.fw-kid', { has: page.locator('h3', { hasText: 'Dennis' }) });
  await dennisCard.getByRole('button', { name: 'Edit rules', exact: true }).click();
  await page.getByLabel('Strict DNS', { exact: true }).check();
  await page.getByLabel('Safe browsing', { exact: true }).check();
  await page.getByRole('button', { name: 'Commit DNS-only change', exact: true }).click();
  await expect.poll(() => calls.filter((c) => c === 'configuration_dns_update').length, { timeout: 15000 }).toBe(1);
  const enabled = doc();
  expect(dennis(enabled).dns).toBe('1');
  expect(dennis(enabled).client_id).toMatch(/^fw[0-9a-f]{32}$/);
  expect(dennis(enabled).protections).toContain('safe_browsing');
  // The accumulated pact minimum now enforces the captain's DNS.
  expect(enabled.pact.dns.enabled).toBe('1');
  expect(enabled.pact.dns.domain).toBe('dns.example.net');
  expect(enabled.pact.dns.client_id).toBe(dennis(enabled).client_id);
  expect(enabled.pact.dns.protections).toContain('safe_browsing');
  expect(shell('uci -q get familywarden.dennis.dns').trim()).toBe('1');
  expect(shell('uci -q get familywarden.dennis.dns_client_id').trim()).toBe(dennis(enabled).client_id);

  // Reopen the saved rules; the accumulated pact minimum stays in force.
  await page.getByRole('tab', { name: 'Rules', exact: true }).click();
  await page.getByLabel('Strict DNS', { exact: true }).uncheck();
  await page.getByRole('button', { name: 'Commit DNS-only change', exact: true }).click();
  await expect.poll(() => calls.filter((c) => c === 'configuration_dns_update').length, { timeout: 15000 }).toBe(2);
  const after = doc();
  // Current loosens, but the enforced minimum keeps the captain's DNS in place.
  expect(dennis(after).dns).not.toBe('1');
  expect(dennis(after).client_id).toBe(dennis(enabled).client_id);
  expect(after.pact.dns.enabled).toBe('1');
  expect(after.pact.dns.client_id).toBe(dennis(enabled).client_id);
  expect(after.pact.dns.domain).toBe('dns.example.net');
  expect(after.pact.dns.protections).toContain('safe_browsing');

  // No ordinary CAS write, amendment or UCI staging rode along with the pact DNS.
  expect(calls.filter((c) => ['configuration_apply', 'configuration_amend', 'configuration_confirm']
    .includes(c))).toHaveLength(0);
  expect(calls.filter((c) => /^uci\/(set|add|delete|commit|apply)$/.test(c))).toHaveLength(0);
  // A changed UCI file is expected: only the core's adapter projects it.
  expect(shell('uci -q get familywarden.dennis.dns || true').trim()).toBe(dennis(after).dns);
  expect(shell('uci -q get familywarden.dennis.dns_client_id').trim()).toBe(dennis(after).client_id);
  fixture('restart');
  const restarted = doc();
  expect(restarted.revision).toBe(after.revision);
  expect(restarted.pact.dns).toEqual(after.pact.dns);
});
