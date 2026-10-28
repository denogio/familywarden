const { test, expect } = require('@playwright/test');
const BASE = process.env.FW_BASE_URL || 'http://localhost:8080';
const CONTROL = `${BASE}/cgi-bin/luci/admin/services/familywarden/control`;

function replies() {
  return {
    configuration_export:{owner:'core',pending:false,configuration:{version:1,revision:'r1',pact:null,current:{
      settings:{enabled:'1',default_policy:'allow',captain:'alex',delay_hours:'24',dns_domain:'dns.example.test',resolver_url:''},
      profiles:[{id:'alex',name:'Alex',enabled:'1',policy:'allow',block_router:'0',devices:[],dns:'0',client_id:'',protections:null,services:null,
        apps:'1',suspend_new:'1',screen_minutes:'40',screen_weekday:'',screen_weekend:'',always_allowed:null}],
      schedules:null,apps:null,downtime:null,legacy_phones:null}}},
    configuration_report:{profiles:{alex:{name:'Alex',status:'allowed',policy:'allow',override:'none',override_expires:0}},configuration:{enabled:true}},
    core_status:{running:true,core:'test-version'},
    configuration_status:{owner:'core',network_application:{state:'acknowledged',intent_id:'fixture',revision:'r1',acknowledged_at:1800000000}},
    configuration_dns_status:{format:'dns-state-v1',credentials:false,time:0,error:'',pending:true,profiles:{}},
    devices:{devices:[],pending:[]},usage:{profiles:[],day:'2027-01-15'},requests:{requests:[]},new_apps:{profiles:[]},
    configuration_dns_services:{services:[{id:'youtube',name:'YouTube'}]}
  };
}

async function neutral(page, { lang = 'en', data = replies(), interval = 60000 } = {}) {
  await page.route(`${BASE}/neutral-control`, route => route.fulfill({contentType:'text/html',body:'<!doctype html><html><body><div id="control"></div></body></html>'}));
  await page.goto(`${BASE}/neutral-control`);
  await page.evaluate(async ({data,lang,interval}) => {
    window.replies = data; window.calls = []; window.hold = ''; window.release = null;
    const module = await import('/luci-static/resources/familywarden/shared/control.js');
    window.control = module.mount(document.getElementById('control'),{lang,interval,call:async (method,params = {}) => {
      window.calls.push({method,params:JSON.parse(JSON.stringify(params))});
      if (method === window.hold) await new Promise(resolve => {window.release = resolve;});
      if (method === 'configuration_apply') {
        window.replies.configuration_export.configuration.current = params.current;
        window.replies.configuration_export.configuration.revision = 'r2';
        window.replies.configuration_export.pending = true;
        return {status:'confirmation_required',revision:'r2',token:'test-confirmation'};
      }
      if (method === 'configuration_confirm') {window.replies.configuration_export.pending = false;return {status:'ok',revision:'r2'};}
      if (Object.hasOwn(window.replies,method)) return window.replies[method];
      return {status:'ok'};
    }});
  },{data,lang,interval});
  await expect(page.locator('.fwa .fw-status')).toContainText(lang === 'da' ? 'Opdateret' : 'Updated');
  await page.waitForFunction(() => document.querySelector('#control > link[rel="stylesheet"]')?.sheet != null);
}

test.describe('shared control without LuCI globals', () => {
  test('complete EN/DA navigation and editors use the same production module', async ({ page }) => {
    for (const lang of ['en','da']) {
      await neutral(page,{lang});
      expect(await page.evaluate(() => typeof window.L)).toBe('undefined');
      await expect(page.locator('.fw-nav button')).toHaveText(lang === 'da'
        ? ['Familie','Alex','Anmodninger','Enheder','Ulysses','Administration'] : ['Family','Alex','Requests','Devices','Ulysses','Administration']);
      await page.getByRole('button',{name:lang === 'da' ? 'Rediger regler' : 'Edit rules',exact:true}).click();
      await expect(page.getByLabel(lang === 'da' ? 'Navn' : 'Name',{exact:true})).toHaveValue('Alex');
      await expect(page.getByText(lang === 'da' ? 'Regler pr. app' : 'Per-app rules',{exact:true})).toBeVisible();
      await page.evaluate(() => window.control.stop());
    }
  });

  test('prototype layout keeps sections spaced, cards in columns and optional content out of the DOM', async ({ page }) => {
    const data = replies();
    data.configuration_export.configuration.current.profiles.push({
      ...data.configuration_export.configuration.current.profiles[0], id:'sam', name:'Sam'
    });
    data.configuration_report.app_profiles = {alex:{enabled:true,screen_budget_minutes:40}};
    data.usage.profiles = [{id:'alex',total:1200,devices:[],apps:[{package:'com.example.video',label:'Video',seconds:600}]}];
    await page.setViewportSize({width:1440,height:1100});
    await neutral(page,{data});
    await expect(page.locator('[data-profile="alex"] .fw-big')).toHaveText('20min left');
    await expect(page.locator('[data-profile="sam"]')).toContainText('Status unknown');
    await expect(page.locator('[data-profile="sam"]')).toContainText('Unavailable');
    await expect(page.locator('.fw-content')).not.toContainText(/\bnull\b|\[object /);
    expect(await page.locator('.fw-content').evaluate(node => getComputedStyle(node).gap)).toBe('22px');
    const cards = page.locator('.fw-kid');
    const first = await cards.nth(0).boundingBox(), second = await cards.nth(1).boundingBox();
    expect(second.x).toBeGreaterThan(first.x + first.width);
    expect(second.y).toBe(first.y);
    await page.locator('[data-profile="alex"]').getByRole('button',{name:'Today',exact:true}).click();
    await expect(page.getByRole('button',{name:'Block until reset',exact:true})).toBeVisible();
    await expect(page.locator('.fw-approw').getByText('Video',{exact:true})).toBeVisible();
    await expect(page.locator('.fw-content')).not.toContainText(/\bnull\b|\[object /);
    await expect(page.getByRole('tab',{name:'Today',exact:true})).toHaveAttribute('aria-selected','true');
  });

  test('an empty usage aggregate is not a measured zero without a phone report', async ({ page }) => {
    const data = replies();
    data.configuration_report.app_profiles = {alex:{enabled:true,screen_budget_minutes:40}};
    data.usage.profiles = [{id:'alex',total:0,devices:[],apps:[],week:[{day:data.usage.day,total:0,reported:false}]}];
    await neutral(page,{data});
    await expect(page.locator('[data-profile="alex"]')).toContainText('Unavailable');
    await expect(page.locator('[data-profile="alex"] .fw-big')).toHaveCount(0);
    await page.evaluate(() => {
      window.replies.usage.profiles[0].devices = [{name:'Alex phone',total:0}];
      return window.control.refresh();
    });
    await expect(page.locator('[data-profile="alex"] .fw-big')).toHaveText('40min left');
    await page.locator('[data-profile="alex"]').getByRole('button',{name:'Today',exact:true}).click();
    await expect(page.locator('.fw-big')).toHaveText('0min');
  });

  for (const lang of ['en','da']) test(`History uses the prototype panels without inventing periods, budgets or reporting coverage (${lang})`, async ({ page }) => {
    const data = replies();
    data.usage.profiles = [{id:'alex',total:3600,devices:[{name:'Alex phone',total:3600}],
      apps:[{package:'com.example.video',label:'Video',seconds:1800}],week:[
        {day:'2027-01-09',total:1200,reported:true}, {day:'2027-01-10',total:0,reported:false},
        {day:'2027-01-11',total:0,reported:true}, {day:'2027-01-12',total:600},
        {day:'2027-01-13',total:0}, {day:'2027-01-14',total:1800,reported:true},
        {day:'2027-01-15',total:3600,reported:true}
      ]}];
    await neutral(page,{data,lang});
    await page.locator('.fw-nav').getByRole('button',{name:'Alex',exact:true}).click();
    await page.getByRole('tab',{name:lang === 'da' ? 'Historik' : 'History',exact:true}).click();
    await expect(page.locator('.fw-history-screen h2')).toHaveText(lang === 'da' ? 'Skærmtid' : 'Screen time');
    await expect(page.locator('.fw-histsum dd')).toHaveText(lang === 'da'
      ? ['2 t','24 min','—','4 af 7 dage'] : ['2 h','24 min','—','4 of 7 days']);
    await expect(page.locator('[data-history-day="2027-01-10"] .bar')).toHaveCount(0);
    await expect(page.locator('[data-history-day="2027-01-11"] .val')).toHaveText('0 min');
    await expect(page.locator('[data-history-day="2027-01-13"] .bar')).toHaveCount(0);
    await expect(page.locator('.fw-hist .bar')).toHaveCount(5);
    await expect(page.getByRole('button',{name:lang === 'da' ? 'Måned' : 'Month',exact:true})).toBeDisabled();
    await expect(page.getByRole('button',{name:lang === 'da' ? 'År' : 'Year',exact:true})).toBeDisabled();
    await expect(page.locator('.fw-history-apps').getByText('Video',{exact:true})).toBeVisible();
    expect((await page.locator('.fw-history-apps .fw-meter').boundingBox()).width).toBeGreaterThan(0);
    await page.locator('[data-history-day="2027-01-14"] button').click();
    await expect(page.locator('.fw-history-apps').getByText('Video',{exact:true})).toHaveCount(0);
    await page.locator('[data-history-day="2027-01-15"] button').click();
    await expect(page.locator('.fw-history-apps').getByText('Video',{exact:true})).toBeVisible();
    for (const width of [390,320]) {
      await page.setViewportSize({width,height:844});
      expect(await page.locator('.fwa').evaluate(node => node.scrollWidth <= node.clientWidth + 1)).toBe(true);
    }
    await page.evaluate(() => {
      window.replies.usage.profiles[0].week.forEach(day => {day.total = 0;day.reported = false;});
      return window.control.refresh();
    });
    await expect(page.locator('.fw-hist .bar')).toHaveCount(0);
    await expect(page.locator('.fw-histsum dd').nth(0)).toHaveText('—');
    expect(await page.evaluate(() => window.calls.some(c => c.method === 'configuration_apply'))).toBe(false);
  });

  test('person tabs do not scroll vertically and keep horizontal navigation on narrow hosts', async ({ page }) => {
    await neutral(page);
    await page.locator('.fw-nav').getByRole('button',{name:'Alex',exact:true}).click();
    await page.getByRole('tab',{name:'History',exact:true}).click();
    const tabs = page.getByRole('tablist');
    for (const width of [1440,390,320]) {
      await page.setViewportSize({width,height:844});
      const size = await tabs.evaluate(node => ({height:node.clientHeight,scrollHeight:node.scrollHeight}));
      expect(size.scrollHeight).toBeLessThanOrEqual(size.height);
    }
    expect(await tabs.evaluate(node => node.scrollWidth > node.clientWidth)).toBe(true);
    expect(await tabs.evaluate(node => {node.scrollLeft = node.scrollWidth;return node.scrollLeft;})).toBeGreaterThan(0);
    await page.getByRole('tab',{name:'Devices',exact:true}).click();
    await expect(page.getByRole('tab',{name:'Devices',exact:true})).toHaveAttribute('aria-selected','true');
  });

  test('Rules keeps person tabs available and preserves a draft until navigation is confirmed', async ({ page }) => {
    await neutral(page);
    await page.getByRole('button',{name:'Today',exact:true}).click();
    await page.getByRole('tab',{name:'Rules',exact:true}).click();
    await expect(page.getByRole('tab')).toHaveCount(4);
    await expect(page.getByRole('tab',{name:'Rules',exact:true})).toHaveAttribute('aria-selected','true');
    await expect(page.getByRole('checkbox',{name:'Enabled',exact:true})).not.toHaveClass(/fw-input/);
    await page.getByLabel('Name',{exact:true}).fill('Unsaved Alex');
    await page.getByRole('tab',{name:'Rules',exact:true}).click();
    await expect(page.getByLabel('Name',{exact:true})).toHaveValue('Unsaved Alex');
    page.once('dialog', dialog => dialog.dismiss());
    await page.getByRole('tab',{name:'Today',exact:true}).click();
    await expect(page.getByLabel('Name',{exact:true})).toHaveValue('Unsaved Alex');
    page.once('dialog', dialog => dialog.accept());
    await page.getByRole('tab',{name:'Today',exact:true}).click();
    await expect(page.getByRole('tab',{name:'Today',exact:true})).toHaveAttribute('aria-selected','true');
    await expect(page.getByLabel('Name',{exact:true})).toHaveCount(0);
    expect(await page.evaluate(() => window.calls.filter(c => c.method === 'configuration_apply').length)).toBe(0);
  });

  test('Rules follows the prototype panels and commits budget patterns and app choices without flattening other fields', async ({ page }) => {
    const data = replies();
    data.configuration_export.configuration.current.apps = [{id:'a_video',profile:'alex',package:'com.example.video',minutes:'15',blocked:'0'}];
    data.configuration_export.configuration.current.profiles[0].always_allowed = ['com.example.reader'];
    data.usage.profiles = [{id:'alex',total:600,devices:[],apps:[{package:'com.example.video',label:'Video',seconds:600}]}];
    data.new_apps.profiles = [{id:'alex',pending:[{package:'com.example.draw',label:'Drawing'}],approved:[]}];
    await neutral(page,{data});
    await page.getByRole('button',{name:'Edit rules',exact:true}).click();
    await expect(page.locator('.fw-rules > .fw-rule-panel .fw-h2')).toHaveText([
      'Daily screen-time budget','Apps','Downtime','Home internet schedule'
    ]);
    await expect(page.getByRole('region',{name:'Unsaved changes',exact:true})).toHaveCount(0);
    await expect(page.locator('.fw-rule-apps')).toContainText('Drawing');
    await expect(page.locator('.fw-rule-apps')).toContainText('Waiting for a decision');
    await expect(page.getByLabel('Rule for Drawing',{exact:true})).toHaveCount(0);
    await page.getByRole('button',{name:'Weekdays + weekend',exact:true}).click();
    await page.getByLabel('Weekday screen time (optional)',{exact:true}).fill('25');
    await page.getByLabel('Weekend screen time (optional)',{exact:true}).fill('60');
    await page.getByLabel('Rule for Video',{exact:true}).selectOption('unlimited');
    await expect(page.getByRole('switch',{name:'Pause newly installed apps',exact:true})).toHaveAttribute('aria-checked','true');
    await page.getByRole('switch',{name:'Pause newly installed apps',exact:true}).click();
    await expect(page.getByRole('region',{name:'Unsaved changes',exact:true})).toBeVisible();
    await page.getByRole('button',{name:'Save rules',exact:true}).click();
    const applied = await page.evaluate(() => window.calls.find(c => c.method === 'configuration_apply'));
    expect(applied.params.current.profiles[0]).toMatchObject({screen_minutes:'40',screen_weekday:'25',screen_weekend:'60',suspend_new:'0',always_allowed:['com.example.reader','com.example.video']});
    expect(applied.params.current.apps).toEqual([]);
    expect(applied.params.current.schedules).toBeNull();
    expect(applied.params.current.profiles[0].protections).toBeNull();
  });

  test('Rules edits remain local, discard restores the draft, and the layout fits a phone', async ({ page }) => {
    await neutral(page);
    await page.getByRole('button',{name:'Edit rules',exact:true}).click();
    await page.getByLabel('Daily screen time (minutes)',{exact:true}).fill('45');
    await page.getByRole('button',{name:'Discard',exact:true}).click();
    await expect(page.getByLabel('Daily screen time (minutes)',{exact:true})).toHaveValue('40');
    await expect(page.getByRole('region',{name:'Unsaved changes',exact:true})).toHaveCount(0);
    await page.setViewportSize({width:390,height:844});
    await page.getByLabel('Daily screen time (minutes)',{exact:true}).fill('50');
    expect(await page.locator('.fwa').evaluate(node => node.scrollWidth <= node.clientWidth + 1)).toBe(true);
    expect(await page.evaluate(() => window.calls.filter(c => c.method === 'configuration_apply').length)).toBe(0);
    await expect(page.locator('.fw-content')).not.toContainText(/null|\[object /);
  });

  test('profile save preserves untouched fields and requires explicit connection confirmation', async ({ page }) => {
    await neutral(page);
    await page.getByRole('button',{name:'Edit rules',exact:true}).click();
    await page.getByLabel('Name',{exact:true}).fill('Alex updated');
    await page.getByRole('button',{name:'Save rules',exact:true}).click();
    await expect(page.getByRole('button',{name:'Confirm connection',exact:true})).toBeEnabled();
    const calls = await page.evaluate(() => window.calls);
    const applied = calls.find(c => c.method === 'configuration_apply');
    expect(applied.params.revision).toBe('r1');
    expect(applied.params.current.profiles[0].name).toBe('Alex updated');
    expect(applied.params.current.profiles[0].protections).toBeNull();
    expect(applied.params.current.schedules).toBeNull();
    expect(applied.params.current.legacy_phones).toBeNull();
    expect(calls.filter(c => c.method === 'configuration_confirm')).toHaveLength(0);
    await page.getByRole('button',{name:'Confirm connection',exact:true}).click();
    await expect.poll(() => page.evaluate(() => window.calls.filter(c => c.method === 'configuration_confirm').length)).toBe(1);
    await expect(page.getByRole('button',{name:'Confirm connection',exact:true})).toHaveCount(0);
  });

  test('background polling retains drafts and CAS conflicts cannot overwrite a concurrent edit', async ({ page }) => {
    await neutral(page,{interval:60});
    await page.getByRole('button',{name:'Edit rules',exact:true}).click();
    await page.getByLabel('Name',{exact:true}).fill('Unsaved draft');
    await page.evaluate(() => {window.replies.configuration_export.configuration.revision = 'other';window.replies.configuration_export.configuration.current.profiles[0].name = 'Concurrent edit';});
    await page.waitForTimeout(150);
    await expect(page.getByLabel('Name',{exact:true})).toHaveValue('Unsaved draft');
    await page.getByRole('button',{name:'Refresh',exact:true}).click();
    await expect(page.getByLabel('Name',{exact:true})).toHaveValue('Unsaved draft');
    await page.getByRole('button',{name:'Save rules',exact:true}).click();
    await expect(page.locator('[role=alert]')).toContainText('configuration changed');
    expect(await page.evaluate(() => window.calls.filter(c => c.method === 'configuration_apply').length)).toBe(0);
    await page.getByRole('button',{name:'Reload editor from core',exact:true}).click();
    await expect(page.getByLabel('Name',{exact:true})).toHaveValue('Concurrent edit');
  });

  test('failed critical reads keep old data visible and disable mutations until recovery', async ({ page }) => {
    await neutral(page);
    await page.evaluate(() => {window.replies.configuration_report = {};});
    await page.getByRole('button',{name:'Refresh',exact:true}).click();
    await expect(page.locator('.fwa .fw-status')).toContainText('actions are disabled');
    await expect(page.getByRole('heading',{name:'Alex',exact:true})).toBeVisible();
    await expect(page.getByRole('button',{name:'Edit rules',exact:true})).toBeDisabled();
    await page.evaluate(() => {window.replies.configuration_report = {profiles:{},configuration:{}};});
    await page.getByRole('button',{name:'Refresh',exact:true}).click();
    await expect(page.getByRole('button',{name:'Edit rules',exact:true})).toBeEnabled();
  });

  test('DNS failures keep acknowledged profiles visible instead of presenting an empty successful state', async ({ page }) => {
    const data = replies();
    data.configuration_dns_status.error = 'AdGuard Home unreachable';
    data.configuration_dns_status.profiles = {alex:{hostname:'fw-alex.dns.example.test'}};
    await neutral(page,{data});
    await page.locator('.fw-nav').getByRole('button',{name:'Administration',exact:true}).click();
    await expect(page.getByText('AdGuard Home unreachable',{exact:true})).toBeVisible();
    await expect(page.getByText('fw-alex.dns.example.test',{exact:true})).toBeVisible();
    await expect(page.getByText('not live dataplane or phone verification',{exact:false})).toBeVisible();
  });

  test('Ulysses form renders and the primary action creates the chosen pact', async ({ page }) => {
    await neutral(page);
    await page.locator('.fw-nav').getByRole('button',{name:'Ulysses',exact:true}).click();
    await page.getByRole('button',{name:'Set sail',exact:true}).click();
    await expect(page.getByLabel('Pact',{exact:true})).toBeFocused();
    await page.getByLabel('Pact',{exact:true}).selectOption('ongoing');
    page.once('dialog', dialog => dialog.accept());
    await page.getByRole('button',{name:'Make a pact',exact:true}).click();
    await expect.poll(() => page.evaluate(() => window.calls.filter(c => c.method === 'configuration_pact').length)).toBe(1);
    const call = await page.evaluate(() => window.calls.find(c => c.method === 'configuration_pact'));
    expect(call.params.until).toBe(0);
    expect(call.params.window_day).toBe('sun');
  });

  test('DNS credential button submits the write-only form', async ({ page }) => {
    await neutral(page);
    await page.locator('.fw-nav').getByRole('button',{name:'Administration',exact:true}).click();
    await page.getByLabel('Username',{exact:true}).fill('fixture-admin');
    await page.getByLabel('Password (write-only)',{exact:true}).fill('fixture-password');
    await page.getByRole('button',{name:'Store credentials',exact:true}).click();
    await expect.poll(() => page.evaluate(() => window.calls.filter(c => c.method === 'configuration_dns_auth').length)).toBe(1);
    expect(await page.evaluate(() => window.calls.find(c => c.method === 'configuration_dns_auth').params))
      .toEqual({username:'fixture-admin',password:'fixture-password'});
    await expect(page.getByLabel('Password (write-only)',{exact:true})).toHaveValue('');
  });

  test('fingerprint confirmation is required and the observed fingerprint is bound to approval', async ({ page }) => {
    const data = replies();data.devices.pending = [{id:'pending-device',name:'Alex phone',fingerprint:'fixture-fingerprint'}];
    await neutral(page,{data});
    await page.locator('.fw-nav').getByRole('button',{name:'Devices',exact:true}).click();
    await page.getByRole('button',{name:'Approve phone',exact:true}).click();
    expect(await page.evaluate(() => window.calls.filter(c => c.method === 'device_approve').length)).toBe(0);
    await page.getByRole('checkbox',{name:'I compared this fingerprint with the one shown on the phone.',exact:true}).check();
    await page.getByRole('button',{name:'Approve phone',exact:true}).click();
    await expect.poll(() => page.evaluate(() => window.calls.filter(c => c.method === 'device_approve').length)).toBe(1);
    const call = await page.evaluate(() => window.calls.find(c => c.method === 'device_approve'));
    expect(call.params.fingerprint).toBe('fixture-fingerprint');
  });

  test('legacy ownership needs explicit import acknowledgement and cannot change silently', async ({ page }) => {
    const data = replies();data.configuration_export.owner = 'legacy';
    await neutral(page,{data});
    await expect(page.getByRole('button',{name:'Import router configuration',exact:true})).toBeDisabled();
    await page.getByRole('checkbox',{name:/I have backed up/}).check();
    await page.getByRole('button',{name:'Import router configuration',exact:true}).click();
    await expect.poll(() => page.evaluate(() => window.calls.filter(c => c.method === 'configuration_import').length)).toBe(1);
  });

  test('frozen profiles offer DNS-only commits but cannot submit ordinary rule edits', async ({ page }) => {
    const data = replies();
    data.configuration_export.configuration.pact = {captain:'alex',type:'ongoing',until:'0',window_day:'sun',window_start:'10:00',window_stop:'11:00'};
    data.configuration_report.profiles.alex.pact_in_window = false;
    data.configuration_export.configuration.current.schedules = [{id:'w_fixture',profile:'alex',days:'mon',start:'07:00',stop:'20:00'}];
    data.configuration_dns_update = {status:'ok',revision:'r2'};
    await neutral(page,{data});
    await page.getByRole('button',{name:'Edit rules',exact:true}).click();
    await expect(page.getByLabel('Name',{exact:true})).toBeDisabled();
    await expect(page.getByRole('group',{name:'Internet allow windows',exact:true}).getByRole('button',{name:'Monday',exact:true})).toBeDisabled();
    await expect(page.getByRole('button',{name:'Delete',exact:true})).toBeDisabled();
    await page.getByRole('checkbox',{name:'Strict DNS',exact:true}).check();
    await expect(page.getByRole('button',{name:'Save rules',exact:true})).toBeDisabled();
    await page.getByRole('button',{name:'Commit DNS-only change',exact:true}).click();
    await expect.poll(() => page.evaluate(() => window.calls.filter(c => c.method === 'configuration_dns_update').length)).toBe(1);
    const call = await page.evaluate(() => window.calls.find(c => c.method === 'configuration_dns_update'));
    expect(call.params.current.profiles[0].dns).toBe('1');
    expect(call.params.current.profiles[0].name).toBe('Alex');
    expect(call.params.current.profiles[0].always_allowed).toBeNull();
    expect(await page.evaluate(() => window.calls.filter(c => c.method === 'configuration_apply').length)).toBe(0);
  });

  test('a held mutation disables related controls and a refusal survives successful refresh', async ({ page }) => {
    const data = replies();data.configuration_block = {error:'fixture refusal'};
    await neutral(page,{data});
    await page.getByRole('button',{name:'Today',exact:true}).click();
    await page.evaluate(() => {window.hold = 'configuration_block';});
    await page.getByRole('button',{name:'Block until reset',exact:true}).click();
    await expect.poll(() => page.evaluate(() => typeof window.release)).toBe('function');
    await expect(page.getByRole('button',{name:'Add 15 minutes',exact:true})).toBeDisabled();
    await page.evaluate(() => window.release());
    await expect(page.locator('[role=alert]')).toContainText('fixture refusal');
    await page.getByRole('button',{name:'Refresh',exact:true}).click();
    await expect(page.locator('[role=alert]')).toContainText('fixture refusal');
    await expect(page.getByRole('button',{name:'Add 15 minutes',exact:true})).toBeEnabled();
  });

  test('shared layouts fit narrow and large-text hosts without changing host styles', async ({ page }) => {
    await neutral(page,{lang:'da'});
    const before = await page.locator('body').evaluate(node => getComputedStyle(node).color);
    for (const width of [1100,720,390,320]) {
      await page.setViewportSize({width,height:1000});
      expect(await page.locator('.fwa').evaluate(node => node.scrollWidth <= node.clientWidth + 1)).toBe(true);
    }
    await page.evaluate(() => {document.documentElement.style.fontSize = '200%';});
    expect(await page.locator('.fwa').evaluate(node => node.scrollWidth <= node.clientWidth + 1)).toBe(true);
    expect(await page.locator('body').evaluate(node => getComputedStyle(node).color)).toBe(before);
  });

  test('Requests keeps its draft across workspace refreshes without restyling the host', async ({ page }) => {
    const data = replies(), now = Math.floor(Date.now() / 1000);
    data.requests.requests = [{request_id:'r-layout-fixture',profile:'alex',profile_name:'Alex',
      target:'total',minutes:30,received:now,expires:now+300,state:'pending'}];
    await neutral(page,{data});
    const color = await page.locator('#control').evaluate(node => getComputedStyle(node).color);
    await page.locator('.fw-nav').getByRole('button',{name:'Requests',exact:true}).click();
    await page.locator('.fwx').getByLabel('Minutes').fill('20');
    await page.evaluate(() => window.control.refresh());
    await expect(page.locator('.fwx').getByLabel('Minutes')).toHaveValue('20');
    expect(await page.locator('#control').evaluate(node => getComputedStyle(node).color)).toBe(color);
    await expect(page.locator('.fw-content')).not.toContainText(/null|\[object /);
  });

  test('stopping a mount ignores late replies and stops polling', async ({ page }) => {
    await neutral(page,{interval:50});
    await page.evaluate(() => {window.hold = 'configuration_export';window.control.refresh();});
    await expect.poll(() => page.evaluate(() => typeof window.release)).toBe('function');
    await page.evaluate(() => {window.control.stop();window.release();});
    const count = await page.evaluate(() => window.calls.length);
    await page.waitForTimeout(120);
    expect(await page.evaluate(() => window.calls.length)).toBe(count);
    await expect(page.getByRole('button',{name:'Edit rules',exact:true})).toBeDisabled();
  });
});

test.describe('shared control through the native LuCI bridge', () => {
  test.skip(process.env.FW_E2E_SHARED_PHASE !== '1' || !process.env.FW_E2E_CONTAINER?.startsWith('familywarden-e2e-') || process.env.FW_E2E_ALLOW_PACT !== '1',
    'explicit ownership import requires a disposable container');
  test.describe.configure({mode:'serial'});
  test('import, shared profile editor, explicit confirm and native routing work without UCI forms', async ({ page }) => {
    await page.goto(`${BASE}/cgi-bin/luci/admin/login`);
    await page.fill('[name=luci_username]','root');
    await page.fill('[name=luci_password]',process.env.FW_PASSWORD || 'e2e-sirens');
    await page.getByRole('button',{name:'Log in',exact:true}).click();
    await page.waitForLoadState('networkidle');
    await page.goto(CONTROL);
    await expect(page.locator('.fwa .fw-status')).toContainText('Updated');
    const importButton = page.getByRole('button',{name:'Import router configuration',exact:true});
    if (await importButton.count()) {
      await page.getByRole('checkbox',{name:/I have backed up/}).check();
      await importButton.click();
    }
    await expect(page.locator('.fwa')).toBeVisible();
    const card = page.locator('[data-profile="alex"]');
    await expect(card).toBeVisible();
    await card.getByRole('button',{name:'Edit rules',exact:true}).click();
    const oldName = await page.getByLabel('Name',{exact:true}).inputValue();
    const nextName = oldName === 'Alex shared' ? 'Alex shared again' : 'Alex shared';
    await page.getByLabel('Name',{exact:true}).fill(nextName);
    await page.getByRole('button',{name:'Save rules',exact:true}).click();
    await expect(page.getByRole('button',{name:'Confirm connection',exact:true})).toBeEnabled();
    await page.getByRole('button',{name:'Confirm connection',exact:true}).click();
    await expect(page.getByRole('heading',{name:nextName,exact:true,level:1})).toBeVisible();
    await expect(page.getByRole('button',{name:'Confirm connection',exact:true})).toHaveCount(0);
    await page.goto(`${BASE}/cgi-bin/luci/admin/services/familywarden`);
    await expect(page.locator('.fwa')).toBeVisible();
    await expect(page.getByRole('heading',{name:nextName,exact:true})).toBeVisible();
  });

  test('shared app, schedule and settings editors commit through core CAS without UCI writes', async ({ page }) => {
    await page.goto(`${BASE}/cgi-bin/luci/admin/login`);
    await page.fill('[name=luci_username]','root');
    await page.fill('[name=luci_password]',process.env.FW_PASSWORD || 'e2e-sirens');
    await page.getByRole('button',{name:'Log in',exact:true}).click();
    await page.waitForLoadState('networkidle');
    const writes = [];
    page.on('request',request => {
      if (!/\/ubus(?:\/|$)/.test(request.url())) return;
      const data = request.postDataJSON();
      for (const call of Array.isArray(data) ? data : [data]) if (call?.params?.[1] === 'uci' && ['set','add','delete','commit','apply'].includes(call.params[2])) writes.push(call.params[2]);
    });
    await page.goto(CONTROL);
    await page.locator('[data-profile="alex"]').getByRole('button',{name:'Edit rules',exact:true}).click();
    const currentName = await page.getByLabel('Name',{exact:true}).inputValue();
    await page.getByRole('checkbox',{name:'App control',exact:true}).check();
    await page.getByLabel('Daily screen time (minutes)',{exact:true}).fill('45');
    const saveBar = await page.getByRole('region',{name:'Unsaved changes',exact:true}).boundingBox();
    expect(saveBar.y + saveBar.height).toBeLessThanOrEqual(page.viewportSize().height);
    await page.getByLabel('Internet policy',{exact:true}).selectOption('scheduled');
    const windows = page.getByRole('group',{name:'Internet allow windows',exact:true});
    await windows.getByRole('button',{name:'+ Add window',exact:true}).click();
    await windows.getByLabel('Start',{exact:true}).fill('22:00');
    await windows.getByLabel('Stop',{exact:true}).fill('06:00');
    const apps = page.getByRole('group',{name:'Per-app rules',exact:true});
    await apps.getByRole('button',{name:'+ Add app rule',exact:true}).click();
    await apps.getByLabel('Package name',{exact:true}).fill('com.example.game');
    await apps.getByLabel('Minutes',{exact:true}).fill('10');
    await page.getByRole('button',{name:'Save rules',exact:true}).click();
    await page.getByRole('button',{name:'Confirm connection',exact:true}).click();
    await expect(page.getByRole('heading',{name:currentName,exact:true,level:1})).toBeVisible();
    await expect(page.getByRole('button',{name:'Confirm connection',exact:true})).toHaveCount(0);
    const session = await page.evaluate(() => L.env.sessionid);
    const response = await page.request.post(`${BASE}/ubus/`,{data:{jsonrpc:'2.0',id:1,method:'call',params:[session,'familywarden','configuration_export',{}]}});
    const doc = (await response.json()).result[1].configuration;
    expect(doc.current.profiles.find(p => p.id === 'alex').screen_minutes).toBe('45');
    expect(doc.current.apps.find(p => p.profile === 'alex').package).toBe('com.example.game');
    expect(doc.current.schedules.find(p => p.profile === 'alex').stop).toBe('06:00');
    await page.locator('.fw-nav').getByRole('button',{name:'Administration',exact:true}).click();
    await page.getByRole('button',{name:'Edit rules',exact:true}).click();
    await page.getByLabel('Unassigned network devices',{exact:true}).selectOption('block');
    await page.getByRole('button',{name:'Apply changes',exact:true}).click();
    await page.getByRole('button',{name:'Confirm connection',exact:true}).click();
    await expect(page.getByRole('definition').filter({hasText:/^Last native apply acknowledged$/})).toBeVisible();
    await page.locator('.fw-nav').getByRole('button',{name:currentName,exact:true}).click();
    await page.getByRole('tab',{name:'History',exact:true}).click();
    await expect(page.locator('.fw-history-screen h2')).toHaveText('Screen time');
    await expect(page.locator('.fw-hist-row')).toHaveCount(7);
    await expect(page.locator('.fw-hist .bar')).toHaveCount(0);
    await expect(page.getByRole('button',{name:'Month',exact:true})).toBeDisabled();
    expect(writes).toEqual([]);
  });
});
