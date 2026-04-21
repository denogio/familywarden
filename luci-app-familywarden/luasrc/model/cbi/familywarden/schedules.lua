-- Copyright 2026 Dennis Højgaard
-- Licensed under Apache 2.0

local m, s, o

m = Map("familywarden", translate("Schedules"),
    translate("Define time-based access schedules for device profiles."))

-- Helper to get list of profiles
local function get_profiles()
    local profiles = {}
    local uci = require "luci.model.uci".cursor()
    uci:foreach("familywarden", "profile", function(s)
        if s[".name"] and s.name then
            profiles[s[".name"]] = s.name
        end
    end)
    return profiles
end

-- Schedules section
s = m:section(TypedSection, "schedule", translate("Time Schedules"))
s.addremove = true
s.anonymous = true
s.template = "cbi/tblsection"

-- Profile selector
o = s:option(DummyValue, "_profile", translate("Profile"))
function o.cfgvalue(self, section)
    local profile = m:get(section, "profile") or ""
    local name = get_profiles()[profile] or profile
    return name
end

o = s:option(ListValue, "profile", translate("Profile"))
local profiles = get_profiles()
for id, name in pairs(profiles) do
    o:value(id, name)
end
o.rmempty = true

-- Days selector
o = s:option(MultiValue, "days", translate("Days"))
o:value("mon", translate("Monday"))
o:value("tue", translate("Tuesday"))
o:value("wed", translate("Wednesday"))
o:value("thu", translate("Thursday"))
o:value("fri", translate("Friday"))
o:value("sat", translate("Saturday"))
o:value("sun", translate("Sunday"))
o.rmempty = true

-- Time range
o = s:option(Value, "start", translate("Start Time"))
o.datatype = "time"
o.placeholder = "07:00"
o.rmempty = true

o = s:option(Value, "stop", translate("Stop Time"))
o.datatype = "time"
o.placeholder = "20:00"
o.rmempty = true

-- Action
o = s:option(ListValue, "action", translate("Action"))
o:value("allow", translate("Allow Internet"))
o:value("block", translate("Block Internet"))
o.rmempty = false

-- Display schedule summary
o = s:option(DummyValue, "_schedule", translate("Schedule Summary"))
function o.cfgvalue(self, section)
    local days = m:get(section, "days") or ""
    local start = m:get(section, "start") or ""
    local stop = m:get(section, "stop") or ""
    local action = m:get(section, "action") or "allow"

    if days == "" or start == "" or stop == "" then
        return translate("(incomplete)")
    end

    local day_names = {
        mon = "Mon", tue = "Tue", wed = "Wed",
        thu = "Thu", fri = "Fri", sat = "Sat", sun = "Sun"
    }

    local day_list = {}
    for day in string.gmatch(days, "%w+") do
        table.insert(day_list, day_names[day] or day)
    end

    local days_str = table.concat(day_list, ", ")
    local action_str = (action == "allow") and "✓" or "✗"

    return string.format("%s: %s - %s [%s]", days_str, start, stop, action_str)
end

return m
