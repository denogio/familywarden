'use strict';
'require baseclass';
'require rpc';

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
