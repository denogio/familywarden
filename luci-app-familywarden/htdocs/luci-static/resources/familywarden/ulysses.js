'use strict';
'require baseclass';
'require rpc';
'require ui';

// Shared Ulysses (self-binding lock) helpers for the overview and profile views.
// Backend fields per profile: locked (standing lock), lock_pending_until (epoch
// a pending edit takes effect, 0 = none), pact_until (ad-hoc pact expiry, 0 = none).

var callPact = rpc.declare({
	object: 'familywarden',
	method: 'pact',
	params: [ 'profile', 'minutes' ]
});

function fmtTime(epoch) {
	var d = new Date(epoch * 1000);
	var now = new Date();
	var time = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
	if (d.toDateString() === now.toDateString())
		return time;
	return d.toLocaleDateString([], { weekday: 'short', day: 'numeric', month: 'short' }) + ' ' + time;
}

function fmtDuration(minutes) {
	var h = Math.floor(minutes / 60), m = minutes % 60;
	return h > 0 ? (h + ' h ' + m + ' min') : (m + ' min');
}

// Minutes from now until the next occurrence of HH:MM (today or tomorrow)
function minutesUntil(hhmm) {
	var parts = hhmm.split(':');
	var target = new Date();
	target.setHours(+parts[0], +parts[1], 0, 0);
	if (target <= new Date())
		target.setDate(target.getDate() + 1);
	return Math.ceil((target - new Date()) / 60000);
}

return baseclass.extend({
	pactActive: function(p) {
		return !!(p && p.pact_until && p.pact_until * 1000 > Date.now());
	},

	// Allow/ticket are refused by the backend for locked or pacted profiles
	isBound: function(p) {
		return !!(p && (p.locked || this.pactActive(p)));
	},

	// Info line(s) for a bound profile, or null
	note: function(p) {
		var lines = [];
		if (this.pactActive(p))
			lines.push('🔒 ' + _('Bound by pact until %s').format(fmtTime(p.pact_until)));
		else if (p && p.locked)
			lines.push('🔒 ' + _('Ulysses lock: rules can only be loosened 24 h after an edit'));
		if (p && p.lock_pending_until > 0)
			lines.push('⏳ ' + _('Saved changes take effect %s').format(fmtTime(p.lock_pending_until)));
		if (!lines.length)
			return null;
		return E('div', { 'class': 'fw-ulysses' }, lines.map(function(l) { return E('div', {}, l); }));
	},

	// Modal: "bind me until HH:MM". Resolves once the pact is made or cancelled.
	showPactModal: function(id, name, current) {
		var self = this;
		return new Promise(function(resolve) {
			var input = E('input', { 'type': 'time', 'class': 'cbi-input-text', 'value': '06:00' });
			var preview = E('p', { 'style': 'font-weight:bold' });
			var update = function() {
				if (!input.value) { preview.textContent = ''; return; }
				var mins = minutesUntil(input.value);
				preview.textContent = _('Internet off until %s (%s)').format(
					fmtTime(Math.floor(Date.now() / 1000) + mins * 60), fmtDuration(mins));
			};
			input.addEventListener('input', update);
			update();

			ui.showModal(_('Bind to the mast') + ' · ' + (name || id), [
				E('p', {}, _('Blocks every device in this profile until the chosen time. Until then it cannot be allowed, given a ticket, shortened or lifted - not from LuCI, not from Home Assistant. Only root over SSH can break it.')),
				self.pactActive(current)
					? E('p', {}, _('Current pact ends %s. A pact can only be extended.').format(fmtTime(current.pact_until)))
					: '',
				E('label', {}, [ _('Until') + ' ', input ]),
				preview,
				E('div', { 'class': 'right' }, [
					E('button', { 'class': 'cbi-button', 'click': function() { ui.hideModal(); resolve(false); } }, _('Cancel')),
					' ',
					E('button', { 'class': 'cbi-button cbi-button-negative', 'click': function(ev) {
						if (!input.value) return;
						ev.currentTarget.disabled = true;
						return callPact(id, minutesUntil(input.value)).then(function(res) {
							ui.hideModal();
							if (res && res.error)
								ui.addNotification(null, E('p', {}, _('Pact refused: %s').format(res.error)), 'warning');
							resolve(!(res && res.error));
						}).catch(function(err) {
							ui.hideModal();
							ui.addNotification(null, E('p', {}, _('Pact failed: %s').format(err.message)), 'danger');
							resolve(false);
						});
					} }, '🔒 ' + _('Bind me'))
				])
			]);
		});
	}
});
