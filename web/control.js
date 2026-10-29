// Production shared administrator workspace (A20). No LuCI, UCI, native paths,
// standalone login or product eligibility decisions: the host supplies core RPCs.
// Presentation follows docs/design/control-prototype-review.md: rail navigation,
// attention inbox, child cards with budgets, delivery pipeline and the Ulysses
// voyage. Unsupported data shows as unavailable; nothing is simulated.
import { strings } from './control-i18n.js';
import { mount as mountRequests } from './family.js';

const array = value => Array.isArray(value) ? value : [];
const clone = value => JSON.parse(JSON.stringify(value));
const object = value => value && typeof value === 'object' && !Array.isArray(value);
const days = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'];
const protections = ['filter_lists', 'safe_browsing', 'parental', 'safe_search'];
const lines = value => String(value).split(/\r?\n/).map(v => v.trim()).filter(Boolean);

export function validateExport(reply) {
  const c = reply?.configuration;
  if (reply?.error || !['core', 'legacy'].includes(reply?.owner) || !object(c) ||
      c.version !== 1 || typeof c.revision !== 'string' || !object(c.current?.settings) ||
      !Array.isArray(c.current?.profiles) || ['schedules','apps','downtime'].some(k => c.current[k] != null && !Array.isArray(c.current[k])))
    throw new Error(reply?.error || 'Invalid configuration reply');
  return reply;
}

// Presentation only: missing reports never become measured zeroes, and current
// policy is not projected backwards as a historical budget.
export function historyModel(usage) {
  const entries = array(usage?.week).filter(object).map(d => {
    const reported = typeof d.reported === 'boolean' ? d.reported : null;
    const valid = Number.isFinite(d.total) && d.total >= 0;
    const measured = valid && (reported === true || (reported == null && d.total > 0));
    return { day: typeof d.day === 'string' ? d.day : '', reported, measured, seconds: measured ? d.total : null };
  });
  const measured = entries.filter(d => d.measured);
  const total = measured.length ? measured.reduce((sum, d) => sum + d.seconds, 0) : null;
  return { entries, total, average: measured.length ? total / measured.length : null,
    reported: measured.filter(d => d.reported === true).length,
    unknown: entries.filter(d => d.reported == null).length };
}

export function mount(root, { call, callHost, lang = 'en', recoveryURL, theme = 'auto', interval = 15000 }) {
  const doc = root.ownerDocument, win = doc.defaultView, t = strings(lang);
  const el = (tag, attrs = {}, ...children) => {
    const node = doc.createElement(tag);
    for (const [key, value] of Object.entries(attrs)) {
      if (value == null || value === false) continue;
      if (key.startsWith('on')) node.addEventListener(key.slice(2), value);
      else node.setAttribute(key, value === true ? '' : String(value));
    }
    for (const child of children.flat(Infinity)) if (child != null) node.append(child instanceof win.Node ? child : String(child));
    return node;
  };
  const css = el('link', { rel: 'stylesheet', href: new URL('./control.css', import.meta.url).href });
  const spriteHost = el('div');
  spriteHost.innerHTML = `<svg xmlns="http://www.w3.org/2000/svg" style="display:none">
  <symbol id="i-fw" viewBox="0 0 24 24"><path d="M9 6l3-3 3 3zM10 6h4v3h-4zM9.5 9h5l1.2 11H8.3z"/></symbol>
  <symbol id="i-people" viewBox="0 0 24 24"><circle cx="9" cy="8" r="3.2"/><path d="M3 19c.8-3.3 3.2-5 6-5s5.2 1.7 6 5"/><circle cx="17" cy="9" r="2.5"/><path d="M16 14.2c2.4.2 4.2 1.8 4.8 4.8"/></symbol>
  <symbol id="i-inbox" viewBox="0 0 24 24"><path d="M4 13l2.5-8h11L20 13v6H4z"/><path d="M4 13h5l1 2h4l1-2h5"/></symbol>
  <symbol id="i-phone" viewBox="0 0 24 24"><rect x="7" y="3" width="10" height="18" rx="2"/><path d="M11 18h2"/></symbol>
  <symbol id="i-mast" viewBox="0 0 24 24"><path d="M12 2.5v17M12 4.5l6.5 10H12M4 19.5c2.7 1.6 5.3 1.6 8 0s5.3-1.6 8 0"/></symbol>
  <symbol id="i-shield" viewBox="0 0 24 24"><path d="M12 3l7 3v5c0 4.5-3 8.3-7 10-4-1.7-7-5.5-7-10V6z"/></symbol>
  <symbol id="i-check" viewBox="0 0 24 24"><circle cx="12" cy="12" r="9"/><path d="M8 12.5l2.7 2.7L16 9.5"/></symbol>
  <symbol id="i-alert" viewBox="0 0 24 24"><path d="M12 4l9 16H3z"/><path d="M12 10v4M12 17h.01"/></symbol>
  <symbol id="i-sync" viewBox="0 0 24 24"><path d="M20 11a8 8 0 0 0-14.3-4.3L4 9M4 4v5h5M4 13a8 8 0 0 0 14.3 4.3L20 15M20 20v-5h-5"/></symbol>
  <symbol id="i-clock" viewBox="0 0 24 24"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></symbol></svg>`;
  // Hosts may select a theme explicitly; auto follows preference changes.
  const colorPreference = win.matchMedia?.('(prefers-color-scheme: dark)');
  const updateTheme = () => root.classList.toggle('is-dark', theme === 'dark' || (theme !== 'light' && !!colorPreference?.matches));
  updateTheme();
  if (theme === 'auto') colorPreference?.addEventListener('change', updateTheme);
  const setTheme = value => {
    colorPreference?.removeEventListener('change', updateTheme);
    theme = value;
    updateTheme();
    if (theme === 'auto') colorPreference?.addEventListener('change', updateTheme);
  };
  const notice = el('p', { class: 'sr', role: 'alert' });
  const toasts = el('div', { class: 'fw-toasts' });
  const status = el('p', { class: 'fw-status', 'aria-live': 'polite' }, t('loading'));
  const rail = el('nav', { class: 'fw-rail', 'aria-label': t('navigation') });
  const main = el('div', { class: 'fw-main' });
  const viewEl = el('div', { class: 'fw-content' });
  main.append(viewEl);
  root.classList.add('fwa');
  root.replaceChildren(css, spriteHost.firstChild, notice, toasts, el('div', { class: 'fw-shell' }, rail, main));

  let snapshot = null, fresh = false, busy = false, reading = false, stopped = false, timer = null;
  let view = 'family', selected = '', historyDay = '', draft = null, confirmation = null, requestsMount = null, viewEditing = false;
  viewEl.addEventListener('input', () => { viewEditing = true; });
  viewEl.addEventListener('change', () => { viewEditing = true; });

  const epoch = value => +value > 0 ? new Date(+value * 1000).toLocaleString(lang) : t('unavailable');
  const timeHm = value => +value > 0 ? new Date(+value * 1000).toLocaleTimeString(lang, { hour: '2-digit', minute: '2-digit' }) : '';
  const ic = (name, size = 18) => {
    const svg = doc.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('class', 'i'); svg.setAttribute('width', String(size)); svg.setAttribute('height', String(size));
    const use = doc.createElementNS('http://www.w3.org/2000/svg', 'use');
    use.setAttribute('href', '#' + name); svg.append(use);
    return svg;
  };
  const conf = () => snapshot?.export.configuration;
  const profiles = () => array(conf()?.current.profiles);
  const bound = id => conf()?.pact?.captain === id;
  const row = id => snapshot?.report?.profiles?.[id];
  const usageOf = id => array(snapshot.usage?.profiles).find(p => p.id === id);
  const reportedMinutes = usage => Number.isFinite(usage?.total) &&
    (usage.total > 0 || array(usage.devices).length > 0 || array(usage.week).some(day =>
      day.day === snapshot.usage?.day && day.reported === true)) ? Math.floor(usage.total / 60) : null;
  const appState = id => snapshot?.report?.app_profiles?.[id];
  const phones = () => array(snapshot.devices?.devices);
  const toast = text => { const node = el('div', { class: 'fw-toast' }, text); toasts.append(node); win.setTimeout(() => node.remove(), 6000); };
  const message = text => { notice.textContent = text; };
  const chip = (text, kind = '', ...icons) => el('span', { class: `fw-chip ${kind}`.trim() }, ...icons, text);
  const hint = text => el('p', { class: 'fw-hint' }, text);
  const panel = (title, ...content) => { const node = el('section', { class: 'fw-panel' }); if (title) node.append(el('h2', { class: 'fw-h3' }, title)); node.append(...content.flat(Infinity).filter(child => child != null)); return node; };
  const facts = values => el('dl', { class: 'fw-facts' }, values.filter(v => v[1] != null && v[1] !== '').map(([name, value]) => [el('dt', {}, name), el('dd', {}, value)]));
  const actions = (...buttons) => el('div', { class: 'fw-btns' }, ...buttons.filter(b => b != null));
  const locked = (node, value) => { node.dataset.locked = String(!!value); return node; };
  const button = (label, action, { disabled = false, pending = false, primary = false, small = false, gold = false } = {}) =>
    locked(el('button', { type: 'button',
      class: ['fw-btn', primary && 'primary', small && 'small', gold && 'gold'].filter(Boolean).join(' '),
      'data-control': '', 'data-pending': String(pending), onclick: action }, label), disabled);
  const field = (label, value, change, { type = 'text', disabled = false, required = false, ...attrs } = {}) => {
    const input = locked(el(type === 'textarea' ? 'textarea' : 'input', {
      class: type === 'checkbox' ? null : 'fw-input', type: type === 'textarea' ? null : type,
      id: type === 'checkbox' ? null : newID('fwf_'), 'data-control': '', required, ...attrs }), disabled);
    if (type === 'checkbox') input.checked = !!value; else input.value = value ?? '';
    input.addEventListener(type === 'checkbox' ? 'change' : 'input', () => change(type === 'checkbox' ? input.checked : input.value));
    return type === 'checkbox'
      ? el('label', { class: 'fw-check' }, input, el('span', {}, label))
      : el('div', { class: 'fw-field' }, el('label', { for: input.id }, label), input);
  };
  const select = (label, value, choices, change, disabled = false) => {
    const id = newID('fws_');
    const input = locked(el('select', { id, class: 'fw-select', 'data-control': '' },
      choices.map(([v, name]) => el('option', { value: v }, name))), disabled);
    input.value = value ?? '';
    input.addEventListener('change', () => change(input.value));
    return el('div', { class: 'fw-field' }, el('label', { for: id }, label), input);
  };
  const newID = prefix => prefix + Array.from(win.crypto.getRandomValues(new Uint32Array(2)), n => n.toString(16).padStart(8, '0')).join('');

  function controls() {
    root.setAttribute('aria-busy', String(busy || reading));
    for (const node of root.querySelectorAll('[data-control]'))
      node.disabled = busy || reading || !fresh || node.dataset.locked === 'true' ||
        (!!snapshot?.export.pending && node.dataset.pending !== 'true');
    for (const node of rail.querySelectorAll('button')) node.disabled = busy;
  }

  async function request(method, params = {}) {
    const reply = await call(method, params);
    const dnsState = method === 'configuration_dns_status' && reply?.format === 'dns-state-v1';
    if (!object(reply) || (!dnsState && reply.error != null && reply.error !== '')) throw new Error(reply?.error || t('invalid'));
    return reply;
  }

  async function mutate(method, params = {}, { pending = false, done } = {}) {
    if (stopped || !root.isConnected || !fresh || busy || reading || (snapshot?.export.pending && !pending)) return;
    busy = true; message(''); controls();
    try {
      const reply = await request(method, params);
      if (stopped || !root.isConnected) return;
      if (reply.status !== 'ok' && !(method === 'configuration_apply' && reply.status === 'confirmation_required') &&
          !(method === 'configuration_dns_sync' && reply.format === 'dns-state-v1')) throw new Error(t('invalid'));
      if (done) done(reply);
      else { message(t('saved')); toast(t('saved')); }
      if (!draft) viewEditing = false;
    } catch (error) {
      if (!stopped && root.isConnected) { message(t('failed', { error: error.message || error })); toast(t('failed', { error: error.message || error })); }
    } finally {
      busy = false;
      if (!stopped && root.isConnected) await refresh();
    }
  }

  function open(next, id = '') {
    if (busy || ((draft || viewEditing) && !win.confirm(t('cancel') + '?'))) return;
    requestsMount?.stop(); requestsMount = null;
    if (next !== 'history' || id !== selected) historyDay = '';
    draft = null; viewEditing = false; view = next; selected = id; message('');
    render();
  }

  function editProfile(id) {
    const current = clone(conf().current);
    if (!id) {
      id = newID('p_');
      current.profiles.push({ id, name: '', enabled: '1', policy: 'allow', block_router: '0', devices: [], dns: '0', client_id: '', protections: [], services: [],
        apps: '0', suspend_new: '1', screen_minutes: '0', screen_weekday: '', screen_weekend: '', always_allowed: [] });
    }
    viewEditing = false;
    draft = { kind: 'profile', id, current, base: JSON.stringify(current), revision: conf().revision };
    selected = id; view = 'rules'; render();
  }

  function editSettings() {
    viewEditing = false;
    draft = { kind: 'settings', current: clone(conf().current), revision: conf().revision };
    view = 'administration'; render();
  }

  function applyDraft(method = 'configuration_apply', current = draft.current) {
    if (draft.revision !== conf().revision) { message(t('conflict')); toast(t('conflict')); return; }
    return mutate(method, { revision: draft.revision, current }, { done: reply => {
      if (reply.status === 'confirmation_required') {
        if (typeof reply.token !== 'string' || typeof reply.revision !== 'string') throw new Error(t('invalid'));
        confirmation = { token: reply.token, revision: reply.revision };
        message(t('confirmHelp'));
      } else if (reply.status === 'ok' && typeof reply.revision === 'string') {
        draft = null; view = selected ? 'detail' : 'administration'; message(t('saved')); toast(t('saved'));
      } else throw new Error(t('invalid'));
    }});
  }

  function attention() {
    const items = [];
    if (!snapshot || snapshot.export.owner !== 'core') return items;
    const nowEpoch = Math.floor(win.Date.now() / 1000);
    for (const r of array(snapshot.requests?.requests))
      if (r.state === 'pending' && +r.expires > nowEpoch)
        items.push({ what: t('attnRequest', { name: r.profile_name || r.profile,
          what: r.target === 'total' ? t('attnTotal', { n: r.minutes }) : t('attnApp', { n: r.minutes, label: r.label || r.target }) }),
          why: t('attnExpires', { time: epoch(r.expires) }), view: 'requests' });
    for (const p of array(snapshot.new_apps?.profiles)) for (const a of array(p.pending))
      items.push({ what: t('attnNewApp', { name: p.name || p.id, label: a.label || a.package }), why: t('attnPaused'), view: 'requests' });
    for (const d of phones()) if (d.apps_error)
      items.push({ what: t('attnDeviceError', { name: d.name || d.id }), why: d.apps_error, view: 'devices' });
    if (snapshot.dns?.error) items.push({ what: t('attnDns'), why: snapshot.dns.error, view: 'administration' });
    return items;
  }

  function renderRail() {
    const allProfiles = profiles();
    const count = attention().length;
    const nav = key => view === key;
    rail.replaceChildren(
      el('div', { class: 'fw-brand' }, ic('i-fw', 22), el('span', {}, t('title'))),
      el('div', { class: 'fw-nav' },
        el('ul', { class: 'fw-nav-group' },
        el('button', { type: 'button', 'aria-current': nav('family') ? 'page' : null, onclick: () => open('family') },
          ic('i-people', 18), t('family')),
        ...allProfiles.map((p, i) => el('button', { type: 'button',
          'aria-current': ['detail', 'history', 'rules', 'devices'].includes(view) && selected === p.id ? 'page' : null,
          onclick: () => open('detail', p.id) },
          ic('i-phone', 18), el('span', {}, p.name || p.id),
          el('span', { class: 'fw-rail-dot' + (row(p.id)?.status === 'allowed' ? '' : ' off') }))),
        el('button', { type: 'button', 'aria-current': nav('requests') ? 'page' : null, onclick: () => open('requests') },
          ic('i-inbox', 18), t('requests'), count ? el('span', { class: 'count', 'aria-hidden': 'true' }, String(count)) : null),
        el('button', { type: 'button', 'aria-current': nav('devices') ? 'page' : null, onclick: () => open('devices') },
          ic('i-phone', 18), t('devices')),
        ),
        el('div', { class: 'fw-nav-label' }, t('household')),
        el('ul', { class: 'fw-nav-group' },
        el('button', { type: 'button', 'aria-current': nav('ulysses') ? 'page' : null, onclick: () => open('ulysses') },
          ic('i-mast', 18), t('ulysses'), conf()?.pact ? el('span', { class: 'fw-rail-dot' }) : null),
        el('button', { type: 'button', 'aria-current': nav('administration') ? 'page' : null, onclick: () => open('administration') },
          ic('i-shield', 18), t('administration')))),
      el('div', { class: 'fw-who' }, el('strong', {}, t('title')), status,
        button(t('refresh'), () => refresh(true), { small: true })));
  }

  function banner() {
    if (!fresh) return el('div', { class: 'fw-banner is-fault' }, ic('i-alert'),
      el('div', { class: 'body' }, el('p', { class: 't' }, t('stale'))));
    if (confirmation) return el('div', { class: 'fw-banner is-gold' }, ic('i-sync'),
      el('div', { class: 'body' }, el('p', { class: 't' }, t('confirmHelp'))),
      button(t('confirm'), () => mutate('configuration_confirm', confirmation,
        { pending: true, done: () => { confirmation = null; draft = null; view = selected ? 'detail' : 'administration'; } }),
        { pending: true, primary: true }));
    if (snapshot.export.pending) return el('div', { class: 'fw-banner is-gold' }, ic('i-sync'),
      el('div', { class: 'body' }, el('p', { class: 't' }, t('pending'))));
    const items = attention();
    if (items.length) return el('div', { class: 'fw-banner is-attn' }, ic('i-inbox'),
      el('div', { class: 'body' }, el('p', { class: 't' }, t('attentionBanner', { count: items.length }))),
      button(t('show'), () => open('family'), { small: true }));
    return null;
  }

  function statusChip(p) {
    if (p?.status === 'allowed') return chip(t('allowed'), 'ok');
    if (p?.status === 'blocked') return chip(t('blocked'), 'fault');
    return chip(t('unknown'), 'dim');
  }

  function windowsSummary(profile) {
    const p = row(profile.id);
    if (p?.policy === 'block') return t('blockedShort');
    if (p?.policy !== 'scheduled') return t('alwaysOn');
    const windows = array(p.schedules);
    if (!windows.length) return t('scheduled');
    if (windows.length === 1 && windows[0].start === '00:00' && windows[0].stop === '24:00'
        && (windows[0].days || '').split(/\s+/).filter(Boolean).length === 7) return t('alwaysOn');
    return windows.map(w => t('windowsShort', { start: w.start, stop: w.stop, days: w.days })).join(' · ');
  }

  function netRow(profile) {
    const p = row(profile.id);
    return el('div', { class: 'fw-kid-net' },
      statusChip(p),
      el('span', { class: 'muted' }, windowsSummary(profile)),
      p?.override && p.override !== 'none' && +p.override_expires
        ? chip(t('overrideUntil', { time: timeHm(p.override_expires) }), 'attn', ic('i-clock', 14)) : null);
  }

  function budgetBlock(profile) {
    const state = appState(profile.id), usage = usageOf(profile.id);
    const bonus = +row(profile.id)?.bonus_today || 0;
    const budget = state?.screen_budget_minutes;
    const used = reportedMinutes(usage);
    if (state?.enabled === false || !Number.isFinite(budget))
      return el('div', {}, el('h4', {}, t('screenToday')),
        used == null ? hint(t('unavailable')) : el('p', { class: 'fw-big' }, t('minUnit', { n: used })),
        hint(t(state ? 'noScreenBudget' : 'screenBudgetUnavailable')));
    if (used == null)
      return el('div', {}, el('h4', {}, t('screenToday')), hint(t('unavailable')));
    return el('div', {},
      el('div', { class: 'fw-budget-top' },
        el('p', { class: 'fw-big' }, String(Math.max(0, budget - used)), el('small', {}, t('minutesLeft'))),
        el('span', { class: 'muted' }, t('usedOf', { used, budget }))),
      el('div', { class: 'fw-meter' }, el('i', { style: `width:${budget > 0 ? Math.min(100, used / budget * 100) : 100}%` })),
      bonus ? hint(t('bonusToday', { n: bonus })) : null);
  }

  function appRows(profile, limit = 2) {
    const policy = appState(profile.id)?.policy;
    const usageApps = array(usageOf(profile.id)?.apps).slice(0, limit);
    if (!usageApps.length) return [];
    const limits = new Map(array(policy?.limits).map(l => [l.package, +l.minutes || 0]));
    const blocked = new Set(array(policy?.blocked));
    return usageApps.map(a => {
      const used = Math.floor((+a.seconds || 0) / 60);
      const limitMin = limits.get(a.package);
      const right = blocked.has(a.package) ? chip(t('appBlocked'), 'fault')
        : limitMin ? el('div', { class: 'fw-rowmeter' },
          el('div', { class: 'fw-meter', style: 'margin-top:4px' },
            el('i', { style: `width:${Math.min(100, used / limitMin * 100)}%` })),
          el('span', { class: 'muted', style: 'font-size:.8125rem' }, `${used} / ${limitMin}`))
        : chip(t('allowedApp'), 'ok');
      return el('div', { class: 'fw-approw' }, el('span', { class: 'fw-app-identity' },
        el('span', {}, a.label || a.package),
        a.label && a.package ? el('span', { class: 'fw-package mono muted' }, a.package) : null), right);
    });
  }

  function kidCard(profile, index) {
    const apps = phones().filter(d => d.profile === profile.id);
    const lastSeen = Math.max(0, ...apps.map(d => +d.seen || 0));
    return el('article', { class: 'fw-kid', 'data-profile': profile.id },
      el('div', { class: 'fw-kid-head' },
        el('div', { class: `fw-av lg fw-av-${index % 5}` }, (profile.name || profile.id)[0].toUpperCase()),
        el('div', { style: 'min-width:0' },
          el('h3', { class: 'fw-kid-name' }, profile.name || profile.id),
          el('p', { class: 'meta' }, t('devicesCount', { n: apps.length }) + (lastSeen ? ' · ' + t('lastFetch', { time: timeHm(lastSeen) }) : ' · ' + t('noFetch')))),
        row(profile.id)?.captain ? chip(t('captainChip'), 'gold') : null),
      el('div', { class: 'fw-kid-body' },
        netRow(profile),
        budgetBlock(profile),
        el('div', {}, ...appRows(profile))),
      el('div', { class: 'fw-kid-foot' },
        button(t('today'), () => open('detail', profile.id)),
        button(t('edit'), () => editProfile(profile.id)),
        button(t('devices'), () => open('devices', profile.id))));
  }


  function familyView() {
    const allProfiles = profiles();
    const items = attention();
    const admins = phones().filter(d => d.administrator === '1');
    return [
      el('div', { class: 'fw-head' },
        el('div', {}, el('p', { class: 'fw-eyebrow' }, t('family')),
          el('h1', { class: 'fw-h1' }, t('family')),
          el('p', { class: 'fw-sub' }, t('familySub'))),
        button('+ ' + t('add'), () => editProfile(''))),
      items.length ? el('section', { class: 'fw-section', 'aria-labelledby': 'attn-h' },
        el('h2', { class: 'fw-h2', id: 'attn-h' }, t('needsAttention')),
        el('div', { class: 'fw-attn' }, items.map(it => el('div', { class: 'fw-attn-row' },
          el('div', { class: 'who' }, el('div', { class: 'fw-av fw-av-0' }, ic('i-inbox', 18)),
            el('div', { style: 'min-width:0' }, el('p', { class: 'what' }, it.what), el('p', { class: 'why' }, it.why)))),
          button(t('show'), () => open(it.view), { small: true })))) : null,
      allProfiles.length ? el('section', { class: 'fw-section', 'aria-labelledby': 'profiles-h' },
        el('h2', { class: 'fw-h2', id: 'profiles-h' }, t('profilesHeading')),
        el('div', { class: 'fw-kids' }, allProfiles.map((p, i) => kidCard(p, i))))
        : el('div', { class: 'fw-empty' }, el('h2', { class: 'fw-h2' }, t('empty')),
          button(t('add'), () => editProfile(''), { primary: true })),
      admins.length ? el('section', { class: 'fw-section', 'aria-labelledby': 'admin-phones-h' },
        el('h2', { class: 'fw-h2', id: 'admin-phones-h' }, t('administratorPhones')),
        el('div', { class: 'fw-panel', style: 'padding:6px 20px' },
          el('ul', { class: 'fw-list' }, admins.map((d, i) => el('li', { class: 'fw-item' },
            el('div', { class: `fw-av fw-av-${(i + 1) % 5}` }, (d.name || '?')[0].toUpperCase()),
            el('div', { class: 'main' }, el('p', { class: 't' }, d.name || d.id),
              el('p', { class: 's' }, profiles().find(p => p.id === d.profile)?.name || t('unavailable'))),
            chip(t('role'), 'gold')))))) : null
    ];
  }

  function todayView(profile) {
    const p = row(profile.id), usage = usageOf(profile.id);
    const used = reportedMinutes(usage);
    const bonus = +row(profile.id)?.bonus_today || 0;
    const budget = appState(profile.id)?.screen_budget_minutes;
    const meter = el('div', { class: 'fw-meter' },
      el('i', { style: `width:${budget > 0 && used != null ? Math.min(100, used / budget * 100) : 0}%` }));
    return [
      el('div', { class: 'fw-grid2' },
        panel(t('screenToday'),
          el('p', { class: 'fw-big' }, used == null ? t('unavailable') : String(used), used == null ? null : el('small', {}, t('minShort'))),
          Number.isFinite(budget) ? meter : null,
          Number.isFinite(budget) ? hint(t('budgetFor', { n: budget }) + (bonus ? ' · ' + t('bonusToday', { n: bonus }) : ''))
            : hint(t(appState(profile.id) ? 'noScreenBudget' : 'screenBudgetUnavailable')),
          hint(t('usageHelp'))),
        panel(t('internet'),
          el('div', { class: 'fw-kid-net' }, statusChip(p)),
          hint(t('routerEnforced')),
          facts([[ t('policy'), p?.policy === 'scheduled' ? t('scheduled') : p?.policy === 'block' ? t('blockPolicy') : t('unrestricted') ],
            [ t('until'), p?.override && p.override !== 'none' && +p.override_expires ? epoch(p.override_expires) : t('none') ]]),
          el('div', { class: 'fw-kid-net' }, el('span', { class: 'muted' }, windowsSummary(profile))),
          array(p?.schedules).length ? el('div', {}, array(p.schedules).map(w =>
            el('div', { class: 'fw-item', style: 'padding:6px 0' },
              el('span', { class: 'mono' }, w.days),
              el('span', {}, `${w.start}–${w.stop}`),
              w.start === '00:00' && w.stop === '24:00' ? chip(t('alwaysOn'), 'dim') : null))) : null,
          networkActions(profile))),
      appsPanel(profile),
      devicesPanel(profile)
    ];
  }

  function appsPanel(profile) {
    const policy = appState(profile.id)?.policy;
    const rows = appRows(profile, 5);
    const downtime = array(policy?.downtime).map(w => `${w.days} ${w.start}–${w.stop}`).join(' · ');
    return panel(t('apps'), hint(t('managed')),
      profile.apps === '1'
        ? el('div', {}, rows.length ? el('div', {}, ...rows) : hint(t('noAppsToday')),
           downtime ? hint(t('downtime') + ': ' + downtime) : null)
        : hint(t('noAppsToday')));
  }

  function pipeline(device) {
    const enabled = profiles().find(p => p.id === device.profile)?.apps === '1';
    if (!enabled) return [el('li', { class: 'unknown' }, el('span', { class: 'dot' }), t('pipeOff'))];
    const step = (state, label) => el('li', { class: state }, el('span', { class: 'dot' }), label);
    return [
      step('done', t('pipeSaved')),
      device.apps_rev ? step('done', t('pipeAvailable')) : step('now', t('pipeAvailable')),
      +device.seen ? step('done', t('pipeFetched')) : step('now', t('pipeFetched')),
      device.apps_error ? step('failed', t('pipeApplied'))
        : device.apps_applied ? step('done', t('pipeApplied')) : step('unknown', t('pipeApplied'))];
  }

  function perDeviceTable(usage) {
    return el('table', { class: 'fw-table' },
      el('thead', {}, el('tr', {}, el('th', {}, t('perDevice')), el('th', {}, t('minutes')),
        el('th', {}, t('incomplete')), el('th', {}, t('lastReceived')))),
      el('tbody', {}, array(usage?.devices).map(d => el('tr', {},
        el('td', {}, d.name || '—'),
        el('td', {}, t('minUnit', { n: Math.floor((+d.total || 0) / 60) })),
        el('td', {}, d.incomplete ? t('incomplete') : ''),
        el('td', {}, d.received ? epoch(d.received) : '')))));
  }

  function devicesPanel(profile) {
    const list = phones().filter(d => d.profile === profile.id);
    const usage = usageOf(profile.id);
    return panel(t('devices'),
      list.length ? el('ul', { class: 'fw-list' }, list.map(d => deviceItem(d))) : hint(t('noPhones')),
      array(usage?.devices).length ? perDeviceTable(usage) : null,
      hint(t('usageHelp')));
  }

  function historyView(profile) {
    const usage = usageOf(profile.id), model = historyModel(usage);
    const duration = seconds => {
      if (seconds == null) return '—';
      const minutes = Math.floor(seconds / 60), hours = Math.floor(minutes / 60), rest = minutes % 60;
      return hours ? `${hours} ${t('hourShort')}${rest ? ' ' + t('minUnit', { n: rest }) : ''}` : t('minUnit', { n: minutes });
    };
    const dayLabel = day => {
      const date = new Date(day + 'T12:00:00Z');
      return Number.isNaN(date.getTime()) ? t('unavailable')
        : new Intl.DateTimeFormat(lang, { weekday: 'short', month: 'short', day: 'numeric', timeZone: 'UTC' }).format(date);
    };
    const selectedDay = model.entries.find(d => d.day === historyDay && d.measured);
    const maximum = Math.max(1, ...model.entries.filter(d => d.measured).map(d => d.seconds));
    const chart = el('div', { class: 'fw-hist', role: 'table', 'aria-label': t('historyPerDay') },
      model.entries.map(d => el('div', { class: 'fw-hist-row', role: 'row', 'data-history-day': d.day },
        el('span', { role: 'cell' }, locked(el('button', { type: 'button', class: 'fw-daybtn', 'data-control': '',
          'aria-label': t('historySelectDay', { day: d.day }), 'aria-pressed': String(selectedDay?.day === d.day),
          onclick: () => { historyDay = selectedDay?.day === d.day ? '' : d.day; render(); }
        }, dayLabel(d.day)), !d.measured)),
        d.measured ? el('span', { role: 'cell', class: 'bar' },
          el('i', { style: `width:${d.seconds / maximum * 100}%` }))
          : el('span', { role: 'cell', class: 'none' }, t(d.reported === false ? 'historyNoReport' : 'reportUnknown')),
        d.measured ? el('span', { role: 'cell', class: 'val', title: d.reported == null ? t('reportUnknown') : null }, duration(d.seconds)) : null)));
    const ranges = el('div', { class: 'fw-seg', role: 'group', 'aria-label': t('historyRange') },
      [['week', t('historyRange_week')], ['month', t('historyRange_month')], ['year', t('historyRange_year')], ['all', t('historyRange_all')]]
        .map(([range, label]) => locked(el('button', { type: 'button', 'data-control': '',
        'aria-pressed': String(range === 'week'), onclick: range === 'week' ? () => { historyDay = ''; render(); } : undefined, title: range === 'week' ? null : t('historyRangeUnavailable')
      }, label), range !== 'week')));
    const stats = el('dl', { class: 'fw-histsum' },
      [[t('historyTotal'), duration(model.total)], [t('historyAverage'), duration(model.average)],
       [t('historyOverBudget'), '—'],
       [t('reported'), model.entries.length ? t('historyReportedDays', { n: model.reported, total: model.entries.length }) : '—']]
        .map(([label, value]) => el('div', {}, el('dt', {}, label), el('dd', {}, value))));
    const screen = el('section', { class: 'fw-panel fw-history-screen' },
      el('div', { class: 'fw-section-head' }, el('h2', { class: 'fw-h2' }, t('historyScreenTime')),
        el('span', { class: 'muted' }, t('historyBudgetUnavailable'))),
      el('div', { class: 'fw-histbar' }, ranges,
        el('div', { class: 'fw-histnav' },
          locked(el('button', { type: 'button', class: 'fw-daybtn', 'aria-label': t('historyPrevious'), title: t('historyRangeUnavailable'), 'data-control': '' }, '‹'), true),
          el('span', { class: 'lbl' }, t('week')),
          locked(el('button', { type: 'button', class: 'fw-daybtn', 'aria-label': t('historyNext'), title: t('historyRangeUnavailable'), 'data-control': '' }, '›'), true))),
      hint(t('historyRangeUnavailable')),
      model.entries.length ? chart : hint(usage ? t('noWeek') : t('unavailable')),
      stats, hint(t('historyGapHelp')),
      model.unknown ? hint(t('historyUnknownCoverage', { n: model.unknown })) : null,
      hint(t('usageHelp')));
    const showToday = !selectedDay || selectedDay.day === snapshot.usage?.day;
    const appRows = array(usage?.apps).filter(a => Number.isFinite(a.seconds) && a.seconds >= 0);
    const maxApp = Math.max(1, ...appRows.map(a => a.seconds));
    const appSection = el('section', { class: 'fw-panel fw-history-apps' },
      el('div', { class: 'fw-section-head' }, el('h2', { class: 'fw-h2' }, t('historyByApp')),
        el('span', { class: 'muted' }, t('historyWhereTimeGoes'))),
      el('div', { class: 'fw-section-head' },
        el('p', { class: 'fw-hint' }, showToday ? t('historyTodayApps', { day: snapshot.usage?.day || t('unavailable') }) : dayLabel(selectedDay.day)),
        selectedDay ? button(t('historyClearDay'), () => { historyDay = ''; render(); }, { small: true }) : null),
      !showToday ? hint(t('historyPastAppsUnavailable'))
        : !usage ? hint(t('unavailable'))
        : appRows.length ? el('ul', { class: 'fw-list' }, appRows.map(a => el('li', { class: 'fw-item' },
          el('span', { class: 'fw-app-icon', 'aria-hidden': 'true' }, (a.label || a.package || '?')[0].toUpperCase()),
          el('div', { class: 'main' }, el('p', { class: 't' }, a.label || a.package),
            a.label && a.label !== a.package ? el('p', { class: 's mono' }, a.package) : null,
            el('p', { class: 's' }, duration(a.seconds))),
          el('div', { class: 'fw-rowmeter' }, el('div', { class: 'fw-meter' }, el('i', { style: `width:${a.seconds / maxApp * 100}%` }))))))
        : hint(t('noAppsToday')),
      hint(t('historyAppScope')));
    return [screen, appSection];
  }

  function childView(profile) {
    if (!profile) return familyView();
    const idx = Math.max(0, profiles().findIndex(p => p.id === profile.id));
    const p = row(profile.id);
    const tab = view === 'history' ? 'history' : view === 'rules' ? 'rules' : view === 'devices' ? 'devices' : 'detail';
    const head = el('div', { class: 'fw-head' },
      el('div', { style: 'display:flex;align-items:center;gap:14px;min-width:0' },
        el('div', { class: `fw-av lg fw-av-${idx % 5}` }, (profile.name || profile.id)[0].toUpperCase()),
        el('div', { style: 'min-width:0' },
          el('p', { class: 'fw-eyebrow' }, t('family')),
          el('h1', { class: 'fw-h1' }, profile.name || profile.id),
          el('p', { class: 'fw-sub' }, [p?.captain ? t('captainChip') : null,
            p?.policy === 'scheduled' ? t('scheduled') : p?.policy === 'block' ? t('blockedShort') : t('alwaysOn')]
            .filter(Boolean).join(' · ')))));
    return [
      head,
      el('div', { class: 'fw-tabs', role: 'tablist', 'aria-label': profile.name || profile.id },
        [['detail', t('today')], ['history', t('history')], ['rules', t('rules')], ['devices', t('devices')]]
          .map(([key, label]) => el('button', { type: 'button', role: 'tab',
            'aria-selected': String(tab === key), 'data-control': '',
            onclick: () => {
              if (key === view) return;
              key === 'rules' ? editProfile(profile.id) : open(key, profile.id);
            } }, label))),
      view === 'rules' ? (draft ? profileEditor() : el('div', { class: 'fw-empty' }, hint(t('frozen'))))
        : view === 'history' ? historyView(profile)
        : view === 'devices' ? el('div', { class: 'fw-section' },
            el('h2', { class: 'fw-h2' }, t('devices')), devicesPanel(profile))
        : todayView(profile)
    ];
  }

  function deviceItem(device) {
    const profile = profiles().find(p => p.id === device.profile);
    return el('li', { class: 'fw-item', style: 'align-items:flex-start' },
      el('div', { class: 'fw-av fw-av-4' }, ic('i-phone', 18)),
      el('div', { class: 'main' },
        el('p', { class: 't' }, device.name || device.id, ' ',
          device.administrator === '1' ? chip(t('role'), 'gold') : null,
          profile?.id === conf()?.pact?.captain ? chip(t('captainChip'), 'gold') : null),
        el('p', { class: 's' }, [profile?.name || t('unavailable'),
          +device.seen ? t('lastFetch', { time: timeHm(device.seen) }) : t('noFetch')].join(' · ')),
        el('ul', { class: 'fw-pipe' }, ...pipeline(device)),
        +device.remove_until ? el('p', { class: 's' }, t('removalUntil', { time: epoch(device.remove_until) })) : null,
        device.apps_error ? el('p', { class: 's', style: 'color:var(--fault)' }, t('applyError') + ': ' + device.apps_error) : null,
        !+device.seen && !device.apps_error ? el('p', { class: 's' }, t('deviceUnknown')) : null),
      el('div', { style: 'width:100%;display:flex;flex-direction:column;gap:10px' },
        deviceEditor(device, false),
        actions(
          button(t('removeApproval'), () => mutate('device_allow_removal', { device: device.id }), { small: true, disabled: device.frozen === true }),
          button(t('forget'), () => { if (win.confirm(t('forgetWarning'))) mutate('device_delete', { device: device.id }); },
            { small: true, disabled: device.frozen === true }))));
  }

  function deviceEditor(device, pending) {
    const value = { device: device.id, name: device.name || '', profile: device.profile || profiles()[0]?.id || '',
      administrator: device.administrator || '0', approves: device.approves || '', fingerprint: device.fingerprint };
    let compared = false;
    const frozen = device.frozen === true;
    const save = () => { if (form.reportValidity() && (!pending || compared)) mutate(pending ? 'device_approve' : 'device_update', value); };
    const form = el('form', { onsubmit: event => { event.preventDefault(); save(); } });
    form.append(
      field(t('name'), value.name, v => value.name = v, { required: true, disabled: frozen }),
      select(t('profile'), value.profile, profiles().map(p => [p.id, p.name || p.id]), v => value.profile = v, frozen),
      field(t('role'), value.administrator === '1', v => value.administrator = v ? '1' : '0', { type: 'checkbox', disabled: frozen }),
      el('fieldset', { style: 'border:0;margin:0;padding:0' }, el('legend', { class: 'fw-label', style: 'margin-bottom:4px' }, t('scopes')),
        profiles().map(p => field(p.name || p.id, String(value.approves).split(/\s+/).includes(p.id), checked => {
          const set = new Set(String(value.approves).split(/\s+/).filter(Boolean));
          checked ? set.add(p.id) : set.delete(p.id); value.approves = Array.from(set).join(' ');
        }, { type: 'checkbox', disabled: frozen }))),
      frozen ? hint(t('frozenPhone')) : '',
      pending ? field(t('compare'), false, v => { compared = v; }, { type: 'checkbox', required: true }) : '',
      actions(
        button(t(pending ? 'approve' : 'save'), save, { disabled: frozen || !profiles().length }),
        pending ? button(t('reject'), () => mutate('device_reject', { device: device.id })) : null));
    return form;
  }

  function phonesView() {
    const pending = array(snapshot.devices?.pending);
    const list = phones();
    return [
      el('div', { class: 'fw-head' },
        el('div', {}, el('p', { class: 'fw-eyebrow' }, t('title')),
          el('h1', { class: 'fw-h1' }, t('devices')),
          el('p', { class: 'fw-sub' }, t('enforcementSub')))),
      callHost ? pairingCard() : null,
      el('section', { class: 'fw-section', 'aria-labelledby': 'pend-h' },
        el('h2', { class: 'fw-h2', id: 'pend-h' }, t('pendingPhones')),
        pending.length ? pending.map(d => el('div', { class: 'fw-panel' },
          el('div', { class: 'fw-fp', 'aria-label': t('fingerprint') },
            ...(String(d.fingerprint).match(/.{1,4}/g) || []).map(g => el('span', {}, g))),
          deviceEditor(d, true))) : hint(t('noPhones'))),
      el('section', { class: 'fw-section', 'aria-labelledby': 'phones-h' },
        el('h2', { class: 'fw-h2', id: 'phones-h' }, t('devices')),
        list.length ? el('div', { class: 'fw-panel', style: 'padding:6px 20px' },
          el('ul', { class: 'fw-list' }, list.map(deviceItem))) : hint(t('noPhones')))
    ];
  }

  function pairingCard() {
    const node = panel(t('pairing'), hint(t('pairingHelp')),
      button(t('pairing'), async () => {
        try {
          const info = await callHost('pairing', {});
          if (!object(info) || info.error || !info.uri) throw new Error(info?.error || t('unavailable'));
          node.append(facts([[t('fingerprint'), el('code', {}, info.fingerprint)]]),
            el('p', {}, el('code', {}, info.uri)));
          if (info.qr) node.append(el('img', { class: 'fw-qr', alt: t('pairing'),
            src: 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(info.qr) }));
          controls();
        } catch (error) { message(t('failed', { error: error.message || error })); }
      }));
    return node;
  }

  function networkActions(profile) {
    const p = row(profile.id), unknown = !['allowed', 'blocked'].includes(p?.status);
    return actions(
      button(t('block'), () => mutate('configuration_block', { profile: profile.id }), { disabled: unknown }),
      button(t('allow'), () => mutate('configuration_allow', { profile: profile.id }), { disabled: unknown || bound(profile.id) }),
      button(t('reset'), () => mutate('configuration_reset', { profile: profile.id }), { disabled: unknown }),
      [15, 30, 60].map(minutes => button(t('ticket', { minutes }),
        () => mutate('configuration_ticket', { profile: profile.id, minutes }),
        { small: true, disabled: unknown || bound(profile.id) })),
      profile.apps === '1' ? [15, 30, 60].map(minutes => button(t('bonus', { minutes }),
        () => mutate('configuration_bonus_add', { profile: profile.id, minutes }),
        { small: true, disabled: bound(profile.id) })) : null);
  }

  function windowsEditor(current, key, id, disabled) {
    const title = t(key === 'schedules' ? 'schedules' : 'downtime');
    const fieldset = el('fieldset', { class: 'fw-rule-windows' }, el('legend', { class: 'sr' }, title));
    for (const w of array(current[key]).filter(v => v.profile === id)) {
      const selectedDays = () => new Set(String(w.days || '').split(/[ ,]+/).filter(Boolean));
      const dayButtons = el('div', { class: 'fw-days', role: 'group', 'aria-label': t('days') });
      for (const day of days) {
        const dayButton = locked(el('button', { type: 'button', 'data-control': '',
          'aria-label': t(day), 'aria-pressed': String(selectedDays().has(day)),
          onclick: () => {
            const set = selectedDays();
            set.has(day) ? set.delete(day) : set.add(day);
            w.days = days.filter(d => set.has(d)).join(' ');
            dayButton.setAttribute('aria-pressed', String(set.has(day)));
            dayButton.dispatchEvent(new win.Event('change', { bubbles: true }));
          } }, t(day).slice(0, 3)), disabled);
        dayButtons.append(dayButton);
      }
      fieldset.append(el('div', { class: 'fw-window', role: 'group', 'aria-label': title + ' ' + w.id },
        el('div', { class: 'fw-field' }, el('span', { class: 'fw-label' }, t('days')), dayButtons),
        field(t('start'), w.start, v => w.start = v, { disabled, required: true, placeholder: '07:00', pattern: '[0-2][0-9]:[0-5][0-9]' }),
        field(t('stop'), w.stop, v => w.stop = v, { disabled, required: true, placeholder: '20:00', pattern: '[0-2][0-9]:[0-5][0-9]' }),
        button(t('remove'), () => { current[key] = current[key].filter(v => v !== w); render(); }, { small: true, disabled })));
    }
    fieldset.append(button('+ ' + t('addWindow'), () => {
      current[key] = [...array(current[key]), { id: newID('w_'), profile: id, days: 'mon tue wed thu fri', start: '', stop: '' }];
      render();
    }, { small: true, disabled }));
    return fieldset;
  }

  function appRulesEditor(current, profile, disabled) {
    const id = profile.id;
    const configured = array(current.apps).filter(a => a.profile === id);
    const observed = array(usageOf(id)?.apps);
    const pending = array(array(snapshot.new_apps?.profiles).find(p => p.id === id)?.pending);
    const packages = [...new Set([...configured.map(a => a.package), ...array(profile.always_allowed), ...observed.map(a => a.package), ...pending.map(a => a.package)])];
    const node = el('fieldset', { class: 'fw-rule-apps' }, el('legend', { class: 'sr' }, t('appRules')));
    const list = el('ul', { class: 'fw-list' });
    for (const pkg of packages) {
      const entries = configured.filter(a => a.package === pkg), app = entries[0];
      const awaiting = pending.find(a => a.package === pkg);
      const label = observed.find(a => a.package === pkg)?.label || awaiting?.label || pkg;
      if (awaiting) {
        list.append(el('li', { class: 'fw-item' },
          el('span', { class: 'fw-app-icon', 'aria-hidden': 'true' }, initial(label, pkg)),
          el('div', { class: 'main' }, el('p', { class: 't' }, label),
            label !== pkg ? el('p', { class: 's mono' }, pkg) : null), chip(t('awaitingDecision'), 'attn')));
        continue;
      }
      const mode = entries.some(a => a.blocked === '1') ? 'blocked' : array(profile.always_allowed).includes(pkg) ? 'unlimited'
        : app && (app.minutes === '' || +app.minutes > 0) ? 'limit' : 'allowed';
      const setRule = next => {
        const name = app?.package || pkg;
        current.apps = array(current.apps).filter(a => a.profile !== id || a.package !== name);
        profile.always_allowed = array(profile.always_allowed).filter(a => a !== name);
        if (next === 'unlimited') profile.always_allowed.push(name);
        if (next === 'blocked' || next === 'limit') current.apps.push({ id: app?.id || newID('a_'), profile: id,
          package: name, minutes: next === 'limit' ? (app && +app.minutes > 0 ? app.minutes : '') : '0', blocked: next === 'blocked' ? '1' : '0' });
        render();
      };
      list.append(el('li', { class: 'fw-item fw-rule-app' },
        el('span', { class: 'fw-app-icon', 'aria-hidden': 'true' }, pkg ? initial(label, pkg) : '+'),
        el('div', { class: 'main' }, pkg ? el('p', { class: 't' }, label)
          : field(t('package'), app.package, v => app.package = v, { disabled, required: true, placeholder: 'com.example.app' }),
          label !== pkg ? el('p', { class: 's mono', title: pkg }, pkg) : null),
        el('div', { class: 'fw-app-controls' },
          select(t('ruleFor', { name: label || t('package') }), mode,
            [['allowed', t('appAllowed')], ['limit', t('appOwnLimit')], ['unlimited', t('appUnlimited')], ['blocked', t('appBlocked')]], setRule, disabled),
          mode === 'limit' ? field(t('minutes'), app.minutes, v => app.minutes = v, { type: 'number', min: 1, max: 1440, required: true, disabled }) : null,
          app || array(profile.always_allowed).includes(pkg) ? button(t('remove'), () => {
            current.apps = array(current.apps).filter(a => a.profile !== id || a.package !== (app?.package || pkg));
            profile.always_allowed = array(profile.always_allowed).filter(a => a !== pkg);
            render();
          }, { small: true, disabled }) : null)));
    }
    node.append(packages.length ? list : hint(t('noRuleApps')),
      button('+ ' + t('addAppRule'), () => {
        current.apps = [...array(current.apps), { id: newID('a_'), profile: id, package: '', minutes: '', blocked: '0' }];
        render();
      }, { small: true, disabled }));
    return node;
  }

  function profileEditor() {
    const current = draft.current, p = current.profiles.find(v => v.id === draft.id);
    const frozen = bound(p.id), windowOpen = row(p.id)?.pact_in_window === true, ruleLock = frozen && !windowOpen;
    const form = el('form', { class: 'fw-rules', onsubmit: event => { event.preventDefault(); if (form.reportValidity()) applyDraft(); } });
    const changed = () => JSON.stringify(current) !== draft.base;
    const saveBar = el('div', { class: 'fw-dirty', role: 'region', 'aria-label': t('unsavedChanges') },
      el('p', {}, el('strong', {}, t('unsavedChanges')), ' · ', t('unsavedHelp')),
      button(t('discard'), () => {
        if (profiles().some(profile => profile.id === p.id)) editProfile(p.id);
        else { draft = null; viewEditing = false; selected = ''; view = 'family'; render(); }
      }),
      button(t('saveRules'), () => { if (form.reportValidity()) applyDraft(); }, { primary: true, disabled: frozen }),
      frozen && windowOpen ? button(t('amend'), () => {
        if (form.reportValidity() && win.confirm(t('amendWarning'))) applyDraft('configuration_amend');
      }, { primary: true }) : null);
    const updateDirty = () => { saveBar.hidden = !changed(); };
    form.addEventListener('input', updateDirty);
    form.addEventListener('change', updateDirty);
    const rulesPanel = (title, badge, ...content) => el('section', { class: 'fw-panel fw-rule-panel' },
      el('div', { class: 'fw-section-head' }, el('h2', { class: 'fw-h2' }, title), badge ? chip(badge, 'dim') : null), ...content);
    const pattern = draft.budgetMode || (p.screen_weekday !== '' && p.screen_weekday != null || p.screen_weekend !== '' && p.screen_weekend != null ? 'split' : 'all');
    const budgetPattern = el('div', { class: 'fw-seg', role: 'group', 'aria-label': t('budgetPattern') },
      ['all', 'split'].map(mode => locked(el('button', { type: 'button', 'data-control': '', 'aria-pressed': String(pattern === mode), onclick: () => {
        if (mode === pattern) return;
        draft.budgetMode = mode;
        if (mode === 'all') { p.screen_weekday = ''; p.screen_weekend = ''; }
        else { p.screen_weekday = p.screen_weekday || p.screen_minutes; p.screen_weekend = p.screen_weekend || p.screen_minutes; }
        render();
      } }, t(mode === 'all' ? 'allDays' : 'splitDays')), ruleLock)));
    const newAppsSwitch = locked(el('button', { type: 'button', class: 'fw-switch', role: 'switch', 'data-control': '',
      'aria-label': t('suspendNew'), 'aria-checked': String(p.suspend_new === '1'), onclick: () => {
        p.suspend_new = p.suspend_new === '1' ? '0' : '1'; render();
      } }), ruleLock);
    form.append(
      el('div', { class: 'fw-banner is-gold' }, ic('i-alert', 20),
        el('div', { class: 'body' }, el('p', { class: 't' }, t('afterSaving')), el('p', {}, t('rulesSummary')))),
      ruleLock ? hint(t('frozen')) : '',
      rulesPanel(t('dailyBudget'), t('sharedDevices'), budgetPattern,
        el('div', { class: 'fw-budget-fields' }, pattern === 'all'
          ? field(t('screen'), p.screen_minutes, v => p.screen_minutes = v, { type: 'number', min: 0, max: 1440, disabled: ruleLock })
          : [field(t('weekday'), p.screen_weekday, v => p.screen_weekday = v, { type: 'number', min: 0, max: 1440, disabled: ruleLock }),
             field(t('weekend'), p.screen_weekend, v => p.screen_weekend = v, { type: 'number', min: 0, max: 1440, disabled: ruleLock })]),
        hint(t('budgetSharingHelp'))),
      rulesPanel(t('rulesApps'), '', appRulesEditor(current, p, ruleLock),
        el('div', { class: 'fw-item' }, el('div', { class: 'main' }, el('p', { class: 't' }, t('suspendNew')),
          el('p', { class: 's' }, t('newAppsHelp'))), newAppsSwitch)),
      rulesPanel(t('rulesDowntime'), t('onDevices'), hint(t('downtimeHelp')), windowsEditor(current, 'downtime', p.id, ruleLock)),
      rulesPanel(t('homeSchedule'), t('onRouter'), hint(t('homeScheduleHelp')),
        select(t('policy'), p.policy || 'allow', [['allow', t('unrestricted')], ['block', t('blockPolicy')], ['scheduled', t('scheduled')]], v => p.policy = v, ruleLock),
        windowsEditor(current, 'schedules', p.id, ruleLock)));
    const dns = rulesPanel(t('dns'), '', frozen ? hint(t('dnsDeferred')) : null,
      field(t('dns'), p.dns === '1', v => p.dns = v ? '1' : '0', { type: 'checkbox' }),
      facts([[ t('clientID'), p.client_id || t('none') ]]),
      protections.map(protection => field(t(protection), array(p.protections).includes(protection), checked => {
        const set = new Set(array(p.protections));
        checked ? set.add(protection) : set.delete(protection);
        p.protections = Array.from(set);
      }, { type: 'checkbox' })),
      field(t('services'), array(p.services).join('\n'), v => p.services = lines(v), { type: 'textarea' }));
    dns.append(button(t('catalogue'), async () => {
      if (busy || !fresh) return;
      try {
        const reply = await request('configuration_dns_services');
        if (!Array.isArray(reply.services)) throw new Error(t('invalid'));
        dns.append(el('div', {}, reply.services.map(service =>
          field(service.name || service.id, array(p.services).includes(service.id), checked => {
            const set = new Set(array(p.services));
            checked ? set.add(service.id) : set.delete(service.id);
            p.services = Array.from(set);
          }, { type: 'checkbox' }))));
        controls();
      } catch (error) { message(t('failed', { error: error.message })); }
    }));
    if (frozen) dns.append(button(t('saveDNS'), () => {
      const onlyDNS = clone(conf().current), target = onlyDNS.profiles.find(v => v.id === p.id);
      for (const key of ['dns', 'client_id', 'protections', 'services']) target[key] = p[key];
      applyDraft('configuration_dns_update', onlyDNS);
    }, { primary: true }));
    form.append(el('div', { class: 'fw-grid2' },
      rulesPanel(t('profileDetails'), '',
        field(t('name'), p.name, v => p.name = v, { required: true, disabled: ruleLock }),
        field(t('enabled'), p.enabled === '1', v => p.enabled = v ? '1' : '0', { type: 'checkbox', disabled: ruleLock }),
        field(t('apps'), p.apps === '1', v => p.apps = v ? '1' : '0', { type: 'checkbox', disabled: ruleLock }),
        field(t('macs'), array(p.devices).join('\n'), v => p.devices = lines(v), { type: 'textarea', disabled: ruleLock }),
        field(t('blockRouter'), p.block_router === '1', v => p.block_router = v ? '1' : '0', { type: 'checkbox', disabled: ruleLock }),
        field(t('always'), array(p.always_allowed).join('\n'), v => p.always_allowed = lines(v), { type: 'textarea', disabled: ruleLock }),
        actions(button(t('cancel'), () => { draft = null; viewEditing = false; view = 'detail'; render(); }),
          button(t('reloadDraft'), () => editProfile(p.id), { disabled: !profiles().some(v => v.id === p.id) }),
          button(t('delete'), () => {
            if (!win.confirm(t('deleteWarning'))) return;
            const candidate = clone(current);
            candidate.profiles = candidate.profiles.filter(v => v.id !== p.id);
            for (const key of ['schedules', 'apps', 'downtime']) candidate[key] = array(candidate[key]).filter(v => v.profile !== p.id);
            applyDraft('configuration_apply', candidate);
          }, { disabled: frozen }))), dns), saveBar);
    updateDirty();
    return form;
  }

  function settingsEditor() {
    const s = draft.current.settings, frozen = !!conf().pact;
    const form = el('form', { onsubmit: event => { event.preventDefault(); if (form.reportValidity()) applyDraft(); } });
    form.append(el('h2', { class: 'fw-h2' }, t('settingsTitle')), frozen ? hint(t('frozen')) : '',
      field(t('globalEnabled'), s.enabled !== '0', v => s.enabled = v ? '1' : '0', { type: 'checkbox', disabled: frozen }),
      select(t('defaultPolicy'), s.default_policy || 'allow', [['allow', t('unrestricted')], ['block', t('blockPolicy')]], v => s.default_policy = v, frozen),
      select(t('captain'), s.captain || '', [['', t('none')], ...profiles().map(p => [p.id, p.name || p.id])], v => s.captain = v, frozen),
      field(t('delay'), s.delay_hours, v => s.delay_hours = v, { type: 'number', min: 0, disabled: frozen }),
      field(t('domain'), s.dns_domain, v => s.dns_domain = v, { disabled: frozen }),
      field(t('endpoint'), s.resolver_url, v => s.resolver_url = v, { type: 'url' }),
      actions(
        button(t('save'), () => { if (form.reportValidity()) applyDraft(); }, { primary: true }),
        button(t('cancel'), () => { draft = null; viewEditing = false; render(); }),
        button(t('reloadDraft'), editSettings)));
    return el('div', { class: 'fw-section' }, form);
  }

  const WAVE = `<svg class="fw-voyage-sea" viewBox="0 0 600 40" preserveAspectRatio="none" aria-hidden="true"><path d="M0 22 C50 10 100 10 150 22 S250 34 300 22 S400 10 450 22 S550 34 600 22" /><path d="M0 32 C50 22 100 22 150 32 S250 42 300 32 S400 22 450 32 S550 42 600 32" style="opacity:.5"/></svg>`;

  function voyageForm(pact) {
    const value = { type: pact?.type || 'until', until: '', day: pact?.window_day || 'sun',
      start: pact?.window_start || '10:00', stop: pact?.window_stop || '11:00' };
    const form = el('form', { onsubmit: event => { event.preventDefault(); submit(); } });
    if (!pact) form.append(select(t('pact'), value.type, [['until', t('until')], ['ongoing', t('ongoing')]], v => value.type = v));
    form.append(field(t('untilTime'), value.until, v => value.until = v, { type: 'datetime-local' }),
      select(t('days'), value.day, days.map(day => [day, t(day)]), v => value.day = v),
      field(t('start'), value.start, v => value.start = v, { pattern: '[0-2][0-9]:[0-5][0-9]' }),
      field(t('stop'), value.stop, v => value.stop = v, { pattern: '[0-2][0-9]:[0-5][0-9]' }),
      hint(t('makeWarning')),
      button(t(pact ? 'extend' : 'makePact'), submit, { primary: true }));
    function submit() {
      if (!form.reportValidity() || !win.confirm(t('makeWarning'))) return;
      const until = value.type === 'ongoing' ? 0 : Math.floor(new Date(value.until).getTime() / 1000);
      if (!Number.isFinite(until)) { message(t('failed', { error: t('untilTime') })); return; }
      mutate('configuration_pact', { until, window_day: value.day, window_start: value.start, window_stop: value.stop });
    }
    return form;
  }

  function glossary() {
    return el('dl', { class: 'fw-gloss' },
      el('div', {}, el('dt', {}, t('captain')), el('dd', {}, t('glossCaptain'))),
      el('div', {}, el('dt', {}, t('voyageAtMast')), el('dd', {}, t('glossMast'))),
      el('div', {}, el('dt', {}, t('voyageFrozen')), el('dd', {}, t('glossSirens'))),
      el('div', {}, el('dt', {}, t('amendment')), el('dd', {}, t('glossPort'))),
      el('div', {}, el('dt', {}, t('voyageSailHome')), el('dd', {}, t('glossHome'))));
  }

  function voyageView() {
    const c = conf(), pact = c.pact;
    const captain = profiles().find(p => p.id === (pact?.captain || c.current.settings.captain));
    const prow = captain ? row(captain.id) : null;
    const delayH = Math.round((+conf().current.settings.delay_hours || 24));
    const head = el('div', { class: 'fw-head' },
      el('div', {}, el('p', { class: 'fw-eyebrow' }, t('ulysses')),
        el('h1', { class: 'fw-h1' }, t('ulysses'))));
    if (!pact) return [head,
      el('section', { class: 'fw-voyage' }, ic('i-mast', 34),
        el('p', { class: 'fw-voyage-eyebrow' }, t('noPact')),
        el('h2', { class: 'fw-voyage-h' }, t('voyageCalm')),
        el('p', { class: 'fw-voyage-p' }, t('voyageCalmText')),
        el('div', { class: 'fw-btns' }, button(t('setSail'), () => {
          viewEl.querySelector('form')?.scrollIntoView({ block: 'center' });
          viewEl.querySelector('form select')?.focus();
        }, { gold: true })),
        doc.createRange().createContextualFragment(WAVE)),
      el('div', { class: 'fw-grid2' },
        panel(t('setSail'), voyageForm(null), hint(t('recoveryHelp'))),
        glossary())];
    const inWindow = prow?.pact_in_window === true;
    return [head,
      el('section', { class: 'fw-voyage' }, ic('i-mast', 34),
        el('p', { class: 'fw-voyage-eyebrow' }, t('voyageAtMast')),
        el('h2', { class: 'fw-voyage-h' },
          pact.type === 'ongoing' ? t('voyageOngoing') : t('voyageFixed', { date: epoch(pact.until) })),
        el('p', { class: 'fw-voyage-p' }, t('voyageAtMastText')),
        el('dl', { class: 'fw-voyage-facts' },
          el('div', {}, el('dt', {}, t('voyagePortCall')),
            el('dd', {}, t('voyageWindow', { day: t(pact.window_day), start: pact.window_start, stop: pact.window_stop }),
              ' ', chip(inWindow ? t('voyageOpen') : t('voyageClosed'), inWindow ? 'gold' : ''))),
          el('div', {}, el('dt', {}, t('voyageRelease')),
            el('dd', {}, +prow?.release_at ? t('releaseAt', { time: epoch(prow.release_at) }) : t('voyageNotRequested', { h: delayH }))),
          el('div', {}, el('dt', {}, t('voyageDelivered')),
            el('dd', {}, +prow?.mast_last_sync ? epoch(prow.mast_last_sync) : t('voyageNoSync')))),
        doc.createRange().createContextualFragment(WAVE)),
      el('div', { class: 'fw-grid2' },
        panel(t('voyageFrozen'),
          facts([[ t('captain'), captain?.name || t('none') ],
            [ t('schedules'), array(prow?.schedules).length ? String(array(prow.schedules).length) : t('none') ],
            [ t('devices'), captain ? t('devicesCount', { n: array(captain.devices).length }) : t('none') ]])),
        panel(t('voyageHands'), hint(t('voyageHandsText')),
          button(t('edit'), () => captain && editProfile(captain.id)),
          pact.type === 'until' ? voyageForm(pact) : null)),
      panel(t('voyageSailHome'), hint(t('voyageSailHomeText')),
        recoveryURL ? el('a', { class: 'fw-btn', href: recoveryURL, target: '_blank', rel: 'noopener' }, t('recovery'))
          : hint(t('recoveryUnavailable')),
        hint(t('recoveryHelp'))),
      glossary()];
  }

  // Unlabelled packages use their last segment: com.example.games -> G.
  const initial = (label, pkg) => ((label && label !== pkg ? label : String(pkg || '').split('.').pop()) || '?')[0].toUpperCase();

  // Package names are stripped: "familywarden-0.11.0-r12" -> "0.11.0-r12".
  function versions() {
    const plain = v => typeof v === 'string' ? v.replace(/^[a-z-]+?-(?=\d)/, '') : null;
    return { core: snapshot?.health?.core || null, backend: plain(snapshot?.version?.backend), ui: plain(snapshot?.version?.frontend) };
  }

  function versionFacts() {
    const v = versions();
    return [[ t('coreVersion'), v.core || t('unavailable') ], [ t('backendVersion'), v.backend || t('unavailable') ],
      [ t('uiVersion'), v.ui || t('unavailable') ]];
  }

  function administrationView() {
    if (draft?.kind === 'settings') return settingsEditor();
    const application = snapshot.status?.network_application;
    const health = snapshot.health;
    const dns = snapshot.dns;
    const dnsCard = panel(t('dns'),
      chip(dns?.credentials ? t('credentialsSet') : t('credentialsMissing'), dns?.credentials ? 'ok' : 'dim'),
      dns?.error ? el('p', { role: 'status', class: 'fw-error' }, dns.error) : null,
      dns?.pending ? hint(t('dnsPending')) : null,
      button(t('syncDNS'), () => mutate('configuration_dns_sync')),
      el('h4', {}, t('dnsVerified')),
      facts(Object.entries(dns?.profiles || {}).map(([id, p]) =>
        [profiles().find(v => v.id === id)?.name || id, p.hostname])));
    let username = '', password = '';
    const credentials = el('form', { onsubmit: event => {
      event.preventDefault();
      if (!event.target.reportValidity()) return;
      mutate('configuration_dns_auth', { username, password },
        { done: () => { username = ''; password = ''; } });
    } },
      el('h4', {}, t('credentials')),
      field(t('username'), '', v => { username = v; }, { required: true, autocomplete: 'username' }),
      field(t('password'), '', v => { password = v; }, { type: 'password', required: true, autocomplete: 'new-password' }),
      button(t('saveCredentials'), () => credentials.requestSubmit(), { small: true }));
    const phonesList = phones();
    const exportButton = button(t('export'), () => {
      const blob = new win.Blob([JSON.stringify(conf(), null, 2)], { type: 'application/json' });
      const url = win.URL.createObjectURL(blob);
      const anchor = el('a', { href: url, download: 'familywarden-configuration.json' });
      anchor.click(); win.URL.revokeObjectURL(url);
    });
    return [
      el('div', { class: 'fw-head' },
        el('div', {},
          el('p', { class: 'fw-eyebrow' }, t('title')),
          el('h1', { class: 'fw-h1' }, t('administration')),
          el('p', { class: 'fw-sub' }, t('adminSub')))),
      el('section', { class: 'fw-section', 'aria-labelledby': 'health-h' },
        el('h2', { class: 'fw-h2', id: 'health-h' }, t('health')),
        el('div', { class: 'fw-grid2' },
          panel(t('coreHealth'),
            chip(health?.running ? t('coreRunning') : t('coreStopped'), health?.running ? 'ok' : 'fault'),
            facts([...versionFacts(),
              [ t('configurationRevision'), el('code', {}, conf()?.revision || t('unavailable')) ]])),
          panel(t('application'),
            facts([[ t('application'), application?.state === 'acknowledged' ? t('acknowledged') : t('unavailable') ],
              ['ID', el('code', {}, application?.intent_id || t('unavailable'))],
              [ t('acknowledged'), epoch(application?.acknowledged_at) ]]),
            hint(t('notVerified'))),
          dnsCard,
          credentials)),
      el('section', { class: 'fw-section', 'aria-labelledby': 'people-h' },
        el('h2', { class: 'fw-h2', id: 'people-h' }, t('people')),
        el('div', { class: 'fw-panel', style: 'padding:6px 20px' },
          el('ul', { class: 'fw-list' }, phonesList.map((d, i) => el('li', { class: 'fw-item' },
            el('div', { class: `fw-av fw-av-${i % 5}` }, (d.name || '?')[0].toUpperCase()),
            el('div', { class: 'main' },
              el('p', { class: 't' }, d.name || d.id, ' ',
                d.administrator === '1' ? chip(t('role'), 'gold') : null,
                d.profile === conf()?.pact?.captain ? chip(t('captainChip'), 'gold') : null),
              el('p', { class: 's' }, [
                profiles().find(p => p.id === d.profile)?.name || t('unavailable'),
                String(d.approves || '').split(/\s+/).filter(Boolean).length
                  ? t('scopes') + ': ' + String(d.approves || '').split(/\s+/).filter(Boolean)
                      .map(id => profiles().find(p => p.id === id)?.name || id).join(', ')
                  : null,
              ].filter(Boolean).join(' \u00b7 '))),
            +d.seen ? chip(t('lastFetch', { time: timeHm(d.seen) }), 'dim') : null))),
          el('p', { class: 'fw-hint', style: 'padding-bottom:12px' }, t('peopleHelp')))),
      el('section', { class: 'fw-section', 'aria-labelledby': 'settings-h' },
        el('h2', { class: 'fw-h2', id: 'settings-h' }, t('settingsTitle')),
        el('div', { class: 'fw-panel' }, button(t('edit'), editSettings))),
      el('div', { class: 'fw-grid2' },
        panel(t('diagnostics'),
          facts([...versionFacts(),
            [ t('configurationRevision'), el('code', {}, conf()?.revision || t('unavailable')) ],
            [ t('hostAdapter'), t('hostLuCI') ],
            [ t('day'), snapshot.report?.day || t('unavailable') ]])),
        panel(t('export'), exportButton, hint(t('exportHelp'))))
    ];
  }

  function legacyView() {
    const approve = button(t('import'), () => mutate('configuration_import', { revision: conf().revision }), { primary: true, disabled: true });
    return [
      el('div', { class: 'fw-head' },
        el('div', {}, el('p', { class: 'fw-eyebrow' }, t('title')),
          el('h1', { class: 'fw-h1' }, t('title')))),
      el('div', { class: 'fw-empty' }, hint(t('legacy')), hint(t('importHelp')),
        field(t('importWarning'), false, v => { approve.dataset.locked = String(!v); controls(); }, { type: 'checkbox' }),
        approve)];
  }

  function render() {
    if (!snapshot || stopped) return;
    renderRail();
    if (view === 'requests') {
      let panelNode = viewEl.querySelector('.fwx');
      if (!panelNode || !requestsMount) {
        requestsMount?.stop();
        panelNode = el('div');
        requestsMount = mountRequests(panelNode, { lang, call, interval,
          avatar: id => { const i = profiles().findIndex(p => p.id === id); return i < 0 ? null : i; } });
      }
      viewEl.replaceChildren(...[banner(), panelNode].filter(child => child != null));
      controls();
      return;
    }
    requestsMount?.stop(); requestsMount = null;
    const profile = (draft?.kind === 'profile' ? draft.current.profiles : profiles()).find(p => p.id === selected);
    let content;
    if (view === 'family') content = snapshot.export.owner !== 'core' ? legacyView() : familyView();
    else if (view === 'devices') content = profile ? childView(profile) : phonesView();
    else if (view === 'detail' || view === 'history' || view === 'rules') content = childView(profile);
    else if (view === 'ulysses') content = voyageView();
    else content = administrationView();
    viewEl.replaceChildren(...[banner(), content].flat(Infinity).filter(child => child != null));
    controls();
  }

  async function refresh(force = true) {
    if (busy || reading || stopped) return;
    clearTimeout(timer);
    // Never repaint active drafts or disable a field during background polling.
    // Explicit refresh retains the form; CAS still protects its loaded revision.
    if (!force && (draft || viewEditing)) {
      if (root.isConnected) timer = setTimeout(() => refresh(false), interval);
      return;
    }
    reading = true; controls();
    try {
      const exported = validateExport(await request('configuration_export'));
      let next = { export: exported, version: await request('version').catch(() => null) }, missing = [];
      if (exported.owner === 'core') {
        const keys = ['report', 'devices', 'dns', 'usage', 'status', 'health', 'requests', 'new_apps'];
        const results = await Promise.allSettled(keys.map(key => request({
          report: 'configuration_report', devices: 'devices', dns: 'configuration_dns_status',
          usage: 'usage', status: 'configuration_status', health: 'core_status',
          requests: 'requests', new_apps: 'new_apps' }[key])));
        for (let i = 0; i < keys.length; i++) {
          if (results[i].status === 'fulfilled') next[keys[i]] = results[i].value;
          else missing.push(keys[i] + ': ' + results[i].reason.message);
        }
        if (!object(next.report?.profiles) || !object(next.report?.configuration) || !next.status ||
            (next.devices && (!Array.isArray(next.devices.devices) || !Array.isArray(next.devices.pending))) ||
            (next.usage && !Array.isArray(next.usage.profiles)) ||
            (next.dns && (next.dns.format !== 'dns-state-v1' || !object(next.dns.profiles) || typeof next.dns.pending !== 'boolean')) ||
            (next.requests && !Array.isArray(next.requests.requests)) ||
            (next.new_apps && !Array.isArray(next.new_apps.profiles))) throw new Error(t('invalid'));
      }
      if (stopped || !root.isConnected) return;
      snapshot = next; fresh = true;
      if (confirmation && !exported.pending && exported.configuration.revision !== confirmation.revision) {
        confirmation = null; message(t('rolledBack'));
      }
      status.textContent = t('updated', { time: new Date().toLocaleTimeString(lang) }) +
        (versions().ui ? ' · ' + t('uiVersion') + ' ' + versions().ui : '') +
        (missing.length ? ' · ' + t('partial', { error: missing.join('; ') }) : '');
      render();
    } catch (error) {
      if (!stopped && root.isConnected) {
        fresh = false;
        status.textContent = t('stale') + ' ' + (error.message || error);
        requestsMount?.stop(); requestsMount = null;
      }
    } finally {
      reading = false; controls();
      if (!stopped && root.isConnected) timer = setTimeout(() => refresh(false), interval);
    }
  }

  refresh();
  return { refresh, setTheme, stop: () => { stopped = true; fresh = false; colorPreference?.removeEventListener('change', updateTheme); clearTimeout(timer); requestsMount?.stop(); requestsMount = null; controls(); } };
}
