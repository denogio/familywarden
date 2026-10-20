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
  ui.loadStyles('profiles');
  assert.equal(head.children.length, 2);
  assert.equal(head.children[0].attrs.href, '/resources/familywarden/overview.css');
  assert.equal(head.children[1].attrs.href, '/resources/familywarden/profiles.css');
});
