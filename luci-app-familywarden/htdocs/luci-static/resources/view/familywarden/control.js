'use strict';
'require view';
'require familywarden.control-host as host';

// Native host only: the shared source and rendering live outside LuCI in web/.
return view.extend({
 load:function() {return import(L.resource('familywarden/shared/control.js'));},
 render:function(module) {
  var root = E('div');
  var themeRoot = document.documentElement;
  var theme = function() {
   var dark = themeRoot.getAttribute('data-darkmode');
   return dark === 'true' ? 'dark' : dark === 'false' ? 'light' : 'auto';
  };
  var workspace = module.mount(root,Object.assign({},host.options(),{theme:theme()}));
  var observer = new MutationObserver(function() { workspace.setTheme(theme()); });
  observer.observe(themeRoot,{attributes:true,attributeFilter:['data-darkmode']});
  // LuCI replaces the view on navigation; dispose both theme and polling listeners.
  var attached = false;
  var lifetime = new MutationObserver(function() {
   if (root.isConnected) attached = true;
   else if (attached) { observer.disconnect(); lifetime.disconnect(); workspace.stop(); }
  });
  lifetime.observe(document.body,{childList:true,subtree:true});
  // Only the host frame is adjusted here; all product styling lives in web/.
  root.appendChild(E('style', {}, '#maincontent.container:has(.fwa){width:auto;max-width:none;margin-inline:20px}#view>.fwa{min-height:calc(100vh - 100px);border-radius:14px;overflow:clip}'));
  return root;
 },
 handleSave:null,handleSaveApply:null,handleReset:null
});
