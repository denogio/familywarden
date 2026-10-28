package configuration

import (
	"testing"
	"time"

	"github.com/denogio/familywarden/core/internal/platform"
	"github.com/denogio/familywarden/core/internal/rules"
)

// The fixture clock is Friday 2027-01-15 08:00 UTC; alex's only window is
// Monday 07:00-20:00. 1799654400 is the Monday 08:00, inside the window.
func reportProfiles(t *testing.T, r map[string]any) map[string]any {
	t.Helper()
	if r["error"] != nil {
		t.Fatal(r)
	}
	profiles, ok := r["profiles"].(map[string]any)
	if !ok {
		t.Fatal("report without profiles object", r)
	}
	return profiles
}

func reportConfiguration(t *testing.T, r map[string]any) map[string]any {
	t.Helper()
	if r["error"] != nil {
		t.Fatal(r)
	}
	c, ok := r["configuration"].(map[string]any)
	if !ok {
		t.Fatal("report without configuration object", r)
	}
	return c
}

func TestReportAppBudgetUsesCompiledCorePolicy(t *testing.T) {
	a, b, _ := setup(t)
	activate(t, a, b)
	d, _, err := a.load()
	if err != nil {
		t.Fatal(err)
	}
	p := &d.Configuration.Current.Profiles[0]
	p.Apps, p.ScreenMinutes, p.ScreenWeekday = "1", "60", "25"
	d.Bonus.Entries = []bonusEntry{{"alex", "2027-01-15", "total", 15}}
	if err := a.save(d); err != nil {
		t.Fatal(err)
	}
	report := a.API("configuration_report", []byte(`{}`))
	reportProfiles(t, report)
	apps := report["app_profiles"].(map[string]any)
	alex := apps["alex"].(map[string]any)
	policy := alex["policy"].(*rules.AppPolicy)
	if report["day"] != "2027-01-15" || alex["enabled"] != true ||
		*alex["screen_budget_minutes"].(*int) != 40 || policy.Screen["fri"] != 25 || policy.Bonus.Total != 15 {
		t.Fatal(report)
	}
	taylor := apps["taylor"].(map[string]any)
	if taylor["enabled"] != false || taylor["screen_budget_minutes"].(*int) != nil {
		t.Fatal(taylor)
	}
	d.Bonus.Entries = nil
	if err := a.save(d); err != nil {
		t.Fatal(err)
	}
	report = a.API("configuration_report", []byte(`{}`))
	reportProfiles(t, report)
	alex = report["app_profiles"].(map[string]any)["alex"].(map[string]any)
	if *alex["screen_budget_minutes"].(*int) != 25 || alex["policy"].(*rules.AppPolicy).Bonus != nil {
		t.Fatal(alex)
	}
}

func TestNetworkReportPrecedenceWindowsAndStatus(t *testing.T) {
	a, b, _ := setup(t) // Friday 2027-01-15 08:00 UTC, window closed
	activate(t, a, b)

	r := a.API("configuration_report", []byte(`{}`))
	profiles := reportProfiles(t, r)
	alex := profiles["alex"].(map[string]any)
	if _, has := profiles["taylor"]; has {
		t.Fatal("disabled profile listed")
	}
	if alex["name"] != "Alex" || alex["policy"] != "scheduled" || alex["status"] != "blocked" ||
		alex["override"] != "none" || alex["override_expires"] != int64(0) ||
		alex["block_router"] != false || alex["captain"] != false ||
		alex["apps"] != false || alex["bonus_today"] != int(0) {
		t.Fatal(alex)
	}
	devices, ok := alex["devices"].([]string)
	if !ok || len(devices) != 1 || devices[0] != "aa:bb:cc:00:00:01" {
		t.Fatal(alex["devices"])
	}
	windows, ok := alex["schedules"].([]map[string]string)
	if !ok || len(windows) != 1 {
		t.Fatal("schedule windows missing", alex)
	}
	window := windows[0]
	if window["days"] != "mon" || window["start"] != "07:00" || window["stop"] != "20:00" {
		t.Fatal(window)
	}
	configuration := reportConfiguration(t, r)
	if configuration["enabled"] != true || configuration["default_policy"] != "block" ||
		configuration["captain"] != "alex" || configuration["delay_hours"] != "24" {
		t.Fatal(configuration)
	}
	devices, ok = configuration["assigned_devices"].([]string)
	if !ok || len(devices) != 1 {
		t.Fatal(configuration["assigned_devices"])
	}

	// The same profile is allowed inside its window with no override.
	a.Now = func() time.Time { return time.Unix(1799654400+3600, 0).UTC() }
	row := reportProfiles(t, a.API("configuration_report", []byte(`{}`)))["alex"].(map[string]any)
	if row["status"] != "allowed" || row["override"] != "none" {
		t.Fatal(row)
	}

	// Manual overrides outrank the schedule, and expiry applies at read time.
	a.Now = func() time.Time { return time.Unix(1800000000, 0).UTC() }
	b.network = map[string]platform.NetworkOverride{"alex": {Mode: "block"}}
	row = reportProfiles(t, a.API("configuration_report", []byte(`{}`)))["alex"].(map[string]any)
	if row["status"] != "blocked" || row["override"] != "block" {
		t.Fatal(row)
	}
	b.network = map[string]platform.NetworkOverride{"alex": {Mode: "allow", Expires: 1800000999}}
	row = reportProfiles(t, a.API("configuration_report", []byte(`{}`)))["alex"].(map[string]any)
	if row["status"] != "allowed" || row["override"] != "allow" || row["override_expires"] != int64(1800000999) {
		t.Fatal(row)
	}
	b.network = map[string]platform.NetworkOverride{"alex": {Mode: "allow", Expires: 1799999999}}
	row = reportProfiles(t, a.API("configuration_report", []byte(`{}`)))["alex"].(map[string]any)
	if row["status"] != "blocked" || row["override"] != "none" {
		t.Fatal("expired ticket still counts", row)
	}

	// Today's bonus total is reported, not the whole journal.
	d, _, err := a.load()
	if err != nil {
		t.Fatal(err)
	}
	d.Bonus.Entries = append(d.Bonus.Entries, bonusEntry{"alex", "2027-01-15", "total", 42}, bonusEntry{"alex", "2027-01-16", "total", 400})
	if err := a.save(d); err != nil {
		t.Fatal(err)
	}
	row = reportProfiles(t, a.API("configuration_report", []byte(`{}`)))["alex"].(map[string]any)
	if row["bonus_today"] != 42 {
		t.Fatal(row["bonus_today"])
	}

	// Single-profile status is the same row without the list-only extras.
	single, err := a.networkReport(d, strPtr("alex"))
	if err != nil {
		t.Fatal(err)
	}
	if single["name"] != "Alex" {
		t.Fatal(single)
	}
	for _, key := range []string{"apps", "bonus_today", "schedules"} {
		if _, has := single[key]; has {
			t.Fatalf("status row carries list-only field %s", key)
		}
	}
	bad, _ := a.networkReport(d, strPtr("ghost"))
	if bad != nil {
		t.Fatal(bad)
	}
	if _, err := a.networkReport(d, strPtr("taylor")); err == nil || err.Error() != "profile not found or disabled" {
		t.Fatal("disabled profile accepted", err)
	}
	if a.API("configuration_report", request(map[string]any{"profile": "bad id"}))["error"] == nil {
		t.Fatal("invalid profile accepted")
	}
	if a.API("configuration_report", []byte(`{"profile":"alex","profile":"alex"}`))["error"] == nil {
		t.Fatal("duplicate profile field accepted")
	}
	if a.API("configuration_report", []byte(`{"unknown":1}`))["error"] == nil {
		t.Fatal("unknown field accepted")
	}
}

func TestNetworkReportPactDetailsAndPermitRevocation(t *testing.T) {
	a, b, _ := setup(t) // Friday, window closed
	activate(t, a, b)
	// A standing pre-pact allow must read as revoked for the bound captain.
	b.network = map[string]platform.NetworkOverride{"alex": {Mode: "allow"}}
	b.mastSync = 123
	okResult(t, a.API("configuration_pact", []byte(`{"until":1800003600}`)))
	r := a.API("configuration_report", []byte(`{}`))
	alex := reportProfiles(t, r)["alex"].(map[string]any)
	if alex["captain"] != true || alex["override"] != "none" || alex["status"] != "blocked" {
		t.Fatal(alex)
	}
	if alex["pact_type"] != "until" || alex["pact_until"] != int64(1800003600) ||
		alex["pact_window"] != "sun 10:00-11:00" || alex["pact_in_window"] != false ||
		alex["release_at"] != int64(0) || alex["mast_last_sync"] != int64(123) {
		t.Fatal(alex)
	}
	configuration := reportConfiguration(t, r)
	if configuration["pact_delay_seconds"] != int64(86400) {
		t.Fatal(configuration)
	}
}

func TestNetworkReportGlobalDisableAllowsEverything(t *testing.T) {
	a, b, _ := setup(t)
	activate(t, a, b)
	b.network = map[string]platform.NetworkOverride{"alex": {Mode: "block"}}
	d, _, err := a.load()
	if err != nil {
		t.Fatal(err)
	}
	next := clone(d.Configuration)
	next.Current.Settings.Enabled = "0"
	r := a.API("configuration_apply", request(map[string]any{"revision": d.Configuration.Revision, "current": next.Current}))
	okResult(t, r)
	if r["status"] == "confirmation_required" {
		okResult(t, a.API("configuration_confirm", request(map[string]any{"revision": r["revision"], "token": r["token"]})))
	}
	report := a.API("configuration_report", []byte(`{}`))
	configuration := reportConfiguration(t, report)
	if configuration["enabled"] != false {
		t.Fatal(configuration)
	}
	for id, row := range reportProfiles(t, report) {
		if m := row.(map[string]any); m["status"] != "allowed" {
			t.Fatal("disabled router still blocks", id, m)
		}
	}
}

func strPtr(s string) *string { return &s }

// Monday 2027-01-11 09:00, Sunday 2027-01-10 00:30, Monday 2027-01-11 23:30
// and Tuesday 2027-01-12 00:30 UTC are 1799658000, 1799541000, 1799710200 and
// 1799713800.
func TestInternetWindowActiveMatchesTheNativeCompiler(t *testing.T) {
	monday09 := time.Unix(1799658000, 0).UTC()
	sunday0030 := time.Unix(1799541000, 0).UTC()
	monday2330 := time.Unix(1799710200, 0).UTC()
	window := rules.ConfigurationWindow{Days: "mon", Start: "07:00", Stop: "20:00"}
	if !internetWindowActive(window, monday09) {
		t.Fatal("daytime window missed")
	}
	if internetWindowActive(window, sunday0030) {
		t.Fatal("window active on the wrong day")
	}
	if internetWindowActive(rules.ConfigurationWindow{Days: "mon", Start: "20:00", Stop: "24:00"}, monday09) ||
		!internetWindowActive(rules.ConfigurationWindow{Days: "mon", Start: "20:00", Stop: "24:00"}, monday2330) {
		t.Fatal("24:00 exclusive boundary wrong")
	}
	night := rules.ConfigurationWindow{Days: "mon", Start: "22:00", Stop: "01:30"}
	tuesday0030 := time.Unix(1799713800, 0).UTC()
	if !internetWindowActive(night, monday2330) || !internetWindowActive(night, tuesday0030) {
		t.Fatal("overnight window missed")
	}
	if internetWindowActive(night, monday09) || internetWindowActive(night, sunday0030) {
		t.Fatal("overnight window active outside its span")
	}
	if !internetWindowActive(rules.ConfigurationWindow{Days: "mon", Start: "09:00", Stop: "09:00"}, monday09) {
		t.Fatal("equal endpoints must allow the whole day")
	}
	if internetWindowActive(rules.ConfigurationWindow{Days: "mon", Start: "09:00", Stop: "09:00"}, sunday0030) {
		t.Fatal("whole-day window active on the wrong day")
	}
	if internetWindowActive(rules.ConfigurationWindow{Days: "mon", Start: "9:00", Stop: "20:00"}, monday09) {
		t.Fatal("malformed time accepted")
	}
}
