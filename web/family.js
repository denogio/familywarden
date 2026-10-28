// FamilyWarden shared administrator UI: extra-time requests and today's usage (A20).
// Plain ES module, no framework and no LuCI dependency: a platform wrapper passes
// call(method, params) -> Promise<object> talking to the core API. Own EN/DA strings;
// styles are scoped under .fwa and use the host theme's colors where available.
// The legacy .fwx hook stays on the root for existing consumers and screenshots.

const STRINGS = {
  en: {
    title: 'Requests and screen time', intro: 'Answer extra-time requests and see what each child has used today. Phones report usage when they sync; it is information, not enforcement.',
    pending: 'Waiting for an answer', none: 'No requests waiting.', approve: 'Approve', decline: 'Decline',
    minutes: 'Minutes', range: 'Approve 1–{max} minutes.', expires: 'expires {time}', screen: 'Screen time',
    recent: 'Answered in the last day', noRecent: 'Nothing answered recently.',
    approved: 'Approved {granted} of {minutes} min by {by}', denied: 'Declined by {by}', expired: 'Expired unanswered',
    used: '{used} of {granted} min used', delivered: 'delivered to the phone', notDelivered: 'not confirmed by the phone yet',
    today: 'Today', week: 'Last 7 days', noUsage: 'No usage reported today.', incomplete: 'incomplete', min: '{n} min',
    phones: 'Phones', apps: 'Apps', error: 'Could not reach the FamilyWarden core: {error}', failed: 'The router refused: {error}',
    updated: 'Updated {time}', invalidData: 'Invalid reply from FamilyWarden.',
    stale: 'Showing previously received data. Actions are unavailable until refresh succeeds.',
    noReport: 'No usage report for this day', coverageUnknown: 'Reporting coverage unavailable',
    newApps: 'New apps waiting', noNewApps: 'No new apps waiting.', allow: 'Allow', keepPaused: 'Keep paused',
    allowedApps: 'Allowed here', remove: 'Remove', allowedBy: 'allowed by {by}', frozen: 'Frozen by a pact: apps can only be removed.'
  },
  da: {
    title: 'Anmodninger og skærmtid', intro: 'Besvar anmodninger om ekstra tid, og se hvad hvert barn har brugt i dag. Telefonerne rapporterer forbrug, når de synkroniserer; det er information, ikke håndhævelse.',
    pending: 'Venter på svar', none: 'Ingen ventende anmodninger.', approve: 'Godkend', decline: 'Afslå',
    minutes: 'Minutter', range: 'Godkend 1–{max} minutter.', expires: 'udløber {time}', screen: 'Skærmtid',
    recent: 'Besvaret det seneste døgn', noRecent: 'Intet besvaret for nylig.',
    approved: '{granted} af {minutes} min. godkendt af {by}', denied: 'Afslået af {by}', expired: 'Udløbet uden svar',
    used: '{used} af {granted} min. brugt', delivered: 'modtaget af telefonen', notDelivered: 'endnu ikke bekræftet af telefonen',
    today: 'I dag', week: 'Seneste 7 dage', noUsage: 'Intet forbrug rapporteret i dag.', incomplete: 'ufuldstændig', min: '{n} min.',
    phones: 'Telefoner', apps: 'Apps', error: 'Kunne ikke nå FamilyWarden-kernen: {error}', failed: 'Routeren afviste: {error}',
    updated: 'Opdateret {time}', invalidData: 'Ugyldigt svar fra FamilyWarden.',
    stale: 'Tidligere modtagne data vises. Handlinger er utilgængelige, indtil opdateringen lykkes.',
    noReport: 'Ingen forbrugsrapport for denne dag', coverageUnknown: 'Rapportdækning er ukendt',
    newApps: 'Nye apps venter', noNewApps: 'Ingen nye apps venter.', allow: 'Tillad', keepPaused: 'Behold på pause',
    allowedApps: 'Tilladt her', remove: 'Fjern', allowedBy: 'tilladt af {by}', frozen: 'Låst af en pagt: apps kan kun fjernes.'
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
    .map((r) => ({ id: r.request_id, profile: r.profile_name || r.profile, target: r.target, label: r.label || '',
      minutes: r.minutes, expires: r.expires }))
    .sort((a, b) => a.expires - b.expires);
  const recent = list.filter((r) => r.state !== 'pending' && (r.answered_at || r.received) >= nowEpoch - 86400)
    .map((r) => ({ id: r.request_id, profile: r.profile_name || r.profile, target: r.target, label: r.label || '',
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
    pending: (p.pending || []).map((a) => ({ pkg: a.package, label: a.label || a.package })),
    approved: (p.approved || []).map((a) => ({ pkg: a.package, label: a.label || a.package, by: a.by || '' }))
  })).filter((p) => p.pending.length || p.approved.length);
  return { pending, recent, profiles, apps, day: usage?.day || '' };
}

const CSS = `
.fwx{--fwx-fg:var(--ink,#0F1B27);--fwx-muted:var(--muted,#4A5D70);--fwx-line:var(--line,#D5DDE5);
 --fwx-card:var(--surface,#FFFFFF);--fwx-accent:#1b2a3a;--fwx-bar:#c9a227;color:var(--fwx-fg)}
.fwx h3{margin:1.2em 0 .5em}.fwx .fwx-intro{color:var(--fwx-muted);max-width:52em}
.fwx .fwx-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(min(320px,100%),1fr));gap:12px}
.fwx .fwx-card{border:1px solid var(--fwx-line);border-radius:12px;padding:14px 16px;background:var(--fwx-card)}
.fwx .fwx-big{font-size:1.6em;font-weight:600;margin:.1em 0}.fwx .fwx-muted{color:var(--fwx-muted);font-size:.92em}
.fwx .fwx-row{display:flex;justify-content:space-between;gap:12px;padding:2px 0}
.fwx .fwx-actions{display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin-top:10px}
.fwx input[type=number]{width:6em}.fwx .fwx-week{display:flex;align-items:flex-end;gap:6px;height:64px;margin-top:8px}
.fwx .fwx-week span{flex:1;background:var(--fwx-bar);border-radius:3px 3px 0 0;min-height:2px}
.fwx .fwx-week .fwx-gap{background:transparent;min-height:8px;border-bottom:1px dashed var(--fwx-muted)}
.fwx .fwx-week .fwx-coverage-unknown{background:repeating-linear-gradient(135deg,var(--fwx-bar),var(--fwx-bar) 4px,transparent 4px,transparent 8px)}
.fwx .fwx-error{color:#b3261e}`;

export function mount(root, { call, lang, now = () => Math.floor(Date.now() / 1000), interval = 15000 }) {
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
  const style = el('style', {}, CSS);
  const status = el('p', { class: 'fwx-muted', 'aria-live': 'polite' });
  const notice = el('p', { class: 'fwx-error', role: 'alert' });
  const body = el('div');
  root.classList.add('fwx');
  root.replaceChildren(style, el('h2', {}, t('title')), el('p', { class: 'fwx-intro' }, t('intro')), status, notice, body);
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

  function appsCard(p) {
    return el('div', { class: 'fwx-card' }, el('strong', {}, p.name),
      p.frozen ? el('div', { class: 'fwx-muted' }, t('frozen')) : null,
      p.pending.map((a) => el('div', { class: 'fwx-row' }, el('span', {}, a.label),
        el('span', { class: 'fwx-actions' },
          p.frozen ? null : el('button', { class: 'cbi-button cbi-button-positive', onclick: () => decideApp(p.id, a.pkg, 'approve') }, t('allow')),
          el('button', { class: 'cbi-button', onclick: () => decideApp(p.id, a.pkg, 'deny') }, t('keepPaused'))))),
      p.approved.length ? el('div', { class: 'fwx-muted' }, t('allowedApps')) : null,
      p.approved.map((a) => el('div', { class: 'fwx-row' }, el('span', {}, `${a.label} · ${t('allowedBy', { by: a.by })}`),
        el('button', { class: 'cbi-button cbi-button-negative', onclick: () => decideApp(p.id, a.pkg, 'revoke') }, t('remove')))));
  }

  function answer(r, approve, input) {
    const value = approve ? Number(input.value) : 0;
    if (approve && !(Number.isInteger(value) && value >= 1 && value <= r.minutes)) { input.focus(); return; }
    return mutate('request_answer', { request_id: r.id, approve, minutes: value });
  }

  function pendingCard(r) {
    const input = el('input', { type: 'number', min: '1', max: String(r.minutes),
      value: amounts.get(r.id) ?? String(r.minutes), 'aria-label': t('minutes') });
    input.addEventListener('input', () => amounts.set(r.id, input.value));
    return el('div', { class: 'fwx-card' },
      el('div', { class: 'fwx-row' }, el('strong', {}, r.profile), el('span', { class: 'fwx-muted' }, t('expires', { time: time(r.expires) }))),
      el('div', { class: 'fwx-big' }, `${label(r)} · ${t('min', { n: r.minutes })}`),
      el('div', { class: 'fwx-actions' }, input,
        el('button', { class: 'cbi-button cbi-button-positive', onclick: () => answer(r, true, input) }, t('approve')),
        el('button', { class: 'cbi-button cbi-button-negative', onclick: () => answer(r, false, input) }, t('decline'))),
      el('div', { class: 'fwx-muted' }, t('range', { max: r.minutes })));
  }

  function recentRow(r) {
    const what = r.state === 'approved' ? t('approved', { granted: r.granted, minutes: r.minutes, by: r.by })
      : r.state === 'denied' ? t('denied', { by: r.by }) : t('expired');
    const extra = r.state !== 'approved' ? '' : r.used != null ? t('used', { used: r.used, granted: r.granted })
      : t(r.delivered ? 'delivered' : 'notDelivered');
    return el('div', { class: 'fwx-row' }, el('span', {}, `${r.profile} · ${label(r)}`), el('span', { class: 'fwx-muted' }, [what, extra].filter(Boolean).join(' · ')));
  }

  function usageCard(p) {
    const max = Math.max(1, ...p.week.filter((d) => d.reported !== false).map((d) => d.total));
    return el('div', { class: 'fwx-card' },
      el('strong', {}, p.name), el('div', { class: 'fwx-big' }, p.devices.length ? t('min', { n: p.total }) : '—'),
      p.devices.length ? null : el('div', { class: 'fwx-muted' }, t('noUsage')),
      p.devices.map((d) => el('div', { class: 'fwx-row' }, el('span', {}, d.name),
        el('span', { class: 'fwx-muted' }, t('min', { n: d.total }) + (d.incomplete ? ` · ${t('incomplete')}` : '')))),
      p.apps.slice(0, 5).map((a) => el('div', { class: 'fwx-row fwx-muted' }, el('span', {}, a.name), el('span', {}, t('min', { n: a.total })))),
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
      body.replaceChildren(
        el('h3', {}, t('pending')), vm.pending.length ? el('div', { class: 'fwx-grid' }, vm.pending.map(pendingCard)) : el('p', { class: 'fwx-muted' }, t('none')),
        el('h3', {}, t('newApps')), vm.apps.length ? el('div', { class: 'fwx-grid' }, vm.apps.map(appsCard)) : el('p', { class: 'fwx-muted' }, t('noNewApps')),
        el('h3', {}, `${t('today')} · ${vm.day}`), el('div', { class: 'fwx-grid' }, vm.profiles.map(usageCard)),
        el('h3', {}, t('recent')), vm.recent.length ? el('div', { class: 'fwx-card' }, vm.recent.map(recentRow)) : el('p', { class: 'fwx-muted' }, t('noRecent')));
      fresh = true;
      updatedAt = now();
      status.textContent = t('updated', { time: time(updatedAt) });
      status.className = 'fwx-muted';
    } catch (e) {
      if (!stopped) {
        fresh = false;
        status.textContent = t('error', { error: e?.message || e }) +
          (updatedAt == null ? '' : ` ${t('stale')} ${t('updated', { time: time(updatedAt) })}`);
        status.className = 'fwx-error';
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
