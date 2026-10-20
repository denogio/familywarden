'use strict';
'require view';
'require rpc';
'require uci';
'require poll';
'require ui';
'require familywarden.ulysses as ulysses';

// Keep the envelope: an absent profiles field is a failed read, not an empty household.
var callList = rpc.declare({ object: 'familywarden', method: 'list', expect: { '': {} } });
var callBlock = rpc.declare({ object: 'familywarden', method: 'block', params: [ 'profile' ] });
var callAllow = rpc.declare({ object: 'familywarden', method: 'allow', params: [ 'profile' ] });
var callReset = rpc.declare({ object: 'familywarden', method: 'reset', params: [ 'profile' ] });
var callTicket = rpc.declare({ object: 'familywarden', method: 'ticket', params: [ 'profile', 'minutes' ] });
var callDHCPLeases = rpc.declare({
	object: 'luci-rpc', method: 'getDHCPLeases', expect: { dhcp_leases: [] }
});
var callHostHints = rpc.declare({ object: 'luci-rpc', method: 'getHostHints', expect: { '': {} } });

var dashboardNode, lastUpdated = null, refreshing = false, acting = false;
var dayLabels = { mon: _('Mon'), tue: _('Tue'), wed: _('Wed'), thu: _('Thu'),
	fri: _('Fri'), sat: _('Sat'), sun: _('Sun') };

function loadDashboard() {
	return Promise.all([
		callList(), uci.load('familywarden'),
		L.resolveDefault(callDHCPLeases(), []), L.resolveDefault(callHostHints(), {})
	]).then(function(data) {
		var profiles = data[0] && data[0].profiles;
		if (!profiles || typeof profiles !== 'object' || Array.isArray(profiles))
			throw new Error(_('Profile status unavailable'));
		return { profiles: profiles, leases: data[2], hints: data[3] };
	});
}

function lighthouse() {
	var ns = 'http://www.w3.org/2000/svg';
	var svg = document.createElementNS(ns, 'svg');
	svg.setAttribute('viewBox', '0 0 48 48');
	svg.setAttribute('class', 'fw-lighthouse');
	svg.setAttribute('aria-hidden', 'true');
	[
		['fw-beam', 'M19 14.375 L4 9.5 V22.5 L19 17.625 Z M29 14.375 L44 9.5 V22.5 L29 17.625 Z'],
		['fw-tower', 'M16.5 13 L24 6.5 L31.5 13 Z M19.5 21.5 H28.5 L31 40 H17 Z M17 19 H31 V21.5 H17 Z'],
		['fw-lamp', 'M19 13 H29 V19 H19 Z'],
		['fw-stripe', 'M18.55 28.5 H29.45 L29.85 31.5 H18.15 Z'],
		['fw-sea', 'M7 43.5 C10 41.5 13 41.5 16 43.5 S22 45.5 25 43.5 S31 41.5 34 43.5 S38.5 45 41 43.5']
	].forEach(function(part) {
		var path = document.createElementNS(ns, 'path');
		path.setAttribute('class', part[0]);
		path.setAttribute('d', part[1]);
		svg.appendChild(path);
	});
	return svg;
}

// Small stroke icons from the design component board; status text carries the meaning.
function icon(name) {
	var circle = 'M21 12a9 9 0 1 1-18 0a9 9 0 1 1 18 0';
	var paths = {
		check: circle + ' M8 12.5l2.7 2.7L16 9.5',
		ban: circle + ' M5.7 5.7l12.6 12.6',
		moon: 'M19.5 14.2A8 8 0 0 1 9.8 4.5a8 8 0 1 0 9.7 9.7z',
		info: circle + ' M12 11v5M12 8h.01',
		mast: 'M12 2.5v17M12 4.5l6.5 10H12M4 19.5c2.7 1.6 5.3 1.6 8 0s5.3-1.6 8 0',
		hand: 'M8 13V6.5a1.5 1.5 0 0 1 3 0V11M11 10V4.5a1.5 1.5 0 0 1 3 0V11M14 10.5V6a1.5 1.5 0 0 1 3 0v7c0 4-2.5 7-6 7-2.5 0-4-1.2-5.5-3.5L4 14.5a1.5 1.5 0 0 1 2.4-1.8L8 14.5'
	};
	var svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
	svg.setAttribute('class', 'fw-icon');
	svg.setAttribute('viewBox', '0 0 24 24');
	svg.setAttribute('data-icon', name);
	svg.setAttribute('aria-hidden', 'true');
	svg.setAttribute('focusable', 'false');
	var path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
	path.setAttribute('d', paths[name]);
	svg.appendChild(path);
	return svg;
}

function formatDays(value) {
	var days = Array.isArray(value) ? value : (value || '').split(/\s+/);
	var week = Object.keys(dayLabels), selected = week.filter(function(d) { return days.indexOf(d) >= 0; });
	if (selected.length === 7) return _('Every day');
	var groups = [];
	for (var i = 0; i < week.length; i++) {
		if (selected.indexOf(week[i]) < 0) continue;
		var start = i;
		while (i + 1 < week.length && selected.indexOf(week[i + 1]) >= 0) i++;
		groups.push(start === i ? dayLabels[week[i]] : dayLabels[week[start]] + '–' + dayLabels[week[i]]);
	}
	return groups.join(', ');
}

function compactTime(epoch) {
	var date = new Date(epoch * 1000), now = new Date();
	var time = date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
	return date.toDateString() === now.toDateString() ? time
		: date.toLocaleDateString([], { month: 'short', day: 'numeric' }) + ' ' + time;
}

function policyLabel(policy) {
	return { allow: _('Unrestricted'), block: _('Blocked'), scheduled: _('Scheduled') }[policy] || _('Unknown policy');
}

function knownStatus(p) {
	return p.status === 'allowed' || p.status === 'blocked';
}

function statusPanel(p) {
	var allowed = p.status === 'allowed', known = knownStatus(p);
	var timedOverride = known && p.override === 'allow' && +p.override_expires > 0;
	var manualOverride = known && (p.override === 'block' || (p.override === 'allow' && !+p.override_expires));
	var reason = _('Status is unavailable. Refresh before changing access.');
	if (known) {
		if (p.override === 'block') reason = _('Manual block until reset.');
		else if (p.override === 'allow' && +p.override_expires > 0)
			reason = _('Internet ticket · %s min left, ends %s').format(
				Math.max(0, Math.ceil((p.override_expires - Date.now() / 1000) / 60)), compactTime(p.override_expires));
		else if (p.override === 'allow') reason = _('Manual allow until reset.');
		else if (p.policy === 'scheduled') reason = allowed ? _('Within allowed hours.') : _('Outside allowed hours.');
		else if (p.policy === 'block') reason = ulysses.isBound(p)
			? _('Policy is Blocked. The pact prevents Allow and tickets.')
			: _('Policy is Blocked. Allow or a ticket opens it.');
		else reason = _('Policy is Unrestricted. Internet stays allowed until blocked.');
	}
	return E('div', { 'class': 'fw-state fw-status ' + (!known ? 'is-unknown' : allowed ? 'is-allowed' : 'is-blocked') }, [
		E('span', { 'class': 'fw-state-icon' }, icon(!known ? 'info' : allowed ? 'check'
			: p.override !== 'block' && p.policy === 'scheduled' ? 'moon' : 'ban')),
		E('div', {}, [
			E('div', { 'class': 'fw-state-titlerow' }, [
				E('p', { 'class': 'fw-state-title' }, !known ? _('Status unknown') : allowed ? _('Internet allowed') : _('Internet blocked')),
				manualOverride || timedOverride ? E('span', { 'class': 'fw-tag-override' }, [
					icon('hand'), timedOverride ? _('Timed override') : _('Override')
				]) : ''
			]),
			E('p', { 'class': 'fw-state-reason' }, reason)
		])
	]);
}

function hostMap(data) {
	var map = {};
	(data.leases || []).forEach(function(l) {
		if (l.macaddr && l.hostname) map[l.macaddr.toLowerCase()] = l.hostname;
	});
	Object.keys(data.hints || {}).forEach(function(mac) {
		if (!map[mac.toLowerCase()] && data.hints[mac].name)
			map[mac.toLowerCase()] = data.hints[mac].name;
	});
	return map;
}

function fact(label, value) {
	return E('div', { 'class': 'fw-fact' }, [ E('dt', {}, label), E('dd', {}, value) ]);
}

function profileCard(id, p, names) {
	var url = L.url('admin/services/familywarden/profiles') + '?name=' + encodeURIComponent(id);
	var devices = Array.isArray(p.devices) ? p.devices : [];
	var schedules = Array.isArray(p.schedules) ? p.schedules : [];
	var facts = [ fact(_('Devices'), devices.length ? E('ul', { 'class': 'fw-devices' }, devices.map(function(mac) {
		return E('li', {}, [ E('span', {}, names[mac.toLowerCase()] || _('Unknown device')),
			E('span', { 'class': 'fw-mono' }, mac) ]);
	})) : _('No devices')) ];
	if (p.policy === 'scheduled') {
		facts.push(fact(_('Allowed hours'), schedules.length ? E('ul', { 'class': 'fw-schedules' }, schedules.map(function(s) {
			var hours = (s.start || '?') + '–' + (s.stop || '?');
			return E('li', {}, [ E('span', {}, formatDays(s.days)),
				E('span', { 'class': 'fw-hours', 'title': hours }, s.start === '00:00' && s.stop === '23:59' ? _('All day') : hours) ]);
		})) : _('No allow windows; internet is blocked by default.')));
	}
	var note = ulysses.note(p, icon('mast'));
	return E('article', { 'class': 'fw-card', 'data-profile': id, 'aria-label': p.name || id }, [
		E('div', { 'class': 'fw-card-head' }, [
			E('h4', { 'class': 'fw-card-name' }, p.name || id),
			E('span', { 'class': 'fw-card-policy' }, policyLabel(p.policy)),
			id === ulysses.captainId() ? E('span', { 'class': 'fw-captain' }, _('Captain')) : '',
			E('a', { 'class': 'fw-card-edit', 'href': url, 'data-focus': id + ':edit',
				'aria-label': _('Edit profile %s').format(p.name || id) }, _('Edit'))
		]),
		statusPanel(p), E('dl', { 'class': 'fw-facts' }, facts), note || '',
		E('div', { 'class': 'fw-card-foot' }, cardButtons(id, p))
	]);
}

function actionButton(id, label, key, fn) {
	var intent = key.indexOf('ticket-') === 0 ? 'grant' : { allow: 'allow', block: 'block', pact: 'pact' }[key];
	return E('button', { 'type': 'button', 'class': 'cbi-button fw-btn' + (intent ? ' fw-btn-' + intent : ''), 'data-focus': id + ':' + key,
		'click': function(ev) {
			if (acting) return;
			acting = true;
			var button = ev.currentTarget;
			button.disabled = true;
			return Promise.resolve().then(fn).then(function(res) {
				if (res && res.error) throw new Error(res.error);
			}).catch(function(err) {
				ui.addNotification(null, E('p', {}, _('Action failed: %s').format(err.message)), 'warning');
			}).then(function() {
				acting = false;
				return refreshDashboard();
			}).finally(function() { button.disabled = false; });
		} }, label);
}

function cardButtons(id, p) {
	if (!knownStatus(p)) return [];
	var btns = [], bound = ulysses.isBound(p);
	var ticket = p.override === 'allow' && +p.override_expires > Date.now() / 1000;
	var manual = p.override && p.override !== 'none' && !ticket;
	var overrides = [];
	if (manual) {
		var reset = actionButton(id, _('Reset override'), 'reset', function() { return callReset(id); });
		reset.classList.add('fw-btn-reset');
		overrides.push(reset);
	}
	else if (p.status === 'blocked') {
		if (!bound) overrides.push(actionButton(id, _('Allow'), 'allow', function() { return callAllow(id); }));
	} else overrides.push(actionButton(id, _('Block'), 'block', function() { return callBlock(id); }));
	if (overrides.length) btns.push(E('div', { 'class': 'fw-act-zone fw-override', 'role': 'group',
		'aria-label': _('Override for %s').format(p.name || id) }, [
		E('span', { 'class': 'fw-actions-label' }, _('Override')),
		E('div', { 'class': 'fw-act-row' }, overrides)
	]));
	if (!bound && !manual && (ticket || p.status === 'blocked')) {
		var minutes = [5, 10, 30].map(function(mins) {
			var button = actionButton(id, '+' + mins, 'ticket-' + mins, function() { return callTicket(id, mins); });
			button.setAttribute('aria-label', _('Add %s minutes').format(mins));
			return button;
		});
		var actions = [ E('div', { 'class': 'fw-seg' }, minutes) ];
		if (ticket) {
			var cancel = actionButton(id, _('Cancel ticket'), 'reset', function() { return callReset(id); });
			cancel.classList.add('fw-cancel-ticket', 'fw-btn-revoke');
			actions.push(cancel);
		}
		btns.push(E('div', { 'class': 'fw-act-zone fw-ticket', 'role': 'group',
			'aria-label': _('Internet ticket for %s').format(p.name || id) }, [
			E('span', { 'class': 'fw-actions-label' }, _('Internet ticket · minutes')),
			E('div', { 'class': 'fw-act-row' }, actions)
		]));
	}
	if (bound) btns.unshift(E('p', { 'class': 'fw-held' }, _('Bound by the pact: Allow and internet tickets are unavailable.')));
	if (id === ulysses.captainId()) {
		var label = !bound ? _('Make a pact') : p.pact_type === 'until' ? _('Extend pact') : p.pact_in_window ? _('Amend pact') : null;
		if (label) btns.push(E('div', { 'class': 'fw-act-zone fw-pact-actions', 'role': 'group',
			'aria-label': _('Pact for %s').format(p.name || id) }, [
			E('span', { 'class': 'fw-actions-label' }, _('Pact')),
			E('div', { 'class': 'fw-act-row' }, [
				actionButton(id, label, 'pact', function() { return ulysses.showPactModal(p.name, p); })
			])
		]));
	}
	return btns;
}

function notices(data) {
	var profiles = data.profiles, out = [];
	var bound = Object.keys(profiles).some(function(id) { return ulysses.isBound(profiles[id]); });
	if (uci.get('familywarden', 'settings', 'enabled') === '0' && !bound)
		out.push(E('div', { 'class': 'fw-notice' }, _('FamilyWarden is switched off in Settings: nothing is enforced.')));
	if (uci.get('familywarden', 'settings', 'default_policy') === 'block') {
		var assigned = {};
		uci.sections('familywarden', 'profile').forEach(function(p) {
			L.toArray(p.device).forEach(function(mac) { assigned[mac.toLowerCase()] = true; });
		});
		var names = hostMap(data), seen = {};
		var unassigned = (data.leases || []).filter(function(l) {
			var mac = (l.macaddr || '').toLowerCase();
			if (!mac || assigned[mac] || seen[mac]) return false;
			seen[mac] = true;
			return true;
		});
		if (unassigned.length) out.push(E('div', { 'class': 'fw-notice' }, [
			E('strong', {}, _('Devices outside every profile are blocked')),
			E('p', {}, unassigned.map(function(l) {
				return (names[l.macaddr.toLowerCase()] || _('Unknown device')) + ' (' + l.macaddr.toLowerCase() + ')';
			}).join(', ')),
			E('a', { 'href': L.url('admin/services/familywarden/profiles') }, _('Add them to a profile'))
		]));
	}
	return out;
}

function renderContent(data) {
	var profiles = data.profiles, keys = Object.keys(profiles), names = hostMap(data);
	var allowed = 0, blocked = 0, unknown = 0;
	keys.forEach(function(id) {
		if (profiles[id].status === 'allowed') allowed++;
		else if (profiles[id].status === 'blocked') blocked++;
		else unknown++;
	});
	return E('div', { 'class': 'fw-body' }, [
		E('div', { 'class': 'fw-summary' }, [
			E('dl', {}, [ fact(_('Enabled profiles'), String(keys.length)),
				fact(_('Internet allowed'), String(allowed)), fact(_('Internet blocked'), String(blocked))
			].concat(unknown ? [fact(_('Status unknown'), String(unknown))] : [])),
			E('span', { 'class': 'fw-updated' }, _('Updated %s · refreshes every 10 s').format(lastUpdated.toLocaleTimeString()))
		]),
		E('div', { 'class': 'fw-notices' }, notices(data)),
		E('div', { 'class': 'fw-section-head' }, [ E('h3', {}, _('Profiles')),
			E('p', {}, _('Quick actions change whole-profile internet access immediately.')) ]),
		keys.length ? E('div', { 'class': 'fw-grid' }, keys.map(function(id) { return profileCard(id, profiles[id], names); })) :
			E('div', { 'class': 'fw-empty' }, [ E('h3', {}, _('No profiles configured yet.')),
				E('a', { 'href': L.url('admin/services/familywarden/profiles') }, _('Create your first profile')) ])
	]);
}

function showFailure() {
	var node = dashboardNode.querySelector('.fw-refresh-status');
	node.replaceChildren(E('div', { 'class': 'fw-notice is-fault', 'role': 'status' }, [
		E('strong', {}, _('Could not refresh profile status')),
		E('p', {}, lastUpdated ? _('Showing the last successful update from %s. Access may have changed.').format(lastUpdated.toLocaleString()) : _('No profile status has been received. Try refreshing.')),
		E('button', { 'type': 'button', 'class': 'cbi-button', 'click': refreshDashboard }, _('Retry'))
	]));
	dashboardNode.classList.add('is-stale');
	// Stale UI must not offer an action using old pact/permission information.
	dashboardNode.querySelectorAll('.fw-card button').forEach(function(b) { b.disabled = true; });
}

function refreshDashboard() {
	if (refreshing || acting || document.body.classList.contains('modal-overlay-active')) return Promise.resolve();
	refreshing = true;
	return loadDashboard().then(function(data) {
		lastUpdated = new Date();
		var active = document.activeElement;
		var focusKey = dashboardNode.contains(active) && active.getAttribute('data-focus');
		dashboardNode.querySelector('.fw-content').replaceChildren(renderContent(data));
		dashboardNode.querySelector('.fw-refresh-status').replaceChildren();
		dashboardNode.classList.remove('is-stale');
		if (focusKey) {
			var target = Array.from(dashboardNode.querySelectorAll('[data-focus]')).find(function(el) {
				return el.getAttribute('data-focus') === focusKey;
			});
			if (target) target.focus({ preventScroll: true });
		}
	}).catch(showFailure).finally(function() { refreshing = false; });
}

return view.extend({
	load: function() { return L.resolveDefault(loadDashboard(), null); },
	render: function(data) {
		if (!document.getElementById('fw-overview-css')) document.head.appendChild(E('link', {
			'id': 'fw-overview-css', 'rel': 'stylesheet', 'href': L.resource('familywarden/overview.css')
		}));
		lastUpdated = data ? new Date() : null;
		dashboardNode = E('div', { 'id': 'fw-dashboard' }, [
			E('div', { 'class': 'fw-head' }, [ E('div', {}, [
				E('p', { 'class': 'fw-brand' }, [lighthouse(), 'FamilyWarden']),
				E('h2', { 'class': 'fw-title' }, _('Overview')),
				E('p', { 'class': 'fw-lede' }, _('Who has internet access now, which rules apply, and what needs attention.'))
			]), E('a', { 'class': 'cbi-button fw-add-btn', 'href': L.url('admin/services/familywarden/profiles') }, _('Add Profile')) ]),
			E('div', { 'class': 'fw-refresh-status' }),
			E('div', { 'class': 'fw-content' }, data ? renderContent(data) : '')
		]);
		if (!data) showFailure();
		poll.add(refreshDashboard, 10);
		return dashboardNode;
	},
	handleSave: null,
	handleSaveApply: null,
	handleReset: null
});
