'use strict';
'require view';
'require familywarden.control-host as host';

// Native host only: the shared source and rendering live outside LuCI in web/.
return view.extend({
 load:function() {return import(L.resource('familywarden/shared/control.js'));},
 render:function(module) {
  var root = E('div');
  module.mount(root,host.options());
  // Only the host frame is adjusted here; all product styling lives in web/.
  root.appendChild(E('style', {}, '#maincontent.container:has(.fwa){width:auto;max-width:none;margin-inline:20px}#view>.fwa{min-height:calc(100vh - 100px);border-radius:14px;overflow:clip}'));
  return root;
 },
 handleSave:null,handleSaveApply:null,handleReset:null
});
