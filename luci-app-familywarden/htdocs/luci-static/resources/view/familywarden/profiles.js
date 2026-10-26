'use strict';
'require view';
'require form';
'require uci';
'require rpc';
'require poll';
'require ui';
'require familywarden.ulysses as ulysses';
'require familywarden.ui as fwui';

var callConfigurationOwner = rpc.declare({ object: 'familywarden', method: 'configuration_status', expect: { '': {} } });

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

var callDnsServices = rpc.declare({
	object: 'familywarden',
	method: 'dns_services',
	expect: { services: [] }
});

var callDnsStatus = rpc.declare({
	object: 'familywarden',
	method: 'dns_status',
	expect: { '': {} }
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
		fillStatus(statusNode, data);
	});
}

// Pact button, only on the captain's profile: make a pact, extend an "until"
// pact, or amend an ongoing one inside its window
function pactButtonLabel(id, p) {
	if (id !== ulysses.captainId()) return null;
	if (!ulysses.isBound(p)) return _('Make a pact');
	if (p.pact_type === 'until') return _('Extend pact');
	if (p.pact_in_window) return _('Amend pact');
	return null;
}

// Status dot + label + action buttons. Allow/ticket are hidden for the captain
// during a pact (the backend refuses them).
function fillStatus(node, data) {
	var hasOverride = data.override && data.override !== 'none';
	var isBlocked = data.status && data.status.indexOf('blocked') >= 0;
	var known = data.status === 'allowed' || data.status === 'blocked';
	var timed = data.override === 'allow' && +data.override_expires > 0;
	var label = !known ? _('Status unknown') : isBlocked ? _('Internet blocked') : _('Internet allowed');
	if (hasOverride && data.override_expires) {
		var remaining = Math.max(0, Math.round((data.override_expires - Date.now()/1000) / 60));
		if (remaining > 0)
			label += ' — ' + remaining + ' min ' + _('remaining');
	}

	var btns = [];
	if (!known) {
		// An unknown reply must not look allowed or offer stale quick actions.
	} else if (hasOverride) {
		btns.push(E('button', { 'type': 'button', 'class': 'cbi-button ' + (timed ? 'fw-btn-revoke' : 'fw-btn-reset'), 'click': L.ui.createHandlerFn(this, function(ev) { return doAction(ev, callReset); }) }, timed ? _('Cancel ticket') : _('Reset override')));
	} else if (isBlocked) {
		if (!ulysses.isBound(data)) {
			btns.push(E('button', { 'type': 'button', 'class': 'cbi-button fw-btn-allow', 'click': L.ui.createHandlerFn(this, function(ev) { return doAction(ev, callAllow); }) }, _('Allow')));
			btns.push(E('button', { 'type': 'button', 'class': 'cbi-button fw-btn-grant', 'click': L.ui.createHandlerFn(this, function(ev) { return doAction(ev, function(p) { return callTicket(p, 30); }); }) }, '+30 ' + _('min')));
		}
	} else {
		btns.push(E('button', { 'type': 'button', 'class': 'cbi-button fw-btn-block', 'click': L.ui.createHandlerFn(this, function(ev) { return doAction(ev, callBlock); }) }, _('Block')));
	}
	var pactLabel = known ? pactButtonLabel(selectedProfile, data) : null;
	if (pactLabel)
		btns.push(E('button', { 'type': 'button', 'class': 'cbi-button fw-btn-pact', 'click': L.ui.createHandlerFn(this, function(ev) {
			return doAction(ev, function() { return ulysses.showPactModal(data.name, data); });
		}) }, [fwui.icon('mast'), pactLabel]));

	while (node.firstChild) node.removeChild(node.firstChild);
	node.setAttribute('data-status', known ? data.status : 'unknown');
	node.appendChild(E('span', { 'class': 'fw-status-icon' }, fwui.icon(!known ? 'info' : !isBlocked ? 'check'
		: data.override !== 'block' && data.policy === 'scheduled' ? 'moon' : 'ban')));
	node.appendChild(E('span', { 'class': 'fw-status-text' }, label));
	if (hasOverride && known) node.appendChild(E('span', { 'class': 'fw-tag-override' }, [fwui.icon('hand'), timed ? _('Timed override') : _('Override')]));
	node.appendChild(E('div', { 'class': 'fw-profile-actions' }, btns));
	var uNote = ulysses.note(data, fwui.icon('mast'));
	if (uNote) node.appendChild(uNote);
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
		'placeholder': _('e.g. Alex'),
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
					// uci.apply() commits with a rollback timer and resolves BEFORE
					// the detached confirm poll runs — redirecting at that point
					// aborts the confirm and the router reverts the commit ~10s
					// later. Confirm explicitly before navigating away.
					return uci.save().then(function() {
						return uci.apply().then(function() {
							return uci.callConfirm();
						});
					}).catch(function(e) {
						// Even on a client-side timeout the commit may have landed
						// with the rollback timer armed — confirm it anyway.
						console.log('uci.apply error (confirming anyway):', e.message || e);
						return uci.callConfirm().catch(function() {});
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
			'aria-current': isActive ? 'page' : null,
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

	return E('div', { 'class': 'fw-tabs', 'role': 'navigation', 'aria-label': _('Profiles') }, tabs);
}

function buildStatusBar(status) {
	statusNode = E('div', { 'class': 'fw-status-bar' });
	fillStatus(statusNode, status);
	return statusNode;
}

return view.extend({

	load: function() {
		// Save & Apply belongs to the view, not form.Map. Reload only after confirmation.
		document.addEventListener('uci-applied', function() {
			callReload().catch(function(error) {
				ui.addNotification(null, E('p', {}, _('Could not reload FamilyWarden: %s').format(error.message)), 'error');
			});
		});
		return Promise.all([
			callList(),
			uci.load('familywarden'),
			callDHCPLeases(),
			callHostHints(),
			// Optional: AdGuard Home may not be configured (or reachable)
			L.resolveDefault(callDnsServices(), []),
			L.resolveDefault(callDnsStatus(), {}),
			callConfigurationOwner()
		]).then(function(data) {
			if (!data[6] || !['legacy', 'core'].includes(data[6].owner)) throw new Error(_('Configuration ownership unavailable'));
			return data;
		});
	},

	render: function(data) {
		if (data[6].owner === 'core') {
			this.handleSave = this.handleSaveApply = this.handleReset = null;
			return E('div', {}, [ E('h2', {}, _('Core-managed configuration')), E('p', {}, _('Legacy UCI editing is disabled. Use the core configuration API with revision checks and confirmation.')) ]);
		}
		var allProfiles = data[0] || {};
		var leases = data[2] || [];
		var hostHints = data[3] || {};
		var dnsServices = data[4] || [];
		var dnsStatus = data[5] || {};

		fwui.loadStyles('overview');
		fwui.loadStyles('pages');

		selectedProfile = getSelectedProfile();

		// Default to first profile if none selected
		var profileIds = Object.keys(allProfiles);
		if (!selectedProfile && profileIds.length > 0)
			selectedProfile = profileIds[0];

		// No profiles at all
		if (!selectedProfile || !allProfiles[selectedProfile]) {
			var tabs = buildTabs(allProfiles, null);
			return E('div', { 'id': 'fw-profiles' }, [
				fwui.header(_('Profiles'), _('Devices, allowed hours and DNS protection for each profile.')),
				tabs,
				E('div', { 'class': 'fw-empty-profile fw-empty' }, [
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

		// The captain's profile is frozen for the pact's lifetime, except in
		// its weekly window (the backend ignores edits anyway): read-only, no
		// schedule add/delete. DNS stays editable: during a pact the backend
		// applies additions at once and removals only when the pact ends.
		var frozen = ulysses.isFrozen(status);

		var s = m.section(form.NamedSection, selectedProfile, 'profile', _('Profile and internet access'));
		s.addremove = false;

		var nameOption = s.option(form.Value, 'name', _('Name'));
		nameOption.rmempty = false;
		nameOption.readonly = frozen;

		var enabledOption = s.option(form.Flag, 'enabled', _('Enabled'));
		enabledOption.rmempty = false;
		enabledOption.default = '1';
		enabledOption.readonly = frozen;

		var policyOption = s.option(form.ListValue, 'policy', _('Access Policy'),
			_('"Unrestricted" allows all traffic. "Blocked" turns the internet off. "Scheduled" blocks by default and only allows access during the time windows below.'));
		policyOption.value('allow', _('Unrestricted'));
		policyOption.value('block', _('Blocked'));
		policyOption.value('scheduled', _('Scheduled'));
		policyOption.readonly = frozen;

		var blockRouterOption = s.option(form.Flag, 'block_router',
			_('Block router access when blocked'),
			_('Also drop incoming TCP to the router (LuCI, SSH) when this profile is blocked. ' +
				'Has no effect while the profile is allowed, and leaves ICMP/UDP/ARP intact so the device still gets a DHCP lease and DNS.'));
		blockRouterOption.rmempty = true;
		blockRouterOption.default = '0';
		blockRouterOption.readonly = frozen;


		var deviceOption = s.option(form.DynamicList, 'device', _('Devices'),
			_('Select known devices or enter MAC addresses manually. Names come from DHCP and may be missing; a lease does not prove a device is online.'));
		deviceOption.datatype = 'macaddr';
		deviceOption.rmempty = true;
		deviceOption.readonly = frozen;
		var macs = Object.keys(knownDevices).sort(function(a, b) {
			return knownDevices[a].localeCompare(knownDevices[b]);
		});
		macs.forEach(function(mac) {
			deviceOption.value(mac, knownDevices[mac]);
		});

		// A display-only divider keeps one native NamedSection and unique field IDs.
		var dnsHeading = s.option(form.DummyValue, '_dns_heading');
		dnsHeading.renderWidget = function() {
			return E('div', { 'class': 'fw-form-heading' }, [
				E('h4', {}, _('DNS protection on approved phones')),
				E('p', {}, _('These settings configure this profile in AdGuard Home. Router sync does not prove that a phone has applied its policy.'))
			]);
		};

		// DNS filtering: an AdGuard Home client of its own for this profile.
		// Phones approved for the profile (Phones page) use strict
		// DNS-over-TLS to <ClientID>.<DNS domain>; see Settings.
		var dnsOption = s.option(form.Flag, 'dns', _('Strict DNS on this profile\'s phones'),
			_('Approved phones of this profile always resolve through AdGuard Home with the protections below, at home and away. There is no unfiltered fallback if the resolver can\'t be reached. Needs the DNS settings under Settings.'));
		dnsOption.rmempty = true;
		dnsOption.default = '0';
		if (ulysses.isBound(status))
			dnsOption.description += ' ' + _('Bound by the pact: DNS can only be tightened. Added protections and services apply now; removing them or switching DNS off waits until the pact ends (or its weekly window).');

		var protOption = s.option(form.MultiValue, 'dns_protection', _('DNS protections'),
			_('Settings of this profile\'s AdGuard Home client. "Filter lists" are the lists configured globally in AdGuard Home; AdGuard Home has no separate lists per client.'));
		protOption.value('filter_lists', _('Filter lists'));
		protOption.value('safe_browsing', _('Malware and phishing protection'));
		protOption.value('parental', _('Parental protection (adult sites)'));
		protOption.value('safe_search', _('SafeSearch (search engines and YouTube)'));
		protOption.default = 'filter_lists safe_browsing';
		protOption.depends('dns', '1');

		var svcOption;
		if (dnsServices.length) {
			svcOption = s.option(form.MultiValue, 'dns_service', _('Blocked services'),
				_('Services AdGuard Home blocks for this profile.'));
			dnsServices.slice().sort(function(a, b) { return a.name.localeCompare(b.name); }).forEach(function(svc) {
				svcOption.value(svc.id, svc.name);
			});
		} else {
			svcOption = s.option(form.DynamicList, 'dns_service', _('Blocked services'),
				_('AdGuard Home service IDs (e.g. youtube). The list of services appears here once AdGuard Home is reachable.'));
			svcOption.datatype = 'and(minlength(1),maxlength(64))';
		}
		svcOption.rmempty = true;
		svcOption.depends('dns', '1');

		var idOption = s.option(form.DummyValue, '_dns_identity', _('DNS identity'));
		idOption.depends('dns', '1');
		idOption.cfgvalue = function(section_id) {
			var synced = (dnsStatus.profiles || {})[section_id];
			if (synced)
				return _('%s (active in AdGuard Home)').format(synced.hostname);
			var id = uci.get('familywarden', section_id, 'dns_client_id');
			if (dnsStatus.error)
				return _('Not active: %s').format(dnsStatus.error);
			return id ? _('ClientID %s, waiting for sync').format(id) : _('Created on the next sync from the profile name, and kept when the profile is renamed.');
		};

		// --- Screen time and apps on this profile's phones (Family Link model, D9) ---
		var appsHeading = s.option(form.DummyValue, '_apps_heading');
		appsHeading.renderWidget = function() {
			return E('div', { 'class': 'fw-form-heading' }, [
				E('h4', {}, _('Screen time and apps on approved phones')),
				E('p', {}, _('Enforced by the FamilyWarden app on this profile\'s phones, also away from home. Phones apply changes at their next sync.'))
			]);
		};
		var appsOption = s.option(form.Flag, 'apps', _('Screen time and app limits'));
		appsOption.rmempty = true;
		appsOption.default = '0';
		appsOption.readonly = frozen;

		function minutesOption(name, title, description) {
			var o = s.option(form.Value, name, title, description);
			o.datatype = 'range(0,1440)';
			o.placeholder = _('No limit');
			o.rmempty = true;
			o.readonly = frozen;
			o.depends('apps', '1');
			return o;
		}
		minutesOption('screen_minutes', _('Daily screen time, every day (minutes)'),
			_('Total time on the phone per day. When it is used up, every app pauses except calls, the alarm clock and always-allowed apps.'));
		minutesOption('screen_weekday', _('Weekdays, Mon–Fri (minutes)'), _('Leave empty to use the every-day value.'));
		minutesOption('screen_weekend', _('Weekend, Sat–Sun (minutes)'), _('Leave empty to use the every-day value.'));

		var newAppsOption = s.option(form.Flag, 'apps_suspend_new', _('Pause new apps until approved'),
			_('A newly installed app is paused as soon as Android reports it. In rare cases it can open briefly first. Approve it by adding it under App limits.'));
		newAppsOption.rmempty = false;
		newAppsOption.default = '1';
		newAppsOption.readonly = frozen;
		newAppsOption.depends('apps', '1');

		var packageRe = /^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$/;
		function validPackage(section_id, value) {
			return (!value || packageRe.test(value)) ? true : _('Use an Android package name, e.g. com.example.app');
		}
		var alwaysOption = s.option(form.DynamicList, 'always_allowed', _('Always allowed apps'),
			_('Never count towards screen time and keep working after the daily limit and during downtime (Family Link "Unlimited time"). Android package names.'));
		alwaysOption.validate = validPackage;
		alwaysOption.rmempty = true;
		alwaysOption.readonly = frozen;
		alwaysOption.depends('apps', '1');

		// --- Schedules for this profile ---
		// Inline TableSection (NOT GridSection): rows are edited in place with
		// no modal. The modal was the source of a bug where its map.save()
		// re-render left a stale policy widget that reverted policy on Save &
		// Apply. Inline editing makes that bug class structurally impossible.
		var pid = selectedProfile;
		var ss = m.section(form.TableSection, 'schedule', _('Allowed internet windows'),
			_('Internet is off by default in this mode. Add the time windows when access is allowed; any time outside every window is blocked.'));
		ss.addremove = !frozen;
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
		// Days renders as 7 plain inline checkboxes. A ui.Select checkbox-group
		// widget was tried first, but its getValue() returned null so days were
		// silently lost on save. Instead we own BOTH ends of the contract:
		// renderWidget emits plain checkboxes, and formvalue reads the checked
		// boxes straight from the DOM — CBI's save path then sees exactly what
		// the user ticked, with no widget class in between to lose the value.
		var dayKeys = [ 'mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun' ];
		var dayLabels = { mon: _('Mon'), tue: _('Tue'), wed: _('Wed'),
			thu: _('Thu'), fri: _('Fri'), sat: _('Sat'), sun: _('Sun') };

		function addDays(sec) {
		var o = sec.option(form.MultiValue, 'days', _('Days'));
		dayKeys.forEach(function(d) { o.value(d, dayLabels[d]); });
		o.rmempty = true;
		o.renderWidget = function(section_id, option_index, cfgvalue) {
			var selected = L.toArray(cfgvalue);
			return E('div', {
				'id': this.cbid(section_id),
				'class': 'fw-days'
			}, dayKeys.map(function(d) {
				return E('label', { 'class': 'fw-day' }, [
					E('input', {
						'type': 'checkbox',
						'data-day': d,
						'disabled': frozen ? '' : null,
						'checked': (selected.indexOf(d) > -1) ? '' : null
					}),
					dayLabels[d]
				]);
			}));
		};
		o.formvalue = function(section_id) {
			var node = document.getElementById(this.cbid(section_id));
			if (!node) return null;
			var vals = [];
			node.querySelectorAll('input[data-day]:checked').forEach(function(cb) {
				vals.push(cb.getAttribute('data-day'));
			});
			return vals.length ? vals : null;
		};
		return o;
		}
		addDays(ss);

		var o = ss.option(form.Value, 'start', _('Start'));
		o.readonly = frozen;
		o.placeholder = '07:00';
		o.rmempty = true;
		o.validate = function(section_id, value) {
			if (value && !/^\d{2}:\d{2}$/.test(value)) return _('Use HH:MM format');
			return true;
		};

		o = ss.option(form.Value, 'stop', _('Stop'));
		o.readonly = frozen;
		o.placeholder = '20:00';
		o.rmempty = true;
		o.validate = function(section_id, value) {
			if (value && !/^\d{2}:\d{2}$/.test(value)) return _('Use HH:MM format');
			return true;
		};

		// --- App limits: per-app daily minutes or blocked (also approves the app) ---
		function profileSection(type, title, description, addTitle) {
			var sec = m.section(form.TableSection, type, title, description);
			sec.addremove = !frozen;
			sec.anonymous = true;
			sec.addbtntitle = addTitle;
			sec.filter = function(section_id) { return uci.get('familywarden', section_id, 'profile') === pid; };
			sec.handleAdd = function(ev) {
				var id = uci.add('familywarden', type);
				uci.set('familywarden', id, 'profile', pid);
				return this.map.save(null, true);
			};
			return sec;
		}
		var as = profileSection('app', _('App limits'),
			_('Each app listed here is approved. Set minutes per day, or block it. Leave minutes empty for no limit.'), _('Add app'));
		o = as.option(form.Value, 'package', _('App (package name)'));
		o.placeholder = 'com.example.app';
		o.validate = validPackage;
		o.readonly = frozen;
		o.rmempty = true;
		o = as.option(form.Value, 'minutes', _('Minutes per day'));
		o.datatype = 'range(0,1440)';
		o.placeholder = _('No limit');
		o.readonly = frozen;
		o.rmempty = true;
		o = as.option(form.Flag, 'blocked', _('Blocked'));
		o.readonly = frozen;
		o.rmempty = true;

		var ds = profileSection('app_downtime', _('Downtime'),
			_('During downtime every app on the phones pauses except calls, the alarm clock and always-allowed apps (Family Link bedtime). A window that ends before it starts runs overnight.'), _('Add downtime'));
		addDays(ds);
		o = ds.option(form.Value, 'start', _('Start'));
		o.placeholder = '21:00';
		o.readonly = frozen;
		o.rmempty = true;
		o.validate = function(section_id, value) {
			return (!value || /^\d{2}:\d{2}$/.test(value)) ? true : _('Use HH:MM format');
		};
		o = ds.option(form.Value, 'stop', _('Stop'));
		o.placeholder = '07:00';
		o.readonly = frozen;
		o.rmempty = true;
		o.validate = function(section_id, value) {
			return (!value || /^\d{2}:\d{2}$/.test(value)) ? true : _('Use HH:MM format');
		};

		// --- Delete profile ---
		var self = this;
		var deleteSection = E('div', { 'class': 'fw-delete-section' }, [
			E('h3', {}, _('Delete profile')),
			E('p', {}, _('Removes this profile and its internet schedules. This action applies immediately.')),
			E('button', {
				'class': 'cbi-button cbi-button-negative',
				// createHandlerFn keeps the button disabled/spinning while the
				// save/apply/reload/redirect chain runs, preventing double-submit.
				'click': L.ui.createHandlerFn(self, function() {
					if (!confirm(_('Delete this profile and all its schedules?')))
						return;

					// Remove schedules for this profile
					[ 'schedule', 'app', 'app_downtime' ].forEach(function(type) {
						uci.sections('familywarden', type, function(s) {
							if (s.profile === pid)
								uci.remove('familywarden', s['.name']);
						});
					});
					uci.remove('familywarden', pid);
					// Save and apply immediately so the deletion is persisted to disk.
					// Confirm explicitly before redirecting (see create handler):
					// uci.apply() resolves before its detached confirm poll runs,
					// and navigating away unconfirmed makes the router roll back.
					return uci.save().then(function() {
						return uci.apply().then(function() {
							return uci.callConfirm();
						});
					}).catch(function(e) {
						console.log('uci.apply error (confirming anyway):', e.message || e);
						return uci.callConfirm().catch(function() {});
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

			return E('div', { 'id': 'fw-profiles' }, [
				fwui.header(_('Profiles'), _('Devices, allowed hours and DNS protection for each profile. Form edits are staged until Save & Apply.')),
				tabs,
				statusBar,
				E('p', { 'class': 'fw-immediate-note' }, _('Status and quick actions above are live. Form edits below do not change access until Save & Apply.')),
				frozen ? E('div', { 'class': 'fw-profile-lock' }, [
					E('strong', {}, _('Held by the pact')),
					E('p', {}, _('Internet rules, devices and schedules are read-only outside the amendment window. DNS stays editable: protections can be added now; loosening waits until the pact ends or its window. Phone bindings stay frozen throughout the pact.'))
				]) : '',
				formEl,
				ulysses.isFrozen(status) ? '' : deleteSection
			]);
		});
	}
});
