'use strict';
'require view';
'require form';
'require rpc';
'require uci';
'require ui';
'require familywarden.ulysses as ulysses';
'require familywarden.ui as fwui';

var callReload = rpc.declare({
	object: 'familywarden',
	method: 'reload'
});

var callMast = rpc.declare({
	object: 'familywarden',
	method: 'mast',
	params: [ 'host' ]
});

var callDnsStatus = rpc.declare({
	object: 'familywarden',
	method: 'dns_status',
	expect: { '': {} }
});

var callDnsSync = rpc.declare({
	object: 'familywarden',
	method: 'dns_sync',
	expect: { '': {} }
});

var callDnsAuth = rpc.declare({
	object: 'familywarden',
	method: 'dns_auth',
	params: [ 'username', 'password' ]
});

var callList = rpc.declare({
	object: 'familywarden',
	method: 'list',
	expect: { profiles: {} }
});

return view.extend({
	load: function() {
		// Save & Apply belongs to the view, not form.Map. Reload only after confirmation.
		document.addEventListener('uci-applied', function() {
			callReload().catch(function(error) {
				ui.addNotification(null, E('p', {}, _('Could not reload FamilyWarden: %s').format(error.message)), 'error');
			});
		});
		return Promise.all([ callList(), uci.load('familywarden'), callMast(window.location.hostname),
			L.resolveDefault(callDnsStatus(), {}) ]);
	},

	render: function(data) {
		var m, s, o;
		var profiles = data[0] || {};
		var bound = Object.keys(profiles).some(function(id) { return ulysses.isBound(profiles[id]); });
		var mast = data[2] || {};
		var dns = data[3] || {};

		fwui.loadStyles('overview');
		fwui.loadStyles('pages');

		// No Map title: the shared FamilyWarden header replaces it.
		m = new form.Map('familywarden');


		s = m.section(form.TypedSection, 'global', _('General'),
			bound ? _('A pact is active: these are frozen too, so they can not be used as a way out.') : null);
		s.anonymous = true;
		s.addremove = false;

		o = s.option(form.Flag, 'enabled', _('Enable FamilyWarden'),
			_('Off: nothing is enforced - no blocking, no schedules. Profiles and settings are kept.'));
		o.rmempty = false;
		o.default = '1';
		o.readonly = bound;

		o = s.option(form.ListValue, 'default_policy', _('Default Policy'),
			_('What happens to devices that are in no profile. "Block" means every device needs a profile to get online - put servers, TVs and the like in an "Unrestricted" profile first. New devices get no internet until you add them (the overview lists them).'));
		o.value('allow', _('Allow Internet'));
		o.value('block', _('Block Internet'));
		o.default = 'allow';
		o.readonly = bound;

		// Pairing serves every FamilyWarden phone, not only the Ulysses captain.
		s = m.section(form.TypedSection, 'global', _('Phone pairing'),
			_('For every FamilyWarden phone, children\'s and adults\'. Pairing is separate from Ulysses.'));
		s.anonymous = true;
		s.addremove = false;

		o = s.option(form.DummyValue, '_mast', _('Pairing code'));
		o.renderWidget = function() {
			var synced = mast.last_sync > 0
				? _('Last sync: %s (%s)').format(new Date(mast.last_sync * 1000).toLocaleString(), mast.device || '?')
				: _('No phone has synced yet');
			var qr = E('div', { 'class': 'fw-qr' });
			if (mast.qr_svg)
				qr.innerHTML = mast.qr_svg.replace(/width="[^"]*" height="[^"]*"/, 'width="100%" height="100%"');
			else
				qr = E('code', { 'class': 'fw-mono' }, mast.pair_uri || _('Pairing code unavailable'));
			return E('div', { 'class': 'fw-pairing' }, [
				qr,
				E('div', {}, [
					E('ol', { 'class': 'fw-steps' }, [
						E('li', {}, _('On home Wi-Fi, open FamilyWarden on the phone and choose to pair with a router.')),
						E('li', {}, _('Scan this code. Check that the phone shows the router key below.')),
						E('li', {}, _('Approve the phone under Phones after comparing its own fingerprint.'))
					]),
					E('p', { 'class': 'fw-fp-label' }, _('Router key fingerprint')),
					E('div', { 'class': 'fw-fp is-large' }, (mast.fingerprint || '?').match(/.{1,4}/g).map(function(g) {
						return E('span', {}, g + ' ');
					})),
					E('p', { 'class': 'fw-muted' }, synced)
				])
			]);
		};

		// DNS filtering through AdGuard Home. The router manages only its own
		// "familywarden-*" clients and the ClientIDs it added to AdGuard Home's
		// allowed clients; everything else in AdGuard Home is left alone.
		s = m.section(form.TypedSection, 'global', _('DNS · AdGuard Home'),
			_('Phones of profiles with "Strict DNS" use DNS-over-TLS to &lt;ClientID&gt;.&lt;DNS domain&gt;. Your AdGuard Home needs encryption set up with a certificate covering that domain and its subdomains, reachable on port 853 from home and away. FamilyWarden only adds, changes and removes its own "familywarden-…" clients and the ClientIDs it added to the allowed clients list; your other clients, lists and settings are never changed.'));
		s.anonymous = true;
		s.addremove = false;

		o = s.option(form.Value, 'dns_domain', _('DNS-over-TLS domain'),
			_('E.g. dns.example.net: a phone of profile "Alex" uses alex.dns.example.net.'));
		o.datatype = 'hostname';
		o.rmempty = true;

		o = s.option(form.Value, 'agh_url', _('AdGuard Home address'),
			_('The AdGuard Home web interface as the router reaches it, e.g. http://192.168.1.2:3000.'));
		o.placeholder = 'http://192.168.1.2:3000';
		o.validate = function(section_id, value) {
			return (!value || /^https?:\/\/[^\s\/]+\/?$/.test(value)) ? true : _('Expected http://host[:port] or https://host[:port]');
		};
		o.rmempty = true;
		// Honest warning instead of a lock icon: plain HTTP exposes the login on the LAN.
		o.renderWidget = function(section_id, option_index, cfgvalue) {
			var node = form.Value.prototype.renderWidget.apply(this, [section_id, option_index, cfgvalue]);
			var warn = E('div', { 'class': 'fw-warning' }, [
				E('strong', {}, _('Plain HTTP: the login crosses your LAN unencrypted.')),
				' ', _('Use https:// if AdGuard Home offers it, or keep it on a network segment you trust.')
			]);
			var sync = function(value) { warn.hidden = !/^http:\/\//.test(value || ''); };
			sync(cfgvalue);
			node.addEventListener('input', function(ev) { sync(ev.target.value); });
			return E('div', {}, [node, warn]);
		};

		o = s.option(form.DummyValue, '_dns_auth', _('AdGuard Home login'),
			_('Use a separate AdGuard Home user for FamilyWarden (AdGuard Home has no read-only users: any user can change everything). The password is stored on the router readable by root only and is never shown again.'));
		o.renderWidget = function() {
			var user = E('input', { 'type': 'text', 'class': 'cbi-input-text', 'value': 'familywarden', 'autocomplete': 'off' });
			var pass = E('input', { 'type': 'password', 'class': 'cbi-input-password', 'autocomplete': 'new-password' });
			return E('div', { 'class': 'fw-immediate' }, [
				E('span', { 'class': 'fw-immediate-tag' }, _('Applies immediately · not part of Save & Apply')),
				E('div', {}, dns.credentials ? '✓ ' + _('Login stored') : _('No login stored')),
				E('button', { 'class': 'cbi-button', 'click': function(ev) {
					ev.preventDefault();
					ui.showModal(_('AdGuard Home login'), [
						E('div', { 'class': 'cbi-value' }, [ E('label', { 'class': 'cbi-value-title' }, _('Username')), E('div', { 'class': 'cbi-value-field' }, user) ]),
						E('div', { 'class': 'cbi-value' }, [ E('label', { 'class': 'cbi-value-title' }, _('Password')), E('div', { 'class': 'cbi-value-field' }, pass) ]),
						E('div', { 'class': 'right' }, [
							E('button', { 'class': 'cbi-button', 'click': ui.hideModal }, _('Cancel')), ' ',
							E('button', { 'class': 'cbi-button cbi-button-positive', 'click': function() {
								return callDnsAuth(user.value, pass.value).then(function(res) {
									ui.hideModal();
									if (res && res.error)
										ui.addNotification(null, E('p', {}, res.error), 'warning');
									else
										ui.addNotification(null, E('p', {}, _('Login stored. FamilyWarden syncs AdGuard Home within a minute.')), 'info');
								});
							} }, _('Save login'))
						])
					]);
				} }, dns.credentials ? _('Change login…') : _('Set login…'))
			]);
		};

		o = s.option(form.DummyValue, '_dns_status', _('Sync status'));
		o.renderWidget = function() {
			var box = E('div', { 'class': 'fw-immediate' });
			var show = function(st) {
				var lines = [ E('span', { 'class': 'fw-immediate-tag' }, _('Applies immediately · not part of Save & Apply')) ];
				if (st.error)
					lines.push(E('div', { 'class': 'fw-warning is-fault' }, '⚠ ' + st.error));
				else if (+st.time > 0)
					lines.push(E('div', {}, '✓ ' + _('In sync (%s)').format(new Date(st.time * 1000).toLocaleString())));
				else
					lines.push(E('div', {}, _('Not synced yet')));
				var ids = Object.keys(st.profiles || {});
				if (ids.length)
					lines.push(E('table', { 'class': 'fw-table fw-dns-table' }, [
						E('thead', {}, E('tr', {}, [ E('th', {}, _('Profile')), E('th', {}, _('DNS identity')) ])),
						E('tbody', {}, ids.map(function(p) {
							return E('tr', {}, [
								E('td', { 'data-label': _('Profile') }, uci.get('familywarden', p, 'name') || p),
								E('td', { 'data-label': _('DNS identity') }, E('code', { 'class': 'fw-mono' }, st.profiles[p].hostname))
							]);
						}))
					]));
				lines.push(E('p', { 'class': 'fw-muted' }, _('In sync means the router reconciled AdGuard Home\'s client settings. It does not confirm that a phone applied its policy.')));
				lines.push(E('button', { 'class': 'cbi-button', 'click': function(ev) {
					ev.preventDefault();
					ev.currentTarget.disabled = true;
					return callDnsSync().then(show);
				} }, _('Sync now')));
				box.replaceChildren.apply(box, lines);
			};
			show(dns);
			return box;
		};

		// Ulysses. Frozen for the pact's lifetime (the backend uses the captain
		// and cooling-off stored with the pact, so edits here would be ignored).
		s = m.section(form.TypedSection, 'global', _('Ulysses'),
			bound
				? _('A pact is active, so these settings are frozen until it ends or you sail home.')
				: _('Optional: bind yourself to the mast. The Ulysses profile (yours) can make a pact: while it lasts, the profile is frozen and, whenever it is blocked, the router is closed to every device except the sail page on port 7700.'));
		s.anonymous = true;
		s.addremove = false;

		o = s.option(form.ListValue, 'ulysses_profile', _('Ulysses profile'),
			_('The captain: the only profile that can make a pact. Usually your own.'));
		o.value('', _('None'));
		uci.sections('familywarden', 'profile').forEach(function(p) {
			o.value(p['.name'], p.name || p['.name']);
		});
		o.readonly = bound;

		o = s.option(form.Value, 'ulysses_delay', _('Cooling-off (hours)'),
			_('How long "sailing home" on the sail page takes before the pact is lifted. Frozen into each pact when it is made.'));
		o.datatype = 'range(1,720)';
		o.placeholder = '24';
		o.default = '24';
		o.readonly = bound;

		return m.render().then(function(formEl) {
			return E('div', { 'id': 'fw-settings' }, [
				fwui.header(_('Settings'), _('Form fields are staged until Save & Apply. Sections marked "applies immediately" talk to AdGuard Home right away.')),
				formEl
			]);
		});
	}
});
