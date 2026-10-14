'use strict';
'require view';
'require form';
'require rpc';
'require uci';
'require poll';
'require familywarden.ulysses as ulysses';

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

var callTraffic = rpc.declare({
	object: 'familywarden',
	method: 'traffic',
	params: [ 'profile' ],
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

// Is a single schedule window active at the given date/time? Mirrors the
// daemon's logic: the schedule's weekday must match, and the time must fall
// within [start, stop) (handling overnight windows where start > stop).
function scheduleActiveAt(sched, date) {
	var day = dayNames[date.getDay()];
	var days = sched.days || '';
	if (typeof days === 'string') days = days.split(/\s+/);
	if (days.indexOf(day) < 0) return false;
	var min = date.getHours() * 60 + date.getMinutes();
	var startMin = timeToMin(sched.start);
	var stopMin = timeToMin(sched.stop);
	if (startMin < stopMin)
		return min >= startMin && min < stopMin;
	else
		return min >= startMin || min < stopMin;
}

// Profile is allowed at a given time if ANY of its windows is active then.
function isAllowedAt(schedules, date) {
	for (var i = 0; i < schedules.length; i++)
		if (scheduleActiveAt(schedules[i], date)) return true;
	return false;
}

function isScheduleActiveNow(sched) {
	return scheduleActiveAt(sched, new Date());
}

// Find the next moment the profile's allowed/blocked state actually flips, by
// scanning forward minute-by-minute over the coming week. Returns null if no
// change occurs within 7 days. This correctly skips days that have no windows
// (e.g. a Fri-only window does not recur on Saturday).
function getNextChange(schedules) {
	if (!schedules || !schedules.length) return null;
	var now = new Date();
	var current = isAllowedAt(schedules, now);
	for (var m = 1; m <= 7 * 1440; m++) {
		var t = new Date(now.getTime() + m * 60000);
		if (isAllowedAt(schedules, t) !== current) {
			return {
				minutes: m,
				label: current ? _('Access ends') : _('Access starts'),
				date: t,
				sameDay: (m < 1440 && t.getDay() === now.getDay())
			};
		}
	}
	return null;
}

function injectStyles() {
	if (document.getElementById('fw-styles-v2')) return;
	var css = document.createElement('style');
	css.id = 'fw-styles-v2';
	css.textContent = '\
		.fw-body{color:var(--text-color-high,#333)}\
		.fw-summary{display:flex;gap:10px;margin-bottom:22px;flex-wrap:wrap}\
		.fw-stat{flex:1;min-width:110px;text-align:center;padding:12px 14px;\
			background:rgb(255 255 247 / 3%);border:1px solid var(--border-color-medium,#ddd);border-radius:8px}\
		.fw-stat-label{font-size:10px;text-transform:uppercase;letter-spacing:.6px;font-weight:600;\
			color:var(--text-color-medium,#888);margin-bottom:5px}\
		.fw-stat-value{font-size:28px;font-weight:700;line-height:1;color:var(--text-color-high,#222)}\
		.fw-stat-value.online{color:var(--success-color-high,#5e9eff)}\
		.fw-stat-value.offline{color:var(--error-color-high,#ff6b6b)}\
		.fw-stat-value.time{font-size:18px;font-variant-numeric:tabular-nums}\
		.fw-grid{columns:440px;column-gap:14px}\
		.fw-card{background:rgb(255 255 247 / 3%);border:1px solid var(--border-color-medium,#ddd);border-radius:10px;\
			overflow:hidden;transition:box-shadow .2s,transform .2s;\
			break-inside:avoid;-webkit-column-break-inside:avoid;margin-bottom:14px;width:100%}\
		.fw-card:hover{box-shadow:0 4px 14px rgba(0,0,0,.10);transform:translateY(-1px)}\
		.fw-card-head{padding:15px 18px 12px;border-bottom:1px solid var(--border-color-low,#eee);\
			display:flex;justify-content:space-between;align-items:flex-start;gap:10px}\
		.fw-card-titlewrap{display:flex;align-items:center;gap:8px;flex-wrap:wrap;min-width:0}\
		.fw-card-title{font-size:16px;font-weight:600;line-height:1.2;color:var(--text-color-high,#222)}\
		.fw-card-title a{text-decoration:none;color:inherit}\
		.fw-card-title a:hover{text-decoration:underline;text-underline-offset:3px}\
		.fw-card-policy{font-size:10px;font-weight:600;text-transform:uppercase;letter-spacing:.4px;\
			padding:2px 8px;border-radius:4px;white-space:nowrap;\
			background:color-mix(in srgb,var(--primary-color-medium,#5e9eff) 10%,transparent);\
			border:1px solid color-mix(in srgb,var(--primary-color-medium,#5e9eff) 30%,transparent);\
			color:color-mix(in srgb,var(--primary-color-medium,#5e9eff) 78%,transparent)}\
		.fw-card-meta{display:flex;flex-direction:column;align-items:flex-end;gap:5px;flex-shrink:0}\
		.fw-traffic{font-size:13px;font-weight:600;color:var(--primary-color-medium,#5e9eff);\
			font-variant-numeric:tabular-nums;white-space:nowrap;cursor:pointer;\
			padding:2px 9px;border-radius:12px;\
			background:color-mix(in srgb,var(--primary-color-medium,#5e9eff) 12%,transparent)}\
		.fw-traffic:hover{background:color-mix(in srgb,var(--primary-color-medium,#5e9eff) 22%,transparent)}\
		.fw-tr-table{display:flex;flex-direction:column;gap:2px}\
		.fw-tr-row{display:flex;justify-content:space-between;gap:24px;padding:8px 0;border-bottom:1px solid var(--border-color-low,#f0f0f0)}\
		.fw-tr-row:last-child{border-bottom:none}\
		.fw-tr-label{color:var(--text-color-medium,#888)}\
		.fw-tr-val{font-weight:600;font-variant-numeric:tabular-nums}\
		.fw-badge{display:inline-flex;align-items:center;gap:4px;padding:2px 9px;border-radius:12px;\
			font-size:11px;font-weight:600;white-space:nowrap}\
		.fw-badge-green{background:rgba(46,125,50,.12);color:var(--success-color-high,#5e9eff)}\
		.fw-badge-red{background:rgba(198,40,40,.12);color:var(--error-color-high,#ff6b6b)}\
		.fw-badge-amber{background:rgba(230,81,0,.12);color:var(--warn-color-medium,#fbbf24)}\
		.fw-badge .dot{width:6px;height:6px;border-radius:50%;background:currentColor;flex-shrink:0}\
		.fw-card-body{padding:12px 18px}\
		.fw-devlist{margin-top:5px}\
		.fw-dev-row{display:flex;align-items:center;justify-content:space-between;gap:10px;padding:3px 0}\
		.fw-dev-name{font-size:13px;color:var(--text-color-high,#222);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}\
		.fw-dev-mac{font-size:11px;font-family:monospace;color:var(--text-color-medium,#888);flex-shrink:0;\
			padding:1px 7px;border-radius:3px;border:1px solid var(--border-color-low,#e0e0e0)}\
		.fw-no-devs{font-size:12px;color:var(--text-color-medium,#999);font-style:italic;margin-top:4px}\
		.fw-ulysses{padding:7px 11px;border-radius:6px;margin-top:9px;font-size:13px;background:rgba(127,127,127,.12)}\
		.fw-ovr{display:flex;align-items:center;gap:7px;padding:7px 11px;border-radius:6px;margin-top:9px;font-size:13px;font-weight:500}\
		.fw-ovr.blk{background:rgba(198,40,40,.10);color:var(--error-color-high,#ff6b6b)}\
		.fw-ovr.allw{background:rgba(46,125,50,.10);color:var(--success-color-high,#5e9eff)}\
		.fw-sched{margin-top:11px}\
		.fw-sched-hdr,.fw-dev-hdr{font-size:10px;text-transform:uppercase;letter-spacing:.8px;font-weight:600;\
			color:var(--text-color-medium,#999);opacity:.8;margin-bottom:8px;padding-bottom:5px;\
			border-bottom:1px solid var(--border-color-medium,#ccc)}\
		.fw-sched-row{display:flex;align-items:center;gap:7px;padding:3px 0}\
		.fw-sched-dot{width:7px;height:7px;border-radius:50%;flex-shrink:0}\
		.fw-sched-dot.on{background:var(--success-color-high,#5e9eff)}\
		.fw-sched-dot.off{background:var(--border-color-medium,#ccc)}\
		.fw-sched-days{display:flex;gap:3px;flex-wrap:wrap;flex:1}\
		.fw-day{font-size:11px;font-weight:600;padding:1px 5px;border-radius:3px;\
			color:var(--text-color-medium,#777);min-width:26px;text-align:center;\
			background:rgba(128,128,128,.12);border:1px solid var(--border-color-low,#ddd)}\
		.fw-day.on{background:rgba(46,125,50,.12);color:var(--success-color-high,#5e9eff);border-color:rgba(46,125,50,.25)}\
		.fw-sched-time{font-size:12px;color:var(--text-color-high,#333);min-width:96px;\
			text-align:right;font-variant-numeric:tabular-nums;flex-shrink:0;font-weight:500}\
		.fw-sched-time.on{color:var(--success-color-high,#5e9eff);font-weight:600}\
		.fw-sched-next{font-size:11px;color:var(--text-color-medium,#aaa);margin-top:7px;font-style:italic;padding-left:15px}\
		.fw-no-sched{font-size:12px;color:var(--text-color-medium,#bbb);font-style:italic}\
		.fw-card-foot{padding:9px 18px 11px;display:flex;gap:6px;flex-wrap:wrap;justify-content:flex-end;\
			border-top:1px solid var(--border-color-low,#f0f0f0)}\
		.fw-btn{font-size:12px;white-space:nowrap}\
		.fw-btn[disabled]{opacity:.4;cursor:not-allowed}\
		.fw-empty{text-align:center;padding:56px 28px;color:var(--text-color-medium,#999)}\
		.fw-empty h3{font-size:18px;font-weight:600;color:var(--text-color-high,#333);margin:0 0 8px}\
		.fw-empty a{color:var(--primary-color-high,#5e9eff);text-decoration:none;font-weight:500}\
		.fw-empty a:hover{text-decoration:underline}\
		.fw-updated{text-align:right;font-size:11px;color:var(--text-color-medium,#bbb);margin-top:18px;opacity:.6}\
		.fw-add-btn{display:inline-block;margin-bottom:18px;text-decoration:none}\
	';
	document.head.appendChild(css);
}

function statusBadge(status, override) {
	var cls, label;
	if (override && override !== 'none') {
		cls = 'fw-badge fw-badge-amber';
		label = override === 'block' ? _('Override: Blocked') : _('Override: Allowed');
		return E('span', { 'class': cls }, [E('span', { 'class': 'dot' }), ' ' + label]);
	}
	if (status && status.indexOf('blocked') >= 0) {
		cls = 'fw-badge fw-badge-red';
		label = _('Blocked');
	} else {
		cls = 'fw-badge fw-badge-green';
		label = _('Allowed');
	}
	return E('span', { 'class': cls }, [E('span', { 'class': 'dot' }), ' ' + label]);
}

// Human-readable byte count (1.2 GB, 340 MB, ...).
function formatBytes(n) {
	if (n < 1024) return n + ' B';
	var units = ['KB', 'MB', 'GB', 'TB'];
	var v = n, i = -1;
	do { v /= 1024; i++; } while (v >= 1024 && i < units.length - 1);
	return (v < 10 ? v.toFixed(1) : Math.round(v)) + ' ' + units[i];
}

// Traffic element for a profile; null when nlbwmon is unavailable (-1).
// Clicking opens a day/week/month breakdown fetched on demand.
function trafficEl(bytes, id, name) {
	if (bytes == null || bytes < 0) return null;
	return E('span', {
		'class': 'fw-traffic',
		'title': _('Click for traffic breakdown'),
		'click': function() { showTrafficModal(id, name); }
	}, '⇅ ' + formatBytes(bytes));
}

function trafficRow(label, bytes) {
	return E('div', { 'class': 'fw-tr-row' }, [
		E('span', { 'class': 'fw-tr-label' }, label),
		E('span', { 'class': 'fw-tr-val' }, formatBytes(bytes))
	]);
}

function showTrafficModal(id, name) {
	L.ui.showModal(_('Traffic') + ' · ' + (name || id), [
		E('p', { 'class': 'spinning' }, _('Loading…'))
	]);
	callTraffic(id).then(function(t) {
		var body;
		if (!t || !t.available) {
			body = [ E('p', {}, _('Traffic accounting is unavailable. Install nlbwmon to enable per-profile traffic.')) ];
		} else if (t.daily) {
			body = [ E('div', { 'class': 'fw-tr-table' }, [
				trafficRow(_('Today'), t.day),
				trafficRow(_('This week'), t.week),
				trafficRow(_('This month'), t.month)
			]) ];
		} else {
			body = [
				E('div', { 'class': 'fw-tr-table' }, [
					trafficRow(_('This period'), t.period)
				]),
				E('p', { 'style': 'font-size:12px;color:var(--text-color-medium,#999);margin-top:10px' },
					(t.period_start ? (_('Since') + ' ' + t.period_start + '. ') : '') +
					_('Set nlbwmon to daily accounting periods to see day/week/month breakdowns.'))
			];
		}
		L.ui.showModal(_('Traffic') + ' · ' + (name || id), body.concat([
			E('div', { 'class': 'right', 'style': 'margin-top:16px' },
				E('button', { 'class': 'cbi-button', 'click': L.ui.hideModal }, _('Close')))
		]));
	}).catch(function() {
		L.ui.hideModal();
	});
}

function policyLabel(policy) {
	switch (policy) {
		case 'scheduled': return _('Scheduled');
		case 'block': return _('Blocked');
		case 'allow': return _('Unrestricted');
		default: return policy || '';
	}
}

function doAction(ev, profile, fn) {
	var btn = ev && ev.currentTarget;
	if (btn) btn.disabled = true;
	return fn(profile).then(refreshDashboard).catch(function() { return refreshDashboard(); }).then(function() {
		if (btn) btn.disabled = false;
	});
}

var dashboardNode = null;
var scheduleData = {};
var hostMap = {};

// Build a MAC -> friendly name map from DHCP leases and host hints, so device
// chips can show a hostname instead of a bare MAC address.
function buildHostMap(leases, hostHints) {
	var map = {};
	(leases || []).forEach(function(l) {
		if (l.macaddr) {
			var mac = l.macaddr.toLowerCase();
			if (l.hostname) map[mac] = l.hostname;
		}
	});
	Object.keys(hostHints || {}).forEach(function(mac) {
		var k = mac.toLowerCase();
		if (!map[k]) {
			var h = hostHints[mac];
			if (h && h.name) map[k] = h.name;
		}
	});
	return map;
}

// A device row: resolved hostname (or muted "Unknown") on the left, with the
// MAC address always shown on the right.
function deviceRow(mac) {
	var key = (mac || '').toLowerCase();
	var name = hostMap[key];
	var nameEl = E('span', { 'class': 'fw-dev-name' }, name || _('Unknown'));
	return E('div', { 'class': 'fw-dev-row' }, [
		nameEl,
		E('span', { 'class': 'fw-dev-mac' }, mac)
	]);
}

// Schedules as enforced, from the backend's list reply. Not from UCI: for a
// Ulysses-locked profile UCI may hold an edit that only takes effect in 24 h.
function schedulesOf(profiles) {
	var out = {};
	Object.keys(profiles || {}).forEach(function(id) {
		out[id] = Array.isArray(profiles[id].schedules) ? profiles[id].schedules : [];
	});
	return out;
}

function refreshDashboard() {
	return Promise.all([callList(), uci.load('familywarden'), callDHCPLeases(), callHostHints()]).then(function(results) {
		if (!dashboardNode) return;
		var profiles = results[0];
		hostMap = buildHostMap(results[2], results[3]);
		scheduleData = schedulesOf(profiles);

		// Instead of replacing .fw-body (which restarts CSS animations), update
		// individual cards in place. If card structure changes, rebuild just the grid.
		var grid = dashboardNode.querySelector('.fw-grid');
		var now = new Date();
		var timeStr = (now.getHours() < 10 ? '0' : '') + now.getHours() + ':' +
		             (now.getMinutes() < 10 ? '0' : '') + now.getMinutes();
		var allowed = 0, blocked = 0;
		var keys = Object.keys(profiles || {});
		keys.forEach(function(k) {
			if (profiles[k].status && profiles[k].status.indexOf('blocked') >= 0) blocked++;
			else allowed++;
		});

		// Update summary stats
		var stats = dashboardNode.querySelectorAll('.fw-stat-value');
		if (stats[0]) stats[0].textContent = keys.length;
		if (stats[1]) stats[1].textContent = allowed;
		if (stats[2]) stats[2].textContent = blocked;
		if (stats[3]) stats[3].textContent = dayLabels[dayNames[now.getDay()]] + ' ' + timeStr;

		// Update timestamp
		var updated = dashboardNode.querySelector('.fw-updated');
		if (updated) updated.textContent = _('Updated') + ' ' + now.toLocaleTimeString();

		// If a real list came back, refresh the grid. If keys is empty but
		// we already have profiles, the call likely failed transiently —
		// keep the existing grid so the UI doesn't flash to "no profiles".
		var previousCount = grid ? parseInt(grid.dataset.count || '0', 10) : 0;
		var realList = profiles && typeof profiles === 'object' && Object.keys(profiles).length > 0;
		var hadProfiles = previousCount > 0;
		var keysStable = (grid && grid.dataset.count === String(keys.length));

		// Rebuild only when count genuinely changed AND we trust the result,
		// or when we don't have a grid yet.
		if (!grid || (realList && !keysStable) || (!hadProfiles && keys.length > 0)) {
			var oldBody = dashboardNode.querySelector('.fw-body');
			if (oldBody) oldBody.parentNode.removeChild(oldBody);
			dashboardNode.appendChild(buildContent(profiles, scheduleData));
			return;
		}
		// If empty result but we previously had profiles, skip the in-place
		// updates entirely and just bail — next refresh will pick up the truth.
		if (keys.length === 0 && hadProfiles) return;

		// Update each card in place
		var cards = grid.querySelectorAll('.fw-card');
		cards.forEach(function(card) {
			var link = card.querySelector('.fw-card-title a');
			if (!link) return;
			var id = decodeURIComponent((link.href.split('name=')[1] || '').split('&')[0] || '');
			var p = profiles[id];
			if (!p) return;

			var hasOverride = p.override && p.override !== 'none';

			// Update badge + traffic
			var meta = card.querySelector('.fw-card-meta');
			if (meta) {
				meta.innerHTML = '';
				meta.appendChild(statusBadge(p.status, p.override));
				var tEl = trafficEl(p.traffic_bytes, id, p.name || id);
				if (tEl) meta.appendChild(tEl);
			}

			// Update policy label
			var policyEl = card.querySelector('.fw-card-policy');
			if (policyEl) policyEl.textContent = policyLabel(p.policy);

			// Update body: devices / override bar
			var body = card.querySelector('.fw-card-body');
			if (body) {
				body.innerHTML = '';
				var devices = Array.isArray(p.devices) ? p.devices : [];
				if (devices.length > 0) {
					body.appendChild(E('div', { 'class': 'fw-dev-hdr' }, _('Devices')));
					var devWrap = E('div', { 'class': 'fw-devlist' });
					devices.forEach(function(d) { devWrap.appendChild(deviceRow(d)); });
					body.appendChild(devWrap);
				} else {
					body.appendChild(E('div', { 'class': 'fw-no-devs' }, _('No devices')));
				}
				if (hasOverride && p.override_expires) {
					var remaining = Math.max(0, Math.round((p.override_expires - Date.now() / 1000) / 60));
					if (remaining > 0)
						body.appendChild(E('div', { 'class': 'fw-ovr ' + (p.override === 'block' ? 'blk' : 'allw') },
							E('span', {}, remaining + ' ' + _('min remaining'))));
				}
				var uNote = ulysses.note(p);
				if (uNote) body.appendChild(uNote);
				// Re-add schedule section (was lost when body was cleared)
				var pScheds = scheduleData[id] || [];
				if (p.policy === 'scheduled' && pScheds.length > 0)
					body.appendChild(buildScheduleSection(pScheds));
			}

			// Update foot buttons
			var foot = card.querySelector('.fw-card-foot');
			if (foot) {
				foot.innerHTML = '';
				cardButtons(id, p).forEach(function(b) { foot.appendChild(b); });
			}
		});
	});
}

// Pact button, only on the captain's profile: make a pact, extend an "until"
// pact, or amend an ongoing one inside its window
function pactButtonLabel(id, p) {
	if (id !== ulysses.captainId()) return null;
	if (!ulysses.isBound(p)) return '⚓ ' + _('Make a pact');
	if (p.pact_type === 'until') return '⚓ ' + _('Extend pact');
	if (p.pact_in_window) return '⚓ ' + _('Amend pact');
	return null;
}

// Action buttons for a profile card. Allow/tickets are hidden for the captain
// during a pact (the backend refuses them anyway); Block and Reset stay, since
// they only ever tighten back towards the pact.
function cardButtons(id, p) {
	var isBlocked = p.status && p.status.indexOf('blocked') >= 0;
	var hasOverride = p.override && p.override !== 'none';
	var btns = [];
	if (hasOverride) {
		btns.push(makeBtn('cbi-button-reset', _('Reset'), function() { return callReset(id); }));
	} else if (isBlocked) {
		if (!ulysses.isBound(p)) {
			btns.push(makeBtn('cbi-button-positive', _('Allow'), function() { return callAllow(id); }));
			[5, 10, 30].forEach(function(mins) {
				btns.push(makeBtn('cbi-button-action', '+' + mins, function() { return callTicket(id, mins); }));
			});
		}
	} else {
		btns.push(makeBtn('cbi-button-negative', _('Block'), function() { return callBlock(id); }));
	}
	var pactLabel = pactButtonLabel(id, p);
	if (pactLabel)
		btns.push(makeBtn('cbi-button-neutral', pactLabel, function() {
			return ulysses.showPactModal(p.name, p);
		}));
	return btns;
}

function makeBtn(cls, label, fn) {
	return E('button', { 'class': 'cbi-button ' + cls + ' fw-btn', 'click': function(ev) {
		var btn = ev.currentTarget;
		btn.disabled = true;
		return fn().then(refreshDashboard).catch(function() { return refreshDashboard(); }).then(function() { btn.disabled = false; });
	} }, label);
}

function buildScheduleSection(schedules) {
	if (!schedules || schedules.length === 0)
		return E('div', { 'class': 'fw-no-sched' }, _('No schedules configured'));
	var todayName = dayNames[new Date().getDay()];
	var rows = schedules.map(function(s) {
		var active = isScheduleActiveNow(s);
		var days = s.days || '';
		if (typeof days === 'string') days = days.split(/\s+/);
		var dayBadges = days.map(function(d) {
			// Only highlight the day that is actually live right now (today,
			// while the window is active) — not every day the window covers.
			var on = active && d === todayName;
			return E('span', { 'class': 'fw-day' + (on ? ' on' : '') }, (dayLabels[d] || d).substring(0, 3));
		});
		var timeStr = (s.start || '?') + ' – ' + (s.stop || '?');
		return E('div', { 'class': 'fw-sched-row' }, [
			E('span', { 'class': 'fw-sched-dot ' + (active ? 'on' : 'off') }),
			E('div', { 'class': 'fw-sched-days' }, dayBadges),
			E('span', { 'class': 'fw-sched-time' + (active ? ' on' : '') }, timeStr)
		]);
	});
	var nextChange = getNextChange(schedules);
	var schedChildren = rows.slice();
	if (nextChange) {
		var mins = nextChange.minutes;
		var d = Math.floor(mins / 1440);
		var h = Math.floor((mins % 1440) / 60);
		var mm = mins % 60;
		var inStr = d > 0 ? (d + 'd ' + h + 'h') : (h > 0 ? (h + 'h ' + mm + 'm') : (mm + 'm'));
		var t = nextChange.date;
		var hh = (t.getHours() < 10 ? '0' : '') + t.getHours();
		var mn = (t.getMinutes() < 10 ? '0' : '') + t.getMinutes();
		var when = (nextChange.sameDay ? '' : dayLabels[dayNames[t.getDay()]] + ' ') + hh + ':' + mn;
		schedChildren.push(E('div', { 'class': 'fw-sched-next' },
			nextChange.label + ' ' + when + ' (' + inStr + ')'));
	}
	return E('div', { 'class': 'fw-sched' }, [
		E('div', { 'class': 'fw-sched-hdr' }, _('Schedules')),
		E('div', {}, schedChildren)
	]);
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
		E('div', { 'class': 'fw-stat' }, [
			E('div', { 'class': 'fw-stat-label' }, _('Profiles')),
			E('div', { 'class': 'fw-stat-value', 'data-stat': 'count' }, String(keys.length))
		]),
		E('div', { 'class': 'fw-stat' }, [
			E('div', { 'class': 'fw-stat-label' }, _('Online')),
			E('div', { 'class': 'fw-stat-value online' }, String(allowed))
		]),
		E('div', { 'class': 'fw-stat' }, [
			E('div', { 'class': 'fw-stat-label' }, _('Offline')),
			E('div', { 'class': 'fw-stat-value offline' }, String(blocked))
		]),
		E('div', { 'class': 'fw-stat' }, [
			E('div', { 'class': 'fw-stat-label' }, _('Time')),
			E('div', { 'class': 'fw-stat-value time' }, dayLabels[dayNames[now.getDay()]] + ' ' + timeStr)
		])
	]);

	var cards = [];
	if (keys.length === 0) {
		cards.push(E('div', { 'class': 'fw-empty' }, [
			E('h3', {}, _('No profiles configured yet.')),
			E('p', {}, E('a', { 'href': L.url('admin/services/familywarden/profiles') }, _('Create your first profile')))
		]));
	}

	keys.forEach(function(id) {
		var p = profiles[id];
		var hasOverride = p.override && p.override !== 'none';
		var profileUrl = L.url('admin/services/familywarden/profiles') + '?name=' + encodeURIComponent(id);

		var devices = Array.isArray(p.devices) ? p.devices : [];
		var deviceChips = devices.length > 0
			? devices.map(function(d) { return deviceRow(d); })
			: null;

		// Override bar — only meaningful for a timed override (ticket); the
		// status badge already conveys a manual override's direction.
		var overrideBar = null;
		if (hasOverride && p.override_expires) {
			var remaining = Math.max(0, Math.round((p.override_expires - Date.now() / 1000) / 60));
			if (remaining > 0)
				overrideBar = E('div', { 'class': 'fw-ovr ' + (p.override === 'block' ? 'blk' : 'allw') },
					E('span', {}, remaining + ' ' + _('min remaining')));
		}

		var profileSchedules = schedsByProfile[id] || [];
		var schedSection = (p.policy === 'scheduled' && profileSchedules.length > 0)
			? buildScheduleSection(profileSchedules)
			: null;

		var footBtns = cardButtons(id, p);

		// Build card body content
		var bodyContent = [];
		if (deviceChips) {
			bodyContent.push(E('div', { 'class': 'fw-dev-hdr' }, _('Devices')));
			bodyContent.push(E('div', { 'class': 'fw-devlist' }, deviceChips));
		} else {
			bodyContent.push(E('div', { 'class': 'fw-no-devs' }, _('No devices')));
		}
		if (overrideBar) bodyContent.push(overrideBar);
		var uNote = ulysses.note(p);
		if (uNote) bodyContent.push(uNote);
		if (schedSection) bodyContent.push(schedSection);

		cards.push(E('div', { 'class': 'fw-card' }, [
			E('div', { 'class': 'fw-card-head' }, [
				E('div', { 'class': 'fw-card-titlewrap' }, [
					E('div', { 'class': 'fw-card-title' },
						E('a', { 'href': profileUrl }, p.name || id)),
					E('div', { 'class': 'fw-card-policy' }, policyLabel(p.policy))
				]),
				E('div', { 'class': 'fw-card-meta' },
					[statusBadge(p.status, p.override)].concat(trafficEl(p.traffic_bytes, id, p.name || id) ? [trafficEl(p.traffic_bytes, id, p.name || id)] : []))
			]),
			E('div', { 'class': 'fw-card-body' }, bodyContent),
			E('div', { 'class': 'fw-card-foot' }, footBtns)
		]));
	});

	var gridEl = E('div', { 'class': 'fw-grid', 'data-count': keys.length }, cards);

	return E('div', { 'class': 'fw-body' }, [
		E('a', { 'class': 'cbi-button cbi-button-add fw-add-btn', 'href': L.url('admin/services/familywarden/profiles') }, '+ ' + _('Add Profile')),
		summary,
		gridEl,
		E('div', { 'class': 'fw-updated' }, _('Updated') + ' ' + now.toLocaleTimeString())
	]);
}

return view.extend({
	load: function() {
		return Promise.all([callList(), uci.load('familywarden'), callDHCPLeases(), callHostHints()]);
	},

	render: function(data) {
		injectStyles();
		var profiles = data[0];
		hostMap = buildHostMap(data[2], data[3]);
		scheduleData = schedulesOf(profiles);

		// Use a CBI form.Map container so we inherit the native LuCI theme
		// styling (title bar, margins, card-like background) without having
		// to recreate it ourselves. The actual content is rendered separately
		// and injected into the form element once it's built.
		var m = new form.Map('familywarden', _('FamilyWarden'),
			_('Profile-based parental control. Toggle access, set time schedules, and apply temporary overrides.'));

		return m.render().then(function(formEl) {
			dashboardNode = document.createElement('div');
			dashboardNode.id = 'fw-dashboard';
			dashboardNode.appendChild(buildContent(profiles, scheduleData));
			formEl.appendChild(dashboardNode);

			poll.add(refreshDashboard, 10);
			return formEl;
		});
	}
});
