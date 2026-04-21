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
	if (document.getElementById('fw-detail-styles')) return;
	var css = document.createElement('style');
	css.id = 'fw-detail-styles';
	css.textContent = '\
		.fw-detail-header{margin-bottom:24px}\
		.fw-back{display:inline-block;margin-bottom:12px;color:var(--text-color-medium,#6c757d);text-decoration:none;font-size:14px}\
		.fw-back:hover{color:var(--text-color-high,#333)}\
		.fw-status-bar{display:flex;align-items:center;gap:16px;padding:16px;\
			background:var(--background-color-high,#f8f9fa);border:1px solid var(--border-color-medium,#dee2e6);\
			border-radius:8px;flex-wrap:wrap}\
		.fw-status-dot{width:12px;height:12px;border-radius:50%;flex-shrink:0}\
		.fw-status-dot.allowed{background:#28a745;box-shadow:0 0 6px #28a745}\
		.fw-status-dot.blocked{background:#dc3545;box-shadow:0 0 6px #dc3545}\
		.fw-status-dot.override{background:#ffc107;box-shadow:0 0 6px #ffc107}\
		.fw-status-text{font-size:16px;font-weight:600;flex:1}\
		.fw-action-btn{padding:6px 14px;border:none;border-radius:4px;cursor:pointer;\
			font-size:13px;font-weight:500;color:white;transition:opacity .15s}\
		.fw-action-btn:hover{opacity:.85}\
		.fw-action-btn:disabled{opacity:.5;cursor:wait}\
		.fw-action-btn.allow{background:#28a745}\
		.fw-action-btn.block{background:#dc3545}\
		.fw-action-btn.reset{background:#6c757d}\
		.fw-action-btn.ticket{background:#17a2b8}\
	';
	document.head.appendChild(css);
}

var profileId = null;
var statusNode = null;

function refreshStatus() {
	if (!profileId || !statusNode) return Promise.resolve();
	return callStatus(profileId).then(function(data) {
		if (!data || !data.status) return;

		var dot = statusNode.querySelector('.fw-status-dot');
		var text = statusNode.querySelector('.fw-status-text');
		if (!dot || !text) return;

		var hasOverride = data.override && data.override !== 'none';
		var isBlocked = data.status.indexOf('blocked') >= 0;

		dot.className = 'fw-status-dot ' + (hasOverride ? 'override' : (isBlocked ? 'blocked' : 'allowed'));

		var label;
		if (hasOverride)
			label = _('Override: ') + (data.override === 'block' ? _('Blocked') : _('Allowed'));
		else if (isBlocked)
			label = _('Blocked');
		else
			label = _('Allowed');

		if (data.blocked_since)
			label += ' — ' + _('since') + ' ' + new Date(data.blocked_since).toLocaleTimeString();

		text.textContent = label;
	});
}

function doAction(ev, fn) {
	var btn = ev.currentTarget;
	btn.disabled = true;
	return fn(profileId).then(refreshStatus).then(function() { btn.disabled = false; });
}

return view.extend({
	load: function() {
		// Get profile ID from query string
		var params = new URLSearchParams(window.location.search);
		profileId = params.get('name');

		if (!profileId) {
			return Promise.resolve([null, null, null]);
		}

		return Promise.all([
			callStatus(profileId),
			callDHCPLeases(),
			callHostHints()
		]);
	},

	render: function(data) {
		// No profile selected — redirect to overview
		if (!profileId || !data[0]) {
			window.location = L.url('admin/services/familywarden/overview');
			return E('div', {}, _('Redirecting...'));
		}

		var status = data[0];
		var leases = data[1] || [];
		var hostHints = data[2] || {};

		injectStyles();

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

		// --- Status bar ---
		var hasOverride = status.override && status.override !== 'none';
		var isBlocked = status.status && status.status.indexOf('blocked') >= 0;
		var dotClass = hasOverride ? 'override' : (isBlocked ? 'blocked' : 'allowed');
		var statusLabel;
		if (hasOverride)
			statusLabel = _('Override: ') + (status.override === 'block' ? _('Blocked') : _('Allowed'));
		else if (isBlocked)
			statusLabel = _('Blocked');
		else
			statusLabel = _('Allowed');

		statusNode = E('div', { 'class': 'fw-status-bar' }, [
			E('span', { 'class': 'fw-status-dot ' + dotClass }),
			E('span', { 'class': 'fw-status-text' }, statusLabel),
			E('button', { 'class': 'fw-action-btn allow', 'click': function(ev) { return doAction(ev, callAllow); } }, _('Allow')),
			E('button', { 'class': 'fw-action-btn block', 'click': function(ev) { return doAction(ev, callBlock); } }, _('Block')),
			E('button', { 'class': 'fw-action-btn reset', 'click': function(ev) { return doAction(ev, callReset); } }, _('Reset')),
			E('button', { 'class': 'fw-action-btn ticket', 'click': function(ev) { return doAction(ev, function(p) { return callTicket(p, 30); }); } }, '+30 ' + _('min'))
		]);

		var header = E('div', { 'class': 'fw-detail-header' }, [
			E('a', { 'class': 'fw-back', 'href': L.url('admin/services/familywarden/overview') }, '\u2190 ' + _('Back to Overview')),
			E('h2', {}, status.name || profileId),
			statusNode
		]);

		// --- Profile settings form ---
		var m = new form.Map('familywarden');

		var s = m.section(form.NamedSection, profileId, 'profile', _('Profile Settings'));
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
		var ss = m.section(form.GridSection, 'schedule', _('Schedules'),
			_('Time windows for this profile. Block always wins over Allow if schedules overlap. Outside all schedules, access is blocked.'));
		ss.addremove = true;
		ss.anonymous = true;
		ss.addbtntitle = _('Add schedule');
		ss.modaltitle = function(section_id) {
			return _('Edit Schedule');
		};

		// Filter: only show schedules for this profile
		ss.filter = function(section_id) {
			return uci.get('familywarden', section_id, 'profile') === profileId;
		};

		// Auto-set profile on new schedules
		ss.handleAdd = function(ev) {
			var id = uci.add('familywarden', 'schedule');
			uci.set('familywarden', id, 'profile', profileId);
			return this.renderMoreOptionsModal(id);
		};

		// Move add button above table
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

		// Action column in table
		o = ss.option(form.ListValue, 'action', _('Action'));
		o.value('allow', _('Allow'));
		o.value('block', _('Block'));
		o.rmempty = false;
		o.default = 'allow';
		o.editable = true;

		// Hidden profile field (pre-filled, shown in modal for reference)
		o = ss.option(form.Value, 'profile', _('Profile'));
		o.modalonly = true;
		o.readonly = true;
		o.default = profileId;

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

		// --- Render ---
		return m.render().then(function(formEl) {
			poll.add(refreshStatus, 10);

			return E('div', {}, [
				header,
				formEl
			]);
		});
	}
});
