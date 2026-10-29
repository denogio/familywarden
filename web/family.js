// FamilyWarden shared administrator UI: extra-time requests and today's usage (A20).
// Plain ES module, no framework and no LuCI dependency: a platform wrapper passes
// call(method, params) -> Promise<object> talking to the core API. Own EN/DA strings;
// styles are scoped under .fwa and use the host theme's colors where available.
// The legacy .fwx hook stays on the root for existing consumers and screenshots.

// Only bounded PNG data is ever placed in an image URL; failed loads keep initials.
export function appIcon(icon, label, pkg) {
  const tile = document.createElement('span');
  tile.className = 'fw-app-icon';
  tile.setAttribute('aria-hidden', 'true');
  const fallback = ((label && label !== pkg ? label : String(pkg || '').split('.').pop()) || '?')[0].toUpperCase();
  tile.textContent = fallback;
  if (typeof icon === 'string' && icon.length <= 54616 && /^iVBORw0KGgo[A-Za-z0-9+/]*={0,2}$/.test(icon)) {
    const image = document.createElement('img');
    image.alt = '';
    image.addEventListener('error', () => { tile.classList.remove('has-image'); tile.textContent = fallback; });
    tile.classList.add('has-image');
    image.src = 'data:image/png;base64,' + icon;
    tile.replaceChildren(image);
  }
  return tile;
}

// One loader per workspace, also passed to Requests. Only rendered identities are
// fetched. Every bounded page is drained, including pages containing missing icons.
export function iconLoader(call) {
  const cache = new Map(), pending = new Map();
  let queued = false;
  async function flush() {
    queued = false;
    const groups = [...pending.entries()]; pending.clear();
    for (const [profile, packages] of groups) {
      const names = [...packages.keys()];
      for (let start = 0; start < names.length; start += 32) {
        const requested = names.slice(start, start + 32);
        let offset = 0;
        try {
          while (offset < requested.length) {
            const reply = await call('inventory_icons', { profile, packages: requested, offset });
            if (reply?.error || !Number.isInteger(reply?.next) || reply.next <= offset || reply.next > requested.length) break;
            for (let i = offset; i < reply.next; i++) {
              const pkg = requested[i], icon = reply.icons?.[pkg];
              cache.set(profile + '|' + pkg, { icon, at: Date.now() });
              for (const { tile, label } of packages.get(pkg)) {
                const replacement = appIcon(icon, label, pkg);
                tile.className = replacement.className;
                tile.replaceChildren(...replacement.childNodes);
                // The error handler belongs to replacement; restore this tile too.
                tile.querySelector('img')?.addEventListener('error', () => {
                  const fallback = appIcon(null, label, pkg);
                  tile.className = fallback.className; tile.replaceChildren(...fallback.childNodes);
                });
              }
            }
            offset = reply.next;
          }
        } catch (_) { /* Optional assets never interrupt administration. */ }
      }
    }
  }
  return (icon, label, pkg, profile) => {
    const key = profile + '|' + pkg, saved = cache.get(key);
    if (saved && Date.now() - saved.at < 5 * 60 * 1000) return appIcon(saved.icon, label, pkg);
    // Keep cached artwork visible while a stale observation is renewed.
    const tile = appIcon(saved ? saved.icon : icon, label, pkg);
    if (!profile || !pkg || pkg === 'total') return tile;
    if (!pending.has(profile)) pending.set(profile, new Map());
    const group = pending.get(profile);
    if (!group.has(pkg)) group.set(pkg, []);
    group.get(pkg).push({ tile, label });
    if (!queued) { queued = true; queueMicrotask(flush); }
    return tile;
  };
}

const STRINGS = {
  en: {
    eyebrow: 'Approvals', title: 'Requests', intro: 'Extra time and new apps from the children. The first parent to answer decides; the others see who did. Phones report usage when they sync; it is information, not enforcement.',
    pending: 'Waiting', none: 'No requests waiting.', approve: 'Approve', decline: 'Decline',
    minutes: 'Minutes', range: 'Approve 1–{max} minutes.', expires: 'expires {time}', screen: 'Screen time',
    recent: 'Recent decisions', noRecent: 'Nothing answered in the last day.',
    askTotal: '+{n} min total screen time', askApp: '+{n} min for {app}', askNewApp: 'new app: {app}', paused: 'paused until decided',
    gotTotal: '+{n} min total', gotApp: '+{n} min {app}', chipApproved: 'Approved', chipDenied: 'Declined', chipExpired: 'Expired',
    approved: 'Approved {granted} of {minutes} min by {by}', denied: 'Declined by {by}', expired: 'Expired unanswered',
    used: '{used} of {granted} min used', delivered: 'delivered to the phone', notDelivered: 'not confirmed by the phone yet',
    today: 'Today', week: 'Last 7 days', noUsage: 'No usage reported today.', incomplete: 'incomplete', min: '{n} min',
    phones: 'Phones', apps: 'Apps', error: 'Could not reach the FamilyWarden core: {error}', failed: 'The router refused: {error}',
    updated: 'Updated {time}', invalidData: 'Invalid reply from FamilyWarden.',
    stale: 'Showing previously received data. Actions are unavailable until refresh succeeds.',
    noReport: 'No usage report for this day', coverageUnknown: 'Reporting coverage unavailable',
    allow: 'Allow', keepPaused: 'Keep paused',
    allowedApps: 'Allowed new apps', remove: 'Remove', allowedBy: 'allowed by {by}', frozen: 'Frozen by a pact: apps can only be removed.'
  },
  da: {
    eyebrow: 'Godkendelser', title: 'Anmodninger', intro: 'Ekstra tid og nye apps fra børnene. Den første forælder, der svarer, bestemmer; de andre kan se hvem. Telefonerne rapporterer forbrug, når de synkroniserer; det er information, ikke håndhævelse.',
    pending: 'Venter', none: 'Ingen ventende anmodninger.', approve: 'Godkend', decline: 'Afslå',
    minutes: 'Minutter', range: 'Godkend 1–{max} minutter.', expires: 'udløber {time}', screen: 'Skærmtid',
    recent: 'Seneste beslutninger', noRecent: 'Intet besvaret det seneste døgn.',
    askTotal: '+{n} min. samlet skærmtid', askApp: '+{n} min. til {app}', askNewApp: 'ny app: {app}', paused: 'på pause, indtil der er taget stilling',
    gotTotal: '+{n} min. samlet', gotApp: '+{n} min. {app}', chipApproved: 'Godkendt', chipDenied: 'Afslået', chipExpired: 'Udløbet',
    approved: '{granted} af {minutes} min. godkendt af {by}', denied: 'Afslået af {by}', expired: 'Udløbet uden svar',
    used: '{used} af {granted} min. brugt', delivered: 'modtaget af telefonen', notDelivered: 'endnu ikke bekræftet af telefonen',
    today: 'I dag', week: 'Seneste 7 dage', noUsage: 'Intet forbrug rapporteret i dag.', incomplete: 'ufuldstændig', min: '{n} min.',
    phones: 'Telefoner', apps: 'Apps', error: 'Kunne ikke nå FamilyWarden-kernen: {error}', failed: 'Routeren afviste: {error}',
    updated: 'Opdateret {time}', invalidData: 'Ugyldigt svar fra FamilyWarden.',
    stale: 'Tidligere modtagne data vises. Handlinger er utilgængelige, indtil opdateringen lykkes.',
    noReport: 'Ingen forbrugsrapport for denne dag', coverageUnknown: 'Rapportdækning er ukendt',
    allow: 'Tillad', keepPaused: 'Behold på pause',
    allowedApps: 'Tilladte nye apps', remove: 'Fjern', allowedBy: 'tilladt af {by}', frozen: 'Låst af en pagt: apps kan kun fjernes.'
  }
};

export function strings(lang) {
  const base = String(lang || 'en').toLowerCase().split(/[-_]/)[0];
  const table = STRINGS[base] || STRINGS.en;
  return (key, vars = {}) => (table[key] || STRINGS.en[key] || key).replace(/\{(\w+)\}/g, (_, k) => String(vars[k] ?? ''));
}

const minutes = (seconds) => Math.floor((Number(seconds) || 0) / 60);

// Pure view model: what the page shows, from the core's `requests` and `usage` replies.
export function viewModel(requests, usage, nowEpoch, newApps) {
  const list = Array.isArray(requests?.requests) ? requests.requests : [];
  const pending = list.filter((r) => r.state === 'pending' && r.expires > nowEpoch)
    .map((r) => ({ id: r.request_id, profileId: r.profile, profile: r.profile_name || r.profile, target: r.target, label: r.label || '', icon: r.icon,
      minutes: r.minutes, expires: r.expires }))
    .sort((a, b) => a.expires - b.expires);
  const recent = list.filter((r) => r.state !== 'pending' && (r.answered_at || r.received) >= nowEpoch - 86400)
    .map((r) => ({ id: r.request_id, profileId: r.profile, profile: r.profile_name || r.profile, target: r.target, label: r.label || '', icon: r.icon,
      state: r.state, minutes: r.minutes, granted: r.granted || 0, by: r.answered_by || '',
      used: r.used == null ? null : minutes(r.used), delivered: r.delivered === true }));
  const profiles = (Array.isArray(usage?.profiles) ? usage.profiles : []).map((p) => ({
    id: p.id, name: p.name || p.id, total: minutes(p.total),
    devices: (p.devices || []).map((d) => ({ name: d.name, total: minutes(d.total), incomplete: !!d.incomplete })),
    apps: (p.apps || []).map((a) => ({ name: a.label || a.package, total: minutes(a.seconds) })),
    week: (p.week || []).map((d) => ({ day: d.day, total: minutes(d.total),
      reported: typeof d.reported === 'boolean' ? d.reported : null }))
  }));
  const apps = (Array.isArray(newApps?.profiles) ? newApps.profiles : []).map((p) => ({
    id: p.id, name: p.name || p.id, frozen: !!p.frozen,
    pending: (p.pending || []).map((a) => ({ pkg: a.package, label: a.label || a.package, icon: a.icon })),
    approved: (p.approved || []).map((a) => ({ pkg: a.package, label: a.label || a.package, icon: a.icon, by: a.by || '' }))
  })).filter((p) => p.pending.length || p.approved.length);
  return { pending, recent, profiles, apps, day: usage?.day || '' };
}

// Layout comes from the workspace's .fwa components (control.css); only Requests-specific parts live here.
const CSS = `
.fwx{--fwx-bar:var(--gold,#c9a227)}
.fwx,.fwx .fwx-body{display:flex;flex-direction:column;gap:22px;min-width:0}
.fwx .fwx-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(min(320px,100%),1fr));gap:12px}
.fwx .fwx-big{margin:0;font-family:var(--serif);font-size:1.75rem;font-weight:600;line-height:1.2}
.fwx .fwx-head{display:flex;align-items:center;gap:10px}
.fwx .fwx-row{display:flex;justify-content:space-between;gap:12px;font-size:.875rem}
.fwx .fwx-row.fwx-muted{color:var(--muted)}
.fwx .fw-attn-row .fw-input{width:5.5rem;min-width:0;min-height:34px}
.fwx .fwx-week{display:flex;align-items:flex-end;gap:6px;height:64px}
.fwx .fwx-week span{flex:1;background:var(--fwx-bar);border-radius:3px 3px 0 0;min-height:2px}
.fwx .fwx-week .fwx-gap{background:transparent;min-height:8px;border-bottom:1px dashed var(--muted)}
.fwx .fwx-week .fwx-coverage-unknown{background:repeating-linear-gradient(135deg,var(--fwx-bar),var(--fwx-bar) 4px,transparent 4px,transparent 8px)}`;

export function mount(root, { call, lang, now = () => Math.floor(Date.now() / 1000), interval = 15000, avatar, icons = iconLoader(call) }) {
  const appIcon = icons;
  const t = strings(lang);
  const doc = root.ownerDocument;
  const el = (tag, attrs = {}, ...kids) => {
    const n = doc.createElement(tag);
    for (const [k, v] of Object.entries(attrs)) {
      if (k.startsWith('on')) n.addEventListener(k.slice(2), v); else n.setAttribute(k, v);
    }
    for (const k of kids.flat()) if (k != null) n.append(k instanceof Node ? k : String(k));
    return n;
  };
  const Node = doc.defaultView.Node;
  const time = (epoch) => new Date(epoch * 1000).toLocaleTimeString(lang, { hour: '2-digit', minute: '2-digit' });
  const label = (r) => r.target === 'total' ? t('screen') : (r.label || r.target);
  // Avatar colour: the host's profile order when given, else a stable hash of the id.
  const av = (id, name) => {
    const i = avatar?.(id) ?? [...String(id)].reduce((h, c) => h + c.charCodeAt(0), 0);
    return el('span', { class: `fw-av fw-av-${Math.max(0, i) % 5}`, 'aria-hidden': 'true' }, String(name || id || '?')[0].toUpperCase());
  };
  const section = (title, ...kids) => el('section', { class: 'fw-section' }, el('h2', { class: 'fw-h2' }, title), ...kids);
  const style = el('style', {}, CSS);
  const status = el('p', { class: 'fw-hint', 'aria-live': 'polite' });
  const notice = el('p', { class: 'fw-error', role: 'alert' });
  const body = el('div', { class: 'fwx-body' });
  root.classList.add('fwx');
  root.replaceChildren(style, el('div', { class: 'fw-head' }, el('div', {}, el('p', { class: 'fw-eyebrow' }, t('eyebrow')),
    el('h1', { class: 'fw-h1' }, t('title')), el('p', { class: 'fw-sub' }, t('intro')), status)), notice, body);
  let busy = false, reading = false, fresh = false, stopped = false;
  let updatedAt = null;
  let timer = null;
  const amounts = new Map();

  function controls() {
    root.setAttribute('aria-busy', String(!stopped && (busy || reading)));
    for (const node of body.querySelectorAll('button, input')) node.disabled = busy || reading || !fresh;
  }

  async function mutate(method, params) {
    if (busy || reading || !fresh || stopped) return;
    busy = true;
    notice.textContent = '';
    controls();
    try {
      const reply = await call(method, params);
      if (!stopped && reply?.error) notice.textContent = t('failed', { error: reply.error });
    } catch (e) {
      if (!stopped) notice.textContent = t('error', { error: e?.message || e });
    } finally {
      busy = false;
      if (!stopped) await refresh();
    }
  }

  function decideApp(profile, pkg, action) {
    return mutate('app_decision', { profile, package: pkg, action });
  }

  const row = (who, what, why, ...actions) => el('div', { class: 'fw-attn-row fwx-card' },
    el('div', { class: 'who' }, who, el('div', { style: 'min-width:0' }, el('p', { class: 'what' }, what), el('p', { class: 'why' }, why))),
    el('div', { class: 'fw-btns' }, ...actions));

  function answer(r, approve, input) {
    const value = approve ? Number(input.value) : 0;
    if (approve && !(Number.isInteger(value) && value >= 1 && value <= r.minutes)) { input.focus(); return; }
    return mutate('request_answer', { request_id: r.id, approve, minutes: value });
  }

  function pendingRow(r) {
    const input = el('input', { class: 'fw-input', type: 'number', min: '1', max: String(r.minutes),
      value: amounts.get(r.id) ?? String(r.minutes), 'aria-label': t('minutes') });
    input.addEventListener('input', () => amounts.set(r.id, input.value));
    const ask = r.target === 'total' ? t('askTotal', { n: r.minutes }) : t('askApp', { n: r.minutes, app: label(r) });
    return row(r.target === 'total' ? av(r.profileId, r.profile) : appIcon(r.icon, r.label, r.target, r.profileId), `${r.profile} · ${ask}`,
      `${t('expires', { time: time(r.expires) })} · ${t('range', { max: r.minutes })}`, input,
      el('button', { class: 'fw-btn small primary', onclick: () => answer(r, true, input) }, t('approve')),
      el('button', { class: 'fw-btn small', onclick: () => answer(r, false, input) }, t('decline')));
  }

  function newAppRow(p, a) {
    return row(appIcon(a.icon, a.label, a.pkg, p.id), `${p.name} · ${t('askNewApp', { app: a.label })}`, p.frozen ? t('frozen') : t('paused'),
      p.frozen ? null : el('button', { class: 'fw-btn small primary', onclick: () => decideApp(p.id, a.pkg, 'approve') }, t('allow')),
      el('button', { class: 'fw-btn small', onclick: () => decideApp(p.id, a.pkg, 'deny') }, t('keepPaused')));
  }

  const item = (who, title, detail, end) => el('li', { class: 'fw-item' }, who,
    el('div', { class: 'main' }, el('p', { class: 't' }, title), el('p', { class: 's' }, detail)), end);
  const list = (items) => el('div', { class: 'fw-panel', style: 'padding:6px 20px' }, el('ul', { class: 'fw-list' }, items));

  function recentItem(r) {
    const what = r.state === 'approved' ? t('approved', { granted: r.granted, minutes: r.minutes, by: r.by })
      : r.state === 'denied' ? t('denied', { by: r.by }) : t('expired');
    const extra = r.state !== 'approved' ? '' : r.used != null ? t('used', { used: r.used, granted: r.granted })
      : t(r.delivered ? 'delivered' : 'notDelivered');
    const got = r.target === 'total' ? t('gotTotal', { n: r.minutes }) : t('gotApp', { n: r.minutes, app: label(r) });
    const chip = r.state === 'approved' ? ['ok', 'chipApproved'] : r.state === 'denied' ? ['', 'chipDenied'] : ['dim', 'chipExpired'];
    return item(r.target === 'total' ? av(r.profileId, r.profile) : appIcon(r.icon, r.label, r.target, r.profileId), `${r.profile} · ${got}`, [what, extra].filter(Boolean).join(' · '),
      el('span', { class: `fw-chip ${chip[0]}`.trim() }, t(chip[1])));
  }

  function usageCard(p) {
    const max = Math.max(1, ...p.week.filter((d) => d.reported !== false).map((d) => d.total));
    return el('div', { class: 'fw-panel fwx-card' },
      el('div', { class: 'fwx-head' }, av(p.id, p.name), el('strong', {}, p.name)),
      el('p', { class: 'fwx-big' }, p.devices.length ? t('min', { n: p.total }) : '—'),
      p.devices.length ? null : el('p', { class: 'fw-hint' }, t('noUsage')),
      p.devices.length ? el('div', {}, p.devices.map((d) => el('div', { class: 'fwx-row' }, el('span', {}, d.name),
        el('span', {}, t('min', { n: d.total }) + (d.incomplete ? ` · ${t('incomplete')}` : ''))))) : null,
      p.apps.length ? el('div', {}, p.apps.slice(0, 5).map((a) => el('div', { class: 'fwx-row fwx-muted' }, el('span', {}, a.name), el('span', {}, t('min', { n: a.total }))))) : null,
      el('div', { class: 'fwx-week', role: 'group', 'aria-label': t('week') }, p.week.map((d) => {
        const measured = d.reported === true || (d.reported == null && d.total > 0);
        const detail = measured ? t('min', { n: d.total }) : t(d.reported === false ? 'noReport' : 'coverageUnknown');
        const title = `${d.day}: ${detail}` + (measured && d.reported == null ? ` · ${t('coverageUnknown')}` : '');
        return el('span', { class: !measured ? 'fwx-gap' : d.reported == null ? 'fwx-coverage-unknown' : '',
          style: `height:${measured ? Math.round((d.total / max) * 100) : 0}%`, title,
          role: 'img', 'aria-label': title });
      })));
  }

  async function refresh(force = true) {
    if (busy || reading || stopped) return;
    clearTimeout(timer);
    // Polling must not discard the amount currently being edited.
    if (!force && body.contains(doc.activeElement) && doc.activeElement.tagName === 'INPUT') {
      if (root.isConnected) timer = setTimeout(() => refresh(false), interval);
      return;
    }
    reading = true;
    controls();
    try {
      const [requests, usage, newApps] = await Promise.all([call('requests', {}), call('usage', {}), call('new_apps', {})]);
      if (stopped) return;
      if (requests?.error || usage?.error || newApps?.error) throw new Error(requests?.error || usage?.error || newApps?.error);
      if (!Array.isArray(requests?.requests) || !Array.isArray(usage?.profiles) || !Array.isArray(newApps?.profiles))
        throw new Error(t('invalidData'));
      const vm = viewModel(requests, usage, now(), newApps);
      const pendingIds = new Set(vm.pending.map((r) => r.id));
      for (const id of amounts.keys()) if (!pendingIds.has(id)) amounts.delete(id);
      const waiting = [...vm.pending.map(pendingRow), ...vm.apps.flatMap((p) => p.pending.map((a) => newAppRow(p, a)))];
      const allowed = vm.apps.flatMap((p) => p.approved.map((a) => item(appIcon(a.icon, a.label, a.pkg, p.id), `${p.name} · ${a.label}`, t('allowedBy', { by: a.by }),
        el('button', { class: 'fw-btn small', onclick: () => decideApp(p.id, a.pkg, 'revoke') }, t('remove')))));
      body.replaceChildren(...[
        section(t('pending'), waiting.length ? el('div', { class: 'fw-attn' }, waiting) : el('div', { class: 'fw-empty' }, el('p', { style: 'margin:0' }, t('none')))),
        section(t('recent'), vm.recent.length ? list(vm.recent.map(recentItem)) : el('p', { class: 'fw-hint' }, t('noRecent'))),
        allowed.length ? section(t('allowedApps'), list(allowed)) : null,
        vm.profiles.length ? section(`${t('today')} · ${vm.day}`, el('div', { class: 'fwx-grid' }, vm.profiles.map(usageCard))) : null]
        .filter(Boolean));
      fresh = true;
      updatedAt = now();
      status.textContent = t('updated', { time: time(updatedAt) });
      status.className = 'fw-hint';
    } catch (e) {
      if (!stopped) {
        fresh = false;
        status.textContent = t('error', { error: e?.message || e }) +
          (updatedAt == null ? '' : ` ${t('stale')} ${t('updated', { time: time(updatedAt) })}`);
        status.className = 'fw-error';
      }
    } finally {
      reading = false;
      controls();
      if (!stopped && root.isConnected) timer = setTimeout(() => refresh(false), interval);
    }
  }
  refresh();
  return { refresh, stop: () => { stopped = true; fresh = false; clearTimeout(timer); controls(); } };
}
