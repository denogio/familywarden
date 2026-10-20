'use strict';
'require view';
'require rpc';
'require uci';
'require poll';
'require ui';
'require familywarden.ulysses as ulysses';
'require familywarden.ui as fwui';

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

function icon(name) {
	return fwui.icon(name);
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
		fwui.loadStyles('overview');
		lastUpdated = data ? new Date() : null;
		dashboardNode = E('div', { 'id': 'fw-dashboard' }, [
			fwui.header(_('Overview'), _('Who has internet access now, which rules apply, and what needs attention.'), [
				E('a', { 'class': 'cbi-button fw-add-btn', 'href': L.url('admin/services/familywarden/profiles') }, _('Add Profile'))
			]),
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
