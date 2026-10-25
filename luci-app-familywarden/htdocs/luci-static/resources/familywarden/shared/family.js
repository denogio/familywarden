// FamilyWarden shared administrator UI: extra-time requests and today's usage (A20).
// Plain ES module, no framework and no LuCI dependency: a platform wrapper passes
// call(method, params) -> Promise<object> talking to the core API. Own EN/DA strings;
// styles are scoped under .fwx and use the host theme's colors where available.

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
    updated: 'Updated {time}'
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
    updated: 'Opdateret {time}'
  }
};

export function strings(lang) {
  const base = String(lang || 'en').toLowerCase().split(/[-_]/)[0];
  const table = STRINGS[base] || STRINGS.en;
  return (key, vars = {}) => (table[key] || STRINGS.en[key] || key).replace(/\{(\w+)\}/g, (_, k) => String(vars[k] ?? ''));
}

const minutes = (seconds) => Math.floor((Number(seconds) || 0) / 60);

// Pure view model: what the page shows, from the core's `requests` and `usage` replies.
export function viewModel(requests, usage, nowEpoch) {
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
    week: (p.week || []).map((d) => ({ day: d.day, total: minutes(d.total) }))
  }));
  return { pending, recent, profiles, day: usage?.day || '' };
}

const CSS = `
.fwx{--fwx-fg:var(--text-color-high,inherit);--fwx-muted:var(--text-color-medium,#5b6773);--fwx-line:var(--border-color-medium,#d6dde4);
 --fwx-card:var(--background-color-high,transparent);--fwx-accent:#1b2a3a;--fwx-bar:#c9a227;color:var(--fwx-fg)}
.fwx h3{margin:1.2em 0 .5em}.fwx .fwx-intro{color:var(--fwx-muted);max-width:52em}
.fwx .fwx-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(320px,1fr));gap:12px}
.fwx .fwx-card{border:1px solid var(--fwx-line);border-radius:12px;padding:14px 16px;background:var(--fwx-card)}
.fwx .fwx-big{font-size:1.6em;font-weight:600;margin:.1em 0}.fwx .fwx-muted{color:var(--fwx-muted);font-size:.92em}
.fwx .fwx-row{display:flex;justify-content:space-between;gap:12px;padding:2px 0}
.fwx .fwx-actions{display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin-top:10px}
.fwx input[type=number]{width:6em}.fwx .fwx-week{display:flex;align-items:flex-end;gap:6px;height:64px;margin-top:8px}
.fwx .fwx-week span{flex:1;background:var(--fwx-bar);border-radius:3px 3px 0 0;min-height:2px}
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
  const body = el('div');
  root.classList.add('fwx');
  root.replaceChildren(style, el('h2', {}, t('title')), el('p', { class: 'fwx-intro' }, t('intro')), status, body);
  let busy = false;
  let timer = null;

  async function answer(r, approve, input) {
    const value = approve ? Number(input.value) : 0;
    if (approve && !(Number.isInteger(value) && value >= 1 && value <= r.minutes)) { input.focus(); return; }
    busy = true;
    try {
      const reply = await call('request_answer', { request_id: r.id, approve, minutes: value });
      if (reply?.error) status.textContent = t('failed', { error: reply.error });
    } catch (e) { status.textContent = t('error', { error: e?.message || e }); }
    busy = false;
    refresh();
  }

  function pendingCard(r) {
    const input = el('input', { type: 'number', min: '1', max: String(r.minutes), value: String(r.minutes), 'aria-label': t('minutes') });
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
    const max = Math.max(1, ...p.week.map((d) => d.total));
    return el('div', { class: 'fwx-card' },
      el('strong', {}, p.name), el('div', { class: 'fwx-big' }, t('min', { n: p.total })),
      p.devices.length ? null : el('div', { class: 'fwx-muted' }, t('noUsage')),
      p.devices.map((d) => el('div', { class: 'fwx-row' }, el('span', {}, d.name),
        el('span', { class: 'fwx-muted' }, t('min', { n: d.total }) + (d.incomplete ? ` · ${t('incomplete')}` : '')))),
      p.apps.slice(0, 5).map((a) => el('div', { class: 'fwx-row fwx-muted' }, el('span', {}, a.name), el('span', {}, t('min', { n: a.total })))),
      el('div', { class: 'fwx-week', title: t('week') }, p.week.map((d) =>
        el('span', { style: `height:${Math.round((d.total / max) * 100)}%`, title: `${d.day}: ${t('min', { n: d.total })}` }))));
  }

  async function refresh() {
    if (busy) return;
    clearTimeout(timer);
    try {
      const [requests, usage] = await Promise.all([call('requests', {}), call('usage', {})]);
      if (requests?.error || usage?.error) throw new Error(requests?.error || usage?.error);
      const vm = viewModel(requests, usage, now());
      body.replaceChildren(
        el('h3', {}, t('pending')), vm.pending.length ? el('div', { class: 'fwx-grid' }, vm.pending.map(pendingCard)) : el('p', { class: 'fwx-muted' }, t('none')),
        el('h3', {}, `${t('today')} · ${vm.day}`), el('div', { class: 'fwx-grid' }, vm.profiles.map(usageCard)),
        el('h3', {}, t('recent')), vm.recent.length ? el('div', { class: 'fwx-card' }, vm.recent.map(recentRow)) : el('p', { class: 'fwx-muted' }, t('noRecent')));
      status.textContent = t('updated', { time: time(now()) });
      status.className = 'fwx-muted';
    } catch (e) {
      status.textContent = t('error', { error: e?.message || e });
      status.className = 'fwx-error';
    }
    if (root.isConnected) timer = setTimeout(refresh, interval);
  }
  refresh();
  return { refresh, stop: () => clearTimeout(timer) };
}
