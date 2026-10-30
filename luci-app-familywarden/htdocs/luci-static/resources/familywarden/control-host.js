'use strict';
'require baseclass';
'require rpc';

// Thin host contract only. All presentation lives in the repository's web/;
// LuCI supplies its existing session/ACL RPC path and optional native facts.
var definitions = {
 core_status: ['core_status'], version: ['version'],
 configuration_status: ['configuration_status'], configuration_export: ['configuration_export'],
 configuration_import: ['configuration_import','revision'], configuration_report: ['list'],
 configuration_apply: ['configuration_apply','revision','current'], configuration_confirm: ['configuration_confirm','revision','token'],
 configuration_amend: ['configuration_amend','revision','current'], configuration_dns_update: ['configuration_dns_update','revision','current'],
 configuration_block: ['block','profile'], configuration_allow: ['allow','profile'], configuration_reset: ['reset','profile'],
 configuration_ticket: ['ticket','profile','minutes'], configuration_bonus_add: ['bonus','profile','minutes','package'],
 configuration_pact: ['pact','until','window_day','window_start','window_stop'],
 configuration_dns_status: ['dns_status'], configuration_dns_sync: ['dns_sync'], configuration_dns_auth: ['dns_auth','username','password'],
 configuration_dns_services: ['dns_services'],
 devices: ['devices'], profiles: ['profiles'], requests: ['requests'], usage: ['usage'], new_apps: ['new_apps'], admin_installed: ['admin_installed','profile','search','filter','page','sort'], inventory_icons: ['inventory_icons','profile','packages','offset'],
 device_approve: ['device_approve','device','profile','name','administrator','fingerprint','approves'],
 device_update: ['device_update','device','profile','name','administrator','approves'],
 device_reject: ['device_reject','device'], device_delete: ['device_delete','device'], device_allow_removal: ['device_allow_removal','device'],
 app_decision: ['app_decision','profile','package','action'], request_answer: ['request_answer','request_id','approve','minutes']
};
var methods = {};
Object.keys(definitions).forEach(function(name) {
 var definition = definitions[name];
 methods[name] = rpc.declare({object:'familywarden',method:definition[0],params:definition.slice(1),expect:{'':{}}});
});
var callMast = rpc.declare({object:'familywarden',method:'mast',params:['host'],expect:{'':{}}});

function call(method, params) {
 var definition = definitions[method];
 if (!definition) return Promise.reject(new Error('Unsupported core method: ' + method));
 params = params || {};
 return methods[method].apply(null,definition.slice(1).map(function(key) {return params[key];}));
}

return baseclass.extend({
 call: call,
 options: function() {
  var port = new URLSearchParams(window.location.search).get('sail_port');
  // Forwarded development ports must be explicit, never point at another VM's
  // recovery service. Normal router-origin LuCI uses its native port 7700.
  if (!port && (!window.location.port || window.location.port === '80' || window.location.port === '443')) port = '7700';
  if (!/^[0-9]+$/.test(port || '') || +port < 1 || +port > 65535) port = null;
  var endpoint = port ? window.location.hostname + ':' + port : null;
  return {
   call:call, lang:document.documentElement.lang || 'en',
   recoveryURL:endpoint ? 'http://' + endpoint + '/' : null,
   callHost:function(method) {
    if (method !== 'pairing' || !endpoint) return Promise.reject(new Error('Native enrollment address unavailable'));
    return callMast(endpoint).then(function(reply) {
     if (reply.error) throw new Error(reply.error);
     return {uri:reply.pair_uri,fingerprint:reply.fingerprint,qr:reply.qr_svg};
    });
   }
  };
 }
});
