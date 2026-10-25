#
# Copyright (C) 2026 denogio <dennis@hojgaard.me>
#
# This is free software, licensed under the Apache 2.0 License.

include $(TOPDIR)/rules.mk

PKG_NAME:=familywarden
PKG_VERSION:=0.11.0_rc4
PKG_RELEASE:=1

PKG_MAINTAINER:=denogio <dennis@hojgaard.me>
PKG_LICENSE:=Apache-2.0

include $(INCLUDE_DIR)/package.mk

define Package/familywarden
  SECTION:=utils
  CATEGORY:=Utilities
  TITLE:=OpenWrt Parental Control Daemon
  DEPENDS:=+familywarden-core +firewall4 +nftables +rpcd +uhttpd +qrencode +jsonfilter +uclient-fetch +ucode +ucode-mod-fs
  URL:=https://github.com/denogio/familywarden
  PKGARCH:=all
endef

define Package/familywarden/description
  Profile-based parental control with schedules and internet blocking.
  Schedules are enforced by the kernel via nftables meta hour/day matching.
  Exposed via ubus API for Home Assistant integration.
endef

define Build/Compile
endef

define Package/familywarden/install
	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_CONF) ./etc/config/familywarden $(1)/etc/config/familywarden

	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./etc/init.d/familywarden $(1)/etc/init.d/familywarden

	$(INSTALL_DIR) $(1)/usr/libexec/rpcd
	$(INSTALL_BIN) ./usr/lib/rpcd/familywarden $(1)/usr/libexec/rpcd/familywarden

	# CLI for root (reload, tick, ...)
	$(INSTALL_DIR) $(1)/usr/sbin
	$(LN) ../libexec/rpcd/familywarden $(1)/usr/sbin/familywarden

	# Ulysses sail page, served on port 7700 by its own uhttpd instance
	$(INSTALL_DIR) $(1)/www-familywarden/cgi-bin
	$(INSTALL_DATA) ./www-familywarden/index.html $(1)/www-familywarden/index.html
	$(INSTALL_BIN) ./www-familywarden/cgi-bin/sail $(1)/www-familywarden/cgi-bin/sail
	$(INSTALL_BIN) ./www-familywarden/cgi-bin/mast $(1)/www-familywarden/cgi-bin/mast
	$(INSTALL_BIN) ./www-familywarden/cgi-bin/app $(1)/www-familywarden/cgi-bin/app

	$(INSTALL_DIR) $(1)/usr/share/rpcd/acl.d
	$(INSTALL_DATA) ./usr/share/rpcd/acl.d/familywarden.json $(1)/usr/share/rpcd/acl.d/familywarden.json
endef

define Package/familywarden/postinst
#!/bin/sh
# Only on a live system, not while assembling a firmware image. (No
# "configure" arg check: that is an opkg convention and apk never passes it.)
[ -n "$${IPKG_INSTROOT}" ] && exit 0
# Idempotent: add firewall include if not already present. The include is an
# anonymous section, so match on its path (same as prerm).
if ! uci show firewall | grep -q "path='/var/etc/familywarden.include'"; then
    uci batch <<-EOF
	add firewall include
	set firewall.@include[-1].fw4_compatible='1'
	set firewall.@include[-1].type='nftables'
	set firewall.@include[-1].path='/var/etc/familywarden.include'
	commit firewall
EOF
fi
# Ulysses sail page: a separate uhttpd instance on port 7700 (static page + one
# CGI, no LuCI, no /ubus). Kept reachable during a Ulysses lockdown.
if ! uci -q get uhttpd.familywarden >/dev/null; then
    uci batch <<-EOF
	set uhttpd.familywarden=uhttpd
	add_list uhttpd.familywarden.listen_http='0.0.0.0:7700'
	add_list uhttpd.familywarden.listen_http='[::]:7700'
	set uhttpd.familywarden.home='/www-familywarden'
	set uhttpd.familywarden.cgi_prefix='/cgi-bin'
	set uhttpd.familywarden.no_dirlists='1'
	set uhttpd.familywarden.max_requests='3'
	commit uhttpd
EOF
    /etc/init.d/uhttpd reload 2>/dev/null
fi
exit 0
endef

define Package/familywarden/prerm
#!/bin/sh
# Remove firewall include on package removal
idx=$$(uci show firewall | grep "path='/var/etc/familywarden.include'" | head -1 | cut -d. -f2)
[ -n "$$idx" ] && uci delete "firewall.$$idx" && uci commit firewall
if uci -q get uhttpd.familywarden >/dev/null; then
    uci delete uhttpd.familywarden && uci commit uhttpd
    /etc/init.d/uhttpd reload 2>/dev/null
fi
exit 0
endef

$(eval $(call BuildPackage,familywarden))
