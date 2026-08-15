package host

import (
	"testing"
	"time"
)

func TestParseSystemctlOutputSkipsNotFoundUnits(t *testing.T) {
	output := `UNIT                                    LOAD      ACTIVE   SUB     DESCRIPTION
saltbox_managed_qbittorrent.service      not-found inactive dead    saltbox_managed_qbittorrent.service
saltbox_managed_sonarr.service           loaded    active   running Sonarr service

2 loaded units listed. Pass --all to see loaded but inactive units, too.
`

	serviceMap := make(map[string]ServiceInfo)
	parseSystemctlOutput(output, ServiceFilter{Pattern: "saltbox_managed_", IsPrefix: true}, serviceMap)

	if _, ok := serviceMap["saltbox_managed_qbittorrent"]; ok {
		t.Fatalf("expected not-found service to be excluded")
	}

	svc, ok := serviceMap["saltbox_managed_sonarr"]
	if !ok {
		t.Fatalf("expected loaded service to be included")
	}
	if svc.Active != "active" || svc.Sub != "running" {
		t.Fatalf("unexpected service status: active=%q sub=%q", svc.Active, svc.Sub)
	}
}

func TestParseSystemctlOutputKeepsInactiveAndFailedLoadedUnits(t *testing.T) {
	output := `UNIT                                  LOAD   ACTIVE   SUB    DESCRIPTION
saltbox_managed_radarr.service          loaded inactive dead   Radarr service
● saltbox_managed_overseerr.service     loaded failed   failed Overseerr service
`

	serviceMap := make(map[string]ServiceInfo)
	parseSystemctlOutput(output, ServiceFilter{Pattern: "saltbox_managed_", IsPrefix: true}, serviceMap)

	radarr, ok := serviceMap["saltbox_managed_radarr"]
	if !ok {
		t.Fatalf("expected inactive loaded service to be included")
	}
	if radarr.Active != "inactive" || radarr.Sub != "dead" {
		t.Fatalf("unexpected radarr status: active=%q sub=%q", radarr.Active, radarr.Sub)
	}

	overseerr, ok := serviceMap["saltbox_managed_overseerr"]
	if !ok {
		t.Fatalf("expected failed loaded service to be included")
	}
	if overseerr.Active != "failed" || overseerr.Sub != "failed" {
		t.Fatalf("unexpected overseerr status: active=%q sub=%q", overseerr.Active, overseerr.Sub)
	}
}

func TestParseSystemctlShowProperties(t *testing.T) {
	output := `LoadState=loaded
ActiveState=active
SubState=waiting
NextElapseUSecRealtime=2026-03-13T15:00:00Z
`

	props := parseSystemctlShowProperties(output)

	if props["LoadState"] != "loaded" {
		t.Fatalf("unexpected LoadState: %q", props["LoadState"])
	}
	if props["ActiveState"] != "active" {
		t.Fatalf("unexpected ActiveState: %q", props["ActiveState"])
	}
	if props["SubState"] != "waiting" {
		t.Fatalf("unexpected SubState: %q", props["SubState"])
	}
	if props["NextElapseUSecRealtime"] != "2026-03-13T15:00:00Z" {
		t.Fatalf("unexpected NextElapseUSecRealtime: %q", props["NextElapseUSecRealtime"])
	}
}

func TestFormatNextIn(t *testing.T) {
	now := time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC)

	if got := formatNextIn("2026-03-13T13:30:00Z", now); got != "1h 30m" {
		t.Fatalf("expected 1h 30m, got %q", got)
	}

	if got := formatNextIn("2026-03-13T11:59:00Z", now); got != "soon" {
		t.Fatalf("expected soon, got %q", got)
	}

	if got := formatNextIn("n/a", now); got != "" {
		t.Fatalf("expected empty for n/a, got %q", got)
	}
}

func TestParseListTimersJSON(t *testing.T) {
	now := time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC)
	output := `[
  {"next": 1773423000000000, "unit": "saltbox_managed_rclone_dev_refresh.timer", "activates": "saltbox_managed_rclone_dev_refresh.service"},
  {"next": null, "unit": "motd-news.timer", "activates": null}
]`

	nextByService, err := parseListTimersJSON(output, now)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	got, ok := nextByService["saltbox_managed_rclone_dev_refresh"]
	if !ok {
		t.Fatalf("expected next fire for rclone refresh service")
	}
	if got != "5h 30m" {
		t.Fatalf("expected 5h 30m, got %q", got)
	}
}
