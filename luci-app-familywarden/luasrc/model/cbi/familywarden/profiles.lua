-- Copyright 2026 Dennis Højgaard
-- Licensed under Apache 2.0

local m, s, o

m = Map("familywarden", translate("Profiles"),
    translate("Manage device profiles for parental control."))

-- Helper to get MAC addresses from DHCP leases
local function get_dhcp_leases()
    local leases = {}
    local uci = require "luci.model.uci".cursor()

    -- Try to read dnsmasq/dhcp leases
    local f = io.open("/tmp/dhcp.leases", "r")
    if f then
        for line in f:lines() do
            local mac = line:match("^%d+%s+([%x%x:%x%x:%x%x:%x%x:%x%x:%x%x]+)")
            if mac then
                leases[#leases + 1] = mac
            end
        end
        f:close()
    end

    return leases
end

-- Profiles section
s = m:section(TypedSection, "profile", translate("Device Profiles"))
s.addremove = true
s.anonymous = false
s.template = "cbi/tblsection"

-- Enable/Disable
o = s:option(Flag, "enabled", translate("Enabled"))
o.rmempty = false
o.width = "10%"

-- Profile Name
o = s:option(Value, "name", translate("Name"))
o.rmempty = false
o.width = "25%"

-- Policy Type
o = s:option(ListValue, "policy", translate("Policy"))
o:value("allow", translate("Always allow"))
o:value("block", translate("Always block"))
o:value("scheduled", translate("By schedule"))
o.rmempty = false
o.width = "25%"

-- Device Count (display only)
o = s:option(DummyValue, "_devices", translate("Devices"))
o.width = "20%"
function o.cfgvalue(self, section)
    local devices = self.map:get(section, "device") or {}
    if type(devices) == "string" then
        devices = { devices }
    end
    return string.format("%d %s", #devices, #devices == 1 and "device" or "devices")
end

-- Action buttons (edit/delete)
o = s:option(DummyValue, "_actions", translate("Actions"))
o.width = "10%"
function o.cfgvalue(self, section)
    local url = luci.dispatcher.build_url()
    return string.format(
        '<a href="%s">%s</a>',
        url .. "/admin/services/familywarden/profiles?" .. section .. ".edit=1",
        translate("Edit")
    )
end

-- Detailed profile editor (form view)
s = m:section(TypedSection, "profile", translate("Edit Profile"))
s.addremove = true
s.anonymous = false
s.template = "cbi/tblsection"

o = s:option(Flag, "enabled", translate("Enabled"))
o.rmempty = false

o = s:option(Value, "name", translate("Profile Name"))
o.rmempty = false
o.placeholder = translate("Child name")

o = s:option(ListValue, "policy", translate("Access Policy"),
    translate("When this profile can access the Internet"))
o:value("allow", translate("Always allow"))
o:value("block", translate("Always block"))
o:value("scheduled", translate("By schedule"))

-- Devices list with helper
o = s:option(DynamicList, "device", translate("Devices"),
    translate("MAC addresses assigned to this profile"))

-- Get current devices to show dropdown
local current_devices = get_dhcp_leases()
if #current_devices > 0 then
    o.description = translate("Available devices from DHCP: ") .. table.concat(current_devices, ", ")
end
o.datatype = "macaddr"
o.rmempty = true

-- DNS Profile for AdGuard Home integration
o = s:option(Value, "dns_profile", translate("DNS Filtering Profile"),
    translate("AdGuard Home client group for DNS filtering"))
o.rmempty = true
o.placeholder = "child"

return m
