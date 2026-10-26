'use strict';
'require baseclass';
'require rpc';
'require uci';

// Core-configuration CAS writes. Apply the candidate against the revision that
// was read at export time; the router activates it and waits for an explicit
// confirmation (revision + token). A lost browser or timeout is never a
// confirmation: an unconfirmed edit is rolled back on its deadline or restart.
var callApply = rpc.declare({
	object: 'familywarden',
	method: 'configuration_apply',
	params: [ 'revision', 'current' ]
});

var callConfirm = rpc.declare({
	object: 'familywarden',
	method: 'configuration_confirm',
	params: [ 'revision', 'token' ]
});

// The router projects the core document into /etc/config/familywarden with the
// document's own section IDs; the same option names as the shell's legacy
// export (_cfg_profile/_cfg_window/_cfg_app) map them back.
var PROFILE = { id: '.name', name: 'name', enabled: 'enabled', policy: 'policy',
	block_router: 'block_router', dns: 'dns', client_id: 'dns_client_id', apps: 'apps',
	suspend_new: 'apps_suspend_new', screen_minutes: 'screen_minutes',
	screen_weekday: 'screen_weekday', screen_weekend: 'screen_weekend' };
var PROFILE_LISTS = { devices: 'device', protections: 'dns_protection',
	services: 'dns_service', always_allowed: 'always_allowed' };
var WINDOW = { id: '.name', profile: 'profile', days: 'days', start: 'start', stop: 'stop' };
var APP = { id: '.name', profile: 'profile', package: 'package', minutes: 'minutes', blocked: 'blocked' };

function text(v) {
	return v == null ? '' : Array.isArray(v) ? v.join(' ') : String(v);
}

function list(v) {
	return v == null ? [] : Array.isArray(v) ? v.slice() : [ String(v) ];
}

function read(type, fields, lists) {
	return uci.sections('familywarden', type).map(function(s) {
		var out = {};
		Object.keys(fields).forEach(function(k) { out[k] = text(s[fields[k]]); });
		Object.keys(lists || {}).forEach(function(k) { out[k] = list(s[lists[k]]); });
		return out;
	});
}

// Order-insensitive, null-slice-tolerant form of the editable document parts.
function canonical(current) {
	return JSON.stringify([ 'profiles', 'schedules', 'apps', 'downtime' ].map(function(key) {
		return (current[key] || []).slice().sort(function(a, b) {
			return a.id < b.id ? -1 : a.id > b.id ? 1 : 0;
		});
	}), function(k, v) {
		if (v === null) return [];
		if (v && typeof v === 'object' && !Array.isArray(v))
			return Object.keys(v).sort().reduce(function(o, key) { o[key] = v[key]; return o; }, {});
		return v;
	});
}

function ensure(res) {
	if (!res || res.error)
		throw new Error(res && res.error || 'Configuration write failed');
}

// configurationExport() returns {owner, configuration, pending};
// commit(configuration, current) writes current against configuration.revision
// and confirms it.
return baseclass.extend({
	configurationExport: rpc.declare({
		object: 'familywarden',
		method: 'configuration_export',
		expect: { '': {} }
	}),
	// The candidate document: settings and legacy phones from the exported
	// document, profiles/windows/apps from the (locally edited) UCI cache.
	fromUci: function(current) {
		return Object.assign({}, current, {
			profiles: read('profile', PROFILE, PROFILE_LISTS),
			schedules: read('schedule', WINDOW),
			apps: read('app', APP),
			downtime: read('app_downtime', WINDOW)
		});
	},
	// True when the loaded UCI projection still equals the exported document;
	// otherwise editing it would overwrite core state the form never showed.
	matchesUci: function(current) {
		return canonical(this.fromUci(current)) === canonical(current);
	},
	commit: function(configuration, current) {
		return callApply(configuration.revision, current).then(function(res) {
			ensure(res);
			if (res.status === 'confirmation_required')
				return callConfirm(res.revision, res.token).then(function(ok) {
					ensure(ok);
					configuration.revision = res.revision;
					return configuration;
				});
			configuration.revision = res.revision;
			return configuration;
		});
	}
});
