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

var callAmend = rpc.declare({
	object: 'familywarden',
	method: 'configuration_amend',
	params: [ 'revision', 'current' ]
});

var callDNSUpdate = rpc.declare({
	object: 'familywarden',
	method: 'configuration_dns_update',
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
		// sections() enumerates live IDs but retains stale properties for locally
		// deleted options. get() honors those staged unsets without saving UCI.
		Object.keys(fields).forEach(function(k) {
			out[k] = text(fields[k] === '.name' ? s['.name'] : uci.get('familywarden', s['.name'], fields[k]));
		});
		Object.keys(lists || {}).forEach(function(k) { out[k] = list(uci.get('familywarden', s['.name'], lists[k])); });
		return out;
	});
}

function ordered(records) {
	return (records || []).slice().sort(function(a, b) {
		return a.id < b.id ? -1 : a.id > b.id ? 1 : 0;
	});
}

function normalized(value) {
	return JSON.stringify(value, function(k, v) {
		if (v === null) return [];
		if (v && typeof v === 'object' && !Array.isArray(v))
			return Object.keys(v).sort().reduce(function(o, key) { o[key] = v[key]; return o; }, {});
		return v;
	});
}

// Order-insensitive, null-slice-tolerant form of the editable document parts.
function canonical(current) {
	return normalized([ 'profiles', 'schedules', 'apps', 'downtime' ].map(function(key) {
		return ordered(current[key]);
	}));
}

function ensure(res) {
	if (!res || res.error)
		throw new Error(res && res.error || 'Configuration write failed');
}

function commitImmediate(call, configuration, current) {
	return call(configuration.revision, current).then(function(res) {
		ensure(res);
		if (res.status !== 'ok' || typeof res.revision !== 'string')
			throw new Error('Invalid committed configuration reply');
		configuration.revision = res.revision;
		return configuration;
	});
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
		var out = Object.assign({}, current, {
			profiles: read('profile', PROFILE, PROFILE_LISTS),
			schedules: read('schedule', WINDOW),
			apps: read('app', APP),
			downtime: read('app_downtime', WINDOW)
		});
		[ 'profiles', 'schedules', 'apps', 'downtime' ].forEach(function(key) {
			var before = new Map((current[key] || []).map(function(record) { return [record.id, record]; }));
			out[key] = out[key].map(function(record) {
				var original = before.get(record.id);
				if (!original) return record;
				record = Object.assign({}, original, record);
				Object.keys(original).forEach(function(field) {
					if (normalized(original[field]) === normalized(record[field]))
						record[field] = original[field];
				});
				return normalized(original) === normalized(record) ? original : record;
			});
			// Do not turn null lists into [] or reorder unchanged records: the
			// core's pact guard compares the untouched profile/snapshot exactly.
			if (Object.prototype.hasOwnProperty.call(current, key) &&
				normalized(ordered(current[key])) === normalized(ordered(out[key])))
				out[key] = current[key];
		});
		return out;
	},
	// A frozen form may normalize readonly defaults while parsing. A DNS-only
	// write must keep those unrelated values exact, not stage profile changes.
	dnsFromUci: function(current, id) {
		var edited = this.fromUci(current).profiles.find(function(p) { return p.id === id; });
		if (!edited) throw new Error('Profile unavailable');
		return Object.assign({}, current, { profiles: current.profiles.map(function(p) {
			return p.id !== id ? p : Object.assign({}, p, { dns: edited.dns, client_id: edited.client_id,
				protections: edited.protections, services: edited.services });
		}) });
	},
	// True when the loaded UCI projection still equals the exported document;
	// otherwise editing it would overwrite core state the form never showed.
	matchesUci: function(current) {
		return canonical(this.fromUci(current)) === canonical(current);
	},
	// Explicit pact amendment: committed immediately, no connection rollback.
	// The caller must obtain the user's acknowledgement before invoking this.
	amend: function(configuration, current) {
		return commitImmediate(callAmend, configuration, current);
	},
	// Only DNS may change; the core retains the pact's accumulated minimum.
	updateDNS: function(configuration, current) {
		return commitImmediate(callDNSUpdate, configuration, current);
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
