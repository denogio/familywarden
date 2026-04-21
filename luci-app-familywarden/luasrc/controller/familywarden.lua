-- Copyright 2026 Dennis Højgaard
-- Licensed under Apache 2.0

module("luci.controller.familywarden", package.seeall)

function index()
    entry({"admin", "services", "familywarden"}, firstchild(), _("FamilyWarden"), 60).dependent = false

    -- Overview/Dashboard
    entry({"admin", "services", "familywarden", "overview"},
        template("familywarden/overview"), _("Overview"), 10).leaf = true

    -- Profiles management (CBI)
    entry({"admin", "services", "familywarden", "profiles"},
        cbi("familywarden/profiles"), _("Profiles"), 20).leaf = true

    -- Schedules (CBI)
    entry({"admin", "services", "familywarden", "schedules"},
        cbi("familywarden/schedules"), _("Schedules"), 30).leaf = true

    -- Settings
    entry({"admin", "services", "familywarden", "settings"},
        cbi("familywarden/settings"), _("Settings"), 40).leaf = true

    -- AJAX endpoints for quick actions
    entry({"admin", "services", "familywarden", "action"},
        call("action_block"), nil).leaf = true
    entry({"admin", "services", "familywarden", "ticket"},
        call("action_ticket"), nil).leaf = true
end

-- Get ubus connection
local function ubus_connect()
    local ubus = require "ubus"
    local conn = ubus.connect()
    if not conn then
        return nil, "Failed to connect to ubus"
    end
    return conn
end

-- Quick action: block/allow/reset profile
function action_block()
    local u = ubus_connect()
    if not u then
        luci.http.status(500, "UBus connection failed")
        return
    end

    local profile = luci.http.formvalue("profile")
    local action = luci.http.formvalue("action") -- block, allow, reset

    if not profile or not action then
        luci.http.status(400, "Bad Request")
        return
    end

    -- Validate action
    if action ~= "block" and action ~= "allow" and action ~= "reset" then
        luci.http.status(400, "Invalid action")
        return
    end

    local result = u:call("familywarden", action, { profile = profile })
    u:disconnect()

    luci.http.prepare_content("application/json")
    luci.http.write_json(result or { status = "error", message = "No response" })
end

-- Give time ticket to profile
function action_ticket()
    local u = ubus_connect()
    if not u then
        luci.http.status(500, "UBus connection failed")
        return
    end

    local profile = luci.http.formvalue("profile")
    local minutes = tonumber(luci.http.formvalue("minutes") or 30)

    if not profile then
        luci.http.status(400, "Bad Request")
        return
    end

    if minutes < 1 or minutes > 480 then
        luci.http.status(400, "Invalid minutes (1-480)")
        return
    end

    local result = u:call("familywarden", "ticket", { profile = profile, minutes = minutes })
    u:disconnect()

    luci.http.prepare_content("application/json")
    luci.http.write_json(result or { status = "error", message = "No response" })
end
