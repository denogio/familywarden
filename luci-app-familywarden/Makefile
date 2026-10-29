#
# Copyright (C) 2026 denogio <dennis@hojgaard.me>
#
# This is free software, licensed under the Apache 2.0 License.

include $(TOPDIR)/rules.mk

LUCI_TITLE:=LuCI support for FamilyWarden Parental Control
LUCI_DEPENDS:=+familywarden +luci-base
LUCI_PKGARCH:=all

PKG_VERSION:=0.11.0
PKG_RELEASE:=9

PKG_MAINTAINER:=denogio <dennis@hojgaard.me>
PKG_LICENSE:=Apache-2.0

# Canonical UI sources live outside LuCI; SDK builds expose the repository
# through FW_SRC. Other native hosts package the very same web/ assets.
FAMILYWARDEN_WEB ?= $(if $(FW_SRC),$(FW_SRC)/web,$(CURDIR)/../web)
PKG_FILE_DEPENDS += $(wildcard $(FAMILYWARDEN_WEB)/*)

include ../../luci.mk

# Define files to install
define Package/luci-app-familywarden/install
	# One page only: the shared workspace from web/ plus this thin host adapter.
	$(INSTALL_DIR) $(1)/www/luci-static/resources/view/familywarden
	$(INSTALL_DATA) ./htdocs/luci-static/resources/view/familywarden/control.js $(1)/www/luci-static/resources/view/familywarden/control.js

	$(INSTALL_DIR) $(1)/www/luci-static/resources/familywarden
	$(INSTALL_DATA) ./htdocs/luci-static/resources/familywarden/control-host.js $(1)/www/luci-static/resources/familywarden/control-host.js
	# Shared administrator UI (A20): platform-neutral ES modules
	$(INSTALL_DIR) $(1)/www/luci-static/resources/familywarden/shared
	$(INSTALL_DATA) $(FAMILYWARDEN_WEB)/*.js $(1)/www/luci-static/resources/familywarden/shared/
	$(INSTALL_DATA) $(FAMILYWARDEN_WEB)/*.css $(1)/www/luci-static/resources/familywarden/shared/

	$(INSTALL_DIR) $(1)/usr/share/luci/menu.d
	$(INSTALL_DATA) ./root/usr/share/luci/menu.d/luci-app-familywarden.json $(1)/usr/share/luci/menu.d/luci-app-familywarden.json

	$(INSTALL_DIR) $(1)/usr/share/rpcd/acl.d
	$(INSTALL_DATA) ./root/usr/share/rpcd/acl.d/luci-app-familywarden.json $(1)/usr/share/rpcd/acl.d/luci-app-familywarden.json
endef

$(eval $(call BuildPackage,luci-app-familywarden))
