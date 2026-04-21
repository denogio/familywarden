#!/bin/sh
# familywarden daemon - OpenWrt parental control
# Handles profile-based internet blocking with schedules

. /lib/functions.sh
. /lib/functions/network.sh

FW_VERSION="0.1.0"
FW_CONFIG="/etc/config/familywarden"
FW_STATE="/tmp/familywarden.state"
FW_LOCK="/tmp/familywarden.lock"
FW_STATUS="/tmp/familywarden.status"
FW_CHAIN="familywarden_forward"
FW_NFT_FILE="/etc/nftables.d/10-familywarden.nft"

# Logging
log() {
    logger -t familywarden "$1"
}

# Initialize nftables chain
nft_init() {
    nft list chain inet fw4 ${FW_CHAIN} 2>/dev/null || \
        nft add chain inet fw4 ${FW_CHAIN} "{ type filter hook forward priority 0; policy accept; }"
    log "nftables chain initialized"
}

# Remove all rules for a profile
nft_clear_profile() {
    local profile="$1"
    local handles
    handles=$(nft -a list chain inet fw4 ${FW_CHAIN} 2>/dev/null | \
        grep "familywarden:${profile}" | sed 's/.*# handle //')
    for handle in $handles; do
        nft delete rule inet fw4 ${FW_CHAIN} handle ${handle}
    done
}

# Block devices in a profile
nft_block_profile() {
    local profile="$1"
    nft_clear_profile "$profile"
    _nft_block_profile="$profile"
    config_load familywarden
    config_list_foreach "$profile" device _nft_block_device_cb
}

_nft_block_device_cb() {
    local mac="$1"
    nft add rule inet fw4 ${FW_CHAIN} ether saddr ${mac} counter drop comment "\"familywarden:${_nft_block_profile}\""
}

# Check if a schedule is currently active
is_schedule_active() {
    local schedule="$1"
    local now_day now_time start stop action days

    config_load familywarden
    config_get days "$schedule" days ""
    config_get start "$schedule" start ""
    config_get stop "$schedule" stop ""
    config_get action "$schedule" action "allow"

    [ -z "$days" ] || [ -z "$start" ] || [ -z "$stop" ] && return 1

    now_day=$(date +%a | tr 'A-Z' 'a-z')

    # Check if today is in schedule
    echo "$days" | grep -q "$now_day" || return 1

    # Convert HH:MM to minutes for numeric comparison
    # Use %-H/%-M to strip leading zeros (08/09 are invalid octal in ash)
    local now_min start_min stop_min sh sm eh em
    now_min=$(($(date +%-H) * 60 + $(date +%-M)))
    sh=${start%:*}; sm=${start#*:}
    eh=${stop%:*}; em=${stop#*:}
    start_min=$(( ${sh#0} * 60 + ${sm#0} ))
    stop_min=$(( ${eh#0} * 60 + ${em#0} ))

    # Check time range (handles overnight schedules)
    if [ "$start_min" -lt "$stop_min" ]; then
        [ "$now_min" -ge "$start_min" ] && [ "$now_min" -lt "$stop_min" ]
    else
        # Overnight schedule (e.g., 22:00 - 06:00)
        [ "$now_min" -ge "$start_min" ] || [ "$now_min" -lt "$stop_min" ]
    fi
}

# Find effective action from all active schedules for a profile.
# Returns: "block", "allow", or "" (no active schedule).
# Rule: block wins over allow when schedules overlap.
get_schedule_action() {
    local profile="$1"
    _sched_result=""
    _sched_profile="$profile"

    config_load familywarden
    config_foreach _collect_schedule schedule

    echo "$_sched_result"
}

_collect_schedule() {
    local schedule="$1"
    local profile action

    config_get profile "$schedule" profile ""
    [ "$profile" = "$_sched_profile" ] || return 0

    if is_schedule_active "${schedule}"; then
        config_get action "$schedule" action "allow"
        if [ "$action" = "block" ]; then
            # Block always wins
            _sched_result="block"
        elif [ "$_sched_result" != "block" ]; then
            _sched_result="allow"
        fi
    fi
    return 0
}

# Evaluate a single profile
evaluate_profile() {
    local profile="$1"
    local name policy enabled status override override_expires next_change
    local devices mac schedule_info schedule_action

    config_load familywarden
    config_get name "$profile" name "$profile"
    config_get enabled "$profile" enabled "0"
    config_get policy "$profile" policy "allow"

    [ "$enabled" = "1" ] || return 0

    # Get state
    status=$(state_get "${profile}.status" "allowed")
    override=$(state_get "${profile}.override" "none")
    override_expires=$(state_get "${profile}.override_expires" "")

    # Check if override expired (override_expires is stored as epoch seconds)
    if [ -n "$override_expires" ]; then
        if [ "$(date +%s)" -gt "$override_expires" ]; then
            state_set "${profile}.override" "none"
            state_set "${profile}.override_expires" ""
            override="none"
        fi
    fi

    # Determine desired state
    local desired_state="allowed"

    case "$override" in
        block)
            desired_state="blocked"
            ;;
        allow)
            desired_state="allowed"
            ;;
        none)
            case "$policy" in
                block)
                    desired_state="blocked"
                    ;;
                allow)
                    desired_state="allowed"
                    ;;
                scheduled)
                    schedule_action=$(get_schedule_action "${profile}")
                    case "$schedule_action" in
                        block)  desired_state="blocked" ;;
                        allow)  desired_state="allowed" ;;
                        *)      desired_state="blocked" ;;  # No active schedule = blocked
                    esac
                    ;;
            esac
            ;;
    esac

    # Apply state change
    if [ "$status" != "$desired_state" ]; then
        if [ "$desired_state" = "blocked" ]; then
            nft_block_profile "${profile}"
            state_set "${profile}.status" "blocked"
            state_set "${profile}.blocked_since" "$(date -Iseconds)"
            log "${profile} (${name}) blocked"
        else
            nft_clear_profile "${profile}"
            state_set "${profile}.status" "allowed"
            state_set "${profile}.blocked_since" ""
            log "${profile} (${name}) allowed"
        fi
    fi
}

# Evaluate all profiles
evaluate_all() {
    config_load familywarden
    config_foreach evaluate_profile profile
    write_status
}

# State file helpers
state_set() {
    local key="$1"
    local value="$2"
    mkdir -p "$(dirname "${FW_STATE}")"
    sed -i "/^${key}=/d" "${FW_STATE}" 2>/dev/null
    echo "${key}=${value}" >> "${FW_STATE}"
}

state_get() {
    local key="$1"
    local default="${2:-}"
    local line
    line=$(grep "^${key}=" "${FW_STATE}" 2>/dev/null) || { echo "$default"; return; }
    echo "${line#*=}"
}

# Device list callback (appends MAC to $devices variable)
_append_device() {
    local mac="$1"
    if [ -n "$devices" ]; then
        devices="${devices}, \"${mac}\""
    else
        devices="\"${mac}\""
    fi
}

# Write status JSON for ubus
write_status() {
    local first=1

    echo '{"profiles": {' > "${FW_STATUS}"

    _status_first=1
    config_load familywarden
    config_foreach _write_profile_status profile

    echo '}}' >> "${FW_STATUS}"
}

_write_profile_status() {
    local profile="$1"
    local name policy enabled status override devices blocked_since override_expires full_status

    config_get name "$profile" name "$profile"
    config_get enabled "$profile" enabled "0"
    config_get policy "$profile" policy "allow"

    [ "$enabled" = "1" ] || return 0

    status=$(state_get "${profile}.status" "allowed")
    override=$(state_get "${profile}.override" "none")
    blocked_since=$(state_get "${profile}.blocked_since" "")
    override_expires=$(state_get "${profile}.override_expires" "")

    devices=""
    config_list_foreach "$profile" device _append_device

    full_status="$status"
    [ "$override" != "none" ] && full_status="override_${status}"

    if [ "$_status_first" = "1" ]; then
        _status_first=0
    else
        printf ',' >> "${FW_STATUS}"
    fi

    printf '"%s": {"name":"%s","enabled":%s,"policy":"%s","devices":[%s],"status":"%s","override":"%s","blocked_since":"%s","override_expires":%s}' \
        "$profile" "$name" "$enabled" "$policy" "$devices" "$full_status" "$override" "$blocked_since" "${override_expires:-0}" >> "${FW_STATUS}"
}

# Ubus commands
ubus_list() {
    cat "${FW_STATUS}"
}

ubus_status() {
    local profile="$1"
    local name policy enabled status override devices blocked_since override_expires

    config_load familywarden
    config_get name "$profile" name "$profile"
    config_get enabled "$profile" enabled "0"
    config_get policy "$profile" policy "allow"

    status=$(state_get "${profile}.status" "allowed")
    override=$(state_get "${profile}.override" "none")
    blocked_since=$(state_get "${profile}.blocked_since" "")
    override_expires=$(state_get "${profile}.override_expires" "")

    devices=""
    config_list_foreach "$profile" device _append_device

    cat <<EOF
{
  "name": "${name}",
  "enabled": ${enabled},
  "policy": "${policy}",
  "devices": [${devices}],
  "status": "${status}",
  "override": "${override}",
  "blocked_since": "${blocked_since}",
  "override_expires": ${override_expires:-0}
}
EOF
}

ubus_block() {
    local profile="$1"
    state_set "${profile}.override" "block"
    state_set "${profile}.override_expires" ""
    evaluate_profile "${profile}"
    write_status
    echo '{"status":"ok","profile":"'${profile}'","state":"blocked","override":true}'
}

ubus_allow() {
    local profile="$1"
    state_set "${profile}.override" "allow"
    state_set "${profile}.override_expires" ""
    evaluate_profile "${profile}"
    write_status
    echo '{"status":"ok","profile":"'${profile}'","state":"allowed","override":true}'
}

ubus_reset() {
    local profile="$1"
    state_set "${profile}.override" "none"
    state_set "${profile}.override_expires" ""
    evaluate_profile "${profile}"
    write_status
    echo '{"status":"ok","profile":"'${profile}'","state":"scheduled"}'
}

ubus_ticket() {
    local profile="$1"
    local minutes="${2:-30}"
    local expires_epoch expires_iso

    expires_epoch=$(($(date +%s) + minutes * 60))
    expires_iso=$(date -d "@${expires_epoch}" -Iseconds 2>/dev/null || date -u -d "@${expires_epoch}" +%Y-%m-%dT%H:%M:%S 2>/dev/null || echo "${expires_epoch}")
    state_set "${profile}.override" "allow"
    state_set "${profile}.override_expires" "${expires_epoch}"
    evaluate_profile "${profile}"
    write_status
    echo '{"status":"ok","profile":"'${profile}'","expires":"'${expires_iso}'"}'
}

# Main entry point
main() {
    local cmd="$1"
    shift

    case "$cmd" in
        init)
            nft_init
            evaluate_all
            ;;
        eval)
            evaluate_all
            ;;
        list)
            ubus_list
            ;;
        status)
            ubus_status "$1"
            ;;
        block)
            ubus_block "$1"
            ;;
        allow)
            ubus_allow "$1"
            ;;
        reset)
            ubus_reset "$1"
            ;;
        ticket)
            ubus_ticket "$1" "$2"
            ;;
        reload)
            config_load familywarden
            evaluate_all
            echo '{"status":"ok"}'
            ;;
        *)
            echo "Usage: $0 {init|eval|list|status|block|allow|reset|ticket|reload}" >&2
            exit 1
            ;;
    esac
}

main "$@"
