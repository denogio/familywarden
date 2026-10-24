'use strict';
'require view';
'require rpc';
'require uci';
'require poll';
'require ui';
'require familywarden.ui as fwui';

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
	params: [ 'device', 'profile', 'name', 'administrator', 'fingerprint', 'approves' ]
});

var callReject = rpc.declare({
	object: 'familywarden',
	method: 'device_reject',
	params: [ 'device' ]
});

var callUpdate = rpc.declare({
	object: 'familywarden',
	method: 'device_update',
	params: [ 'device', 'profile', 'name', 'administrator', 'approves' ]
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

// SHA-256 of the phone key, grouped in fours as the app shows it. The full value
// stays in the text (and title) so it can be compared and copied, never truncated.
function fingerprint(fp, large) {
	var groups = (fp || '?').match(/.{1,4}/g);
	return E('div', { 'class': 'fw-fp' + (large ? ' is-large' : ''), 'title': fp || '' },
		groups.map(function(g) { return E('span', {}, g + ' '); }));
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

function consequences(lines) {
	return E('ul', { 'class': 'fw-consequence' }, lines.map(function(l) { return E('li', {}, l); }));
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
		// Parent scope: never the phone's own profile, so the list follows the profile select.
		var scope = (dev.approves || '').split(' ').filter(Boolean);
		var approvesBox = E('div', { 'class': 'fw-approves', 'role': 'group', 'aria-label': _('May approve requests for') });
		function renderScope() {
			approvesBox.replaceChildren.apply(approvesBox, profiles.filter(function(p) {
				return p['.name'] !== profileSelect.value;
			}).map(function(p) {
				return E('label', { 'class': 'fw-day' }, [
					E('input', { 'type': 'checkbox', 'value': p['.name'], 'checked': scope.indexOf(p['.name']) >= 0 ? '' : null,
						'change': function(ev) {
							scope = scope.filter(function(s) { return s !== p['.name']; });
							if (ev.currentTarget.checked) scope.push(p['.name']);
						} }),
					p.name || p['.name']
				]);
			}));
		}
		profileSelect.addEventListener('change', function() {
			scope = scope.filter(function(s) { return s !== profileSelect.value; });
			renderScope();
		});
		renderScope();

		ui.showModal(approve ? _('Approve this phone?') : _('Edit phone'), [
			approve ? E('p', {}, _('On that phone, open FamilyWarden and compare its key fingerprint with this one, group by group. Only approve a phone you can see.')) : '',
			E('p', { 'class': 'fw-fp-label' }, _('Key fingerprint · SHA-256 · as received by the router')),
			fingerprint(dev.fingerprint, true),
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
						_('An administrator\'s phone may remove FamilyWarden itself when no pact applies to it. Other phones need your approval each time. It is separate from being the Ulysses captain.')) ])
			]),
			E('div', { 'class': 'cbi-value' }, [
				E('label', { 'class': 'cbi-value-title' }, _('Parent of')),
				E('div', { 'class': 'cbi-value-field' }, [ approvesBox,
					E('div', { 'class': 'cbi-value-description' },
						_('Profiles this phone may later approve extra time and new apps for. Not its own profile, and not administrator rights. While a pact binds a profile, no phone holds parent power over it.')) ])
			]),
			E('div', { 'class': 'right' }, [
				E('button', { 'class': 'cbi-button', 'click': ui.hideModal }, _('Cancel')),
				' ',
				E('button', { 'class': 'cbi-button ' + (approve ? 'fw-btn-allow' : 'cbi-button-positive'), 'click': function(ev) {
					ev.currentTarget.disabled = true;
					var name = nameInput.value || nameInput.placeholder;
					var admin = adminInput.checked ? '1' : '0';
					// The fingerprint shown here is sent back: if the phone's key changed
					// meanwhile, the router refuses instead of approving an unseen key.
					var req = approve
						? callApprove(dev.id, profileSelect.value, name, admin, dev.fingerprint, scope.join(' '))
						: callUpdate(dev.id, profileSelect.value, name, admin, scope.join(' '));
					return req.then(function(res) {
						ui.hideModal();
						notify(res);
						return self.refresh();
					});
				} }, approve ? _('It matches · Approve') : _('Save'))
			])
		]);
	},

	confirm: function(title, body, label, fn, alternative) {
		var self = this;
		var run = function(action) {
			return function(ev) {
				ev.currentTarget.disabled = true;
				return action().then(function(res) {
					ui.hideModal();
					notify(res);
					return self.refresh();
				});
			};
		};
		ui.showModal(title, body.concat([
			E('div', { 'class': 'right' }, [
				E('button', { 'class': 'cbi-button', 'click': ui.hideModal }, _('Cancel')),
				alternative ? [ ' ', E('button', { 'class': 'cbi-button', 'click': run(alternative.fn) }, alternative.label) ] : '',
				' ',
				E('button', { 'class': 'cbi-button cbi-button-negative', 'click': run(fn) }, label)
			])
		]));
	},

	allowRemoval: function(d) {
		this.confirm(_('Allow removal for 1 hour?'), [
			E('p', {}, _('For the next hour, "%s" may remove FamilyWarden through the app\'s own removal flow.').format(d.name || d.id)),
			consequences([
				_('Nothing is removed from this router. The phone stays enrolled and keeps its policy until someone completes removal on the phone.'),
				_('The permission lapses after one hour if it isn\'t used.'),
				_('Not available while a pact applies to this phone.')
			])
		], _('Allow for 1 hour'), function() { return callAllowRemoval(d.id); });
	},

	removeEnrollment: function(d) {
		this.confirm(_('Remove enrollment of "%s"?').format(d.name || d.id), [
			E('p', {}, _('The router stops sending this phone new policy. That is all it does.')),
			consequences([
				_('It does not uninstall FamilyWarden from the phone.'),
				_('It does not erase the protection the phone already has: the phone keeps enforcing its last cached policy until the app is removed there.'),
				_('Scanning the pairing code again puts it back under "Waiting for approval".')
			]),
			E('p', {}, _('To let the phone leave cleanly, allow removal for 1 hour instead.'))
		], _('Remove enrollment'), function() { return callDelete(d.id); }, {
			label: _('Allow removal for 1 hour instead'),
			fn: function() { return callAllowRemoval(d.id); }
		});
	},

	renderPending: function(pending) {
		var self = this;
		if (!pending.length)
			return E('p', { 'class': 'fw-muted' },
				_('No phone is waiting. Scan the pairing code under Settings with the FamilyWarden app; the phone appears here within a minute.'));
		return E('div', { 'class': 'fw-pending-list' }, pending.map(function(d) {
			return E('div', { 'class': 'fw-pending' }, [
				E('div', { 'class': 'fw-pending-top' }, [
					fwui.icon('info'),
					E('h4', {}, d.name || _('Unknown model')),
					E('span', { 'class': 'fw-muted' }, _('Asked to enroll %s · no profile yet').format(fmtTime(d.last_seen)))
				]),
				E('div', {}, [
					E('p', { 'class': 'fw-fp-label' }, _('Key fingerprint · SHA-256')),
					fingerprint(d.fingerprint, true)
				]),
				E('div', { 'class': 'fw-rowactions' }, [
					E('button', { 'class': 'cbi-button fw-btn-allow', 'click': function() {
						self.showDeviceModal(d, true);
					} }, _('Compare and approve…')),
					E('button', { 'class': 'cbi-button fw-btn-block', 'click': function() {
						return callReject(d.id).then(function(res) { notify(res); return self.refresh(); });
					} }, _('Reject'))
				])
			]);
		}));
	},

	renderDevices: function(devices) {
		var self = this;
		if (!devices.length)
			return E('p', { 'class': 'fw-muted' }, _('No phone approved yet.'));
		var heads = [ _('Phone'), _('Profile'), _('Role'), _('Key fingerprint (SHA-256)'), _('Last policy fetch'), _('App policy'), _('Actions') ];
		return E('table', { 'class': 'fw-table' }, [
			E('thead', {}, E('tr', {}, heads.map(function(h) { return E('th', {}, h); }))),
			E('tbody', {}, devices.map(function(d) {
				var actions = d.frozen
					? E('p', { 'class': 'fw-held' }, [ fwui.icon('mast'), _('Pact-bound: edit, removal and unenrollment wait until the pact ends.') ])
					: E('div', { 'class': 'fw-rowactions' }, [
						E('button', { 'class': 'cbi-button', 'click': function() { self.showDeviceModal(d, false); } }, _('Edit…')),
						E('button', { 'class': 'cbi-button', 'click': function() { self.allowRemoval(d); } }, _('Allow removal…')),
						E('button', { 'class': 'cbi-button cbi-button-negative', 'click': function() { self.removeEnrollment(d); } }, _('Remove…'))
					]);
				return E('tr', {}, [
					E('td', { 'data-label': heads[0] }, [ E('strong', {}, d.name || d.id),
						+d.remove_until > 0 ? E('div', { 'class': 'fw-deadline' }, _('May remove FamilyWarden until %s').format(fmtTime(d.remove_until))) : '' ]),
					E('td', { 'data-label': heads[1] }, profileName(d.profile)),
					E('td', { 'data-label': heads[2] }, [
						E('div', {}, d.administrator === '1' ? _('Administrator') : _('Member')),
						d.approves_effective ? E('div', {}, _('Parent of %s').format(d.approves_effective.split(' ').map(profileName).join(', '))) : '',
						(d.approves || '') !== (d.approves_effective || '') ? E('div', { 'class': 'fw-muted' }, _('Some parent rights are not active now (a pact binds that profile, or it was removed).')) : ''
					]),
					E('td', { 'data-label': heads[3] }, fingerprint(d.fingerprint)),
					E('td', { 'data-label': heads[4] }, fmtTime(d.seen)),
					E('td', { 'data-label': heads[5] }, d.apps_error === 'unsupported_capabilities'
						? _('Update FamilyWarden on this phone: required features are missing. Cached rules remain.')
						: typeof d.apps_rev !== 'string' || (d.apps_rev !== '' && !/^[0-9a-f]{16}$/.test(d.apps_rev)) ? _('Not reported by this router')
						: !d.apps_rev ? _('Not configured')
						: d.apps_applied === d.apps_rev ? _('Current revision acknowledged') : _('Not applied yet')),
					E('td', { 'data-label': heads[6] }, actions)
				]);
			}))
		]);
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
		var pending = data.pending || [], devices = data.devices || [];
		return E('div', { 'id': 'fw-devices' }, [
			E('section', { 'class': 'fw-panel' }, [
				E('h3', {}, _('Waiting for approval (%d)').format(pending.length)),
				E('p', { 'class': 'fw-muted' }, _('Approve only a phone you can see. Its key must match the one it shows.')),
				this.renderPending(pending)
			]),
			E('section', { 'class': 'fw-panel' }, [
				E('h3', {}, _('Approved phones (%d)').format(devices.length)),
				E('p', { 'class': 'fw-muted' }, _('Last policy fetch is when a phone last downloaded its policy from this router. It doesn\'t show that the phone applied it or is reachable now. Phones keep their cached policy, including on mobile data, when they can\'t reach home.')),
				this.renderDevices(devices)
			])
		]);
	},

	render: function(data) {
		var self = this;
		fwui.loadStyles('overview');
		fwui.loadStyles('pages');
		poll.add(function() { return self.refresh(); }, 5);
		return E('div', { 'id': 'fw-phones' }, [
			fwui.header(_('Phones'), _('Approve phones that asked to enroll, and manage the ones that receive policy from this router. A device id alone grants nothing.')),
			this.renderBody(data[0] || {})
		]);
	},

	handleSave: null,
	handleSaveApply: null,
	handleReset: null
});
