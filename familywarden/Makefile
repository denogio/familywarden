#
# Copyright (C) 2026 Dennis Højgaard
#
# This is free software, licensed under the Apache 2.0 License.

include $(TOPDIR)/rules.mk

PKG_NAME:=familywarden
PKG_VERSION:=0.7.1
PKG_RELEASE:=1

PKG_MAINTAINER:=Dennis Højgaard <github@dehoej.dk>
PKG_LICENSE:=Apache-2.0

include $(INCLUDE_DIR)/package.mk

define Package/familywarden
  SECTION:=utils
  CATEGORY:=Utilities
  TITLE:=OpenWrt Parental Control Daemon
  DEPENDS:=+firewall4 +nftables +rpcd
  URL:=https://github.com/dehoej/familywarden
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

	$(INSTALL_DIR) $(1)/usr/share/rpcd/acl.d
	$(INSTALL_DATA) ./usr/share/rpcd/acl.d/familywarden.json $(1)/usr/share/rpcd/acl.d/familywarden.json
endef

define Package/familywarden/postinst
#!/bin/sh
[ "$$1" = "configure" ] || exit 0
# Idempotent: add firewall include if not already present
if ! uci -q get firewall.familywarden >/dev/null 2>&1; then
    uci batch <<-EOF
	add firewall include
	set firewall.@include[-1].fw4_compatible='1'
	set firewall.@include[-1].type='nftables'
	set firewall.@include[-1].path='/var/etc/familywarden.include'
	commit firewall
EOF
fi
exit 0
endef

define Package/familywarden/prerm
#!/bin/sh
# Remove firewall include on package removal
local idx
idx=$$(uci show firewall | grep "path='/var/etc/familywarden.include'" | head -1 | cut -d. -f2)
[ -n "$$idx" ] && uci delete "firewall.$$idx" && uci commit firewall
exit 0
endef

$(eval $(call BuildPackage,familywarden))
