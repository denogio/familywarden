'use strict';
'require view';
'require rpc';

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

function row(label, value, extra) {
	return E('tr', { 'class': 'tr' }, [
		E('td', { 'class': 'td left', 'width': '45%' }, E('strong', {}, label)),
		E('td', { 'class': 'td left' }, extra ? [ value, ' ', extra ] : value)
	]);
}

return view.extend({
	load: function() {
		return callVersion().catch(function() {
			return { backend: 'unknown', frontend: 'unknown' };
		});
	},

	render: function(v) {
		v = v || {};
		var backend = v.backend || 'unknown';
		var installedFrontend = pkgVer(v.frontend);
		// If the Makefile didn't substitute (running straight from source), say so.
		var runningFrontend = (FRONTEND_VERSION.indexOf('@@') === 0) ? 'dev (unbuilt)' : FRONTEND_VERSION;

		// Cache check: the JS we are running vs the installed luci package.
		var cacheNote = null;
		if (runningFrontend !== 'dev (unbuilt)' && installedFrontend !== 'unknown'
			&& installedFrontend !== runningFrontend) {
			cacheNote = E('span', { 'style': 'color:var(--error-color-high,#dc3545);font-weight:600' },
				'⚠ ' + _('cached — hard-refresh (Ctrl+Shift+R)'));
		}

		return E('div', {}, [
			E('h2', {}, _('Info')),
			E('div', { 'class': 'cbi-section' }, [
				E('h3', {}, _('Versions')),
				E('table', { 'class': 'table' }, [
					row(_('Frontend (running JS)'), runningFrontend, cacheNote),
					row(_('Frontend (installed pkg)'), installedFrontend),
					row(_('Backend (installed pkg)'), pkgVer(backend))
				])
			]),
			E('div', { 'class': 'cbi-section' }, [
				E('p', {}, _('FamilyWarden — profile-based parental control with kernel-enforced schedules, exposed over ubus for Home Assistant.'))
			])
		]);
	},

	handleSave: null,
	handleSaveApply: null,
	handleReset: null
});
