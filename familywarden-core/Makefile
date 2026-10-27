#
# Copyright (C) 2026 denogio <dennis@hojgaard.me>
#
# This is free software, licensed under the Apache 2.0 License.

include $(TOPDIR)/rules.mk

PKG_NAME:=familywarden-core
PKG_VERSION:=0.13.0
PKG_RELEASE:=16

PKG_MAINTAINER:=denogio <dennis@hojgaard.me>
PKG_LICENSE:=Apache-2.0

# Two ways to build:
# - prebuilt/familywardend present (scripts/build-apk-docker.sh cross-compiles it with
#   plain Go in seconds): package that binary, no Go toolchain in the SDK;
# - otherwise compile from ./src (a symlink to ../core; copy with cp -L) with the
#   feeds' golang, which first builds the whole Go toolchain (~10 min).
PREBUILT:=$(wildcard $(CURDIR)/prebuilt/familywardend)

ifeq ($(PREBUILT),)
PKG_BUILD_DEPENDS:=golang/host
PKG_BUILD_PARALLEL:=1
PKG_BUILD_FLAGS:=no-mips16
GO_PKG:=github.com/denogio/familywarden/core
GO_PKG_BUILD_PKG:=$(GO_PKG)/cmd/familywardend
GO_PKG_LDFLAGS_X:=main.version=$(PKG_VERSION)-r$(PKG_RELEASE)
endif

include $(INCLUDE_DIR)/package.mk
ifeq ($(PREBUILT),)
include $(TOPDIR)/feeds/packages/lang/golang/golang-package.mk
CORE_BIN:=$(GO_PKG_BUILD_BIN_DIR)/familywardend
CORE_ARCH_DEPENDS:=$(GO_ARCH_DEPENDS)
else
CORE_BIN:=$(PREBUILT)
CORE_ARCH_DEPENDS:=
define Build/Compile
endef
endif

define Package/familywarden-core
  SECTION:=utils
  CATEGORY:=Utilities
  TITLE:=FamilyWarden core (phone identity and signed policy)
  # No +familywarden: the shell package depends on the core (phone identity).
  DEPENDS:=$(CORE_ARCH_DEPENDS)
  URL:=https://github.com/denogio/familywarden
endef

define Package/familywarden-core/description
  The router-agnostic FamilyWarden core: phone enrollment, device registry and
  signed phone sync. It listens only on a root-only unix socket; the familywarden
  package is the OpenWrt adapter (firewall, schedules, pacts, policy content).
endef

define Package/familywarden-core/install
	$(INSTALL_DIR) $(1)/usr/sbin
	$(INSTALL_BIN) $(CORE_BIN) $(1)/usr/sbin/familywardend

	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/familywarden-core.init $(1)/etc/init.d/familywarden-core

	# ubus bridge: `ubus call familywarden-core status`
	$(INSTALL_DIR) $(1)/usr/libexec/rpcd
	$(INSTALL_BIN) ./files/familywarden-core.rpcd $(1)/usr/libexec/rpcd/familywarden-core
endef

# rpcd only scans exec plugins at (re)load: without this the ubus object is
# missing after install and lingers after removal. An upgrade must also restart
# the daemon, or the old binary keeps serving until the next reboot.
define Package/familywarden-core/postinst
#!/bin/sh
[ -n "$${IPKG_INSTROOT}" ] && exit 0
/etc/init.d/rpcd reload
/etc/init.d/familywarden-core enabled && /etc/init.d/familywarden-core restart
exit 0
endef

define Package/familywarden-core/postrm
#!/bin/sh
[ -n "$${IPKG_INSTROOT}" ] || /etc/init.d/rpcd reload
exit 0
endef

ifeq ($(PREBUILT),)
$(eval $(call GoBinPackage,familywarden-core))
endif
$(eval $(call BuildPackage,familywarden-core))
