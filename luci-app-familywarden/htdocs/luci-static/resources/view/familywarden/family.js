'use strict';
'require view';
'require rpc';

// LuCI wrapper for the shared administrator UI (A20): LuCI login, session and ACLs,
// rpcd shim to the core. The page itself lives in familywarden/shared/family.js.
var methods = {
	requests: rpc.declare({ object: 'familywarden', method: 'requests', expect: { '': {} } }),
	usage: rpc.declare({ object: 'familywarden', method: 'usage', expect: { '': {} } }),
	request_answer: rpc.declare({ object: 'familywarden', method: 'request_answer',
		params: [ 'request_id', 'approve', 'minutes' ], expect: { '': {} } })
};

return view.extend({
	load: function() {
		return import(L.resource('familywarden/shared/family.js'));
	},

	render: function(module) {
		var root = E('div', { 'class': 'cbi-map' });
		module.mount(root, {
			lang: document.documentElement.lang || 'en',
			call: function(method, params) {
				var fn = methods[method];
				if (!fn) return Promise.reject(new Error('unknown method ' + method));
				return method === 'request_answer'
					? fn(params.request_id, params.approve, params.minutes)
					: fn();
			}
		});
		return root;
	},

	handleSave: null,
	handleSaveApply: null,
	handleReset: null
});
