'use strict';
'require view';
'require form';
'require rpc';
'require uci';
'require familywarden.ulysses as ulysses';

var callReload = rpc.declare({
	object: 'familywarden',
	method: 'reload'
});

var callList = rpc.declare({
	object: 'familywarden',
	method: 'list',
	expect: { profiles: {} }
});

return view.extend({
	load: function() {
		return Promise.all([ callList(), uci.load('familywarden') ]);
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

		s = m.section(form.TypedSection, 'global', _('Global Settings'));
		s.anonymous = true;
		s.addremove = false;

		o = s.option(form.Flag, 'enabled', _('Enable FamilyWarden'));
		o.rmempty = false;
		o.default = '1';

		o = s.option(form.ListValue, 'default_policy', _('Default Policy'),
			_('Default action for devices not assigned to any profile'));
		o.value('allow', _('Allow Internet'));
		o.value('block', _('Block Internet'));
		o.default = 'allow';

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

		// Return the form.Map directly so LuCI renders it with native page
		// chrome (title + tab bar) like every other config page.
		return m.render();
	}
});
