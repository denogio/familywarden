'use strict';
'require baseclass';
'require rpc';
'require uci';
'require ui';

// Shared Ulysses helpers for the overview, profile and settings views.
// The captain is settings.ulysses_profile. Backend fields per profile: captain
// (bound by an active pact), and for the captain pact_type ("until" or
// "ongoing"), pact_until (0 when ongoing), pact_window ("sun 10:00-11:00"),
// pact_in_window and release_at (0 unless sailing home was requested).

var SAIL_PORT = 7700;

var callPact = rpc.declare({
	object: 'familywarden',
	method: 'pact',
	params: [ 'until', 'window_day', 'window_start', 'window_stop' ]
});

var DAYS = [ 'mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun' ];

function fmtTime(epoch) {
	var d = new Date(epoch * 1000);
	var time = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
	if (d.toDateString() === new Date().toDateString())
		return time;
	return d.toLocaleDateString([], { weekday: 'short', day: 'numeric', month: 'short' }) + ' ' + time;
}

// "YYYY-MM-DDTHH:MM" in local time, for <input type="datetime-local">
function localInput(date) {
	var pad = function(n) { return String(n).padStart(2, '0'); };
	return date.getFullYear() + '-' + pad(date.getMonth() + 1) + '-' + pad(date.getDate()) +
		'T' + pad(date.getHours()) + ':' + pad(date.getMinutes());
}

function dayLabel(d) {
	return { mon: _('Monday'), tue: _('Tuesday'), wed: _('Wednesday'), thu: _('Thursday'),
		fri: _('Friday'), sat: _('Saturday'), sun: _('Sunday') }[d] || d;
}

function windowLabel(w) {
	var m = (w || '').match(/^(\w+) (\S+)-(\S+)$/);
	return m ? dayLabel(m[1]) + ' ' + m[2] + '–' + m[3] : w;
}

return baseclass.extend({
	// The configured captain profile id (pact or not), or ''
	captainId: function() {
		return uci.get('familywarden', 'settings', 'ulysses_profile') || '';
	},

	// Captain bound by an active pact
	isBound: function(p) {
		return !!(p && p.captain);
	},

	// The captain's profile can't be edited (bound and outside the window)
	isFrozen: function(p) {
		return this.isBound(p) && !p.pact_in_window;
	},

	sailUrl: function() {
		return window.location.protocol + '//' + window.location.hostname + ':' + SAIL_PORT + '/';
	},

	// Info lines for the captain, or null
	note: function(p, mastIcon) {
		if (!this.isBound(p))
			return null;
		var headline = p.pact_type === 'ongoing'
			? _('Bound to the mast by an ongoing pact. Changes only in the window: %s').format(windowLabel(p.pact_window))
			: _('Bound to the mast until %s').format(fmtTime(p.pact_until));
		var lines = [ E('div', { 'class': 'fw-pact-heading' }, mastIcon
			? [mastIcon, E('span', {}, headline)] : '⚓ ' + headline) ];
		if (p.pact_in_window)
			lines.push(E('div', {}, _('The window is open: changes to this profile apply now.')));
		if (p.release_at > 0)
			lines.push(E('div', {}, _('Sailing home: free at %s').format(fmtTime(p.release_at))));
		if (p.mast_last_sync > 0)
			lines.push(E('div', {}, _('Last policy fetch: %s').format(fmtTime(p.mast_last_sync))));
		return E('div', { 'class': 'fw-ulysses' }, lines);
	},

	// Modal to make or amend the captain's pact. Resolves true once made.
	showPactModal: function(name, p) {
		var self = this;
		var bound = this.isBound(p);
		var parts = (bound && p.pact_window || 'sun 10:00-11:00').match(/^(\w+) (\S+)-(\S+)$/) || [];

		var tomorrow6 = new Date();
		tomorrow6.setDate(tomorrow6.getDate() + 1);
		tomorrow6.setHours(6, 0, 0, 0);

		var typeUntil = E('input', { 'type': 'radio', 'name': 'fw-pact-type', 'value': 'until' });
		var typeOngoing = E('input', { 'type': 'radio', 'name': 'fw-pact-type', 'value': 'ongoing' });
		var untilInput = E('input', { 'type': 'datetime-local', 'class': 'cbi-input-text',
			'value': localInput(bound && p.pact_until ? new Date(p.pact_until * 1000) : tomorrow6) });
		var daySelect = E('select', { 'class': 'cbi-input-select' }, DAYS.map(function(d) {
			return E('option', { 'value': d, 'selected': d === (parts[1] || 'sun') ? '' : null }, dayLabel(d));
		}));
		var startInput = E('input', { 'type': 'time', 'class': 'cbi-input-text', 'value': parts[2] || '10:00' });
		var stopInput = E('input', { 'type': 'time', 'class': 'cbi-input-text', 'value': parts[3] || '11:00' });

		var untilRow = E('div', { 'style': 'margin:4px 0 12px 24px' }, [ untilInput ]);
		var ongoingRow = E('div', { 'style': 'margin:4px 0 12px 24px' }, [
			_('Weekly window') + ' ', daySelect, ' ', startInput, ' – ', stopInput
		]);

		(bound && p.pact_type === 'ongoing' ? typeOngoing : typeUntil).checked = true;
		var sync = function() {
			untilRow.style.opacity = typeUntil.checked ? 1 : .4;
			ongoingRow.style.opacity = typeOngoing.checked ? 1 : .4;
		};
		typeUntil.addEventListener('change', sync);
		typeOngoing.addEventListener('change', sync);
		sync();

		var delay = uci.get('familywarden', 'settings', 'ulysses_delay') || '24';

		return new Promise(function(resolve) {
			ui.showModal('⚓ ' + _('Bind to the mast') + ' · ' + (name || ''), [
				E('p', {}, _('For as long as the pact lasts, this profile is frozen: no changes, no Allow, no tickets. While you are blocked, the router is closed to every device except the sail page.')),
				E('p', {}, _('The only way out early is the sail page (%s): the router password, the line from the song, and then %s of waiting.').format(
					self.sailUrl(), delay == '1' ? _('1 hour') : _('%s hours').format(delay))),
				(!bound && p && p.status && p.status.indexOf('blocked') >= 0)
					? E('p', { 'class': 'alert-message warning' }, _('You are blocked right now: the router closes the moment you bind yourself, and this page will stop responding.'))
					: '',
				bound ? E('p', { 'style': 'font-weight:bold' }, p.pact_type === 'until'
					? _('Current pact ends %s. It can only be extended.').format(fmtTime(p.pact_until))
					: _('The window is open: you can amend the pact now.')) : '',
				E('label', { 'style': 'display:block' }, [ typeUntil, ' ', _('Until a time') ]),
				untilRow,
				E('label', { 'style': 'display:block' }, [ typeOngoing, ' ', _('Ongoing, runs on its own') ]),
				ongoingRow,
				E('div', { 'class': 'right' }, [
					E('button', { 'class': 'cbi-button', 'click': function() { ui.hideModal(); resolve(false); } }, _('Cancel')),
					' ',
					E('button', { 'class': 'cbi-button cbi-button-negative', 'click': function(ev) {
						var until = 0;
						if (typeUntil.checked) {
							until = Math.floor(new Date(untilInput.value).getTime() / 1000);
							if (!until) return;
						}
						ev.currentTarget.disabled = true;
						return callPact(until, daySelect.value, startInput.value, stopInput.value).then(function(res) {
							ui.hideModal();
							if (res && res.error)
								ui.addNotification(null, E('p', {}, _('Pact refused: %s').format(res.error)), 'warning');
							resolve(!(res && res.error));
						}).catch(function(err) {
							ui.hideModal();
							ui.addNotification(null, E('p', {}, _('Pact failed: %s').format(err.message)), 'danger');
							resolve(false);
						});
					} }, '⚓ ' + _('Bind me'))
				])
			]);
		});
	}
});
