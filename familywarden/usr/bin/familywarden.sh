#!/bin/sh
# familywarden - deprecated standalone daemon
#
# DEPRECATED: This script is no longer used. The rpcd plugin at
# /usr/libexec/rpcd/familywarden handles all familywarden operations.
# This stub remains for backward compatibility but does nothing.
#
# The new architecture uses procd triggers + fw4 reload instead of
# cron-driven polling.

echo "familywarden: this script is deprecated (use /usr/libexec/rpcd/familywarden)"
exit 0
