'use strict';
'require view';
'require form';
'require rpc';
'require uci';
'require ui';
'require familywarden.ulysses as ulysses';

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
		return Promise.all([ callList(), uci.load('familywarden'), callMast(window.location.hostname),
			L.resolveDefault(callDnsStatus(), {}) ]);
	},

	render: function(data) {
		var m, s, o;
		var profiles = data[0] || {};
		var bound = Object.keys(profiles).some(function(id) { return ulysses.isBound(profiles[id]); });

		m = new form.Map('familywarden', _('FamilyWarden Settings'),
			_('Configure global FamilyWarden options.'));

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

		s = m.section(form.TypedSection, 'global', _('Global Settings'),
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

		// Ulysses. Frozen for the pact's lifetime (the backend uses the captain
		// and cooling-off stored with the pact, so edits here would be ignored).
		s = m.section(form.TypedSection, 'global', '⚓ ' + _('Ulysses'),
			bound
				? _('A pact is active, so these settings are frozen until it ends or you sail home.')
				: _('Bind yourself to the mast. The Ulysses profile (yours) can make a pact: while it lasts, the profile is frozen and, whenever it is blocked, the router is closed to every device except the sail page on port 7700.'));
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

		// Ulysses Mast: pair the captain's phone by scanning this QR in the app
		var mast = data[2] || {};
		o = s.option(form.DummyValue, '_mast', '📱 ' + _('Pair a phone'),
			_('Scan this code with the FamilyWarden app (or the older Ulysses Mast app); the phone must be on this network. A FamilyWarden phone then waits for your approval under Phones, where you compare its fingerprint.'));
		o.renderWidget = function() {
			var synced = mast.last_sync > 0
				? _('Last sync: %s (%s)').format(new Date(mast.last_sync * 1000).toLocaleString(), mast.device || '?')
				: _('No phone has synced yet');
			var qr = E('div', { 'style': 'width:220px;height:220px;background:#fff;padding:8px;border-radius:8px' });
			if (mast.qr_svg)
				qr.innerHTML = mast.qr_svg.replace(/width="[^"]*" height="[^"]*"/, 'width="100%" height="100%"');
			else
				qr = E('code', { 'style': 'word-break:break-all' }, mast.pair_uri || _('Pairing code unavailable'));
			return E('div', {}, [
				qr,
				E('div', { 'style': 'margin-top:8px' }, _('Key fingerprint: %s').format(mast.fingerprint || '?')),
				E('div', {}, synced)
			]);
		};

		// DNS filtering through AdGuard Home. The router manages only its own
		// "familywarden-*" clients and the ClientIDs it added to AdGuard Home's
		// allowed clients; everything else in AdGuard Home is left alone.
		var dns = data[3] || {};
		s = m.section(form.TypedSection, 'global', '🛡 ' + _('DNS filtering (AdGuard Home)'),
			_('Phones of profiles with "Strict DNS" use DNS-over-TLS to &lt;ClientID&gt;.&lt;DNS domain&gt;. Your AdGuard Home needs encryption set up with a certificate covering that domain and its subdomains, reachable on port 853 from home and away. FamilyWarden only adds, changes and removes its own "familywarden-…" clients and the ClientIDs it added to the allowed clients list; your other clients, lists and settings are never changed.'));
		s.anonymous = true;
		s.addremove = false;

		o = s.option(form.Value, 'dns_domain', _('DNS-over-TLS domain'),
			_('E.g. dns.example.net: a phone of profile "Alex" uses alex.dns.example.net.'));
		o.datatype = 'hostname';
		o.rmempty = true;

		o = s.option(form.Value, 'agh_url', _('AdGuard Home address'),
			_('The AdGuard Home web interface as the router reaches it, e.g. http://192.168.1.2:3000. Plain http:// sends the password unencrypted, so only use it within your own network.'));
		o.placeholder = 'http://192.168.1.2:3000';
		o.validate = function(section_id, value) {
			return (!value || /^https?:\/\/[^\s\/]+\/?$/.test(value)) ? true : _('Expected http://host[:port] or https://host[:port]');
		};
		o.rmempty = true;

		o = s.option(form.DummyValue, '_dns_auth', _('AdGuard Home login'),
			_('Use a separate AdGuard Home user for FamilyWarden (AdGuard Home has no read-only users: any user can change everything). The password is stored on the router readable by root only and is never shown again.'));
		o.renderWidget = function() {
			var user = E('input', { 'type': 'text', 'class': 'cbi-input-text', 'value': 'familywarden', 'autocomplete': 'off' });
			var pass = E('input', { 'type': 'password', 'class': 'cbi-input-password', 'autocomplete': 'new-password' });
			return E('div', {}, [
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
			var box = E('div', {});
			var show = function(st) {
				var lines = [];
				if (st.error)
					lines.push(E('div', { 'style': 'color:var(--error-color-medium,#c00)' }, '⚠ ' + st.error));
				else if (+st.time > 0)
					lines.push(E('div', {}, '✓ ' + _('In sync (%s)').format(new Date(st.time * 1000).toLocaleString())));
				else
					lines.push(E('div', {}, _('Not synced yet')));
				Object.keys(st.profiles || {}).forEach(function(p) {
					lines.push(E('div', {}, [ uci.get('familywarden', p, 'name') || p, ': ', E('code', {}, st.profiles[p].hostname) ]));
				});
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

		// Return the form.Map directly so LuCI renders it with native page
		// chrome (title + tab bar) like every other config page.
		return m.render();
	}
});
