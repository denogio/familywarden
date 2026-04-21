'use strict';
'require view';
'require rpc';
'require uci';
'require poll';

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

var dayNames = ['sun', 'mon', 'tue', 'wed', 'thu', 'fri', 'sat'];
var dayLabels = { mon: 'Mon', tue: 'Tue', wed: 'Wed', thu: 'Thu', fri: 'Fri', sat: 'Sat', sun: 'Sun' };

function timeToMin(hhmm) {
	var parts = (hhmm || '').split(':');
	return (parseInt(parts[0], 10) || 0) * 60 + (parseInt(parts[1], 10) || 0);
}

function minToTime(m) {
	var h = Math.floor(m / 60) % 24;
	var min = m % 60;
	return (h < 10 ? '0' : '') + h + ':' + (min < 10 ? '0' : '') + min;
}

function isScheduleActiveNow(sched) {
	var now = new Date();
	var today = dayNames[now.getDay()];
	var nowMin = now.getHours() * 60 + now.getMinutes();

	var days = sched.days || '';
	if (typeof days === 'string') days = days.split(/\s+/);
	if (days.indexOf(today) < 0) return false;

	var startMin = timeToMin(sched.start);
	var stopMin = timeToMin(sched.stop);

	if (startMin < stopMin)
		return nowMin >= startMin && nowMin < stopMin;
	else
		return nowMin >= startMin || nowMin < stopMin;
}

// Calculate minutes until next schedule boundary for a profile
function getNextChange(schedules, currentlyBlocked) {
	var now = new Date();
	var today = dayNames[now.getDay()];
	var nowMin = now.getHours() * 60 + now.getMinutes();

	var nearest = Infinity;
	var nearestLabel = '';

	schedules.forEach(function(s) {
		var days = s.days || '';
		if (typeof days === 'string') days = days.split(/\s+/);
		if (days.indexOf(today) < 0) return;

		var startMin = timeToMin(s.start);
		var stopMin = timeToMin(s.stop);
		var active = isScheduleActiveNow(s);

		// Minutes until this schedule starts or stops
		var boundary, label;
		if (active) {
			// It will stop
			boundary = stopMin > nowMin ? (stopMin - nowMin) : (1440 - nowMin + stopMin);
			label = s.action === 'allow' ? _('Allow ends') : _('Block ends');
		} else {
			// It will start
			boundary = startMin > nowMin ? (startMin - nowMin) : (1440 - nowMin + startMin);
			label = s.action === 'allow' ? _('Allow starts') : _('Block starts');
		}

		if (boundary < nearest) {
			nearest = boundary;
			nearestLabel = label;
		}
	});

	if (nearest === Infinity) return null;

	var changeTime = new Date(now.getTime() + nearest * 60000);
	return {
		minutes: nearest,
		label: nearestLabel,
		time: minToTime(changeTime.getHours() * 60 + changeTime.getMinutes())
	};
}

function injectStyles() {
	if (document.getElementById('fw-styles')) return;
	var css = document.createElement('style');
	css.id = 'fw-styles';
	css.textContent = '\
		.fw-summary{display:flex;gap:16px;margin-bottom:24px;flex-wrap:wrap}\
		.fw-summary-card{flex:1;min-width:140px;text-align:center;padding:16px;border-radius:8px;\
			background:var(--background-color-high,#f8f9fa);border:1px solid var(--border-color-medium,#dee2e6)}\
		.fw-summary-label{color:var(--text-color-medium,#6c757d);font-size:13px;text-transform:uppercase;letter-spacing:.5px}\
		.fw-summary-value{font-size:28px;font-weight:bold;margin-top:4px}\
		.fw-summary-value.online{color:#28a745}.fw-summary-value.offline{color:#dc3545}\
		.fw-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(380px,1fr));gap:16px}\
		.fw-card{border-radius:8px;overflow:hidden;\
			background:var(--background-color-high,white);border:1px solid var(--border-color-medium,#dee2e6);\
			transition:box-shadow .2s}\
		.fw-card:hover{box-shadow:0 2px 12px rgba(0,0,0,.08)}\
		.fw-card-green{border-left:4px solid #28a745}\
		.fw-card-red{border-left:4px solid #dc3545}\
		.fw-card-yellow{border-left:4px solid #ffc107}\
		.fw-card-head{padding:14px 16px;border-bottom:1px solid var(--border-color-medium,#dee2e6);\
			display:flex;justify-content:space-between;align-items:center}\
		.fw-card-head h3{margin:0;font-size:17px}\
		.fw-card-body{padding:12px 16px}\
		.fw-row{display:flex;justify-content:space-between;padding:6px 0;\
			border-bottom:1px solid var(--border-color-low,#f1f3f4);font-size:14px}\
		.fw-row:last-child{border-bottom:none}\
		.fw-row-label{color:var(--text-color-medium,#6c757d)}\
		.fw-row-value{font-weight:500}\
		.fw-card-foot{padding:10px 16px;border-top:1px solid var(--border-color-medium,#dee2e6);\
			background:var(--background-color-medium,#f8f9fa);display:flex;gap:8px;flex-wrap:wrap;justify-content:flex-end}\
		.fw-btn{padding:7px 14px;border:none;border-radius:4px;cursor:pointer;\
			font-size:13px;font-weight:500;color:white;max-width:115px;transition:opacity .15s}\
		.fw-btn:hover{opacity:.85}.fw-btn:disabled{opacity:.5;cursor:wait}\
		.fw-btn-allow{background:#28a745}.fw-btn-block{background:#dc3545}\
		.fw-btn-reset{background:#6c757d}.fw-btn-ticket{background:#17a2b8}\
		.fw-badge{display:inline-block;padding:3px 10px;border-radius:12px;\
			font-size:11px;font-weight:600;color:white}\
		.fw-badge-green{background:#28a745}.fw-badge-red{background:#dc3545}\
		.fw-badge-yellow{background:#ffc107;color:#333}\
		.fw-sched-section{margin-top:10px;padding-top:8px;border-top:1px dashed var(--border-color-medium,#dee2e6)}\
		.fw-sched-row{display:flex;align-items:center;gap:8px;padding:3px 0;font-size:13px}\
		.fw-sched-dot{width:8px;height:8px;border-radius:50%;flex-shrink:0}\
		.fw-sched-dot.active{background:#28a745;box-shadow:0 0 4px #28a745}\
		.fw-sched-dot.inactive{background:var(--text-color-medium,#adb5bd)}\
		.fw-sched-text{color:var(--text-color-medium,#6c757d)}\
		.fw-sched-text.active{color:var(--text-color-high,#333);font-weight:500}\
		.fw-next-change{font-size:13px;color:var(--text-color-medium,#6c757d);padding:6px 0;font-style:italic}\
		.fw-empty{text-align:center;padding:60px 20px;color:var(--text-color-medium,#6c757d)}\
		.fw-updated{text-align:right;font-size:12px;color:var(--text-color-medium,#999);margin-top:16px}\
		.fw-topbar{display:flex;justify-content:space-between;align-items:center;margin-bottom:16px}\
		.fw-add-btn{padding:8px 18px;border:none;border-radius:4px;cursor:pointer;\
			font-size:14px;font-weight:500;color:white;background:#007bff;text-decoration:none}\
		.fw-add-btn:hover{opacity:.85;color:white}\
	';
	document.head.appendChild(css);
}

function statusBadge(status, override) {
	if (override && override !== 'none')
		return E('span', { 'class': 'fw-badge fw-badge-yellow' },
			override === 'block' ? _('Override: Blocked') : _('Override: Allowed'));
	if (status && status.indexOf('blocked') >= 0)
		return E('span', { 'class': 'fw-badge fw-badge-red' }, _('Blocked'));
	return E('span', { 'class': 'fw-badge fw-badge-green' }, _('Allowed'));
}

function doAction(ev, profile, fn) {
	var btn = ev.currentTarget;
	btn.disabled = true;
	return fn(profile).then(refreshDashboard).then(function() { btn.disabled = false; });
}

var dashboardNode = null;
var scheduleData = {};

function refreshDashboard() {
	return Promise.all([
		callList(),
		uci.load('familywarden')
	]).then(function(results) {
		if (!dashboardNode) return;
		var profiles = results[0];

		// Rebuild schedule data from UCI
		scheduleData = {};
		uci.sections('familywarden', 'schedule', function(s) {
			var prof = s.profile;
			if (!prof) return;
			if (!scheduleData[prof]) scheduleData[prof] = [];
			scheduleData[prof].push({
				days: s.days,
				start: s.start,
				stop: s.stop,
				action: s.action || 'allow'
			});
		});

		var old = dashboardNode.querySelector('.fw-content');
		if (old) old.parentNode.removeChild(old);
		dashboardNode.appendChild(buildContent(profiles, scheduleData));
	});
}

function buildScheduleRows(profileId, schedules) {
	if (!schedules || schedules.length === 0)
		return E('div', { 'class': 'fw-sched-section' },
			E('div', { 'class': 'fw-sched-row' },
				E('span', { 'class': 'fw-sched-text' }, _('No schedules configured'))));

	var rows = schedules.map(function(s) {
		var active = isScheduleActiveNow(s);
		var days = s.days || '';
		if (typeof days === 'string') days = days.split(/\s+/);
		var dayStr = days.map(function(d) { return dayLabels[d] || d; }).join(', ');
		var actionIcon = s.action === 'allow' ? '\u2714' : '\u2718';
		var label = dayStr + ' ' + (s.start || '?') + '-' + (s.stop || '?') + ' ' + actionIcon + ' ' + s.action;

		return E('div', { 'class': 'fw-sched-row' }, [
			E('span', { 'class': 'fw-sched-dot ' + (active ? 'active' : 'inactive') }),
			E('span', { 'class': 'fw-sched-text ' + (active ? 'active' : '') }, label)
		]);
	});

	var nextChange = getNextChange(schedules);
	if (nextChange) {
		var hrs = Math.floor(nextChange.minutes / 60);
		var mins = nextChange.minutes % 60;
		var inStr = hrs > 0 ? (hrs + 'h ' + mins + 'm') : (mins + 'm');
		rows.push(E('div', { 'class': 'fw-next-change' },
			nextChange.label + ' ' + _('at') + ' ' + nextChange.time + ' (' + _('in') + ' ' + inStr + ')'));
	}

	return E('div', { 'class': 'fw-sched-section' }, rows);
}

function buildContent(profiles, schedsByProfile) {
	var keys = Object.keys(profiles || {});
	var allowed = 0, blocked = 0;
	keys.forEach(function(k) {
		if (profiles[k].status && profiles[k].status.indexOf('blocked') >= 0) blocked++;
		else allowed++;
	});

	var now = new Date();
	var timeStr = (now.getHours() < 10 ? '0' : '') + now.getHours() + ':' +
	             (now.getMinutes() < 10 ? '0' : '') + now.getMinutes();

	var summary = E('div', { 'class': 'fw-summary' }, [
		E('div', { 'class': 'fw-summary-card' }, [
			E('div', { 'class': 'fw-summary-label' }, _('Profiles')),
			E('div', { 'class': 'fw-summary-value' }, String(keys.length))
		]),
		E('div', { 'class': 'fw-summary-card' }, [
			E('div', { 'class': 'fw-summary-label' }, _('Online')),
			E('div', { 'class': 'fw-summary-value online' }, String(allowed))
		]),
		E('div', { 'class': 'fw-summary-card' }, [
			E('div', { 'class': 'fw-summary-label' }, _('Offline')),
			E('div', { 'class': 'fw-summary-value offline' }, String(blocked))
		]),
		E('div', { 'class': 'fw-summary-card' }, [
			E('div', { 'class': 'fw-summary-label' }, _('Time')),
			E('div', { 'class': 'fw-summary-value', 'style': 'font-size:24px' },
				dayLabels[dayNames[now.getDay()]] + ' ' + timeStr)
		])
	]);

	var cards = [];
	if (keys.length === 0) {
		cards.push(E('div', { 'class': 'fw-empty' }, [
			E('p', {}, _('No profiles configured yet.')),
			E('a', { 'href': L.url('admin/services/familywarden/profiles') }, _('Create your first profile'))
		]));
	}

	keys.forEach(function(id) {
		var p = profiles[id];
		var isBlocked = p.status && p.status.indexOf('blocked') >= 0;
		var hasOverride = p.override && p.override !== 'none';
		var borderClass = hasOverride ? 'fw-card-yellow' : (isBlocked ? 'fw-card-red' : 'fw-card-green');

		var devices = Array.isArray(p.devices) ? p.devices : [];
		var deviceEls = devices.length > 0
			? devices.map(function(d) { return E('code', { 'style': 'margin-right:6px' }, d); })
			: [ E('em', {}, _('No devices')) ];

		var rows = [
			E('div', { 'class': 'fw-row' }, [
				E('span', { 'class': 'fw-row-label' }, _('Policy')),
				E('span', { 'class': 'fw-row-value' }, p.policy)
			]),
			E('div', { 'class': 'fw-row' }, [
				E('span', { 'class': 'fw-row-label' }, _('Devices')),
				E('span', {}, deviceEls)
			])
		];

		if (hasOverride) {
			var overrideText = p.override;
			if (p.override_expires) {
				var remaining = Math.max(0, Math.round((p.override_expires - Date.now()/1000) / 60));
				if (remaining > 0)
					overrideText += ' (' + remaining + ' min ' + _('remaining') + ')';
			}
			rows.push(E('div', { 'class': 'fw-row' }, [
				E('span', { 'class': 'fw-row-label' }, _('Override')),
				E('span', { 'class': 'fw-row-value', 'style': 'color:#ffc107' }, overrideText)
			]));
		}

		if (p.blocked_since) {
			rows.push(E('div', { 'class': 'fw-row' }, [
				E('span', { 'class': 'fw-row-label' }, _('Blocked since')),
				E('span', { 'class': 'fw-row-value' }, new Date(p.blocked_since).toLocaleTimeString())
			]));
		}

		// Schedule info for this profile
		var profileSchedules = schedsByProfile[id] || [];
		if (p.policy === 'scheduled') {
			rows.push(buildScheduleRows(id, profileSchedules));
		}

		var profileUrl = L.url('admin/services/familywarden/profiles') + '?name=' + encodeURIComponent(id);

		cards.push(E('div', { 'class': 'fw-card ' + borderClass }, [
			E('div', { 'class': 'fw-card-head' }, [
				E('h3', {}, E('a', { 'href': profileUrl, 'style': 'text-decoration:none;color:inherit' }, p.name || id)),
				statusBadge(p.status, p.override)
			]),
			E('div', { 'class': 'fw-card-body' }, rows),
			(function() {
				var footBtns = [];
				if (hasOverride) {
					footBtns.push(E('button', { 'class': 'fw-btn fw-btn-reset', 'click': function(ev) { return doAction(ev, id, callReset); } }, _('Reset')));
				} else if (isBlocked) {
					footBtns.push(E('button', { 'class': 'fw-btn fw-btn-allow', 'click': function(ev) { return doAction(ev, id, callAllow); } }, _('Allow')));
					footBtns.push(E('button', { 'class': 'fw-btn fw-btn-ticket', 'click': function(ev) { return doAction(ev, id, function(p) { return callTicket(p, 30); }); } }, '+30 ' + _('min')));
				} else {
					footBtns.push(E('button', { 'class': 'fw-btn fw-btn-block', 'click': function(ev) { return doAction(ev, id, callBlock); } }, _('Block')));
				}
				return E('div', { 'class': 'fw-card-foot' }, footBtns);
			})()
		]));
	});

	var updated = E('div', { 'class': 'fw-updated' },
		_('Updated') + ' ' + now.toLocaleTimeString());

	return E('div', { 'class': 'fw-content' }, [
		summary,
		E('div', { 'class': 'fw-grid' }, cards),
		updated
	]);
}

return view.extend({
	load: function() {
		return Promise.all([
			callList(),
			uci.load('familywarden')
		]);
	},

	render: function(data) {
		var profiles = data[0];

		// Collect schedules per profile from UCI
		scheduleData = {};
		uci.sections('familywarden', 'schedule', function(s) {
			var prof = s.profile;
			if (!prof) return;
			if (!scheduleData[prof]) scheduleData[prof] = [];
			scheduleData[prof].push({
				days: s.days,
				start: s.start,
				stop: s.stop,
				action: s.action || 'allow'
			});
		});

		injectStyles();

		var topbar = E('div', { 'class': 'fw-topbar' }, [
			E('h2', { 'style': 'margin:0' }, _('FamilyWarden')),
			E('button', {
				'class': 'fw-add-btn',
				'click': L.ui.createHandlerFn(this, 'handleAddProfile')
			}, '+ ' + _('Add Profile'))
		]);

		dashboardNode = E('div', {}, [
			topbar,
			buildContent(profiles, scheduleData)
		]);

		poll.add(refreshDashboard, 10);
		return dashboardNode;
	},

	handleAddProfile: function() {
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
						L.ui.hideModal();
						return uci.save().then(function() {
							return uci.apply();
						}).then(function() {
							return callReload();
						}).then(function() {
							window.location = L.url('admin/services/familywarden/profiles') + '?name=' + id;
						});
					})
				}, _('Create'))
			])
		]);

		setTimeout(function() { nameInput.focus(); }, 50);
	},

	handleSaveApply: null,
	handleSave: null,
	handleReset: null
});
