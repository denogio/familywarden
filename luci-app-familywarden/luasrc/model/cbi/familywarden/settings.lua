-- Copyright 2026 Dennis Højgaard
-- Licensed under Apache 2.0

local m, s, o

m = Map("familywarden", translate("FamilyWarden Settings"),
    translate("Configure global FamilyWarden options."))

-- Global settings section
s = m:section(TypedSection, "global", translate("Global Settings"))
s.anonymous = true
s.addremove = false

-- Enable/Disable
o = s:option(Flag, "enabled", translate("Enable FamilyWarden"))
o.rmempty = false
o.default = "1"

-- Default policy
o = s:option(ListValue, "default_policy", translate("Default Policy"),
    translate("Default action for devices not assigned to any profile"))
o:value("allow", translate("Allow Internet"))
o:value("block", translate("Block Internet"))
o.default = "allow"

-- Log level
o = s:option(ListValue, "log_level", translate("Log Level"))
o:value("0", translate("Error"))
o:value("1", translate("Warning"))
o:value("2", translate("Info"))
o:value("3", translate("Debug"))
o.default = "1"

return m
