const { test, expect } = require('@playwright/test');

const BASE_URL = 'http://192.168.1.1';
const FW_URL = `${BASE_URL}/cgi-bin/luci/admin/services/familywarden`;

async function loginAsRoot(page) {
  await page.goto(`${BASE_URL}/cgi-bin/luci/admin/login`);
  await page.fill('input[name="luci_username"]', 'root');
  await page.fill('input[name="luci_password"]', '');
  await page.click('button:has-text("Log in")');
  await page.waitForLoadState('networkidle', { timeout: 15000 });
}

// Select weekdays in the first schedule row's LuCI multi-select dropdown.
// Operate on the LAST schedule row (the one just added) so tests are robust
// to schedules left behind by earlier tests.
async function selectDays(schedSection, page, values) {
  const dd = schedSection.locator('[id$=".days"].cbi-dropdown').last();
  await dd.click();                       // open
  for (const v of values)
    await dd.locator(`ul > li[data-value="${v}"]`).first().click();
  await page.keyboard.press('Escape');    // close
  await page.waitForTimeout(200);
}

// Read selected day values back from the last row's dropdown (selected li's
// carry a `display` attr). Used to assert days actually round-trip through save.
async function selectedDays(schedSection) {
  return schedSection.locator('[id$=".days"].cbi-dropdown').last()
    .locator('li[data-value][display]')
    .evaluateAll(els => els.map(e => e.getAttribute('data-value')).filter(Boolean));
}

test.describe('FamilyWarden LuCI UI', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsRoot(page);
  });

  test('Overview page loads and shows profiles', async ({ page }) => {
    await page.goto(FW_URL);
    await expect(page.locator('h2, h3').first()).toBeVisible({ timeout: 10000 });
  });

  test('Profiles page loads', async ({ page }) => {
    await page.goto(`${FW_URL}/profiles`);
    await expect(page.locator('h2').first()).toContainText(/profile/i, { timeout: 10000 });
  });

  test('Settings page loads', async ({ page }) => {
    await page.goto(`${FW_URL}/settings`);
    await expect(page.locator('h2').first()).toContainText(/setting/i, { timeout: 10000 });
  });

  test('Block button changes profile to blocked', async ({ page }) => {
    await page.goto(FW_URL);
    const blockBtn = page.locator('button:has-text("Block"), .fw-action-btn.block');
    if (await blockBtn.count() > 0) {
      await blockBtn.first().click();
      await page.waitForTimeout(2000);
      const status = page.locator('.status, .fw-status').first();
      await expect(status).toContainText(/block/i, { timeout: 5000 });
    }
  });

  test('Allow button changes profile to allowed', async ({ page }) => {
    await page.goto(FW_URL);
    const allowBtn = page.locator('button:has-text("Allow"), .fw-action-btn.allow');
    if (await allowBtn.count() > 0) {
      await allowBtn.first().click();
      await page.waitForTimeout(2000);
      const status = page.locator('.status, .fw-status').first();
      await expect(status).toContainText(/allow/i, { timeout: 5000 });
    }
  });
});

test.describe('FamilyWarden Profile Creation', () => {
  test.beforeEach(async ({ page }) => {
    // Auto-accept the delete confirm() dialog. Registered before any click so
    // it can never be missed (the per-test handler raced and let confirm()
    // return false, silently aborting the delete).
    page.on('dialog', dialog => dialog.accept());
    await loginAsRoot(page);
  });

  test('creates a new profile via Add Profile modal', async ({ page }) => {
    await page.goto(`${FW_URL}/profiles`);
    await page.waitForLoadState('networkidle');

    const getProfileCount = async () => {
      return await page.locator('.fw-tab:not(.fw-tab-add)').count();
    };

    const initialCount = await getProfileCount();
    console.log(`Initial profile count: ${initialCount}`);

    const addBtn = page.locator('.fw-tab-add');
    await expect(addBtn).toBeVisible();
    await addBtn.click();

    await page.waitForSelector('div.modal', { timeout: 5000 });

    const nameInput = page.locator('div.modal input[type="text"]');
    await expect(nameInput).toBeVisible();
    const testProfileName = 'TestProfile_' + Date.now();
    await nameInput.fill(testProfileName);
    console.log(`Creating profile: ${testProfileName}`);

    const createBtn = page.locator('div.modal .cbi-button-positive');
    await expect(createBtn).toBeVisible();
    await createBtn.click();

    await page.waitForURL(/\/profiles/, { timeout: 10000 });
    console.log(`Redirected to: ${page.url()}`);

    const saveBtn = page.locator('button:has-text("Save"), li:has-text("Save & Apply")').first();
    if (await saveBtn.isVisible()) {
      console.log('Clicking Save & Apply');
      await saveBtn.click();
      await page.waitForTimeout(5000);
    }

    await page.reload();
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(2000);

    const newCount = await getProfileCount();
    console.log(`Profile count after create: ${newCount}`);
    expect(newCount).toBe(initialCount + 1);
  });

  test('adds a device to existing profile', async ({ page }) => {
    await page.goto(`${FW_URL}/profiles`);
    await page.waitForLoadState('networkidle');

    let profileTabs = page.locator('.fw-tab:not(.fw-tab-add)');
    let profileCount = await profileTabs.count();

    if (profileCount === 0) {
      console.log('No profiles exist, creating one first...');
      await page.locator('.fw-tab-add').click();
      await page.waitForSelector('div.modal', { timeout: 5000 });
      await page.locator('div.modal input[type="text"]').fill('DeviceTestProfile');
      await page.locator('div.modal .cbi-button-positive').click();
      await page.waitForURL(/\/profiles/, { timeout: 10000 });
      await page.waitForLoadState('networkidle');
      await page.waitForTimeout(2000);
      profileTabs = page.locator('.fw-tab:not(.fw-tab-add)');
    }

    await profileTabs.first().waitFor({ timeout: 5000 });
    const profileName = await profileTabs.first().textContent();
    console.log(`Testing device add on profile: ${profileName}`);

    await profileTabs.first().click();
    await page.waitForLoadState('networkidle');

    const deviceSelect = page.locator('select[name*="device"]').first();

    if (await deviceSelect.isVisible()) {
      const options = await deviceSelect.locator('option').count();
      console.log(`Device options available: ${options}`);

      if (options > 0) {
        await deviceSelect.selectOption({ index: 1 });
        console.log('Selected a device');
      }
    }

    const saveBtn = page.locator('.cbi-button[type="submit"], .cbi-button[value="Save"]').first();
    if (await saveBtn.isVisible()) {
      await saveBtn.click();
      console.log('Clicked Save');
      await page.waitForTimeout(3000);
    }
  });

  test('deletes a profile', async ({ page }) => {
    await page.goto(`${FW_URL}/profiles`);
    await page.waitForLoadState('networkidle');

    let profileTabs = page.locator('.fw-tab:not(.fw-tab-add)');
    let profileCount = await profileTabs.count();

    if (profileCount === 0) {
      console.log('No profiles, creating one for delete test...');
      await page.locator('.fw-tab-add').click();
      await page.waitForSelector('div.modal', { timeout: 5000 });
      await page.locator('div.modal input[type="text"]').fill('DeleteMeProfile');
      await page.locator('div.modal .cbi-button-positive').click();
      await page.waitForURL(/\/profiles/, { timeout: 10000 });
      await page.waitForLoadState('networkidle');
      await page.waitForTimeout(2000);
      profileTabs = page.locator('.fw-tab:not(.fw-tab-add)');
      profileCount = await profileTabs.count();
    }

    if (profileCount < 2) {
      console.log(`Only ${profileCount} profile(s), creating one more for delete test...`);
      await page.locator('.fw-tab-add').click();
      await page.waitForSelector('div.modal', { timeout: 5000 });
      await page.locator('div.modal input[type="text"]').fill('DeleteMeProfile');
      await page.locator('div.modal .cbi-button-positive').click();
      await page.waitForURL(/\/profiles/, { timeout: 10000 });
      await page.waitForLoadState('networkidle');
      await page.waitForTimeout(1000);

      const saveBtn = page.locator('button:has-text("Save"), li:has-text("Save & Apply")').first();
      if (await saveBtn.isVisible()) {
        await saveBtn.click();
        await page.waitForTimeout(5000);
      }
      await page.reload();
      await page.waitForLoadState('networkidle');
      await page.waitForTimeout(2000);

      profileTabs = page.locator('.fw-tab:not(.fw-tab-add)');
      profileCount = await profileTabs.count();
    }

    const firstTab = profileTabs.first();
    const profileName = await firstTab.textContent();
    console.log(`Deleting profile: ${profileName}`);

    await firstTab.click();
    await page.waitForLoadState('networkidle');

    // confirm() dialog is auto-accepted in beforeEach.
    const deleteBtn = page.locator('button:has-text("Delete Profile")').first();

    if (await deleteBtn.isVisible()) {
      console.log('Found delete button, clicking');
      await deleteBtn.click();

      await page.waitForURL(/\/profiles/, { timeout: 10000 });
      await page.waitForLoadState('networkidle');
      await page.waitForTimeout(1000);

      const saveBtn = page.locator('button:has-text("Save"), li:has-text("Save & Apply")').first();
      if (await saveBtn.isVisible()) {
        console.log('Clicking Save & Apply');
        await saveBtn.click();
        await page.waitForTimeout(5000);
      }

      await page.reload();
      await page.waitForLoadState('networkidle');
      await page.waitForTimeout(2000);

      const newCount = await page.locator('.fw-tab:not(.fw-tab-add)').count();
      console.log(`Profile count after delete: ${newCount}`);
      expect(newCount).toBe(profileCount - 1);
    }
  });
});

test.describe('FamilyWarden Schedules', () => {
  test.beforeEach(async ({ page }) => {
    await loginAsRoot(page);
  });

  test('creates a schedule for a profile', async ({ page }) => {
    await page.goto(`${FW_URL}/profiles`);
    await page.waitForLoadState('networkidle');

    // Ensure we have a profile with scheduled policy
    let profileTabs = page.locator('.fw-tab:not(.fw-tab-add)');
    let profileCount = await profileTabs.count();

    if (profileCount === 0) {
      console.log('No profiles, creating one...');
      await page.locator('.fw-tab-add').click();
      await page.waitForSelector('div.modal', { timeout: 5000 });
      await page.locator('div.modal input[type="text"]').fill('ScheduleTestProfile');
      await page.locator('div.modal .cbi-button-positive').click();
      await page.waitForURL(/\/profiles/, { timeout: 10000 });
      await page.waitForLoadState('networkidle');
      await page.waitForTimeout(1000);

      const saveBtn = page.locator('button:has-text("Save"), li:has-text("Save & Apply")').first();
      if (await saveBtn.isVisible()) {
        await saveBtn.click();
        await page.waitForTimeout(5000);
      }
      await page.reload();
      await page.waitForLoadState('networkidle');
      await page.waitForTimeout(2000);

      profileTabs = page.locator('.fw-tab:not(.fw-tab-add)');
    }

    // Click on first profile
    await profileTabs.first().click();
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(1000);

    // Policy = Scheduled reveals the schedule section
    await page.locator('select[id*="policy"]').first().selectOption('scheduled');
    await page.waitForTimeout(500);

    // Add a schedule inline (no modal)
    const schedSection = page.locator('#cbi-familywarden-schedule');
    await schedSection.locator('.cbi-button-add').click();
    await page.waitForTimeout(1000);
    await selectDays(schedSection, page, ['mon', 'tue']);
    await schedSection.locator('input[id$=".start"]').last().fill('07:00');
    await schedSection.locator('input[id$=".stop"]').last().fill('20:00');

    // Save & Apply
    const saveApplyBtn = page.locator('button:has-text("Save"), li:has-text("Save & Apply")').first();
    if (await saveApplyBtn.isVisible()) {
      await saveApplyBtn.click({ force: true });
      await page.waitForTimeout(5000);
    }

    // Reload and verify a schedule row exists
    await page.reload();
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(2000);
    const rows = await schedSection.locator('input[id$=".start"]').count();
    console.log('Schedule rows after reload:', rows);
    expect(rows).toBeGreaterThan(0);
  });

  test('policy stays Scheduled when adding a schedule in the same save', async ({ page }) => {
    // Regression: setting policy=Scheduled AND adding a schedule then Save &
    // Apply used to revert policy to Unrestricted (the schedule modal's
    // map.save() re-rendered the form and discarded the unsaved policy widget).
    await page.goto(`${FW_URL}/profiles`);
    await page.waitForLoadState('networkidle');

    var profileTabs = page.locator('.fw-tab:not(.fw-tab-add)');
    if (await profileTabs.count() === 0) {
      await page.locator('.fw-tab-add').click();
      await page.waitForSelector('div.modal', { timeout: 5000 });
      await page.locator('div.modal input[type="text"]').fill('PolicyBugProfile');
      await page.locator('div.modal .cbi-button-positive').click();
      await page.waitForURL(/\/profiles/, { timeout: 10000 });
      await page.waitForLoadState('networkidle');
      await page.waitForTimeout(1000);
      const sb = page.locator('button:has-text("Save"), li:has-text("Save & Apply")').first();
      if (await sb.isVisible()) { await sb.click(); await page.waitForTimeout(5000); }
      await page.reload(); await page.waitForLoadState('networkidle'); await page.waitForTimeout(1500);
      profileTabs = page.locator('.fw-tab:not(.fw-tab-add)');
    }

    await profileTabs.first().click();
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(1000);

    // LuCI ListValue renders as a native <select> with an id (no name attr).
    const policySelect = page.locator('select[id*="policy"]').first();

    // Make sure we start from a non-scheduled policy so the change is real.
    await policySelect.selectOption('allow');
    await page.waitForTimeout(300);

    // 1. Set policy = Scheduled (do NOT save yet). This also reveals the
    //    schedule section (hidden while policy != scheduled).
    await policySelect.selectOption('scheduled');
    await page.waitForTimeout(500);
    console.log('Set policy=scheduled (unsaved)');

    // 2. Add a schedule in the same go. Inline TableSection — no modal; the
    //    new row's fields appear directly in the schedule table.
    const schedSection = page.locator('#cbi-familywarden-schedule');
    await schedSection.locator('.cbi-button-add').click();
    await page.waitForTimeout(1000);
    await selectDays(schedSection, page, ['mon', 'tue']);
    await schedSection.locator('input[id$=".start"]').last().fill('07:00');
    await schedSection.locator('input[id$=".stop"]').last().fill('20:00');

    // 3. Save & Apply
    const saveApply = page.locator('button:has-text("Save"), li:has-text("Save & Apply")').first();
    if (await saveApply.isVisible()) { await saveApply.click({ force: true }); await page.waitForTimeout(5000); }

    // 4. Reload and assert policy is STILL scheduled (the regression)
    await page.reload();
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(1500);
    const policyAfter = await page.locator('select[id*="policy"]').first().inputValue();
    console.log('Policy after save+reload:', policyAfter);
    expect(policyAfter).toBe('scheduled');

    // 5. And assert the DAYS actually persisted (this is what silently broke
    //    before: the row existed but days were empty on disk).
    const days = await selectedDays(schedSection);
    console.log('Days after save+reload:', JSON.stringify(days));
    expect(days).toContain('mon');
    expect(days).toContain('tue');
  });
});
