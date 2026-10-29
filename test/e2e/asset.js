// Packaged LuCI assets live in a per-release directory (luci-app-familywarden/Makefile FW_ASSET).
const { readFileSync } = require('node:fs');
const make = readFileSync(require('node:path').resolve(__dirname, '../../luci-app-familywarden/Makefile'), 'utf8');
const field = name => make.match(new RegExp(`^${name}:=(.+)$`, 'm'))[1].trim();
const ASSET = `v${field('PKG_VERSION').replace(/\./g, '_')}_${field('PKG_RELEASE')}`;
module.exports = { ASSET, SHARED: `/luci-static/resources/familywarden/${ASSET}/shared` };
