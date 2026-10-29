// R6 browser driver: real service.New + recording/failing adapter, no mocked
// RPC replies, LuCI globals, UCI forms, router processes or native product code.
const assert = require('node:assert/strict');
const {mkdirSync} = require('node:fs');
const { chromium, expect } = require('@playwright/test');

(async () => {
  const base = process.argv[2];
  assert.match(base, /^http:\/\/127\.0\.0\.1:\d+$/);
  const browser = await chromium.launch();
  try {
    const page = await browser.newPage({ viewport:{width:1440,height:1000},deviceScaleFactor:2 });
    const errors = [], calls = [];
    page.on('pageerror', error => errors.push(error.message));
    page.on('request', request => {
      if (request.url().startsWith('data:image/png;base64,')) return;
      const url = new URL(request.url());
      if (url.pathname.startsWith('/api/')) calls.push(url.pathname.slice(5));
      assert.equal(url.origin, base, 'UI left its local test host');
      assert.equal(url.pathname.includes('luci'), false, 'shared UI required LuCI');
    });
    await page.goto(base);
    await expect(page.locator('.fw-status')).toContainText('Updated');
    assert.equal(await page.evaluate(() => typeof window.L), 'undefined');
    const card = page.locator('[data-profile="alex"]');
    await expect(card).toContainText('15 of 40 min');
    await card.getByRole('button',{name:'Edit rules',exact:true}).click();
    await expect(page.locator('.fw-rule-apps .fw-app-icon img')).toHaveCount(1);
    page.once('dialog', dialog => dialog.accept('com.example.manual'));
    await page.getByRole('button',{name:'+ Add app rule',exact:true}).click();
    await expect(page.getByRole('dialog')).toContainText('Done updates your draft only');
    await page.getByRole('dialog').getByRole('button',{name:'Cancel',exact:true}).click();
    await page.getByLabel('Daily screen time (minutes)',{exact:true}).fill('45');
    await page.getByLabel('Name',{exact:true}).fill('Alex portable');
    await page.getByRole('button',{name:'Save rules',exact:true}).click();
    await page.getByRole('button',{name:'Confirm connection',exact:true}).click();
    await expect(page.getByRole('button',{name:'Confirm connection',exact:true})).toHaveCount(0);
    await expect(page.getByRole('heading',{name:'Alex portable',level:1,exact:true})).toBeVisible();
    await page.getByRole('tab',{name:'History',exact:true}).click();
    await expect(page.locator('.fw-hist-row')).toHaveCount(7);
    await expect(page.locator('.fw-hist .bar')).toHaveCount(1);
    await expect(page.locator('.fw-history-apps').getByText('Video',{exact:true})).toBeVisible();
    const image = page.locator('.fw-history-apps .fw-app-icon img');
    await expect(image).toHaveCount(1);
    await expect.poll(() => image.evaluate(img => img.complete && img.naturalWidth === 96)).toBe(true);
    await expect(image).toHaveAttribute('src', /^data:image\/png;base64,/);
    await expect(page.locator('.fw-history-apps .fw-app-icon').filter({hasText:'L'})).toHaveCount(1);
    await expect(page.locator('.fw-history-apps')).toContainText('com.example.video');
    await expect(image.locator('..')).toHaveClass(/has-image/);
    assert.equal(await image.evaluate(img => getComputedStyle(img).objectFit), 'cover');
    assert.deepEqual(await image.evaluate(img => [img.clientWidth, img.clientHeight]), [34,34]);
    const screenshots = process.env.FW_ICON_SCREENSHOTS;
    if (screenshots) { mkdirSync(screenshots,{recursive:true}); await page.screenshot({path:screenshots + '/control-history-dpr2.png',fullPage:true}); }

    await page.evaluate(async () => {
      const {appIcon} = await import('/web/family.js');
      const tile = appIcon('iVBORw0KGgoAAAA', 'Broken', 'com.example.broken');
      tile.id = 'broken-icon-test';
      document.body.append(tile);
      const svg = appIcon('data:image/svg+xml,<svg onload="alert(1)"/>', 'Safe', 'com.example.safe');
      svg.id = 'unsafe-icon-test'; document.body.append(svg);
    });
    await expect(page.locator('#broken-icon-test')).toHaveText('B');
    await expect(page.locator('#broken-icon-test img')).toHaveCount(0);
    await expect(page.locator('#unsafe-icon-test')).toHaveText('S');
    await expect(page.locator('#unsafe-icon-test img')).toHaveCount(0);
    await page.getByRole('tab',{name:'Today',exact:true}).click();
    await page.getByRole('button',{name:'Block until reset',exact:true}).click();
    await expect(page.getByRole('alert')).toContainText('recording host rejected network apply');
    await expect(page.locator('.fw-status')).toContainText('actions are disabled');
    await expect(page.getByRole('button',{name:'Add 15 minutes',exact:true})).toBeDisabled();
    await expect(page.getByRole('heading',{name:'Alex portable',level:1,exact:true})).toBeVisible();
    assert.equal(calls.filter(method => method === 'configuration_apply').length, 1);
    assert.equal(calls.filter(method => method === 'configuration_confirm').length, 1);
    assert.equal(calls.filter(method => method === 'configuration_block').length, 1);
    assert.deepEqual(errors, []);
    await page.evaluate(() => window.control.stop());
    console.log('Shared UI: real CAS/confirm, authenticated usage/history, failed apply and stale-action guard passed without a router host.');
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
