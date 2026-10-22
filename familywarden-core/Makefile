#
# Copyright (C) 2026 denogio <dennis@hojgaard.me>
#
# This is free software, licensed under the Apache 2.0 License.

include $(TOPDIR)/rules.mk

PKG_NAME:=familywarden-core
PKG_VERSION:=0.1.0
PKG_RELEASE:=1

PKG_MAINTAINER:=denogio <dennis@hojgaard.me>
PKG_LICENSE:=Apache-2.0

PKG_BUILD_DEPENDS:=golang/host
PKG_BUILD_PARALLEL:=1
PKG_BUILD_FLAGS:=no-mips16

# Sources come from ./src (a symlink to ../core in the repository; copy with cp -L).
GO_PKG:=github.com/denogio/familywarden/core
GO_PKG_BUILD_PKG:=$(GO_PKG)/cmd/familywardend
GO_PKG_LDFLAGS_X:=main.version=$(PKG_VERSION)-r$(PKG_RELEASE)

include $(INCLUDE_DIR)/package.mk
include $(TOPDIR)/feeds/packages/lang/golang/golang-package.mk

define Package/familywarden-core
  SECTION:=utils
  CATEGORY:=Utilities
  TITLE:=FamilyWarden core (identity, signed policy, app control)
  DEPENDS:=$(GO_ARCH_DEPENDS) +familywarden
  URL:=https://github.com/denogio/familywarden
endef

define Package/familywarden-core/description
  The compiled FamilyWarden core. It listens only on a root-only unix socket;
  the familywarden shell package stays the firewall/schedule/pact adapter.
endef

define Package/familywarden-core/install
	$(INSTALL_DIR) $(1)/usr/sbin
	$(INSTALL_BIN) $(GO_PKG_BUILD_BIN_DIR)/familywardend $(1)/usr/sbin/familywardend

	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/familywarden-core.init $(1)/etc/init.d/familywarden-core

	# ubus bridge: `ubus call familywarden-core status`
	$(INSTALL_DIR) $(1)/usr/libexec/rpcd
	$(INSTALL_BIN) ./files/familywarden-core.rpcd $(1)/usr/libexec/rpcd/familywarden-core
endef

# rpcd only scans exec plugins at (re)load: without this the ubus object is
# missing after install and lingers after removal.
define Package/familywarden-core/postinst
#!/bin/sh
[ -n "$${IPKG_INSTROOT}" ] || /etc/init.d/rpcd reload
exit 0
endef

define Package/familywarden-core/postrm
#!/bin/sh
[ -n "$${IPKG_INSTROOT}" ] || /etc/init.d/rpcd reload
exit 0
endef

$(eval $(call GoBinPackage,familywarden-core))
$(eval $(call BuildPackage,familywarden-core))
