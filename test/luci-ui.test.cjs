const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const { test } = require('node:test');

// Fast shared-presentation checks, not a replacement for real LuCI/browser validation.
function fixture() {
  function element(tag, attrs = {}, children = []) {
    return {
      tag, attrs, children: Array.isArray(children) ? children : [children],
      setAttribute(key, value) { this.attrs[key] = value; },
      appendChild(child) { this.children.push(child); }
    };
  }
  const head = element('head');
  const document = {
    head,
    createElementNS: (_, tag) => element(tag),
    getElementById: id => head.children.find(child => child.attrs.id === id)
  };
  const source = readFileSync('luci-app-familywarden/htdocs/luci-static/resources/familywarden/ui.js', 'utf8');
  const ui = new Function('baseclass', 'E', 'L', 'document', source)(
    { extend: value => value }, element, { resource: path => '/resources/' + path }, document
  );
  return { ui, head };
}

test('shared header retains the title and lighthouse without browser dependencies', () => {
  const { ui } = fixture();
  const header = ui.header('Profiles', 'Staged edits', ['action']);
  assert.equal(header.attrs.class, 'fw-head');
  assert.equal(header.children[0].children[1].children[0], 'Profiles');
  assert.equal(header.children[0].children[0].children[0].attrs.viewBox, '0 0 48 48');
  assert.deepEqual(header.children[1].children, ['action']);
  for (const name of ['check', 'ban', 'moon', 'info', 'mast', 'hand']) {
    const svg = ui.icon(name);
    assert.equal(svg.attrs['aria-hidden'], 'true');
    assert.ok(svg.children[0].attrs.d);
  }
});

test('shared stylesheet loading is deduplicated across Overview and Profiles', () => {
  const { ui, head } = fixture();
  ui.loadStyles('overview');
  ui.loadStyles('overview');
  ui.loadStyles('pages');
  assert.equal(head.children.length, 2);
  assert.equal(head.children[0].attrs.href, '/resources/familywarden/overview.css');
  assert.equal(head.children[1].attrs.href, '/resources/familywarden/pages.css');
});

// Shared administrator UI (A20): pure view model and EN/DA strings, no browser needed.
const familyModule = () => import('../luci-app-familywarden/htdocs/luci-static/resources/familywarden/shared/family.js');

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

test('family strings have complete English and Danish tables', async () => {
  const { strings } = await familyModule();
  const src = readFileSync('luci-app-familywarden/htdocs/luci-static/resources/familywarden/shared/family.js', 'utf8');
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
