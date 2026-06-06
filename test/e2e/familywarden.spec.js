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
    await loginAsRoot(page);
  });

  test('creates a new profile via Add Profile modal', async ({ page }) => {
    // Go to profiles page
    await page.goto(`${FW_URL}/profiles`);
    await page.waitForLoadState('networkidle');

    // Count existing profiles
    const getProfileCount = async () => {
      return await page.locator('.fw-tab:not(.fw-tab-add)').count();
    };

    const initialCount = await getProfileCount();
    console.log(`Initial profile count: ${initialCount}`);

    // Click the "+ Add Profile" tab
    const addBtn = page.locator('.fw-tab-add');
    await expect(addBtn).toBeVisible();
    await addBtn.click();

    // Wait for the modal to appear
    await page.waitForSelector('div.modal', { timeout: 5000 });

    // Fill in profile name
    const nameInput = page.locator('div.modal input[type="text"]');
    await expect(nameInput).toBeVisible();
    const testProfileName = 'TestProfile_' + Date.now();
    await nameInput.fill(testProfileName);
    console.log(`Creating profile: ${testProfileName}`);

    // Click Create button
    const createBtn = page.locator('div.modal .cbi-button-positive');
    await expect(createBtn).toBeVisible();
    await createBtn.click();

    // Wait for redirect back to profiles list
    await page.waitForURL(/\/profiles/, { timeout: 10000 });
    console.log(`Redirected to: ${page.url()}`);

    // NOW click Save & Apply to commit the changes
    // The button text is "Save & Apply" (HTML entity &amp; = &)
    const saveBtn = page.locator('button:has-text("Save"), li:has-text("Save & Apply")').first();
    if (await saveBtn.isVisible()) {
      console.log('Clicking Save & Apply');
      await saveBtn.click();
      await page.waitForTimeout(5000); // Wait for UCI commit + page reload
    }

    // Reload page to get fresh UCI state
    await page.reload();
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(2000);

    // Verify profile was CREATED
    const newCount = await getProfileCount();
    console.log(`Profile count after create: ${newCount}`);
    expect(newCount).toBe(initialCount + 1);
  });

  test('adds a device to existing profile', async ({ page }) => {
    // Go to profiles page
    await page.goto(`${FW_URL}/profiles`);
    await page.waitForLoadState('networkidle');

    // Ensure we have a profile to work with
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

    // Click on first profile
    await profileTabs.first().click();
    await page.waitForLoadState('networkidle');

    // Look for devices select
    const deviceSelect = page.locator('select[name*="device"]').first();

    if (await deviceSelect.isVisible()) {
      const options = await deviceSelect.locator('option').count();
      console.log(`Device options available: ${options}`);

      if (options > 0) {
        await deviceSelect.selectOption({ index: 1 });
        console.log('Selected a device');
      }
    }

    // Find and click Save
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

    // Ensure we have a profile to delete
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

    // Ensure we have at least 2 profiles (one to view, one to delete)
    if (profileCount < 2) {
      console.log(`Only ${profileCount} profile(s), creating one more for delete test...`);
      await page.locator('.fw-tab-add').click();
      await page.waitForSelector('div.modal', { timeout: 5000 });
      await page.locator('div.modal input[type="text"]').fill('DeleteMeProfile');
      await page.locator('div.modal .cbi-button-positive').click();
      await page.waitForURL(/\/profiles/, { timeout: 10000 });
      await page.waitForLoadState('networkidle');
      await page.waitForTimeout(1000);

      // Now click Save & Apply
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

    page.on('dialog', dialog => {
      console.log(`Dialog: ${dialog.message()}`);
      dialog.accept();
    });

    const deleteBtn = page.locator('.fw-delete-btn, button:has-text("Delete")').first();

    if (await deleteBtn.isVisible()) {
      console.log('Found delete button, clicking');
      await deleteBtn.click();

      await page.waitForURL(/\/profiles/, { timeout: 10000 });
      await page.waitForLoadState('networkidle');
      await page.waitForTimeout(1000);

      // Click Save & Apply to commit the deletion
      const saveBtn = page.locator('button:has-text("Save"), li:has-text("Save & Apply")').first();
      if (await saveBtn.isVisible()) {
        console.log('Clicking Save & Apply');
        await saveBtn.click();
        await page.waitForTimeout(5000);
      }

      // Reload page to get fresh UCI state
      await page.reload();
      await page.waitForLoadState('networkidle');
      await page.waitForTimeout(2000);

      // Verify the profile was deleted
      const newCount = await page.locator('.fw-tab:not(.fw-tab-add)').count();
      console.log(`Profile count after delete: ${newCount}`);
      expect(newCount).toBe(profileCount - 1);
    }
  });
});
