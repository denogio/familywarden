const test = require('node:test');
const assert = require('node:assert/strict');
const { readdirSync, readFileSync, existsSync } = require('node:fs');

const ui = () => import('../web/control.js');

test('all shared web modules live outside LuCI and import without a native host', async () => {
  for (const name of readdirSync('web').filter(n => n.endsWith('.js'))) {
    const source = readFileSync(`web/${name}`, 'utf8');
    assert.doesNotMatch(source, /['"]require (?:uci|rpc|view|form|ui|baseclass)['"]/);
    assert.doesNotMatch(source, /\b(?:L\.(?:resource|url|env)|uci\.(?:get|set|load)|rpc\.declare)\b/);
    await import(`../web/${name}`);
  }
  assert.equal(existsSync('luci-app-familywarden/htdocs/luci-static/resources/familywarden/shared'), false);
});

test('control resources have complete matching English and Danish keys', async () => {
  const { translations, strings } = await import('../web/control-i18n.js');
  assert.deepEqual(Object.keys(translations.en).sort(), Object.keys(translations.da).sort());
  for (const locale of ['en', 'da']) for (const [key, value] of Object.entries(translations[locale])) {
    assert.equal(typeof value, 'string');
    assert.ok(value.length);
    assert.equal(strings(locale)(key), value.replace(/\{\w+\}/g, ''));
  }
  for (const match of readFileSync('web/control.js', 'utf8').matchAll(/\bt\('([^']+)'/g)) {
    assert.ok(Object.hasOwn(translations.en, match[1]), `Missing UI string: ${match[1]}`);
  }
  assert.equal(strings('da-DK')('family'), 'Familie');
  assert.equal(strings('fr')('family'), 'Family');
});

test('configuration replies distinguish valid empty, unavailable and unsupported state', async () => {
  const { validateExport } = await ui();
  const valid = { owner:'core',configuration:{ version:1,revision:'fixture',current:{settings:{},profiles:[],schedules:[],apps:[],downtime:[]} } };
  assert.equal(validateExport(valid), valid);
  for (const invalid of [{}, {error:'offline'}, {...valid,owner:'unknown'},
    {...valid,configuration:{...valid.configuration,version:99}},
    {...valid,configuration:{...valid.configuration,current:{settings:{}}}}]) assert.throws(() => validateExport(invalid));
});

test('history totals exclude gaps and invalid values without projecting current policy', async () => {
  const { historyModel } = await ui();
  const usage = { week: [
    { day:'2027-01-09', total:0, reported:false },
    { day:'2027-01-10', total:0, reported:true },
    { day:'2027-01-11', total:60 },
    { day:'2027-01-12', total:120, reported:true },
    { day:'2027-01-13', total:0 },
    { day:'2027-01-14', total:-1, reported:true },
    { day:'2027-01-15', total:300, reported:true }
  ] };
  const model = historyModel(usage);
  assert.deepEqual(model.entries.map(d => d.seconds), [null, 0, 60, 120, null, null, 300]);
  assert.equal(model.total, 480);
  assert.equal(model.average, 120);
  assert.equal(model.reported, 3);
  assert.equal(model.unknown, 2);
  assert.equal(historyModel({week:[{total:0,reported:false}]}).total, null);
  assert.equal(historyModel({week:[{total:0,reported:true}]}).total, 0);
  assert.equal(historyModel().average, null);
  assert.equal(Object.hasOwn(model, 'budget'), false);
});

// Family (requests/usage) view model: shared web module, no browser needed.
const familyModule = () => import('../web/family.js');

test('family view model separates pending, recent and usage without inventing data', async () => {
  const { viewModel } = await familyModule();
  const now = 1800000000;
  const vm = viewModel({ requests: [
    { request_id: 'r-pending-1', profile: 'alex', profile_name: 'Alex', target: 'com.example.video', label: 'Video', minutes: 30, received: now - 60, expires: now + 3540, state: 'pending' },
    { request_id: 'r-stale-0001', profile: 'alex', target: 'total', minutes: 15, received: now - 4000, expires: now - 400, state: 'pending' },
    { request_id: 'r-approved-1', profile: 'sam', profile_name: 'Sam', target: 'total', minutes: 30, received: now - 900, answered_at: now - 800, answered_by: 'Jordan\'s phone', state: 'approved', granted: 20, used: 605, delivered: true },
    { request_id: 'r-old-00001', profile: 'sam', target: 'total', minutes: 10, received: now - 90000, answered_at: now - 89000, state: 'denied' }
  ] }, { day: '2027-01-15', profiles: [{ id: 'alex', name: 'Alex', total: 4920,
    devices: [{ name: 'Alex\'s phone', total: 4920, incomplete: true }], apps: [{ package: 'com.example.video', label: 'Video', seconds: 1800 }],
    week: [{ day: '2027-01-15', total: 4920 }] }] }, now);
  assert.deepEqual(vm.pending.map((r) => r.id), ['r-pending-1']);
  assert.equal(vm.pending[0].expires, now + 3540);
  assert.deepEqual(vm.recent.map((r) => [r.id, r.granted, r.used]), [['r-approved-1', 20, 10]]);
  assert.equal(vm.profiles[0].total, 82);
  assert.equal(vm.profiles[0].devices[0].incomplete, true);
  assert.equal(vm.profiles[0].apps[0].name, 'Video');
  assert.deepEqual(viewModel({}, {}, now), { pending: [], recent: [], profiles: [], apps: [], day: '' });
});

test('weekly usage keeps missing, zero and legacy coverage distinct', async () => {
  const { viewModel } = await familyModule();
  const vm = viewModel({}, { profiles: [{ id: 'alex', total: 0, devices: [], apps: [], week: [
    { day: '2027-01-13', total: 0, reported: false },
    { day: '2027-01-14', total: 0, reported: true },
    { day: '2027-01-15', total: 0 }
  ] }] }, 1800000000);
  assert.deepEqual(vm.profiles[0].week.map(d => d.reported), [false, true, null]);
});

test('family strings have complete English and Danish tables', async () => {
  const { strings } = await familyModule();
  const src = readFileSync('web/family.js', 'utf8');
  const keys = (lang) => [...src.split(`${lang}: {`)[1].split('\n  }')[0].matchAll(/(\w+): '/g)].map((m) => m[1]).sort();
  assert.deepEqual(keys('da'), keys('en'));
  assert.equal(strings('da-DK')('approve'), 'Godkend');
  assert.equal(strings('fr')('approve'), 'Approve');
  assert.equal(strings('en')('range', { max: 30 }), 'Approve 1–30 minutes.');
});

test('family view model lists waiting and allowed new apps, hides empty profiles', async () => {
  const { viewModel } = await familyModule();
  const vm = viewModel({}, {}, 1800000000, { profiles: [
    { id: 'alex', name: 'Alex', frozen: false, pending: [{ package: 'com.example.draw', label: 'Draw' }],
      approved: [{ package: 'com.example.maps', label: '', by: "Jordan's phone" }] },
    { id: 'sam', name: 'Sam', pending: [], approved: [] }] });
  assert.deepEqual(vm.apps.map((p) => p.id), ['alex']);
  assert.equal(vm.apps[0].pending[0].label, 'Draw');
  assert.equal(vm.apps[0].approved[0].label, 'com.example.maps');
  assert.deepEqual(viewModel({}, {}, 1, undefined).apps, []);
});
