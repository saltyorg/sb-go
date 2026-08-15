package motd

import (
	"strings"
	"testing"

	"github.com/saltyorg/sb-go/host"
)

func TestFormatServiceLineShowsScheduledForTimerGatedService(t *testing.T) {
	svc := host.ServiceInfo{
		Name:        "saltbox_managed_rclone_dev_refresh",
		Active:      "inactive",
		Sub:         "dead",
		TimerActive: "active",
		TimerSub:    "waiting",
		TimerNextIn: "2h 10m",
	}

	line := formatServiceLine(svc, "rclone_dev_refresh", len("rclone_dev_refresh"))

	if !strings.Contains(line, "scheduled • timer active/waiting • next in 2h 10m") {
		t.Fatalf("expected scheduled timer context in line, got %q", line)
	}
}

func TestFormatServiceLineShowsInactiveWithoutTimer(t *testing.T) {
	svc := host.ServiceInfo{
		Name:   "saltbox_managed_example",
		Active: "inactive",
		Sub:    "dead",
	}

	line := formatServiceLine(svc, "example", len("example"))

	if !strings.Contains(line, "inactive") {
		t.Fatalf("expected inactive status in line, got %q", line)
	}
	if strings.Contains(line, "timer") {
		t.Fatalf("did not expect timer context in line, got %q", line)
	}
}
