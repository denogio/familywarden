'use strict';
'require view';
'require form';
'require rpc';

var callReload = rpc.declare({
	object: 'familywarden',
	method: 'reload'
});

var callVersion = rpc.declare({
	object: 'familywarden',
	method: 'version',
	expect: { backend: '', frontend: '' }
});

function buildInfoBox() {
	// Card-style box with backend/frontend versions. Fails soft if the
	// backend ubus 'version' method isn't available (e.g. older rpcd plugin
	// still installed alongside a newer luci frontend).
	var placeholder = E('em', { 'style': 'color:var(--text-color-medium,#888)' }, _('Loading…'));
	var box = E('div', { 'class': 'cbi-section', 'style': 'padding:16px;margin-top:24px;border:1px solid var(--border-color-medium,#dee2e6);border-radius:8px' }, [
		E('h3', { 'style': 'margin-top:0' }, _('Info')),
		E('table', { 'class': 'table' }, [
			E('tr', {}, [ E('td', { 'width': '40%' }, E('strong', {}, _('Backend package'))), E('td', { 'id': 'fw-version-backend' }, placeholder) ]),
			E('tr', {}, [ E('td', {}, E('strong', {}, _('LuCI frontend app'))), E('td', { 'id': 'fw-version-frontend' }, placeholder) ])
		])
	]);
	// Fetch versions asynchronously; resolve both or both-unknown together.
	Promise.all([
		callVersion().then(function(v) { return (v && v.backend) || 'unknown'; })
			.catch(function() { return 'unknown'; }),
		callVersion().then(function(v) { return (v && v.frontend) || 'unknown'; })
			.catch(function() { return 'unknown'; })
	]).then(function(results) {
		var backend = results[0] || 'unknown';
		var frontend = results[1] || 'unknown';
		box.querySelector('#fw-version-backend').textContent = backend;
		box.querySelector('#fw-version-frontend').textContent = frontend;
	});
	return box;
}

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

		return m.render().then(function(formEl) {
			return E('div', {}, [
				E('h2', {}, _('Settings')),
				formEl,
				buildInfoBox()
			]);
		});
	}
});
