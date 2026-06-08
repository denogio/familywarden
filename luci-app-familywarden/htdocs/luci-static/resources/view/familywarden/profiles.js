'use strict';
'require view';
'require form';
'require uci';
'require rpc';
'require poll';
'require ui';

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
		.fw-tab-add{color:var(--primary-color-high,#0069d9);font-weight:500}\
		.fw-tab-add:hover{background:var(--background-color-high,#f8f9fa)}\
		.fw-status-bar{display:flex;align-items:center;gap:16px;padding:16px;\
			background:var(--background-color-high,#f8f9fa);border:1px solid var(--border-color-medium,#dee2e6);\
			border-radius:8px;margin-bottom:20px;flex-wrap:wrap}\
		.fw-status-dot{width:12px;height:12px;border-radius:50%;flex-shrink:0;background:var(--border-color-medium,#ccc)}\
		.fw-status-dot.allowed{background:var(--success-color-high,#5e9eff)}\
		.fw-status-dot.blocked{background:var(--error-color-high,#ff6b6b)}\
		.fw-status-dot.override{background:var(--warn-color-medium,#fbbf24)}\
		.fw-status-text{font-size:16px;font-weight:600;flex:1;margin-right:auto}\
		.fw-empty-profile{text-align:center;padding:60px 20px;color:var(--text-color-medium,#6c757d)}\
		.fw-delete-section{margin-top:24px;padding-top:16px;border-top:1px solid var(--border-color-medium,#dee2e6)}\
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
			btns.push(E('button', { 'class': 'cbi-button cbi-button-neutral', 'click': L.ui.createHandlerFn(this, function(ev) { return doAction(ev, callReset); }) }, _('Reset')));
		} else if (isBlocked) {
			btns.push(E('button', { 'class': 'cbi-button cbi-button-positive', 'click': L.ui.createHandlerFn(this, function(ev) { return doAction(ev, callAllow); }) }, _('Allow')));
			btns.push(E('button', { 'class': 'cbi-button cbi-button-action', 'click': L.ui.createHandlerFn(this, function(ev) { return doAction(ev, function(p) { return callTicket(p, 30); }); }) }, '+30 ' + _('min')));
		} else {
			btns.push(E('button', { 'class': 'cbi-button cbi-button-negative', 'click': L.ui.createHandlerFn(this, function(ev) { return doAction(ev, callBlock); }) }, _('Block')));
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
					// Save and apply immediately so the profile is persisted to disk.
					// uci.apply() may timeout if fw4 reload takes >10s, but changes
					// are still staged and will apply. We catch the error to avoid
					// breaking the flow.
					return uci.save().then(function() {
						return uci.apply();
					}).catch(function(e) {
						console.log('uci.apply timeout (ignored):', e.message);
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
		btns.push(E('button', { 'class': 'cbi-button cbi-button-neutral', 'click': L.ui.createHandlerFn(this, function(ev) { return doAction(ev, callReset); }) }, _('Reset')));
	} else if (isBlocked) {
		btns.push(E('button', { 'class': 'cbi-button cbi-button-positive', 'click': L.ui.createHandlerFn(this, function(ev) { return doAction(ev, callAllow); }) }, _('Allow')));
		btns.push(E('button', { 'class': 'cbi-button cbi-button-action', 'click': L.ui.createHandlerFn(this, function(ev) { return doAction(ev, function(p) { return callTicket(p, 30); }); }) }, '+30 ' + _('min')));
	} else {
		btns.push(E('button', { 'class': 'cbi-button cbi-button-negative', 'click': L.ui.createHandlerFn(this, function(ev) { return doAction(ev, callBlock); }) }, _('Block')));
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
			}).then(function() {
				// Reload UCI state so the UI reflects fresh daemon state
				return uci.load('familywarden');
			});
		};

		var s = m.section(form.NamedSection, selectedProfile, 'profile', _('Profile Settings'));
		s.addremove = false;

		var nameOption = s.option(form.Value, 'name', _('Name'));
		nameOption.rmempty = false;

		var enabledOption = s.option(form.Flag, 'enabled', _('Enabled'));
		enabledOption.rmempty = false;
		enabledOption.default = '1';

		var policyOption = s.option(form.ListValue, 'policy', _('Access Policy'),
			_('"Unrestricted" allows all traffic. "Blocked" turns the internet off. "Scheduled" blocks by default and only allows access during the time windows below.'));
		policyOption.value('allow', _('Unrestricted'));
		policyOption.value('block', _('Blocked'));
		policyOption.value('scheduled', _('Scheduled'));

		var deviceOption = s.option(form.DynamicList, 'device', _('Devices'),
			_('Select known devices or enter MAC addresses manually.'));
		deviceOption.datatype = 'macaddr';
		deviceOption.rmempty = true;
		var macs = Object.keys(knownDevices).sort(function(a, b) {
			return knownDevices[a].localeCompare(knownDevices[b]);
		});
		macs.forEach(function(mac) {
			deviceOption.value(mac, knownDevices[mac]);
		});

		var dnsOption = s.option(form.Value, 'dns_profile', _('DNS Filtering'));
		dnsOption.rmempty = true;
		dnsOption.optional = true;
		dnsOption.description = _('Optional: AdGuard Home / DNS filter group name.');

		// --- Schedules for this profile ---
		// Inline TableSection (NOT GridSection): rows are edited in place with
		// no modal. The modal was the source of a bug where its map.save()
		// re-render left a stale policy widget that reverted policy on Save &
		// Apply. Inline editing makes that bug class structurally impossible.
		var pid = selectedProfile;
		var ss = m.section(form.TableSection, 'schedule', _('Schedules'),
			_('Internet is off by default in this mode. Add the time windows when access is allowed; any time outside every window is blocked.'));
		ss.addremove = true;
		ss.anonymous = true;
		ss.addbtntitle = _('Add schedule');

		ss.filter = function(section_id) {
			return uci.get('familywarden', section_id, 'profile') === pid;
		};

		// New schedules belong to the currently selected profile. Add the
		// section ourselves (to set profile=pid) then save \u2014 do NOT call the
		// prototype handleAdd, which would add a second empty section.
		ss.handleAdd = function(ev) {
			var id = uci.add('familywarden', 'schedule');
			uci.set('familywarden', id, 'profile', pid);
			return this.map.save(null, true);
		};

		// All options inline (no modalonly) \u2192 editable directly in the row.
		// Fields are optional: a freshly added row is briefly blank, and the
		// daemon ignores incomplete schedules (days/start/stop all required to
		// take effect). rmempty=true means a blank field is dropped on save
		// instead of raising "must not be empty"; validators tolerate empty.
		// Stock MultiValue (multi-select dropdown). A custom ui.Select checkbox
		// override was tried but its getValue() returned null → days were lost
		// on save; the stock dropdown round-trips reliably.
		var o = ss.option(form.MultiValue, 'days', _('Days'));
		o.value('mon', _('Mon'));
		o.value('tue', _('Tue'));
		o.value('wed', _('Wed'));
		o.value('thu', _('Thu'));
		o.value('fri', _('Fri'));
		o.value('sat', _('Sat'));
		o.value('sun', _('Sun'));
		o.rmempty = true;

		o = ss.option(form.Value, 'start', _('Start'));
		o.placeholder = '07:00';
		o.rmempty = true;
		o.validate = function(section_id, value) {
			if (value && !/^\d{2}:\d{2}$/.test(value)) return _('Use HH:MM format');
			return true;
		};

		o = ss.option(form.Value, 'stop', _('Stop'));
		o.placeholder = '20:00';
		o.rmempty = true;
		o.validate = function(section_id, value) {
			if (value && !/^\d{2}:\d{2}$/.test(value)) return _('Use HH:MM format');
			return true;
		};

		// --- Delete profile ---
		var self = this;
		var deleteSection = E('div', { 'class': 'fw-delete-section' }, [
			E('button', {
				'class': 'cbi-button cbi-button-negative',
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
					// Save and apply immediately so the deletion is persisted to disk.
					return uci.save().then(function() {
						return uci.apply();
					}).catch(function(e) {
						console.log('uci.apply timeout (ignored):', e.message);
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

			// Schedules only have an effect when policy is "By schedule"
			// (the daemon ignores them otherwise), so only show that section
			// when the policy is set accordingly. Toggle live as the user
			// changes the Access Policy dropdown, without needing to save.
			var scheduleNode = formEl.querySelector('#cbi-familywarden-schedule');
			if (!scheduleNode) {
				var secs = formEl.querySelectorAll('.cbi-section');
				scheduleNode = secs.length ? secs[secs.length - 1] : null;
			}
			var policyUI = policyOption.getUIElement(selectedProfile);
			function syncScheduleVisibility() {
				if (!scheduleNode) return;
				var show = policyUI && policyUI.getValue() === 'scheduled';
				scheduleNode.style.display = show ? '' : 'none';
			}
			if (policyUI && policyUI.node)
				policyUI.node.addEventListener('widget-change', syncScheduleVisibility);
			syncScheduleVisibility();

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
