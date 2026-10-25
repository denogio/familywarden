# Frozen compatibility oracle from ca08ec4, not production code.
# Source after /lib/functions.sh and load the effective configuration first.
_legacy_valid_package() {
    case "$1" in ''|*[!A-Za-z0-9_.]*|*.|*..*) return 1 ;; esac
    case "$1" in [A-Za-z]*.*) ;; *) return 1 ;; esac
    [ "${#1}" -le 255 ]
}

_legacy_apps_line_cb() {
    local s="$1" p pkg min blk
    config_get p "$s" profile ""
    [ "$p" = "$_legacy_apps_profile" ] || return 0
    config_get pkg "$s" package ""
    _legacy_valid_package "$pkg" || { logger -t familywarden "app rule $s: invalid package ignored"; return 0; }
    config_get min "$s" minutes ""
    case "$min" in
        '') ;;
        *[!0-9]*) logger -t familywarden "app rule $s: invalid minutes, blocking $pkg"; min=0 ;;
        *) [ "${#min}" -le 4 ] && [ "$min" -le 1440 ] || min=1440 ;;
    esac
    config_get blk "$s" blocked "0"
    [ "$blk" = "1" ] || blk=0
    echo "app|$pkg|$min|$blk"
}

_legacy_apps_down_cb() {
    local s="$1" p days start stop d
    config_get p "$s" profile ""
    [ "$p" = "$_legacy_apps_profile" ] || return 0
    config_get days "$s" days ""
    config_get start "$s" start ""
    config_get stop "$s" stop ""
    for d in $days; do
        case "$d" in mon|tue|wed|thu|fri|sat|sun) ;; *) days="" ;; esac
    done
    if [ -z "$days" ] || ! echo "$start $stop" | grep -qE '^([01][0-9]|2[0-3]):[0-5][0-9] ([01][0-9]|2[0-3]):[0-5][0-9]$'; then
        logger -t familywarden "app downtime $s: invalid, ignored"
        return 0
    fi
    echo "down|$(echo $days)|$start|$stop"
}

# Canonical, sorted app policy of a profile, one rule per line (empty when app
# control is off). Duplicate packages merge to the strictest rule: blocked wins,
# the smallest limit wins. The revision is a hash of exactly these lines.
# Needs a loaded config.
_legacy_apps_canon() {
    local on susp
    config_get on "$1" apps "0"
    [ "$on" = "1" ] || return 0
    config_get susp "$1" apps_suspend_new "1"
    [ "$susp" = "0" ] || susp=1
    _legacy_apps_profile="$1"
    {
        echo "new|$susp||"
        config_foreach _legacy_apps_line_cb app
        config_foreach _legacy_apps_down_cb app_downtime
        _legacy_apps_screen_lines "$1"
        config_list_foreach "$1" always_allowed _legacy_apps_always_cb
    } | awk -F'|' '
        $1 == "app" {
            if (!($2 in seen)) { seen[$2] = 1; min[$2] = $3; blk[$2] = $4; next }
            if ($4 == 1) blk[$2] = 1
            if ($3 != "" && (min[$2] == "" || $3 + 0 < min[$2] + 0)) min[$2] = $3
            next
        }
        { print }
        END { for (p in seen) print "app|" p "|" min[p] "|" blk[p] }' | sort -u
}

# Daily total screen time (D9): one value for all days, or weekday/weekend values
# that override it. Expanded per weekday so the phone decides offline.
_legacy_valid_minutes() {
    case "$1" in ''|*[!0-9]*) return 1 ;; esac
    [ "${#1}" -le 4 ] && [ "$1" -le 1440 ]
}

_legacy_apps_screen_lines() {
    local all wd we d v
    config_get all "$1" screen_minutes ""
    config_get wd "$1" screen_weekday ""
    config_get we "$1" screen_weekend ""
    for d in mon tue wed thu fri sat sun; do
        case "$d" in sat|sun) v=${we:-$all} ;; *) v=${wd:-$all} ;; esac
        [ -n "$v" ] || continue
        if _legacy_valid_minutes "$v"; then echo "screen|$d|$v|"
        else logger -t familywarden "screen time of $1: invalid minutes '$v' ignored"; fi
    done
}

_legacy_apps_always_cb() {
    _legacy_valid_package "$1" && echo "always|$1||" || logger -t familywarden "always_allowed: invalid package ignored"
}

