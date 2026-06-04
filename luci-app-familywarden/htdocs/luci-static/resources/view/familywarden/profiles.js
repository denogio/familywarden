'use strict';
'require view';
'require form';
'require uci';
'require rpc';
'require poll';

var callStatus = rpc.declare({
	object: 'familywarden',
	method: 'status',
	params: [ 'profile' ],
	expect: { '': {} }
});

var callList = rpc.declare({
	object: 'familywarden',
	method: 'list',
	expect: { profiles: {} }
});

var callBlock = rpc.declare({
	object: 'familywarden',
	method: 'block',
	params: [ 'profile' ]
});

var callAllow = rpc.declare({
	object: 'familywarden',
	method: 'allow',
	params: [ 'profile' ]
});

var callReset = rpc.declare({
	object: 'familywarden',
	method: 'reset',
	params: [ 'profile' ]
});

var callTicket = rpc.declare({
	object: 'familywarden',
	method: 'ticket',
	params: [ 'profile', 'minutes' ]
});

var callReload = rpc.declare({
	object: 'familywarden',
	method: 'reload'
});

var callDHCPLeases = rpc.declare({
	object: 'luci-rpc',
	method: 'getDHCPLeases',
	expect: { dhcp_leases: [] }
});

var callHostHints = rpc.declare({
	object: 'luci-rpc',
	method: 'getHostHints',
	expect: { '': {} }
});

function injectStyles() {
	if (document.getElementById('fw-profiles-styles')) return;
	var css = document.createElement('style');
	css.id = 'fw-profiles-styles';
	css.textContent = '\
		.fw-tabs{display:flex;gap:0;border-bottom:2px solid var(--border-color-medium,#dee2e6);margin-bottom:24px;flex-wrap:wrap}\
		.fw-tab{padding:10px 20px;cursor:pointer;font-size:14px;font-weight:500;\
			border:1px solid transparent;border-bottom:none;border-radius:6px 6px 0 0;margin-bottom:-2px;\
			color:var(--text-color-medium,#6c757d);background:transparent;text-decoration:none;display:block}\
		.fw-tab:hover{color:var(--text-color-high,#333);background:var(--background-color-high,#f8f9fa)}\
		.fw-tab.active{color:var(--text-color-high,#333);background:var(--background-color-high,white);\
			border-color:var(--border-color-medium,#dee2e6);border-bottom-color:var(--background-color-high,white);font-weight:600}\
		.fw-tab-add{color:#007bff !important;font-weight:500}\
		.fw-tab-add:hover{background:rgba(0,123,255,.08) !important}\
		.fw-status-bar{display:flex;align-items:center;gap:16px;padding:16px;\
			background:var(--background-color-high,#f8f9fa);border:1px solid var(--border-color-medium,#dee2e6);\
			border-radius:8px;margin-bottom:20px;flex-wrap:wrap}\
		.fw-status-dot{width:12px;height:12px;border-radius:50%;flex-shrink:0}\
		.fw-status-dot.allowed{background:#28a745;box-shadow:0 0 6px #28a745}\
		.fw-status-dot.blocked{background:#dc3545;box-shadow:0 0 6px #dc3545}\
		.fw-status-dot.override{background:#ffc107;box-shadow:0 0 6px #ffc107}\
		.fw-status-text{font-size:16px;font-weight:600;flex:1;margin-right:auto}\
		.fw-action-btn{padding:6px 14px;border:none;border-radius:4px;cursor:pointer;\
			font-size:13px;font-weight:500;color:white;transition:opacity .15s;max-width:115px}\
		.fw-action-btn:hover{opacity:.85}\
		.fw-action-btn:disabled{opacity:.5;cursor:wait}\
		.fw-action-btn.allow{background:#28a745}\
		.fw-action-btn.block{background:#dc3545}\
		.fw-action-btn.reset{background:#6c757d}\
		.fw-action-btn.ticket{background:#17a2b8}\
		.fw-empty-profile{text-align:center;padding:60px 20px;color:var(--text-color-medium,#6c757d)}\
		.fw-delete-section{margin-top:24px;padding-top:16px;border-top:1px solid var(--border-color-medium,#dee2e6)}\
		.fw-delete-btn{padding:8px 20px;border:1px solid #dc3545;border-radius:4px;cursor:pointer;\
			font-size:13px;color:#dc3545;background:transparent}\
		.fw-delete-btn:hover{background:#dc3545;color:white}\
	';
	document.head.appendChild(css);
}

var selectedProfile = null;
var statusNode = null;

function getSelectedProfile() {
	var params = new URLSearchParams(window.location.search);
	return params.get('name') || null;
}

function profileUrl(id) {
	return L.url('admin/services/familywarden/profiles') + '?name=' + encodeURIComponent(id);
}

function refreshStatus() {
	if (!selectedProfile || !statusNode) return Promise.resolve();
	return callStatus(selectedProfile).then(function(data) {
		if (!data || !data.status) return;

		var hasOverride = data.override && data.override !== 'none';
		var isBlocked = data.status.indexOf('blocked') >= 0;
		var dotClass = hasOverride ? 'override' : (isBlocked ? 'blocked' : 'allowed');

		var label = hasOverride
			? (_('Override: ') + (data.override === 'block' ? _('Blocked') : _('Allowed')))
			: (isBlocked ? _('Blocked') : _('Allowed'));
		if (data.blocked_since)
			label += ' — ' + _('since') + ' ' + new Date(data.blocked_since).toLocaleTimeString();
		if (hasOverride && data.override_expires) {
			var remaining = Math.max(0, Math.round((data.override_expires - Date.now()/1000) / 60));
			if (remaining > 0)
				label += ' — ' + remaining + ' min ' + _('remaining');
		}

		var btns = [];
		if (hasOverride) {
			btns.push(E('button', { 'class': 'fw-action-btn reset', 'click': L.ui.createHandlerFn(this, function(ev) { return doAction(ev, callReset); }) }, _('Reset')));
		} else if (isBlocked) {
			btns.push(E('button', { 'class': 'fw-action-btn allow', 'click': L.ui.createHandlerFn(this, function(ev) { return doAction(ev, callAllow); }) }, _('Allow')));
			btns.push(E('button', { 'class': 'fw-action-btn ticket', 'click': L.ui.createHandlerFn(this, function(ev) { return doAction(ev, function(p) { return callTicket(p, 30); }); }) }, '+30 ' + _('min')));
		} else {
			btns.push(E('button', { 'class': 'fw-action-btn block', 'click': L.ui.createHandlerFn(this, function(ev) { return doAction(ev, callBlock); }) }, _('Block')));
		}

		// Rebuild status bar content
		while (statusNode.firstChild) statusNode.removeChild(statusNode.firstChild);
		statusNode.appendChild(E('span', { 'class': 'fw-status-dot ' + dotClass }));
		statusNode.appendChild(E('span', { 'class': 'fw-status-text' }, label));
		btns.forEach(function(b) { statusNode.appendChild(b); });
	});
}

function doAction(ev, fn) {
	var btn = ev.currentTarget;
	btn.disabled = true;
	return fn(selectedProfile).then(refreshStatus).then(function() { btn.disabled = false; });
}

function showAddProfileModal() {
	var nameInput = E('input', {
		'type': 'text',
		'class': 'cbi-input-text',
		'placeholder': _('e.g. Josva'),
		'style': 'width:100%;margin-top:8px'
	});

	L.ui.showModal(_('Add Profile'), [
		E('p', {}, _('Enter a name for the new profile.')),
		nameInput,
		E('div', { 'class': 'right', 'style': 'margin-top:16px' }, [
			E('button', {
				'class': 'cbi-button',
				'click': L.ui.createHandlerFn(this, function() {
					L.ui.hideModal();
				})
			}, _('Cancel')),
			' ',
			E('button', {
				'class': 'cbi-button cbi-button-positive',
				'click': L.ui.createHandlerFn(this, function() {
					var name = (nameInput.value || '').trim();
					if (!name) {
						nameInput.focus();
						return;
					}
					var id = name.toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_|_$/g, '') || ('p' + Math.random().toString(36).substring(2, 8));
					uci.add('familywarden', 'profile', id);
					uci.set('familywarden', id, 'enabled', '1');
					uci.set('familywarden', id, 'name', name);
					uci.set('familywarden', id, 'policy', 'allow');
					// Keep the modal open (Create button stays disabled/spinning
					// via createHandlerFn) until the whole save/apply/reload chain
					// finishes, so the user can't double-submit during the round-trip.
					return uci.save().then(function() {
						return uci.apply();
					}).catch(function(e) {
						// uci.apply() returns ubus code 5 (ENODATA) when there are
						// no pending changes left to apply — harmless here since the
						// profile was already saved. Swallow it and continue.
						if (!/code 5|No data received/i.test(String(e && e.message || e)))
							throw e;
					}).then(function() {
						return callReload();
					}).then(function() {
						L.ui.hideModal();
						window.location = profileUrl(id);
					});
				})
			}, _('Create'))
		])
	]);

	setTimeout(function() { nameInput.focus(); }, 50);
}

function buildTabs(allProfiles, activeId) {
	var tabs = [];

	Object.keys(allProfiles).forEach(function(id) {
		var p = allProfiles[id];
		var isActive = (id === activeId);
		tabs.push(E('a', {
			'class': 'fw-tab' + (isActive ? ' active' : ''),
			'href': profileUrl(id)
		}, p.name || id));
	});

	tabs.push(E('a', {
		'class': 'fw-tab fw-tab-add',
		'href': '#',
		'click': L.ui.createHandlerFn(this, function(ev) {
			ev.preventDefault();
			showAddProfileModal();
		})
	}, '+ ' + _('Add Profile')));

	return E('div', { 'class': 'fw-tabs' }, tabs);
}

function buildStatusBar(status) {
	var hasOverride = status.override && status.override !== 'none';
	var isBlocked = status.status && status.status.indexOf('blocked') >= 0;
	var dotClass = hasOverride ? 'override' : (isBlocked ? 'blocked' : 'allowed');
	var label = hasOverride
		? (_('Override: ') + (status.override === 'block' ? _('Blocked') : _('Allowed')))
		: (isBlocked ? _('Blocked') : _('Allowed'));

	if (hasOverride && status.override_expires) {
		var remaining = Math.max(0, Math.round((status.override_expires - Date.now()/1000) / 60));
		if (remaining > 0)
			label += ' — ' + remaining + ' min ' + _('remaining');
	}

	var btns = [];
	if (hasOverride) {
		btns.push(E('button', { 'class': 'fw-action-btn reset', 'click': L.ui.createHandlerFn(this, function(ev) { return doAction(ev, callReset); }) }, _('Reset')));
	} else if (isBlocked) {
		btns.push(E('button', { 'class': 'fw-action-btn allow', 'click': L.ui.createHandlerFn(this, function(ev) { return doAction(ev, callAllow); }) }, _('Allow')));
		btns.push(E('button', { 'class': 'fw-action-btn ticket', 'click': L.ui.createHandlerFn(this, function(ev) { return doAction(ev, function(p) { return callTicket(p, 30); }); }) }, '+30 ' + _('min')));
	} else {
		btns.push(E('button', { 'class': 'fw-action-btn block', 'click': L.ui.createHandlerFn(this, function(ev) { return doAction(ev, callBlock); }) }, _('Block')));
	}

	statusNode = E('div', { 'class': 'fw-status-bar' }, [
		E('span', { 'class': 'fw-status-dot ' + dotClass }),
		E('span', { 'class': 'fw-status-text' }, label)
	].concat(btns));

	return statusNode;
}

return view.extend({
	load: function() {
		return Promise.all([
			callList(),
			uci.load('familywarden'),
			callDHCPLeases(),
			callHostHints()
		]);
	},

	render: function(data) {
		var allProfiles = data[0] || {};
		var leases = data[2] || [];
		var hostHints = data[3] || {};

		injectStyles();

		selectedProfile = getSelectedProfile();

		// Default to first profile if none selected
		var profileIds = Object.keys(allProfiles);
		if (!selectedProfile && profileIds.length > 0)
			selectedProfile = profileIds[0];

		// No profiles at all
		if (!selectedProfile || !allProfiles[selectedProfile]) {
			var tabs = buildTabs(allProfiles, null);
			return E('div', {}, [
				E('h2', {}, _('Profiles')),
				tabs,
				E('div', { 'class': 'fw-empty-profile' }, [
					E('p', {}, _('No profiles configured yet.')),
					E('p', {}, _('Click "+ Add Profile" above to create one.'))
				])
			]);
		}

		var status = allProfiles[selectedProfile];

		// Build known devices
		var knownDevices = {};
		leases.forEach(function(l) {
			if (l.macaddr) {
				var mac = l.macaddr.toLowerCase();
				knownDevices[mac] = (l.hostname || l.ipaddr || mac) + ' (' + mac + ')';
			}
		});
		Object.keys(hostHints).forEach(function(mac) {
			var k = mac.toLowerCase();
			if (!knownDevices[k]) {
				var h = hostHints[mac];
				var name = (h && (h.name || (h.ipaddrs && h.ipaddrs[0]))) || k;
				knownDevices[k] = name + ' (' + k + ')';
			}
		});

		// --- Tabs ---
		var tabs = buildTabs(allProfiles, selectedProfile);

		// --- Status bar ---
		var statusBar = buildStatusBar(status);

		// --- Profile settings form ---
		var m = new form.Map('familywarden');
		var origSaveApply = m.handleSaveApply;
		m.handleSaveApply = function(ev, mode) {
			// origSaveApply already saves AND applies the UCI changes; just
			// reload the daemon afterwards. Calling uci.apply() again here would
			// hit ubus code 5 (ENODATA) since no changes remain pending.
			return origSaveApply.apply(this, [ev, mode]).then(function() {
				return callReload();
			});
		};

		var s = m.section(form.NamedSection, selectedProfile, 'profile', _('Profile Settings'));
		s.addremove = false;

		var o = s.option(form.Value, 'name', _('Name'));
		o.rmempty = false;

		o = s.option(form.Flag, 'enabled', _('Enabled'));
		o.rmempty = false;
		o.default = '1';

		o = s.option(form.ListValue, 'policy', _('Access Policy'),
			_('"Always allow/block" ignores schedules. "By schedule" blocks by default and uses schedules below to control access.'));
		o.value('allow', _('Always allow'));
		o.value('block', _('Always block'));
		o.value('scheduled', _('By schedule'));

		o = s.option(form.DynamicList, 'device', _('Devices'),
			_('Select known devices or enter MAC addresses manually.'));
		o.datatype = 'macaddr';
		o.rmempty = true;
		var macs = Object.keys(knownDevices).sort(function(a, b) {
			return knownDevices[a].localeCompare(knownDevices[b]);
		});
		macs.forEach(function(mac) {
			o.value(mac, knownDevices[mac]);
		});

		o = s.option(form.Value, 'dns_profile', _('DNS Filtering'));
		o.rmempty = true;
		o.optional = true;
		o.description = _('Optional: AdGuard Home / DNS filter group name.');

		// --- Schedules for this profile ---
		var pid = selectedProfile;
		var ss = m.section(form.GridSection, 'schedule', _('Schedules'),
			_('Time windows for this profile. Block always wins over Allow when overlapping. Outside all schedules, access is blocked.'));
		ss.addremove = true;
		ss.anonymous = true;
		ss.addbtntitle = _('Add schedule');
		ss.modaltitle = function() { return _('Edit Schedule'); };

		ss.filter = function(section_id) {
			return uci.get('familywarden', section_id, 'profile') === pid;
		};

		ss.handleAdd = function(ev) {
			var id = uci.add('familywarden', 'schedule');
			uci.set('familywarden', id, 'profile', pid);
			return this.renderMoreOptionsModal(id);
		};

		ss.renderSectionAdd = function() {
			var btn = E('button', {
				'class': 'cbi-button cbi-button-add',
				'click': L.ui.createHandlerFn(this, 'handleAdd')
			}, this.addbtntitle || _('Add'));
			return E('div', { 'class': 'cbi-section-create', 'style': 'margin-bottom:16px' }, btn);
		};

		// Summary column
		o = ss.option(form.DummyValue, '_summary', _('Schedule'));
		o.modalonly = false;
		o.textvalue = function(section_id) {
			var days = uci.get('familywarden', section_id, 'days') || '';
			var start = uci.get('familywarden', section_id, 'start') || '';
			var stop = uci.get('familywarden', section_id, 'stop') || '';
			var action = uci.get('familywarden', section_id, 'action') || 'allow';
			if (!days || !start || !stop) return _('(incomplete)');
			var dayMap = { mon:'Mon', tue:'Tue', wed:'Wed', thu:'Thu', fri:'Fri', sat:'Sat', sun:'Sun' };
			var dayArr = (typeof days === 'string') ? days.split(/\s+/) : days;
			var dayStr = dayArr.map(function(d) { return dayMap[d] || d; }).join(', ');
			var icon = (action === 'allow') ? '\u2714' : '\u2718';
			return dayStr + '  ' + start + ' \u2013 ' + stop + '  [' + icon + ' ' + action + ']';
		};

		o = ss.option(form.ListValue, 'action', _('Action'));
		o.value('allow', _('Allow'));
		o.value('block', _('Block'));
		o.rmempty = false;
		o.default = 'allow';
		o.editable = true;

		o = ss.option(form.Value, 'profile', _('Profile'));
		o.modalonly = true;
		o.readonly = true;
		o.default = pid;

		o = ss.option(form.MultiValue, 'days', _('Days'));
		o.value('mon', _('Monday'));
		o.value('tue', _('Tuesday'));
		o.value('wed', _('Wednesday'));
		o.value('thu', _('Thursday'));
		o.value('fri', _('Friday'));
		o.value('sat', _('Saturday'));
		o.value('sun', _('Sunday'));
		o.rmempty = false;
		o.modalonly = true;

		o = ss.option(form.Value, 'start', _('Start Time'));
		o.placeholder = '07:00';
		o.rmempty = false;
		o.modalonly = true;
		o.description = _('24-hour format, e.g. 07:00');
		o.validate = function(section_id, value) {
			if (!/^\d{2}:\d{2}$/.test(value)) return _('Use HH:MM format');
			return true;
		};

		o = ss.option(form.Value, 'stop', _('Stop Time'));
		o.placeholder = '20:00';
		o.rmempty = false;
		o.modalonly = true;
		o.description = _('24-hour format, e.g. 20:00');
		o.validate = function(section_id, value) {
			if (!/^\d{2}:\d{2}$/.test(value)) return _('Use HH:MM format');
			return true;
		};

		// --- Delete profile ---
		var self = this;
		var deleteSection = E('div', { 'class': 'fw-delete-section' }, [
			E('button', {
				'class': 'fw-delete-btn',
				// createHandlerFn keeps the button disabled/spinning while the
				// save/apply/reload/redirect chain runs, preventing double-submit.
				'click': L.ui.createHandlerFn(self, function() {
					if (!confirm(_('Delete this profile and all its schedules?')))
						return;

					// Remove schedules for this profile
					uci.sections('familywarden', 'schedule', function(s) {
						if (s.profile === pid)
							uci.remove('familywarden', s['.name']);
					});
					uci.remove('familywarden', pid);
					return uci.save().then(function() {
						return uci.apply();
					}).catch(function(e) {
						// uci.apply() returns ubus code 5 (ENODATA) when there are
						// no pending changes left to apply — harmless here since the
						// profile was already removed. Swallow it and continue.
						if (!/code 5|No data received/i.test(String(e && e.message || e)))
							throw e;
					}).then(function() {
						return callReload();
					}).then(function() {
						window.location = L.url('admin/services/familywarden/profiles');
					});
				})
			}, _('Delete Profile'))
		]);

		// --- Render ---
		return m.render().then(function(formEl) {
			poll.add(refreshStatus, 10);

			return E('div', {}, [
				E('h2', {}, _('Profiles')),
				tabs,
				statusBar,
				formEl,
				deleteSection
			]);
		});
	}
});
