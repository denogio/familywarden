// R6 browser driver: real service.New + recording/failing adapter, no mocked
// RPC replies, LuCI globals, UCI forms, router processes or native product code.
const assert = require('node:assert/strict');
const { chromium, expect } = require('@playwright/test');

(async () => {
  const base = process.argv[2];
  assert.match(base, /^http:\/\/127\.0\.0\.1:\d+$/);
  const browser = await chromium.launch();
  try {
    const page = await browser.newPage({ viewport:{width:1440,height:1000} });
    const errors = [], calls = [];
    page.on('pageerror', error => errors.push(error.message));
    page.on('request', request => {
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
