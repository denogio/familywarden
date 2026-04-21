#
# Copyright (C) 2026 Dennis Højgaard
#
# This is free software, licensed under the Apache 2.0 License.

include $(TOPDIR)/rules.mk

PKG_NAME:=familywarden
PKG_VERSION:=0.1.0
PKG_RELEASE:=1

PKG_MAINTAINER:=Dennis Højgaard <github@dehoej.dk>
PKG_LICENSE:=Apache-2.0

include $(INCLUDE_DIR)/package.mk

define Package/familywarden
  SECTION:=utils
  CATEGORY:=Utilities
  TITLE:=OpenWrt Parental Control Daemon
  DEPENDS:=+kmod-nftables +nftables +libubox +libubus +libuci +rpcd
  URL:=https://github.com/dehoej/familywarden
endef

define Package/familywarden/description
  Profile-based parental control with schedules, internet blocking,
  and DNS filtering. Exposed via ubus API for Home Assistant integration.
endef

define Build/Compile
endef

define Package/familywarden/install
	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_CONF) ./etc/config/familywarden $(1)/etc/config/familywarden

	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./etc/init.d/familywarden $(1)/etc/init.d/familywarden

	$(INSTALL_DIR) $(1)/usr/bin
	$(INSTALL_BIN) ./usr/bin/familywarden.sh $(1)/usr/bin/familywarden

	$(INSTALL_DIR) $(1)/usr/share/rpcd/ucode
	$(INSTALL_DATA) ./usr/share/rpcd/ucode/familywarden $(1)/usr/share/rpcd/ucode/familywarden

	$(INSTALL_DIR) $(1)/usr/share/rpcd/acl.d
	$(INSTALL_DATA) ./usr/share/rpcd/acl.d/familywarden.json $(1)/usr/share/rpcd/acl.d/familywarden.json
endef

$(eval $(call BuildPackage,familywarden))
