'use strict';
'require view';
'require form';
'require rpc';
'require uci';

var callReload = rpc.declare({
	object: 'familywarden',
	method: 'reload'
});

return view.extend({
	render: function() {
		var m, s, o;

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

		// Return the form.Map directly so LuCI renders it with native page
		// chrome (title + tab bar) like every other config page.
		return m.render();
	}
});
