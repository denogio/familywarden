// README desktop screenshots (fictional fixture from scripts/readme-fixture.sh).
// Usage: FW_BASE_URL=… FW_PASSWORD=… FW_OUT=docs/screenshots node test/e2e/readme-screenshots.cjs
const { chromium } = require('playwright');
const path = require('path');

const BASE = process.env.FW_BASE_URL;
const OUT = process.env.FW_OUT;

async function shoot(scheme, file, url, ready, scrollTo) {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1440, height: 1000 }, colorScheme: scheme,
    deviceScaleFactor: 1, locale: 'en-GB', timezoneId: 'UTC' });
  await page.goto(`${BASE}/cgi-bin/luci/admin/login`);
  await page.fill('[name="luci_username"]', 'root');
  await page.fill('[name="luci_password"]', process.env.FW_PASSWORD);
  await page.getByRole('button', { name: 'Log in' }).click();
  await page.waitForLoadState('networkidle');
  await page.goto(`${BASE}/cgi-bin/luci/admin/services/familywarden/${url}`);
  await page.getByText(ready, { exact: false }).first().waitFor();
  await page.waitForLoadState('networkidle');
  // LuCI's polling indicator is transient chrome, not product UI.
  await page.addStyleTag({ content: '[data-indicator] { visibility: hidden !important; }' });
  if (scrollTo) await page.getByText(scrollTo, { exact: false }).first().evaluate((el) => {
    window.scrollTo(0, el.getBoundingClientRect().top + window.scrollY - 64);
  });
  await page.waitForTimeout(800);
  await page.screenshot({ path: path.join(OUT, file), fullPage: false });
  console.log('wrote', file);
  await browser.close();
}

(async () => {
  await shoot('light', 'luci-overview-light.png', 'overview', 'Alex');
  await shoot('dark', 'luci-overview-dark.png', 'overview', 'Alex');
  await shoot('light', 'luci-profile-apps.png', 'profiles?profile=alex', 'Screen time and apps', 'Screen time and apps on approved phones');
  await shoot('light', 'luci-phones.png', 'devices', "Jordan's phone");
  await shoot('light', 'luci-requests.png', 'family', 'Waiting for an answer');
})().catch((e) => { console.error(e); process.exit(1); });
