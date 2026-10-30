const { readFileSync } = require('node:fs');
const { test, expect } = require('@playwright/test');
const { SHARED } = require('./asset');
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
    version:{backend:'familywarden-0.11.0_rc10-r18',frontend:'luci-app-familywarden-0.11.0-r13',configuration_owner:'core'},
    configuration_status:{owner:'core',network_application:{state:'acknowledged',intent_id:'fixture',revision:'r1',acknowledged_at:1800000000}},
    configuration_dns_status:{format:'dns-state-v1',credentials:false,time:0,error:'',pending:true,profiles:{}},
    devices:{devices:[],pending:[]},usage:{profiles:[],day:'2027-01-15'},requests:{requests:[]},new_apps:{profiles:[]},
    configuration_dns_services:{services:[{id:'youtube',name:'YouTube'}]}
  };
}

async function neutral(page, { lang = 'en', data = replies(), interval = 60000, theme = 'auto' } = {}) {
  await page.route(`${BASE}/neutral-control`, route => route.fulfill({contentType:'text/html',body:'<!doctype html><html><body><div id="control"></div></body></html>'}));
  await page.goto(`${BASE}/neutral-control`);
  await page.evaluate(async ({data,lang,interval,theme,shared}) => {
    window.replies = data; window.calls = []; window.hold = ''; window.release = null;
    const module = await import(`${shared}/control.js`);
    window.control = module.mount(document.getElementById('control'),{lang,interval,theme,call:async (method,params = {}) => {
      window.calls.push({method,params:JSON.parse(JSON.stringify(params))});
      if (method === window.hold) await new Promise(resolve => {window.release = resolve;});
      if (method === 'admin_installed') {
        if (window.inventoryHandler) return window.inventoryHandler(params);
        if (window.replies.admin_installed) return window.replies.admin_installed;
        const p = window.replies.configuration_export.configuration.current.profiles.find(p => p.id === params.profile);
        const configured = window.replies.configuration_export.configuration.current.apps || [];
        const observed = window.replies.usage.profiles.find(p => p.id === params.profile)?.apps || [];
        const pending = window.replies.new_apps.profiles.find(p => p.id === params.profile)?.pending || [];
        const packages = [...new Set([...configured.map(a => a.package), ...(p?.always_allowed || []), ...observed.map(a => a.package), ...pending.map(a => a.package)])].sort();
        let apps = packages.map(pkg => { const a = configured.find(a => a.package === pkg), obs = observed.find(a => a.package === pkg), wait = pending.find(a => a.package === pkg);
          return { package:pkg,label:obs?.label || wait?.label || pkg,on:obs ? [0] : [],seconds:obs?.seconds,minutes:a ? +a.minutes : undefined,
            rule:a?.blocked === '1' ? 'blocked' : p?.always_allowed?.includes(pkg) ? 'unlimited' : a ? 'limit' : wait ? 'pending' : 'none' }; });
        const counts = {all:apps.length,limit:0,unlimited:0,blocked:0,pending:0}; for (const a of apps) if (a.rule in counts) counts[a.rule]++;
        apps = apps.filter(a => (!params.search || (a.label+' '+a.package).toLowerCase().includes(params.search.toLowerCase())) && (params.filter === 'all' || params.filter === a.rule));
        return {profile_id:params.profile,snapshot:'fixture',apps:apps.slice(params.page*20,params.page*20+20),counts,sources:[],total:apps.length,pages:Math.max(1,Math.ceil(apps.length/20)),page:params.page,start:params.page*20};
      }
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
  },{data,lang,interval,theme,shared:SHARED});
  await expect(page.locator('.fwa .fw-status')).toContainText(lang === 'da' ? 'Opdateret' : 'Updated');
  await page.waitForFunction(() => document.querySelector('#control > link[rel="stylesheet"]')?.sheet != null);
}

async function installedBrowser(page, lang = 'en') {
  const data = replies();
  data.configuration_export.configuration.current.apps = [{id:'a_video',profile:'alex',package:'com.example.app000',minutes:'37',blocked:'0'}];
  await neutral(page,{data,lang});
  await page.evaluate(() => {
    window.inventoryRows = Array.from({length:43},(_,i) => ({package:'com.example.app'+String(i).padStart(3,'0'),label:'Alex app '+String(i).padStart(3,'0'),rule:i===0?'limit':i===1?'unlimited':i===2?'blocked':'none',minutes:i===0?37:undefined,on:[0]}));
    window.inventoryDelayed = {};
    window.inventoryHandler = async params => {
      let apps = window.inventoryRows.filter(a => (!params.search || (a.label+' '+a.package).toLowerCase().includes(params.search.toLowerCase())) && (params.filter==='all' || a.rule===params.filter));
      const reply={profile_id:params.profile,snapshot:'inventory-1',counts:{all:window.inventoryRows.length,limit:1,unlimited:1,blocked:1,pending:0},sources:[{reported:true,installed:45,sent:43}],total:apps.length,pages:Math.max(1,Math.ceil(apps.length/20)),page:params.page,start:params.page*20,apps:apps.slice(params.page*20,params.page*20+20)};
      if (params.search==='old') await new Promise(resolve => window.inventoryDelayed.old=resolve);
      return reply;
    };
  });
  await page.getByRole('button',{name:lang==='da'?'Rediger regler':'Edit rules',exact:true}).click();
  await expect(page.locator('.fw-rule-app')).toHaveCount(20);
}

test('UI4 search and rule modal survive LuCI bootstrap resets', async ({page}) => {
  await page.setViewportSize({width:1400,height:900});
  await installedBrowser(page);
  // Global rules from the LuCI bootstrap theme that previously misplaced both controls.
  await page.addStyleTag({content:'*,::before,::after{border:0;box-sizing:border-box;margin:0;padding:0}input[type="search"]{-webkit-appearance:textfield;box-sizing:content-box}'});
  for (const width of [1400,1100,900]) {
    await page.setViewportSize({width,height:900});
    const [field,input,tabs] = await Promise.all(['.fw-app-tools .fw-field','.fw-app-tools input[type=search]','.fw-app-tools .fw-seg'].map(s => page.locator(s).boundingBox()));
    expect(input.x + input.width).toBeLessThanOrEqual(field.x + field.width + 1);
    if (Math.abs(tabs.y - field.y) < field.height) expect(field.x + field.width).toBeLessThanOrEqual(tabs.x);
  }
  await page.setViewportSize({width:1400,height:900});
  await page.locator('.fw-rule-app').first().getByRole('button').click();
  const box = await page.getByRole('dialog').boundingBox();
  expect(Math.abs(box.x + box.width / 2 - 700)).toBeLessThanOrEqual(2);
  expect(box.y).toBeGreaterThan(0);
});

for (const lang of ['en','da']) test(`UI4 full inventory live search, numbered pages, local modal and focus (${lang})`, async ({page}) => {
  await installedBrowser(page,lang);
  const search=page.getByRole('searchbox'), panel=page.locator('.fw-rule-apps');
  const dirty=page.getByRole('region',{name:lang==='da'?'Ændringer er ikke gemt':'Unsaved changes',exact:true});
  await expect(panel).toContainText(lang==='da'?'ufuldstændig':'partial');
  await panel.getByRole('button',{name:'3',exact:true}).click();
  await expect(page.locator('.fw-rule-app')).toHaveCount(3);
  await expect(panel).toContainText('com.example.app042');
  await search.pressSequentially('042');
  await search.evaluate(input=>input.setSelectionRange(1,2));
  await expect(page.locator('.fw-rule-app')).toHaveCount(1);
  await expect(search).toBeFocused();
  expect(await search.evaluate(input => [input.selectionStart,input.selectionEnd])).toEqual([1,2]);
  expect(await page.evaluate(()=>window.calls.filter(c=>c.method==='admin_installed' && c.params.search).length)).toBe(1);
  await expect(dirty).toHaveCount(0);
  expect(await page.evaluate(() => window.calls.filter(c=>c.method==='admin_installed').at(-1).params)).toMatchObject({search:'042',page:0});
  await search.fill('');
  await expect(page.locator('.fw-rule-app')).toHaveCount(20);
  const row=page.locator('.fw-rule-app').filter({has:page.getByText('Alex app 000',{exact:true})});
  await expect(row).toContainText(lang==='da'?'Intet forbrug':'No usage');
  await row.getByRole('button').click();
  const dialog=page.getByRole('dialog');
  await expect(dialog.getByRole('spinbutton')).toHaveValue('37');
  await dialog.getByRole('radio',{name:/90/}).check();
  await page.keyboard.press('Escape');
  await expect(row.getByRole('button')).toBeFocused();
  await expect(row).toContainText('37');
  await expect(dirty).toHaveCount(0);
  await row.getByRole('button').click();
  await dialog.getByRole('radio',{name:/90/}).check();
  await dialog.getByRole('button',{name:lang==='da'?'Annuller':'Cancel',exact:true}).click();
  await expect(row).toContainText('37');
  await expect(dirty).toHaveCount(0);
  await row.getByRole('button').click();
  await dialog.getByRole('radio',{name:/90/}).check();
  await dialog.getByRole('button',{name:lang==='da'?'Færdig':'Done',exact:true}).click();
  await expect(row.getByRole('button')).toBeFocused();
  await expect(row).toContainText('90');
  await expect(dirty).toBeVisible();
  expect(await page.evaluate(()=>window.calls.some(c=>c.method==='configuration_apply'))).toBe(false);
  await panel.getByRole('button',{name:'3',exact:true}).click();
  await panel.getByRole('button',{name:'1',exact:true}).click();
  await expect(row).toContainText('90');
  await panel.locator('[data-key="inventory-tab-blocked"]').click();
  await expect(page.locator('.fw-rule-app')).toHaveCount(1);
  await expect(panel.locator('[data-key="inventory-tab-all"]')).toContainText('43');
  await panel.locator('[data-key="inventory-tab-all"]').click();
  await expect(row).toContainText('90');
  await page.setViewportSize({width:390,height:844});
  expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
});

test('UI4 rejects out-of-order inventory replies, removals and unavailable reads preserve drafts', async ({page}) => {
  await installedBrowser(page);
  const search=page.getByRole('searchbox');
  await search.fill('old');
  await page.waitForFunction(()=>!!window.inventoryDelayed.old);
  await search.fill('042');
  await expect(page.locator('.fw-rule-app')).toHaveCount(1);
  await page.evaluate(()=>window.inventoryDelayed.old());
  await expect(page.locator('.fw-rule-app')).toContainText('com.example.app042');
  await page.evaluate(()=>window.inventoryRows=window.inventoryRows.filter(a=>!a.package.endsWith('042')));
  await search.fill('app042');
  await expect(page.locator('.fw-rule-app')).toHaveCount(0);
  await expect(page.locator('.fw-rule-apps')).toContainText('No matching');
  await page.evaluate(()=>window.inventoryHandler=async()=>({error:'inventory unavailable'}));
  await search.fill('anything');
  await expect(page.locator('.fw-rule-apps')).toContainText('Configured rules only');
  await expect(page.locator('.fw-rule-app')).toContainText('com.example.app000');
});

test('UI4 late inventory from another profile is discarded and frozen pills cannot edit', async ({page}) => {
  const data=replies();
  const alex=data.configuration_export.configuration.current.profiles[0];
  data.configuration_export.configuration.current.profiles.push({...alex,id:'riley',name:'Riley'});
  await neutral(page,{data});
  await page.evaluate(()=> {
    window.inventoryHandler=async params=> {
      if(params.profile==='alex') await new Promise(resolve=>window.oldProfileRelease=resolve);
      return {profile_id:params.profile,snapshot:params.profile,counts:{all:1,limit:0,unlimited:0,blocked:0,pending:0},sources:[],total:1,pages:1,page:0,start:0,
        apps:[{package:'com.example.'+params.profile,label:params.profile,on:[],rule:'none'}]};
    };
  });
  await page.locator('[data-profile="alex"]').getByRole('button',{name:'Edit rules',exact:true}).click();
  await page.waitForFunction(()=>!!window.oldProfileRelease);
  page.once('dialog',dialog=>dialog.accept());
  await page.locator('.fw-nav').getByRole('button',{name:'Riley',exact:true}).click();
  await page.getByRole('tab',{name:'Rules',exact:true}).click();
  await expect(page.locator('.fw-rule-app')).toContainText('com.example.riley');
  await page.evaluate(()=>window.oldProfileRelease());
  await expect(page.locator('.fw-rule-app')).not.toContainText('com.example.alex');
  await page.evaluate(()=>window.replies.configuration_export.configuration.pact={captain:'riley'});
  await page.evaluate(()=>window.control.refresh());
  await expect(page.locator('.fw-rulebtn')).toBeDisabled();
  await expect(page.getByRole('dialog')).toHaveCount(0);
});

test('UI4 pending rows invoke real immediate approval methods without editing the rules draft', async ({page})=>{
  const data=replies();
  data.new_apps.profiles=[{id:'alex',pending:[{package:'com.example.pending',label:'Alex new app'}]}];
  data.app_decision={status:'ok',state:'approved'};
  await neutral(page,{data});
  await page.getByRole('button',{name:'Edit rules',exact:true}).click();
  await page.getByRole('button',{name:'Allow app now',exact:true}).click();
  expect(await page.evaluate(()=>window.calls.find(c=>c.method==='app_decision').params)).toEqual({profile:'alex',package:'com.example.pending',action:'approve'});
  expect(await page.evaluate(()=>window.calls.some(c=>c.method==='configuration_apply'))).toBe(false);
  await expect(page.getByRole('region',{name:'Unsaved changes',exact:true})).toHaveCount(0);
});

for (const lang of ['en','da']) test(`UI4 untouched Rules navigation and polling are clean (${lang})`, async ({page})=>{
  await neutral(page,{lang});
  const dialogs=[];
  page.on('dialog',dialog=>{dialogs.push(dialog.message());return dialog.dismiss();});
  await page.getByRole('button',{name:lang==='da'?'Rediger regler':'Edit rules',exact:true}).click();
  await page.evaluate(()=>window.control.refresh());
  await page.getByRole('tab',{name:lang==='da'?'I dag':'Today',exact:true}).click();
  await expect(page.locator('.fw-rules')).toHaveCount(0);
  expect(dialogs).toEqual([]);
});

test('UI4 query/page/filter navigation and modal Cancel do not dirty Rules', async ({page})=>{
  await installedBrowser(page);
  const dialogs=[];
  page.on('dialog',dialog=>{dialogs.push(dialog.message());return dialog.dismiss();});
  const panel=page.locator('.fw-rule-apps');
  await panel.getByRole('button',{name:'3',exact:true}).click();
  await expect(page.locator('.fw-rule-app')).toHaveCount(3);
  await page.getByRole('searchbox').fill('042');
  await expect(page.locator('.fw-rule-app')).toHaveCount(1);
  await page.getByRole('searchbox').fill('');
  await expect(page.locator('.fw-rule-app')).toHaveCount(20);
  await panel.locator('[data-key="inventory-tab-limit"]').click();
  await expect(page.locator('.fw-rule-app')).toHaveCount(1);
  await page.locator('.fw-rulebtn').click();
  await page.getByRole('dialog').getByRole('radio',{name:/90/}).check();
  await page.getByRole('dialog').getByRole('button',{name:'Cancel',exact:true}).click();
  await page.getByRole('tab',{name:'Today',exact:true}).click();
  await expect(page.locator('.fw-rules')).toHaveCount(0);
  expect(dialogs).toEqual([]);
});

test('UI4 edit then revert fields and app rules restores a clean semantic draft', async ({page})=>{
  await installedBrowser(page);
  const dialogs=[];
  page.on('dialog',dialog=>{dialogs.push(dialog.message());return dialog.dismiss();});
  await page.getByLabel('Name',{exact:true}).fill('Alex draft');
  await page.getByLabel('Name',{exact:true}).fill('Alex');
  await page.getByLabel('Daily screen time (minutes)',{exact:true}).fill('45');
  await page.getByLabel('Daily screen time (minutes)',{exact:true}).fill('40');
  const always=page.getByLabel('Always-allowed packages (one per line)',{exact:true});
  await always.fill('com.example.reader');
  await always.fill('');
  const row=page.locator('.fw-rule-app').filter({has:page.getByText('Alex app 000',{exact:true})});
  await row.getByRole('button').click();
  await page.getByRole('dialog').getByRole('radio',{name:/^Allowed/}).check();
  await page.getByRole('dialog').getByRole('button',{name:'Done',exact:true}).click();
  await expect(page.getByRole('region',{name:'Unsaved changes',exact:true})).toBeVisible();
  await row.getByRole('button').click();
  await page.getByRole('dialog').getByRole('radio',{name:/^Daily limit/}).check();
  await page.getByRole('dialog').getByLabel('Custom minutes',{exact:true}).fill('37');
  await page.getByRole('dialog').getByRole('button',{name:'Done',exact:true}).click();
  await expect(page.getByRole('region',{name:'Unsaved changes',exact:true})).toHaveCount(0);
  await page.getByRole('tab',{name:'Today',exact:true}).click();
  await expect(page.locator('.fw-rules')).toHaveCount(0);
  expect(dialogs).toEqual([]);
});

for (const lang of ['en','da']) test(`UI4 real unsaved edits retain an explicit localized discard guard (${lang})`, async ({page})=>{
  await neutral(page,{lang});
  await page.getByRole('button',{name:lang==='da'?'Rediger regler':'Edit rules',exact:true}).click();
  await page.getByLabel(lang==='da'?'Navn':'Name',{exact:true}).fill('Alex draft');
  const expected=lang==='da'?'Kassér ændringer, der ikke er gemt, og forlad denne side?':'Discard unsaved changes and leave this page?';
  const messages=[];
  page.once('dialog',async dialog=>{messages.push(dialog.message());await dialog.dismiss();});
  await page.getByRole('tab',{name:lang==='da'?'I dag':'Today',exact:true}).click();
  await expect(page.locator('.fw-rules')).toBeVisible();
  await expect(page.getByLabel(lang==='da'?'Navn':'Name',{exact:true})).toHaveValue('Alex draft');
  page.once('dialog',async dialog=>{messages.push(dialog.message());await dialog.accept();});
  await page.getByRole('tab',{name:lang==='da'?'I dag':'Today',exact:true}).click();
  await expect(page.locator('.fw-rules')).toHaveCount(0);
  expect(messages).toEqual([expected,expected]);
});

test('UI4 rule modal agrees with blocked precedence over always allowed', async ({page})=>{
  const data=replies();
  data.configuration_export.configuration.current.apps=[{id:'a_blocked',profile:'alex',package:'com.example.blocked',minutes:'0',blocked:'1'}];
  data.configuration_export.configuration.current.profiles[0].always_allowed=['com.example.blocked'];
  await neutral(page,{data});
  await page.getByRole('button',{name:'Edit rules',exact:true}).click();
  const rule=page.locator('.fw-rule-app').getByRole('button');
  await expect(rule).toContainText('Blocked');
  await rule.click();
  await expect(page.getByRole('dialog').getByRole('radio',{name:/^Blocked/})).toBeChecked();
  await page.getByRole('dialog').getByRole('button',{name:'Done',exact:true}).click();
  await expect(rule).toContainText('Blocked');
  await expect(page.getByRole('region',{name:'Unsaved changes',exact:true})).toHaveCount(0);
  await rule.click();
  await page.getByRole('dialog').getByRole('radio',{name:/^Always allowed/}).check();
  await page.getByRole('dialog').getByRole('button',{name:'Done',exact:true}).click();
  await expect(rule).toContainText('Always allowed');
  await expect(page.getByRole('region',{name:'Unsaved changes',exact:true})).toBeVisible();
});

test('UI4 manual draft rules stay visible, editable and removable outside inventory results', async ({page})=>{
  await installedBrowser(page);
  const panel=page.locator('.fw-rule-apps');
  await panel.getByRole('button',{name:'3',exact:true}).click();
  await expect(page.locator('.fw-rule-app')).toHaveCount(3);
  page.once('dialog',dialog=>dialog.accept('com.example.manual'));
  await page.getByRole('button',{name:'+ Add app rule',exact:true}).click();
  await page.getByRole('dialog').getByRole('radio',{name:/^Daily limit/}).check();
  await page.getByRole('dialog').getByLabel('Custom minutes',{exact:true}).fill('25');
  await page.getByRole('dialog').getByRole('button',{name:'Done',exact:true}).click();
  const local=page.locator('.fw-draft-apps');
  await expect(local).toContainText('com.example.manual');
  await expect(local).toContainText('25');
  await expect(panel).toContainText('Counts still use saved rules');
  await expect(panel.locator('[data-key="inventory-tab-all"]')).toContainText('43');
  await expect(panel).toContainText('41–43 of 43 apps');
  await local.getByRole('button',{name:/Daily limit/}).click();
  await page.getByRole('dialog').getByRole('radio',{name:/90/}).check();
  await page.getByRole('dialog').getByRole('button',{name:'Done',exact:true}).click();
  await expect(local).toContainText('90');
  await page.getByRole('searchbox').fill('042');
  await expect(page.locator('.fw-rule-app')).toHaveCount(1);
  await expect(local).toContainText('com.example.manual');
  await local.getByRole('button',{name:'Remove',exact:true}).click();
  await expect(local).toHaveCount(0);
  await expect(page.getByRole('region',{name:'Unsaved changes',exact:true})).toHaveCount(0);
  expect(await page.evaluate(()=>window.calls.some(c=>c.method==='configuration_apply'))).toBe(false);
});

for (const lang of ['en','da']) for (const configured of [false,true]) test(`UI4 app-control-off read is explained (${lang}, configured=${configured})`, async ({page})=>{
  const data=replies();
  data.admin_installed={error:'app_control_off'};
  data.configuration_export.configuration.current.profiles[0].apps='0';
  if(configured) data.configuration_export.configuration.current.apps=[{id:'a_saved',profile:'alex',package:'com.example.saved',minutes:'15',blocked:'0'}];
  await neutral(page,{data,lang});
  await page.getByRole('button',{name:lang==='da'?'Rediger regler':'Edit rules',exact:true}).click();
  await expect(page.locator('.fw-rule-apps')).toContainText(lang==='da'?'appstyring er aktiveret og gemt':'app control to be enabled and saved');
  if(configured) await expect(page.locator('.fw-rule-app')).toContainText('com.example.saved');
  await expect(page.getByRole('region',{name:lang==='da'?'Ændringer er ikke gemt':'Unsaved changes',exact:true})).toHaveCount(0);
});

test.describe('shared control without LuCI globals', () => {
  for (const lang of ['en','da']) test(`neutral profile labels, measured usage without budget and package identity (${lang})`, async ({ page }) => {
    const data = replies();
    data.devices.devices = [{id:'alex-phone',name:'Alex phone',profile:'alex',administrator:'1'}];
    data.configuration_report.app_profiles = {alex:{enabled:true}};
    data.new_apps.profiles = [{id:'alex',pending:[{package:'com.example.pending',label:'Pending search'}]}];
    data.usage.profiles = [{id:'alex',total:1200,devices:[],apps:[
      {package:'com.example.search',label:'Search',seconds:600}
    ]}];
    await neutral(page,{data,lang});
    await expect(page.getByRole('heading',{name:lang === 'da' ? 'Profiler' : 'Profiles',exact:true})).toBeVisible();
    await expect(page.getByRole('heading',{name:lang === 'da' ? 'Administratortelefoner' : 'Administrator phones',exact:true})).toBeVisible();
    await expect(page.locator('.fw-kid')).toHaveCount(1);
    const card = page.locator('[data-profile="alex"]');
    await expect(card.locator('.fw-big')).toHaveText('20 min');
    await expect(card).toContainText(lang === 'da' ? 'intet samlet skærmtidsbudget' : 'No total screen-time budget');
    await expect(card.getByText('Search',{exact:true})).toBeVisible();
    await expect(card.getByText('com.example.search',{exact:true})).toBeVisible();
    const appLayout = await card.locator('.fw-approw').first().evaluate(row => ({
      identity: row.querySelector('.fw-app-identity').getBoundingClientRect().width,
      badge: row.querySelector('.fw-chip').getBoundingClientRect().width
    }));
    expect(appLayout.identity).toBeGreaterThan(appLayout.badge);
    expect(appLayout.badge).toBeLessThan(160);
    await expect(card.locator('.fw-meter')).toHaveCount(0);
    await card.getByRole('button',{name:lang === 'da' ? 'I dag' : 'Today',exact:true}).click();
    const screen = page.locator('.fw-panel').filter({has:page.getByRole('heading',{name:lang === 'da' ? 'Skærmtid i dag' : 'Screen time today',exact:true})});
    await expect(screen.locator('.fw-big')).toHaveText('20min');
    await expect(screen).toContainText(lang === 'da' ? 'intet samlet skærmtidsbudget' : 'No total screen-time budget');
    await expect(screen.locator('.fw-meter')).toHaveCount(0);
    await page.getByRole('tab',{name:lang === 'da' ? 'Historik' : 'History',exact:true}).click();
    await expect(page.locator('.fw-history-apps').getByText('com.example.search',{exact:true})).toBeVisible();
    await page.getByRole('tab',{name:lang === 'da' ? 'Regler' : 'Rules',exact:true}).click();
    await expect(page.locator('.fw-rule-app').getByText('com.example.search',{exact:true})).toBeVisible();
    await expect(page.locator('.fw-rule-apps').getByText('com.example.pending',{exact:true})).toBeVisible();
    page.once('dialog', dialog => dialog.accept());
    await page.locator('.fw-nav').getByRole('button',{name:lang === 'da' ? 'Familie' : 'Family',exact:true}).click();
    await page.evaluate(() => {
      window.replies.usage.profiles[0].total = 0;
      return window.control.refresh();
    });
    await expect(card.locator('.fw-big')).toHaveCount(0);
    await expect(card).toContainText(lang === 'da' ? 'Utilgængelig' : 'Unavailable');
    await page.evaluate(() => {
      window.replies.usage.profiles[0].devices = [{name:'Alex phone',total:0}];
      return window.control.refresh();
    });
    await expect(card.locator('.fw-big')).toHaveText('0 min');
    await page.evaluate(() => {
      window.replies.configuration_report.app_profiles.alex.enabled = false;
      window.replies.usage.profiles[0].total = 600;
      return window.control.refresh();
    });
    await expect(card.locator('.fw-big')).toHaveText('10 min');
    for (const width of [1440,390,320]) {
      await page.setViewportSize({width,height:900});
      expect(await page.locator('.fwa').evaluate(node => node.scrollWidth <= node.clientWidth + 1)).toBe(true);
    }
  });

  test('explicit light/dark override OS; auto follows changes and stop removes its listener', async ({ page }) => {
    await page.emulateMedia({colorScheme:'dark'});
    await neutral(page,{theme:'light'});
    await expect(page.locator('.fwa')).not.toHaveClass(/is-dark/);
    await expect(page.locator('.fwa')).toHaveCSS('color-scheme','light');
    await page.locator('.fw-kid').getByRole('button',{name:'Edit rules',exact:true}).click();
    await page.getByLabel('Name',{exact:true}).fill('Alex draft');
    await page.getByLabel('Name',{exact:true}).focus();
    await page.evaluate(() => window.control.setTheme('dark'));
    await page.emulateMedia({colorScheme:'light'});
    await expect(page.locator('.fwa')).toHaveClass(/is-dark/);
    await expect(page.locator('.fwa')).toHaveCSS('color-scheme','dark');
    await expect(page.getByLabel('Name',{exact:true})).toHaveValue('Alex draft');
    await expect(page.getByLabel('Name',{exact:true})).toBeFocused();
    await page.evaluate(() => window.control.setTheme('auto'));
    await expect(page.locator('.fwa')).not.toHaveClass(/is-dark/);
    await page.emulateMedia({colorScheme:'dark'});
    await expect(page.locator('.fwa')).toHaveClass(/is-dark/);
    await page.evaluate(() => window.control.stop());
    await page.emulateMedia({colorScheme:'light'});
    await page.waitForTimeout(100);
    await expect(page.locator('.fwa')).toHaveClass(/is-dark/);
  });

  for (const theme of ['light','dark']) test(`host element rules cannot restyle workspace controls (${theme})`, async ({ page }) => {
    const data = replies();
    data.usage.profiles = [{id:'alex',total:60,devices:[{name:'Alex phone',total:60}],apps:[]}];
    await neutral(page,{theme,data});
    // Minimal Proton 2025 global selectors, loaded AFTER the workspace stylesheet.
    await page.addStyleTag({content:`
      *{box-sizing:border-box;margin:0;padding:0}
      p,ul,ol{margin:0 0 1rem}
      h1,h2,h3,h4{color:rgb(255,0,255);font-weight:600;line-height:1.3;margin:0 0 1rem}
      button{display:inline-flex;gap:8px;background:magenta;color:white;font-weight:500;font-size:.9rem;border:none;border-radius:20px;transition:all .2s;white-space:nowrap;padding:10px 20px}
      button:hover{box-shadow:0 8px 20px magenta;transform:translateY(-1px)}
      button:active{transform:translateY(0)}
      input:not([type]),input[type=text],select{max-width:100%;width:100%;min-width:0!important;font-size:16px}
      @media(max-width:600px){button{font-size:1rem;padding:12px 18px}th{display:none}}
    `});
    const today = page.locator('.fw-kid').getByRole('button',{name:'Today',exact:true});
    await today.hover();
    await expect(today).toHaveCSS('padding-left','16px');
    await expect(today).toHaveCSS('padding-top','0px');
    await expect(today).toHaveCSS('transform','none');
    await expect(today).toHaveCSS('box-shadow','none');
    await expect(page.locator('.fw-h1')).toHaveCSS('color',theme === 'dark' ? 'rgb(216, 225, 234)' : 'rgb(15, 27, 39)');
    for (const width of [1440,390,320]) {
      await page.setViewportSize({width,height:900});
      expect(await page.locator('.fwa').evaluate(node => node.scrollWidth <= node.clientWidth + 1)).toBe(true);
      await expect(today).toHaveCSS('padding-top','0px');
    }
    await today.click();
    await expect(page.locator('.fw-table th').first()).toBeVisible();
    await page.getByRole('tab',{name:'Rules',exact:true}).click();
    await expect(page.getByLabel('Name',{exact:true})).toHaveCSS('background-color',theme === 'dark' ? 'rgb(18, 28, 39)' : 'rgb(255, 255, 255)');
  });

  test('LuCI translates explicit host dark state and disposes its observers on navigation', async ({ page }) => {
    const source = readFileSync(require('node:path').resolve(__dirname,
      '../../luci-app-familywarden/htdocs/luci-static/resources/view/familywarden/control.js'),'utf8');
    await neutral(page);
    await page.evaluate(source => {
      window.control.stop();
      document.documentElement.setAttribute('data-darkmode','false');
      window.hostThemes = []; window.hostStops = 0;
      const view = new Function('view','host','E',source)(
        {extend:value => value},{options:() => ({})},
        (tag,attrs,child) => {const node = document.createElement(tag);if (child) node.textContent = child;return node;});
      const root = view.render({mount:(node,options) => {
        window.hostThemes.push(options.theme);
        return {setTheme:value => window.hostThemes.push(value),stop:() => window.hostStops++};
      }});
      root.id = 'host-mount'; window.pendingHostRoot = root;
      document.body.appendChild(document.createElement('div'));
    },source);
    // LuCI can attach a rendered view later; unrelated mutations must not stop it.
    expect(await page.evaluate(() => window.hostStops)).toBe(0);
    await page.evaluate(() => document.body.replaceChildren(window.pendingHostRoot));
    expect(await page.evaluate(() => window.hostThemes)).toEqual(['light']);
    await page.evaluate(() => document.documentElement.setAttribute('data-darkmode','true'));
    await expect.poll(() => page.evaluate(() => window.hostThemes)).toEqual(['light','dark']);
    await page.evaluate(() => document.documentElement.removeAttribute('data-darkmode'));
    await expect.poll(() => page.evaluate(() => window.hostThemes)).toEqual(['light','dark','auto']);
    await page.evaluate(() => document.getElementById('host-mount').remove());
    await expect.poll(() => page.evaluate(() => window.hostStops)).toBe(1);
    await page.evaluate(() => document.documentElement.setAttribute('data-darkmode','true'));
    expect(await page.evaluate(() => window.hostThemes)).toEqual(['light','dark','auto']);
  });

  test('global host element styles do not leak into the workspace', async ({ page }) => {
    // The generic element rules a LuCI theme such as Proton2025 applies page-wide.
    await neutral(page,{theme:'dark'});
    await page.addStyleTag({content:'body{letter-spacing:.01em}button{display:inline-flex;align-items:center;justify-content:center;gap:8px;'+
      'border-radius:12px;white-space:nowrap}button+button{margin-left:8px}p{font-size:20px;line-height:30px;margin:0 0 16px}'+
      'h4{line-height:20px}strong,dt{font-weight:400}label{line-height:30px}input[type=checkbox]{appearance:none;accent-color:blue}'+
      'select{appearance:none}'});
    await expect(page.locator('.fwa')).toHaveCSS('letter-spacing','normal');
    const nav = page.locator('.fw-nav button').nth(1);
    await expect(nav).toHaveCSS('justify-content','normal');
    await expect(nav).toHaveCSS('margin-left','0px');
    await expect(page.locator('.fw-sub').first()).toHaveCSS('font-size','15px');
    await expect(page.locator('.fw-kid h4').first()).toHaveCSS('line-height','36px');
    await page.getByRole('button',{name:'Edit rules',exact:true}).click();
    await expect(page.getByRole('tab',{name:'Rules',exact:true})).toHaveCSS('border-radius','0px');
    const enabled = page.getByLabel('Enabled',{exact:true});
    await expect(enabled).toHaveCSS('appearance','auto');
    await expect(enabled).toHaveCSS('accent-color','rgb(224, 189, 74)');
    await expect(page.locator('.fwa select').first()).toHaveCSS('appearance','auto');
  });

  test('optional public Proton stylesheet on a disposable local host', async ({ page }) => {
    test.skip(!process.env.FW_PROTON_CSS, 'Set FW_PROTON_CSS to a local public theme stylesheet');
    await neutral(page,{theme:'dark'});
    await page.evaluate(() => {
      document.documentElement.setAttribute('data-theme','dark');
      document.documentElement.setAttribute('data-darkmode','true');
    });
    await page.addStyleTag({content:readFileSync(process.env.FW_PROTON_CSS,'utf8')});
    for (const width of [1440,390,320]) {
      await page.setViewportSize({width,height:900});
      await expect(page.locator('.fw-h1')).toHaveCSS('color','rgb(216, 225, 234)');
      const today = page.locator('.fw-kid').getByRole('button',{name:'Today',exact:true});
      await today.hover();
      await expect(today).toHaveCSS('padding-top','0px');
      await expect(today).toHaveCSS('transform','none');
      expect(await page.locator('.fwa').evaluate(node => node.scrollWidth <= node.clientWidth + 1)).toBe(true);
    }
  });

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
    await expect(page.locator('.fw-rule-apps')).toContainText('Paused until decided');
    await expect(page.getByLabel('Rule for Drawing',{exact:true})).toHaveCount(0);
    await page.getByRole('button',{name:'Weekdays + weekend',exact:true}).click();
    await page.getByLabel('Weekday screen time (optional)',{exact:true}).fill('25');
    await page.getByLabel('Weekend screen time (optional)',{exact:true}).fill('60');
    await page.locator('.fw-rule-app').filter({has:page.getByText('Video',{exact:true})}).getByRole('button').click();
    await page.getByRole('dialog').getByRole('radio',{name:/Always allowed/}).check();
    await page.getByRole('dialog').getByRole('button',{name:'Done',exact:true}).click();
    await expect(page.getByRole('switch',{name:'Ask before new apps open',exact:true})).toHaveAttribute('aria-checked','true');
    await page.getByRole('switch',{name:'Ask before new apps open',exact:true}).click();
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

  test('every component version is visible', async ({ page }) => {
    await neutral(page);
    await expect(page.locator('.fw-status')).toContainText('Web interface 0.11.0-r13');
    await page.locator('.fw-nav').getByRole('button',{name:'Administration',exact:true}).click();
    const health = page.locator('.fw-panel').first();
    for (const v of ['test-version','0.11.0_rc10-r18','0.11.0-r13']) await expect(health.getByText(v,{exact:true})).toBeVisible();
  });

  test('app rule controls line up without overlapping', async ({ page }) => {
    const data = replies();
    data.configuration_export.configuration.current.apps = [
      {id:'a_1',profile:'alex',package:'com.example.search',minutes:'15',blocked:'0'},
      {id:'a_2',profile:'alex',package:'com.example.maps',minutes:'0',blocked:'1'}];
    await page.setViewportSize({width:1100,height:900});
    await neutral(page,{data});
    await page.locator('.fw-nav').getByRole('button',{name:'Alex',exact:true}).click();
    await page.getByRole('tab',{name:'Rules',exact:true}).click();
    await expect(page.locator('.fw-rulebtn')).toHaveCount(2);
    const rows = await page.locator('.fw-rule-app').evaluateAll(list => list.map(row => ({
      main:row.querySelector('.main').getBoundingClientRect(),pill:row.querySelector('.fw-rulebtn').getBoundingClientRect()
    })));
    for (const row of rows) expect(row.main.right).toBeLessThanOrEqual(row.pill.left);
    expect(Math.round(rows[0].pill.right)).toBe(Math.round(rows[1].pill.right));
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
    // Name the widest offenders so font-dependent CI failures are diagnosable.
    const fits = async label => {
      const result = await page.locator('.fwa').evaluate(root => {
        const limit = root.getBoundingClientRect().right + 1, wide = [];
        for (const node of root.querySelectorAll('*')) {
          const box = node.getBoundingClientRect();
          if (box.width && box.right > limit) wide.push(`${node.tagName.toLowerCase()}.${[...node.classList].join('.')} right=${Math.round(box.right)} "${(node.textContent || '').trim().slice(0, 40)}"`);
        }
        return { ok: root.scrollWidth <= root.clientWidth + 1, detail: `scroll=${root.scrollWidth} client=${root.clientWidth} font=${getComputedStyle(root).fontFamily}; ${wide.slice(-8).join(' | ')}` };
      });
      expect(result.ok, `${label}: ${result.detail}`).toBe(true);
    };
    for (const width of [1100,720,390,320]) {
      await page.setViewportSize({width,height:1000});
      await fits(`${width}px`);
    }
    await page.evaluate(() => {document.documentElement.style.fontSize = '200%';});
    await fits('320px at 200%');
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
    page.once('dialog', dialog => dialog.accept('com.example.game'));
    await apps.getByRole('button',{name:'+ Add app rule',exact:true}).click();
    await page.getByRole('dialog').getByRole('radio',{name:/Daily limit/}).check();
    await page.getByRole('dialog').getByLabel('Custom minutes',{exact:true}).fill('10');
    await page.getByRole('dialog').getByRole('button',{name:'Done',exact:true}).click();
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
