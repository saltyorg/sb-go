package motd

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// Invented loginctl output in each systemd release's JSON shape: 249
// (22.04) has no state or class, 255 (24.04) adds state, and 259 (26.04)
// adds class and lists each user's service manager as a session.
const (
	loginctl249JSON = `[{"session":"14","uid":1000,"user":"alice","seat":"","tty":"pts/0"},` +
		`{"session":"15","uid":1000,"user":"alice","seat":"","tty":"pts/1"}]`
	loginctl255JSON = `[{"session":"31","uid":1002,"user":"bob","seat":null,"tty":"pts/0","state":"closing","idle":false,"since":null},` +
		`{"session":"32","uid":1002,"user":"bob","seat":null,"tty":"pts/1","state":"active","idle":false,"since":null},` +
		`{"session":"33","uid":0,"user":"root","seat":null,"tty":"pts/2","state":"active","idle":false,"since":null}]`
	loginctl259JSON = `[{"session":"c7","uid":1003,"user":"carol","seat":null,"leader":4100,"class":"manager","tty":null,"idle":false,"since":null},` +
		`{"session":"c8","uid":1003,"user":"carol","seat":null,"leader":4150,"class":"user","tty":null,"idle":false,"since":null},` +
		`{"session":"c9","uid":1003,"user":"carol","seat":null,"leader":4190,"class":"user","tty":null,"idle":false,"since":null},` +
		`{"session":"c10","uid":0,"user":"root","seat":null,"leader":4200,"class":"manager-early","tty":null,"idle":false,"since":null},` +
		`{"session":"c11","uid":0,"user":"root","seat":null,"leader":4210,"class":"background","tty":null,"idle":false,"since":null},` +
		`{"session":"c12","uid":0,"user":"root","seat":null,"leader":4220,"class":"user","tty":null,"idle":false,"since":null}]`
	// 259 takes --output=json silently and writes its table.
	loginctl259Table = `SESSION  UID USER  SEAT LEADER CLASS   TTY IDLE SINCE
     c8 1003 carol -    4150   user    -   no   -

1 sessions listed.`
)

func TestLoginctlJSONOption(t *testing.T) {
	tests := map[string]string{
		"22.04":   "--output=json",
		"24.04":   "--output=json",
		"26.04":   "--json=short",
		"garbage": "--json=short",
		"":        "--json=short",
	}
	for versionID, want := range tests {
		if got := loginctlJSONOption(versionID); got != want {
			t.Errorf("loginctlJSONOption(%q) = %q, want %q", versionID, got, want)
		}
	}
}

func TestLoginSessionIDs(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []string
		ok     bool
	}{
		{name: "systemd 249", output: loginctl249JSON, want: []string{"14", "15"}, ok: true},
		{name: "systemd 255", output: loginctl255JSON, want: []string{"31", "32", "33"}, ok: true},
		{name: "systemd 259 leaves out manager and background classes", output: loginctl259JSON, want: []string{"c8", "c9", "c12"}, ok: true},
		{name: "no sessions", output: "[]", want: []string{}, ok: true},
		{name: "a session without an id still counts", output: `[{"uid":1000,"user":"alice"}]`, want: []string{""}, ok: true},
		{name: "table instead of JSON", output: loginctl259Table},
		{name: "failed command", output: "Not available"},
		{name: "empty output", output: ""},
		{name: "JSON that is not a list", output: `{"session":"1"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := loginSessionIDs(tt.output)
			if ok != tt.ok {
				t.Fatalf("loginSessionIDs() ok = %v, want %v", ok, tt.ok)
			}
			if !ok {
				return
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") || len(got) != len(tt.want) {
				t.Fatalf("loginSessionIDs() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClosingSessionIDs(t *testing.T) {
	output := "Id=31\nState=closing\n\nId=32\nState=active\n\nId=33\nState=online"
	got := closingSessionIDs(output)
	if len(got) != 1 || !got["31"] {
		t.Fatalf("closingSessionIDs() = %v, want only 31", got)
	}
	if got := closingSessionIDs("Not available"); len(got) != 0 {
		t.Fatalf("closingSessionIDs(failed) = %v, want none", got)
	}
}

// fakeRunner answers each command line from a table and records the
// calls; an unknown command fails as ExecCommand reports a failure.
type fakeRunner struct {
	answers map[string]string
	calls   []string
}

func (f *fakeRunner) run(_ context.Context, name string, args ...string) string {
	line := strings.Join(append([]string{name}, args...), " ")
	f.calls = append(f.calls, line)
	if answer, ok := f.answers[line]; ok {
		return answer
	}
	return "Not available"
}

func sessionsText(count int) string {
	if count == 1 {
		return fmt.Sprintf("%s active session", ValueStyle.Render("1"))
	}
	return fmt.Sprintf("%s active sessions", ValueStyle.Render(fmt.Sprint(count)))
}

func TestUserSessions(t *testing.T) {
	tests := []struct {
		name      string
		versionID string
		answers   map[string]string
		want      string
		calls     []string
	}{
		{
			name:      "22.04 leaves out the closing session",
			versionID: "22.04",
			answers: map[string]string{
				"loginctl list-sessions --output=json":       loginctl249JSON,
				"loginctl show-session -p Id -p State 14 15": "Id=14\nState=closing\n\nId=15\nState=active",
			},
			want: sessionsText(1),
			calls: []string{
				"loginctl list-sessions --output=json",
				"loginctl show-session -p Id -p State 14 15",
			},
		},
		{
			name:      "24.04 counts two open sessions",
			versionID: "24.04",
			answers: map[string]string{
				"loginctl list-sessions --output=json":          loginctl255JSON,
				"loginctl show-session -p Id -p State 31 32 33": "Id=31\nState=closing\n\nId=32\nState=active\n\nId=33\nState=online",
			},
			want: sessionsText(2),
		},
		{
			name:      "26.04 leaves out the service managers, background and closing sessions",
			versionID: "26.04",
			answers: map[string]string{
				"loginctl list-sessions --json=short":            loginctl259JSON,
				"loginctl show-session -p Id -p State c8 c9 c12": "Id=c8\nState=closing\n\nId=c9\nState=active\n\nId=c12\nState=active",
			},
			want: sessionsText(2),
			calls: []string{
				"loginctl list-sessions --json=short",
				"loginctl show-session -p Id -p State c8 c9 c12",
			},
		},
		{
			name:      "every listed session counts when show-session fails",
			versionID: "26.04",
			answers: map[string]string{
				"loginctl list-sessions --json=short": loginctl259JSON,
			},
			want: sessionsText(3),
		},
		{
			name:      "only closing sessions",
			versionID: "22.04",
			answers: map[string]string{
				"loginctl list-sessions --output=json":       loginctl249JSON,
				"loginctl show-session -p Id -p State 14 15": "Id=14\nState=closing\n\nId=15\nState=closing",
			},
			want: "No active sessions",
		},
		{
			name:      "only a service manager asks no states",
			versionID: "26.04",
			answers: map[string]string{
				"loginctl list-sessions --json=short": `[{"session":"c1","uid":1000,"user":"alice","class":"manager"}]`,
			},
			want:  "No active sessions",
			calls: []string{"loginctl list-sessions --json=short"},
		},
		{
			name:      "a session without an id counts without being asked about",
			versionID: "24.04",
			answers: map[string]string{
				"loginctl list-sessions --output=json": `[{"uid":1000,"user":"alice"}]`,
			},
			want:  sessionsText(1),
			calls: []string{"loginctl list-sessions --output=json"},
		},
		{
			name:      "failed loginctl",
			versionID: "24.04",
			answers:   map[string]string{},
			want:      "No active sessions",
		},
		{
			name:      "a table instead of JSON",
			versionID: "26.04",
			answers: map[string]string{
				"loginctl list-sessions --json=short": loginctl259Table,
			},
			want:  "No active sessions",
			calls: []string{"loginctl list-sessions --json=short"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeRunner{answers: tt.answers}
			got := userSessions(context.Background(), runner.run, tt.versionID)
			if got != tt.want {
				t.Fatalf("userSessions() = %q, want %q", got, tt.want)
			}
			if tt.calls != nil && strings.Join(runner.calls, "\n") != strings.Join(tt.calls, "\n") {
				t.Fatalf("userSessions() ran %q, want %q", runner.calls, tt.calls)
			}
		})
	}
}

func loginText(user, at, rest, from string) string {
	return fmt.Sprintf("%s at %s%s from %s",
		ValueStyle.Render(user), ValueStyle.Render(at), rest, ValueStyle.Render(from))
}

func TestParseLastLogin(t *testing.T) {
	tests := []struct {
		name   string
		output string
		share  bool
		want   string
		ok     bool
	}{
		{
			name: "util-linux, still logged in",
			output: "alice    pts/0        192.0.2.10       Tue Mar  3 08:15   still logged in\n" +
				"reboot   system boot  6.8.0-1-generic  Tue Mar  3 08:00   still running\n\n" +
				"wtmp begins Tue Mar  3 08:00:00 2026",
			want: loginText("alice", "Tue Mar 3 08:15", " (still logged in)", "192.0.2.10"),
			ok:   true,
		},
		{
			name: "util-linux, logged out after a reboot line",
			output: "reboot   system boot  6.8.0-1-generic  Tue Mar  3 08:00   still running\n" +
				"bob      pts/1        198.51.100.7     Mon Mar  2 21:40 - 23:05  (01:25)\n\n" +
				"wtmp begins Mon Mar  2 20:00:00 2026",
			want: fmt.Sprintf("%s at %s until %s (01:25) from %s", ValueStyle.Render("bob"),
				ValueStyle.Render("Mon Mar 2 21:40"), ValueStyle.Render("23:05"), ValueStyle.Render("198.51.100.7")),
			ok: true,
		},
		{
			name:   "util-linux, a duration over a day",
			output: "bob      pts/1        198.51.100.7     Sat Feb 28 21:40 - 23:05 (2+01:25)",
			want: fmt.Sprintf("%s at %s until %s (2+01:25) from %s", ValueStyle.Render("bob"),
				ValueStyle.Render("Sat Feb 28 21:40"), ValueStyle.Render("23:05"), ValueStyle.Render("198.51.100.7")),
			ok: true,
		},
		{
			name:   "util-linux, a console login has no source",
			output: "root     tty1                          Tue Mar  3 08:20   still logged in",
			want:   loginText("root", "Tue Mar 3 08:20", " (still logged in)", "local"),
			ok:     true,
		},
		{
			name: "wtmpdb, still logged in with its dash and ssh terminal",
			output: "carol    ssh          203.0.113.25     Wed Apr 15 10:02 - still logged in\n" +
				"carol    ssh          203.0.113.25     Wed Apr 15 09:30 - 09:45  (00:15)\n" +
				"reboot   system boot  7.0.0-1-generic  Wed Apr 15 09:00 - still running\n\n" +
				"wtmpdb begins Wed Apr 15 09:00:00 2026",
			want: loginText("carol", "Wed Apr 15 10:02", " (still logged in)", "203.0.113.25"),
			ok:   true,
		},
		{
			name: "wtmpdb, logged out",
			output: "reboot   system boot  7.0.0-1-generic  Wed Apr 15 09:00 - still running\n" +
				"carol    ssh          203.0.113.25     Wed Apr 15 09:30 - 09:45  (00:15)\n\n" +
				"wtmpdb begins Wed Apr 15 09:00:00 2026",
			want: fmt.Sprintf("%s at %s until %s (00:15) from %s", ValueStyle.Render("carol"),
				ValueStyle.Render("Wed Apr 15 09:30"), ValueStyle.Render("09:45"), ValueStyle.Render("203.0.113.25")),
			ok: true,
		},
		{
			name:   "wtmpdb, only the reboot and its trailer",
			output: "reboot   system boot  7.0.0-1-generic  Wed Apr 15 09:00 - still running\n\nwtmpdb begins Wed Apr 15 09:00:00 2026",
		},
		{
			name:   "util-linux, only the trailer",
			output: "\nwtmp begins Tue Mar  3 08:00:00 2026",
		},
		{
			name:   "wide columns keep a long user and an IPv6 source whole",
			output: "averyverylongusername pts/0        2001:db8:85a3::8a2e:370:7334 Thu Apr 16 07:45   still logged in",
			want:   loginText("averyverylongusername", "Thu Apr 16 07:45", " (still logged in)", "2001:db8:85a3::8a2e:370:7334"),
			ok:     true,
		},
		{
			name:   "share mode masks the source",
			output: "carol    ssh          203.0.113.25     Wed Apr 15 10:02 - still logged in",
			share:  true,
			want:   loginText("carol", "Wed Apr 15 10:02", " (still logged in)", "xxx.xxx.xxx.xxx"),
			ok:     true,
		},
		{name: "missing last", output: "Not available"},
		{name: "empty output", output: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseLastLogin(tt.output, tt.share)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("parseLastLogin() = %q, %v; want %q, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestLastLoginFallsThroughAMissingLast(t *testing.T) {
	runner := &fakeRunner{answers: map[string]string{}}
	if got := lastLogin(context.Background(), runner.run); got != "No recent logins" {
		t.Fatalf("lastLogin() = %q, want No recent logins", got)
	}
	want := []string{"last -w -n 5", "lastlog -u root", "who -u"}
	if strings.Join(runner.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("lastLogin() ran %q, want %q", runner.calls, want)
	}
}

func TestLastLoginReadsWtmpdb(t *testing.T) {
	runner := &fakeRunner{answers: map[string]string{
		"last -w -n 5": "carol    ssh          203.0.113.25     Wed Apr 15 10:02 - still logged in\n\n" +
			"wtmpdb begins Wed Apr 15 09:00:00 2026",
	}}
	want := loginText("carol", "Wed Apr 15 10:02", " (still logged in)", "203.0.113.25")
	if got := lastLogin(context.Background(), runner.run); got != want {
		t.Fatalf("lastLogin() = %q, want %q", got, want)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("lastLogin() ran %q, want only last", runner.calls)
	}
}
