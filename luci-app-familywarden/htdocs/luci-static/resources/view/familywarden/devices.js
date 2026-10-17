'use strict';
'require view';
'require rpc';
'require uci';
'require poll';
'require ui';

// Phones running the FamilyWarden app. A phone appears under "Waiting for
// approval" after scanning the pairing code; compare the fingerprint shown in
// the app before approving it. Device changes go through the backend (not the
// UCI form), so a phone bound by a Ulysses pact can't be moved or removed.

var callDevices = rpc.declare({
	object: 'familywarden',
	method: 'devices',
	expect: { '': {} }
});

var callApprove = rpc.declare({
	object: 'familywarden',
	method: 'device_approve',
	params: [ 'device', 'profile', 'name', 'administrator', 'fingerprint' ]
});

var callReject = rpc.declare({
	object: 'familywarden',
	method: 'device_reject',
	params: [ 'device' ]
});

var callUpdate = rpc.declare({
	object: 'familywarden',
	method: 'device_update',
	params: [ 'device', 'profile', 'name', 'administrator' ]
});

var callDelete = rpc.declare({
	object: 'familywarden',
	method: 'device_delete',
	params: [ 'device' ]
});

var callAllowRemoval = rpc.declare({
	object: 'familywarden',
	method: 'device_allow_removal',
	params: [ 'device' ]
});

function fmtTime(epoch) {
	epoch = +epoch;
	return epoch > 0 ? new Date(epoch * 1000).toLocaleString() : _('Never');
}

// SHA-256 of the phone key, grouped in fours as the app shows it
function fingerprint(fp) {
	return E('code', { 'style': 'font-size:110%' }, (fp || '?').replace(/(.{4})(?=.)/g, '$1 '));
}

function profileName(id) {
	return uci.get('familywarden', id, 'name') || id || '?';
}

function notify(res, ok) {
	if (res && res.error)
		ui.addNotification(null, E('p', {}, _('Refused: %s').format(res.error)), 'warning');
	else if (ok)
		ui.addNotification(null, E('p', {}, ok), 'info');
}

return view.extend({
	load: function() {
		return Promise.all([ callDevices(), uci.load('familywarden') ]);
	},

	// Approve (pending) or edit (approved) modal
	showDeviceModal: function(dev, approve) {
		var self = this;
		var profiles = uci.sections('familywarden', 'profile');
		var nameInput = E('input', { 'type': 'text', 'class': 'cbi-input-text', 'maxlength': 64,
			'value': approve ? '' : (dev.name || ''), 'placeholder': approve ? (dev.name || '') : '' });
		var profileSelect = E('select', { 'class': 'cbi-input-select' }, profiles.map(function(p) {
			return E('option', { 'value': p['.name'], 'selected': p['.name'] === dev.profile ? '' : null },
				p.name || p['.name']);
		}));
		var adminInput = E('input', { 'type': 'checkbox', 'checked': dev.administrator === '1' ? '' : null });

		ui.showModal(approve ? _('Approve phone') : _('Edit phone'), [
			approve ? E('p', {}, _('Only approve a phone you recognise. The fingerprint must match the one shown in the FamilyWarden app on that phone:')) : '',
			E('p', {}, fingerprint(dev.fingerprint)),
			E('div', { 'class': 'cbi-value' }, [
				E('label', { 'class': 'cbi-value-title' }, _('Name')),
				E('div', { 'class': 'cbi-value-field' }, nameInput)
			]),
			E('div', { 'class': 'cbi-value' }, [
				E('label', { 'class': 'cbi-value-title' }, _('Profile')),
				E('div', { 'class': 'cbi-value-field' }, [ profileSelect,
					E('div', { 'class': 'cbi-value-description' },
						_('The phone gets this profile\'s DNS protection. Phones of the Ulysses profile are bound by its pact.')) ])
			]),
			E('div', { 'class': 'cbi-value' }, [
				E('label', { 'class': 'cbi-value-title' }, _('Administrator')),
				E('div', { 'class': 'cbi-value-field' }, [ adminInput,
					E('div', { 'class': 'cbi-value-description' },
						_('An administrator\'s phone may remove FamilyWarden itself when no pact applies to it. Other phones need your approval each time.')) ])
			]),
			E('div', { 'class': 'right' }, [
				E('button', { 'class': 'cbi-button', 'click': ui.hideModal }, _('Cancel')),
				' ',
				E('button', { 'class': 'cbi-button cbi-button-positive', 'click': function(ev) {
					ev.currentTarget.disabled = true;
					var name = nameInput.value || nameInput.placeholder;
					var admin = adminInput.checked ? '1' : '0';
					// The fingerprint shown here is sent back: if the phone's key changed
					// meanwhile, the router refuses instead of approving an unseen key.
					var req = approve
						? callApprove(dev.id, profileSelect.value, name, admin, dev.fingerprint)
						: callUpdate(dev.id, profileSelect.value, name, admin);
					return req.then(function(res) {
						ui.hideModal();
						notify(res);
						return self.refresh();
					});
				} }, approve ? _('Approve') : _('Save'))
			])
		]);
	},

	confirm: function(title, text, label, fn) {
		var self = this;
		ui.showModal(title, [
			E('p', {}, text),
			E('div', { 'class': 'right' }, [
				E('button', { 'class': 'cbi-button', 'click': ui.hideModal }, _('Cancel')),
				' ',
				E('button', { 'class': 'cbi-button cbi-button-negative', 'click': function(ev) {
					ev.currentTarget.disabled = true;
					return fn().then(function(res) {
						ui.hideModal();
						notify(res);
						return self.refresh();
					});
				} }, label)
			])
		]);
	},

	renderPending: function(pending) {
		var self = this;
		if (!pending.length)
			return E('p', { 'class': 'cbi-section-descr' },
				_('No phone is waiting. Scan the pairing code under Settings with the FamilyWarden app; the phone appears here within a minute.'));
		return E('table', { 'class': 'table' }, [
			E('tr', { 'class': 'tr table-titles' }, [
				E('th', { 'class': 'th' }, _('Phone')),
				E('th', { 'class': 'th' }, _('Fingerprint')),
				E('th', { 'class': 'th' }, _('Last request')),
				E('th', { 'class': 'th cbi-section-actions' }, '')
			])
		].concat(pending.map(function(d) {
			return E('tr', { 'class': 'tr' }, [
				E('td', { 'class': 'td' }, d.name || _('Unknown model')),
				E('td', { 'class': 'td' }, fingerprint(d.fingerprint)),
				E('td', { 'class': 'td' }, fmtTime(d.last_seen)),
				E('td', { 'class': 'td cbi-section-actions' }, [
					E('button', { 'class': 'cbi-button cbi-button-positive', 'click': function() {
						self.showDeviceModal(d, true);
					} }, _('Approve…')),
					' ',
					E('button', { 'class': 'cbi-button cbi-button-negative', 'click': function() {
						return callReject(d.id).then(function(res) { notify(res); return self.refresh(); });
					} }, _('Reject'))
				])
			]);
		})));
	},

	renderDevices: function(devices) {
		var self = this;
		if (!devices.length)
			return E('p', { 'class': 'cbi-section-descr' }, _('No phone approved yet.'));
		return E('table', { 'class': 'table' }, [
			E('tr', { 'class': 'tr table-titles' }, [
				E('th', { 'class': 'th' }, _('Name')),
				E('th', { 'class': 'th' }, _('Profile')),
				E('th', { 'class': 'th' }, _('Fingerprint')),
				E('th', { 'class': 'th' }, _('Last sync')),
				E('th', { 'class': 'th cbi-section-actions' }, '')
			])
		].concat(devices.map(function(d) {
			var roles = [ profileName(d.profile) ];
			if (d.administrator === '1')
				roles.push(_('administrator'));
			var removal = +d.remove_until > 0
				? E('div', { 'style': 'font-size:90%' }, _('May remove FamilyWarden until %s').format(fmtTime(d.remove_until)))
				: '';
			var actions = d.frozen
				? E('em', {}, '⚓ ' + _('Bound by the Ulysses pact'))
				: E('span', {}, [
					E('button', { 'class': 'cbi-button', 'click': function() { self.showDeviceModal(d, false); } }, _('Edit…')),
					' ',
					E('button', { 'class': 'cbi-button', 'click': function() {
						self.confirm(_('Allow removal'),
							_('For the next hour, "%s" may remove the FamilyWarden app itself. It still can\'t while a pact applies to it.').format(d.name || d.id),
							_('Allow for 1 hour'), function() { return callAllowRemoval(d.id); });
					} }, _('Allow removal…')),
					' ',
					E('button', { 'class': 'cbi-button cbi-button-negative', 'click': function() {
						self.confirm(_('Remove phone'),
							_('"%s" gets no policy any more. The app on the phone keeps its last policy until it is removed there; scanning the pairing code again puts it back under "Waiting for approval".').format(d.name || d.id),
							_('Remove'), function() { return callDelete(d.id); });
					} }, _('Remove…'))
				]);
			return E('tr', { 'class': 'tr' }, [
				E('td', { 'class': 'td' }, [ d.name || d.id, removal ]),
				E('td', { 'class': 'td' }, roles.join(', ')),
				E('td', { 'class': 'td' }, fingerprint(d.fingerprint)),
				E('td', { 'class': 'td' }, fmtTime(d.seen)),
				E('td', { 'class': 'td cbi-section-actions' }, actions)
			]);
		})));
	},

	refresh: function() {
		var self = this;
		return Promise.all([ callDevices(), uci.load('familywarden') ]).then(function(data) {
			var node = document.getElementById('fw-devices');
			if (node)
				node.replaceWith(self.renderBody(data[0] || {}));
		});
	},

	renderBody: function(data) {
		return E('div', { 'id': 'fw-devices' }, [
			E('div', { 'class': 'cbi-section' }, [
				E('h3', {}, _('Waiting for approval')),
				this.renderPending(data.pending || [])
			]),
			E('div', { 'class': 'cbi-section' }, [
				E('h3', {}, _('Approved phones')),
				this.renderDevices(data.devices || [])
			])
		]);
	},

	render: function(data) {
		var self = this;
		poll.add(function() { return self.refresh(); }, 5);
		return E('div', {}, [
			E('h2', {}, _('Phones')),
			E('p', { 'class': 'cbi-map-descr' },
				_('Phones with the FamilyWarden app. Only phones you approve here get their profile\'s DNS protection, and the Ulysses profile\'s phones its pact. A device id alone grants nothing.')),
			this.renderBody(data[0] || {})
		]);
	},

	handleSave: null,
	handleSaveApply: null,
	handleReset: null
});
