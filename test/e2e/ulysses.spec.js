const { test, expect } = require('@playwright/test');

// Makes a real Ulysses pact, so it only runs against a throwaway router:
// scripts/test-e2e.sh sets FW_E2E_ALLOW_PACT=1 for its container.
test.skip(process.env.FW_E2E_ALLOW_PACT !== '1', 'makes a pact - only against a throwaway router');
test.describe.configure({ mode: 'serial', timeout: 90000 });

const BASE_URL = process.env.FW_BASE_URL || 'http://localhost:8080';
const PASSWORD = process.env.FW_PASSWORD ?? 'ulysses';
const FW_URL = `${BASE_URL}/cgi-bin/luci/admin/services/familywarden`;

async function login(page) {
  await page.goto(`${BASE_URL}/cgi-bin/luci/admin/login`);
  await page.fill('input[name="luci_username"]', 'root');
  await page.fill('input[name="luci_password"]', PASSWORD);
  await page.click('button:has-text("Log in")');
  await page.waitForLoadState('networkidle', { timeout: 15000 });
}

const card = (page, name) => page.locator('.fw-card', { hasText: name });

async function saveSettings(page, field, value) {
  await page.goto(`${FW_URL}/settings`);
  await page.locator(`[data-name="${field}"] select`).selectOption(value);
  // Save & Apply is a split-button dropdown (li), not a plain button
  await page.locator('.cbi-page-actions').getByText('Save & Apply', { exact: true }).first().click({ force: true });
  // Wait for LuCI's apply + rollback confirmation to finish
  await expect(page.getByText('Configuration changes applied')).toBeVisible({ timeout: 45000 });
  await page.waitForLoadState('networkidle', { timeout: 20000 });
}

test.beforeEach(async ({ page }) => {
  await login(page);
});

test('default policy "block" lists devices that are in no profile', async ({ page }) => {
  await saveSettings(page, 'default_policy', 'block');
  await page.goto(FW_URL);
  await expect(page.locator('.fw-notices')).toContainText('new-laptop (de:ad:be:ef:00:99)');
  await expect(page.locator('.fw-notices')).not.toContainText('josva-pc');
  await saveSettings(page, 'default_policy', 'allow');
  await page.goto(FW_URL);
  await expect(page.locator('.fw-card').first()).toBeVisible();
  await expect(page.locator('.fw-notices')).not.toContainText('new-laptop');
});

test('only the captain gets the pact button', async ({ page }) => {
  await page.goto(FW_URL);
  await expect(card(page, 'Dennis').locator('button:has-text("Make a pact")')).toBeVisible();
  await expect(card(page, 'Josva').locator('button:has-text("pact")')).toHaveCount(0);
});

test('making an ongoing pact binds the captain', async ({ page }) => {
  await page.goto(FW_URL);
  await card(page, 'Dennis').locator('button:has-text("Make a pact")').click();
  await page.check('.modal input[value=ongoing]');
  await page.click('.modal button:has-text("Bind me")');
  await expect(card(page, 'Dennis').locator('.fw-ulysses')).toContainText('Bound to the mast by an ongoing pact', { timeout: 10000 });
  await expect(card(page, 'Dennis').locator('button:has-text("Allow")')).toHaveCount(0);
  await expect(card(page, 'Dennis').locator('button:has-text("Make a pact")')).toHaveCount(0);
});

test("the captain's profile is frozen", async ({ page }) => {
  await page.goto(`${FW_URL}/profiles?name=dennis`);
  await expect(page.locator('.fw-status-bar .fw-ulysses')).toContainText('Bound to the mast');
  await expect(page.locator('[data-name="policy"] select')).toBeDisabled();
  await expect(page.locator('.fw-days input[data-day]').first()).toBeDisabled();
  await expect(page.locator('button:has-text("Delete Profile")')).toHaveCount(0);
  await expect(page.locator('.cbi-page-actions')).toHaveCount(0);
});

test('other profiles stay editable during the pact', async ({ page }) => {
  await page.goto(`${FW_URL}/profiles?name=josva`);
  await expect(page.locator('[data-name="policy"] select')).toBeEnabled();
  await expect(page.locator('button:has-text("Delete Profile")')).toBeVisible();
});

test('global and Ulysses settings are frozen during the pact', async ({ page }) => {
  await page.goto(`${FW_URL}/settings`);
  await expect(page.locator('[data-name="ulysses_profile"] select')).toBeDisabled();
  await expect(page.locator('[data-name="default_policy"] select')).toBeDisabled();
  await expect(page.locator('[data-name="enabled"] input[type=checkbox]')).toBeDisabled();
});
