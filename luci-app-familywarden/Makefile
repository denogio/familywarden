#
# Copyright (C) 2026 Dennis Højgaard
#
# This is free software, licensed under the Apache 2.0 License.

include $(TOPDIR)/rules.mk

LUCI_TITLE:=LuCI support for FamilyWarden Parental Control
LUCI_DEPENDS:=+familywarden +luci-base
LUCI_PKGARCH:=all

PKG_VERSION:=0.4.0
PKG_RELEASE:=1

PKG_MAINTAINER:=Dennis Højgaard <github@dehoej.dk>
PKG_LICENSE:=Apache-2.0

include ../../luci.mk

# Define files to install
define Package/luci-app-familywarden/install
	$(INSTALL_DIR) $(1)/www/luci-static/resources/view/familywarden
	$(INSTALL_DATA) ./htdocs/luci-static/resources/view/familywarden/overview.js $(1)/www/luci-static/resources/view/familywarden/overview.js
	$(INSTALL_DATA) ./htdocs/luci-static/resources/view/familywarden/profiles.js $(1)/www/luci-static/resources/view/familywarden/profiles.js
	$(INSTALL_DATA) ./htdocs/luci-static/resources/view/familywarden/settings.js $(1)/www/luci-static/resources/view/familywarden/settings.js
	$(INSTALL_DATA) ./htdocs/luci-static/resources/view/familywarden/info.js $(1)/www/luci-static/resources/view/familywarden/info.js
	$(SED) 's/@@VERSION@@/$(PKG_VERSION)-r$(PKG_RELEASE)/' $(1)/www/luci-static/resources/view/familywarden/info.js

	$(INSTALL_DIR) $(1)/usr/share/luci/menu.d
	$(INSTALL_DATA) ./root/usr/share/luci/menu.d/luci-app-familywarden.json $(1)/usr/share/luci/menu.d/luci-app-familywarden.json

	$(INSTALL_DIR) $(1)/usr/share/rpcd/acl.d
	$(INSTALL_DATA) ./root/usr/share/rpcd/acl.d/luci-app-familywarden.json $(1)/usr/share/rpcd/acl.d/luci-app-familywarden.json
endef

$(eval $(call BuildPackage,luci-app-familywarden))
