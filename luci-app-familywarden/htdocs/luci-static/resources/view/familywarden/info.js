'use strict';
'require view';
'require rpc';
'require familywarden.ui as fwui';

// Baked in at build time by the Makefile (sed replaces @@VERSION@@ with
// PKG_VERSION-PKG_RELEASE). This reflects the version of THIS JS file as it
// actually runs in the browser — so if the browser serves a cached copy, this
// value stays old even though the installed package is newer. Comparing it to
// the backend-reported installed package reveals a stale cache.
var FRONTEND_VERSION = '@@VERSION@@';

var callVersion = rpc.declare({
	object: 'familywarden',
	method: 'version',
	// expect takes a SINGLE key; '' selects the whole reply object. Naming a
	// key (e.g. {backend:''}) would return only that field as a string.
	expect: { '': {} }
});

// Pull the bare "X.Y.Z-rN" out of "luci-app-familywarden-X.Y.Z-rN".
function pkgVer(s) {
	if (!s || s === 'unknown') return s || 'unknown';
	var m = s.match(/(\d+\.\d+\.\d+-r\d+)$/);
	return m ? m[1] : s;
}

return view.extend({
	load: function() {
		return callVersion().catch(function() {
			return { backend: 'unknown', frontend: 'unknown' };
		});
	},

	render: function(v) {
		v = v || {};
		fwui.loadStyles('overview');
		fwui.loadStyles('pages');
		var backend = v.backend || 'unknown';
		var installedFrontend = pkgVer(v.frontend);
		// If the Makefile didn't substitute (running straight from source), say so.
		var runningFrontend = (FRONTEND_VERSION.indexOf('@@') === 0) ? 'dev (unbuilt)' : FRONTEND_VERSION;

		// Cache check: the JS we are running vs the installed luci package.
		var stale = runningFrontend !== 'dev (unbuilt)' && installedFrontend !== 'unknown'
			&& installedFrontend !== runningFrontend;

		return E('div', { 'id': 'fw-info' }, [
			fwui.header(_('Info'), _('Installed FamilyWarden packages and the frontend running in this browser.')),
			stale ? E('div', { 'class': 'fw-notice is-fault', 'role': 'status' }, [
				E('strong', {}, _('This page is running an older frontend than the one installed')),
				E('p', {}, _('Your browser cached frontend %s; the router has %s. Reload without the cache so forms and actions match the backend.').format(runningFrontend, installedFrontend)),
				E('button', { 'class': 'cbi-button', 'click': function() { location.reload(); } }, _('Reload page'))
			]) : '',
			E('section', { 'class': 'fw-panel' }, [
				E('h3', {}, _('Versions')),
				E('dl', { 'class': 'fw-kv' }, [
					E('dt', {}, _('Installed backend')), E('dd', { 'class': 'fw-mono' }, pkgVer(backend)),
					E('dt', {}, _('Installed frontend')), E('dd', { 'class': 'fw-mono' }, installedFrontend),
					E('dt', {}, _('Running in this browser')), E('dd', { 'class': 'fw-mono' }, [ runningFrontend,
						stale ? E('span', { 'class': 'fw-warning-inline' }, ' · ' + _('cached — hard-refresh (Ctrl+Shift+R)')) : '' ])
				]),
				E('p', { 'class': 'fw-muted' }, _('FamilyWarden — profile-based parental control with kernel-enforced schedules, exposed over ubus for Home Assistant.'))
			])
		]);
	},

	handleSave: null,
	handleSaveApply: null,
	handleReset: null
});
