package motd

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// journalLogin is an invented wtmp login's second: Tue Mar 3 2026
// 08:15:00 UTC (aliceIn's time).
var journalLogin = at(3, 8, 15).Unix()

// The two lookups' command lines, as sessionEnd runs them.
func newSessionCall(leader int) string {
	return "journalctl --no-pager -q -o json -n 32 --output-fields=SESSION_ID,_BOOT_ID " +
		"MESSAGE_ID=8d45620c1a4348dbb17410da57c60c66 LEADER=" + fmt.Sprint(leader) +
		" _SYSTEMD_UNIT=systemd-logind.service _UID=0"
}

func removedSessionCall(session, boot string) string {
	return "journalctl --no-pager -q -o json -n 32 --output-fields=SESSION_ID,_BOOT_ID " +
		"MESSAGE_ID=3354939424b4456d9802ca8333ed424a SESSION_ID=" + session + " _BOOT_ID=" + boot +
		" _SYSTEMD_UNIT=systemd-logind.service _UID=0"
}

// Both lookups match each of journald's trusted fields, so a user's
// forged "New session" or "Removed session" (logger --journald) matches
// neither.
func TestSessionEndMatchesOnlyLogindsOwnEntries(t *testing.T) {
	var runs [][]string
	run := func(_ context.Context, name string, args ...string) string {
		runs = append(runs, args)
		if len(runs) == 1 {
			return entry("c3", bootA, journalLogin)
		}
		return entry("c3", bootA, journalLogin+60)
	}
	if _, ok := sessionEnd(context.Background(), run, 4100, journalLogin); !ok || len(runs) != 2 {
		t.Fatalf("sessionEnd() = %v after %d runs", ok, len(runs))
	}
	for i, args := range runs {
		for _, field := range []string{"_SYSTEMD_UNIT=systemd-logind.service", "_UID=0"} {
			found := false
			for _, arg := range args {
				found = found || arg == field
			}
			if !found {
				t.Errorf("lookup %d %q doesn't match %s", i+1, args, field)
			}
		}
	}
}

// entry is one invented journalctl JSON line: journalctl always writes
// __CURSOR, __REALTIME_TIMESTAMP (microseconds), __MONOTONIC_TIMESTAMP and
// _BOOT_ID, then the --output-fields asked for.
func entry(session, boot string, second int64) string {
	return fmt.Sprintf(`{"__CURSOR":"s=0;i=%x","__REALTIME_TIMESTAMP":"%d","__MONOTONIC_TIMESTAMP":"42","_BOOT_ID":"%s","SESSION_ID":"%s"}`,
		second, second*1_000_000+250_000, boot, session)
}

// entryAt is an entry at an exact microsecond.
func entryAt(session, boot string, micro int64) string {
	return fmt.Sprintf(`{"__REALTIME_TIMESTAMP":"%d","_BOOT_ID":"%s","SESSION_ID":"%s"}`, micro, boot, session)
}

// A login second whose microseconds don't fit in int64 (a corrupt 64-bit
// time) isn't looked up.
func TestSessionEndSkipsAnOutOfRangeLogin(t *testing.T) {
	for _, login := range []int64{math.MaxInt64/1_000_000 + 1, math.MinInt64/1_000_000 - 1, math.MaxInt64, math.MinInt64} {
		runner := &fakeRunner{answers: map[string]string{}}
		if _, ok := sessionEnd(context.Background(), runner.run, 4100, login); ok || len(runner.calls) != 0 {
			t.Fatalf("sessionEnd(%d) = %v after %q", login, ok, runner.calls)
		}
	}
}

func TestSaturatingSub(t *testing.T) {
	tests := []struct{ a, b, want int64 }{
		{5, 3, 2},
		{3, 5, -2},
		{math.MaxInt64, -1, math.MaxInt64},
		{math.MinInt64, 1, math.MinInt64},
		{math.MinInt64 + 1, 1, math.MinInt64},
		{math.MaxInt64 - 1, -1, math.MaxInt64},
	}
	for _, tt := range tests {
		if got := saturatingSub(tt.a, tt.b); got != tt.want {
			t.Errorf("saturatingSub(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func lines(entries ...string) string { return strings.Join(entries, "\n") }

const (
	bootA = "0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0a"
	bootB = "0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b"
)

func TestSessionEnd(t *testing.T) {
	L := journalLogin
	tests := []struct {
		name    string
		answers map[string]string
		leader  int
		end     int64
		ok      bool
		calls   int // when set, how many journalctl runs
	}{
		{
			name: "logind's removal of the leader's session",
			answers: map[string]string{
				newSessionCall(4100):            entry("c3", bootA, L-1),
				removedSessionCall("c3", bootA): entry("c3", bootA, L+42*60),
			},
			leader: 4100, end: L + 42*60, ok: true,
		},
		{
			name: "a session still open",
			answers: map[string]string{
				newSessionCall(4100): entry("c3", bootA, L),
			},
			leader: 4100,
		},
		{
			name:    "no entry for the leader",
			answers: map[string]string{newSessionCall(4100): ""},
			leader:  4100,
		},
		{
			name: "a new session a minute before the login is the login's",
			answers: map[string]string{
				newSessionCall(4100):            entry("c3", bootA, L-60),
				removedSessionCall("c3", bootA): entry("c3", bootA, L+5),
			},
			leader: 4100, end: L + 5, ok: true,
		},
		{
			name: "a new session over a minute after the login is another's",
			answers: map[string]string{
				newSessionCall(4100):            entry("c3", bootA, L+61),
				removedSessionCall("c3", bootA): entry("c3", bootA, L+90),
			},
			leader: 4100,
		},
		{
			name: "the login's own boot decides between reused IDs",
			answers: map[string]string{
				newSessionCall(4100):           lines(entry("5", bootA, L-50), entry("5", bootB, L)),
				removedSessionCall("5", bootB): entry("5", bootB, L+600),
			},
			leader: 4100, end: L + 600, ok: true,
		},
		{
			name: "the nearest new session within the minute, not the newest",
			answers: map[string]string{
				newSessionCall(4100):            lines(entry("c3", bootA, L-5), entry("c9", bootA, L+50)),
				removedSessionCall("c3", bootA): entry("c3", bootA, L+30),
				removedSessionCall("c9", bootA): entry("c9", bootA, L+55),
			},
			leader: 4100, end: L + 30, ok: true,
		},
		{
			name: "a removal before the start is passed over, the oldest after it wins",
			answers: map[string]string{
				newSessionCall(4100): entry("c3", bootA, L),
				removedSessionCall("c3", bootA): lines(entry("c3", bootA, L-10), entry("c3", bootA, L+80),
					entry("c3", bootA, L+70)),
			},
			leader: 4100, end: L + 70, ok: true,
		},
		{
			name: "the nearest new session without a session ID gives no end",
			answers: map[string]string{
				newSessionCall(4100): lines(`{"__REALTIME_TIMESTAMP":"`+fmt.Sprint(L*1_000_000)+`","_BOOT_ID":"`+bootA+`"}`,
					entry("c3", bootA, L+20)),
				removedSessionCall("c3", bootA): entry("c3", bootA, L+30),
			},
			leader: 4100, calls: 1,
		},
		{
			name: "the nearest new session without a boot ID gives no end",
			answers: map[string]string{
				newSessionCall(4100): lines(`{"__REALTIME_TIMESTAMP":"`+fmt.Sprint(L*1_000_000)+`","SESSION_ID":"c3"}`,
					entry("c3", bootA, L+20)),
				removedSessionCall("c3", bootA): entry("c3", bootA, L+30),
			},
			leader: 4100, calls: 1,
		},
		{
			name: "a new session exactly a minute before the login is the login's",
			answers: map[string]string{
				newSessionCall(4100):            entryAt("c3", bootA, (L-60)*1_000_000),
				removedSessionCall("c3", bootA): entry("c3", bootA, L+5),
			},
			leader: 4100, end: L + 5, ok: true,
		},
		{
			name: "a new session exactly a minute after the login is the login's",
			answers: map[string]string{
				newSessionCall(4100):            entryAt("c3", bootA, (L+60)*1_000_000),
				removedSessionCall("c3", bootA): entry("c3", bootA, L+65),
			},
			leader: 4100, end: L + 65, ok: true,
		},
		{
			name: "a new session a microsecond past the minute is another's",
			answers: map[string]string{
				newSessionCall(4100):            lines(entryAt("c3", bootA, (L-60)*1_000_000-1), entryAt("c4", bootA, (L+60)*1_000_000+1)),
				removedSessionCall("c3", bootA): entry("c3", bootA, L+5),
				removedSessionCall("c4", bootA): entry("c4", bootA, L+65),
			},
			leader: 4100, calls: 1,
		},
		{
			name: "a removal exactly at the start is the end",
			answers: map[string]string{
				newSessionCall(4100):            entryAt("c3", bootA, L*1_000_000+7),
				removedSessionCall("c3", bootA): lines(entryAt("c3", bootA, L*1_000_000+6), entryAt("c3", bootA, L*1_000_000+7)),
			},
			leader: 4100, end: L, ok: true,
		},
		{
			name: "lines that aren't JSON or lack a time are passed over",
			answers: map[string]string{
				newSessionCall(4100): lines("-- No entries --", `{"SESSION_ID":"c1"}`, `{"__REALTIME_TIMESTAMP":"x"}`,
					`{"__REALTIME_TIMESTAMP":[1,2],"SESSION_ID":"c2"}`, entry("c3", bootA, L+2)),
				removedSessionCall("c3", bootA): entry("c3", bootA, L+3),
			},
			leader: 4100, end: L + 3, ok: true,
		},
		{
			name:    "journalctl fails",
			answers: map[string]string{},
			leader:  4100,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeRunner{answers: tt.answers}
			end, ok := sessionEnd(context.Background(), runner.run, int32(tt.leader), journalLogin)
			if ok != tt.ok || (ok && end != tt.end) {
				t.Fatalf("sessionEnd() = %d, %v; want %d, %v", end, ok, tt.end, tt.ok)
			}
			if tt.calls != 0 && len(runner.calls) != tt.calls {
				t.Fatalf("sessionEnd() ran %q, want %d runs", runner.calls, tt.calls)
			}
		})
	}
}

// Both lookups share one 5 s limit: the context the runner sees has a
// deadline no later than that.
func TestSessionEndIsBounded(t *testing.T) {
	var deadlines []time.Time
	run := func(ctx context.Context, name string, args ...string) string {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("no deadline")
		}
		deadlines = append(deadlines, deadline)
		if strings.Contains(strings.Join(args, " "), "LEADER=") {
			return entry("c3", bootA, journalLogin)
		}
		return entry("c3", bootA, journalLogin+60)
	}
	start := time.Now()
	if _, ok := sessionEnd(context.Background(), run, 4100, journalLogin); !ok {
		t.Fatal("no end")
	}
	if len(deadlines) != 2 || !deadlines[0].Equal(deadlines[1]) || deadlines[0].After(start.Add(5*time.Second+time.Second)) {
		t.Fatalf("deadlines %v from %v", deadlines, start)
	}
}

func TestLastLoginTakesAGoneLoginsEndFromTheJournal(t *testing.T) {
	gone := fmt.Sprintf("%s at %s from %s", value("alice"), value("Tue Mar 3 08:15"), value("192.0.2.10"))
	ended := fmt.Sprintf("%s at %s until %s (00:42) from %s", value("alice"), value("Tue Mar 3 08:15"),
		value("08:57"), value("192.0.2.10"))
	journal := map[string]string{
		newSessionCall(4100):            entry("c3", bootA, journalLogin),
		removedSessionCall("c3", bootA): entry("c3", bootA, journalLogin+42*60),
	}
	tests := []struct {
		name    string
		records []testRecord
		journal bool
		answers map[string]string
		want    string
		calls   int
	}{
		{name: "a gone login takes the journal's end", records: []testRecord{aliceIn}, journal: true, answers: journal, want: ended, calls: 2},
		{name: "no end in the journal", records: []testRecord{aliceIn}, journal: true, answers: map[string]string{}, want: gone, calls: 1},
		{name: "no journal lookup asked", records: []testRecord{aliceIn}, answers: journal, want: gone, calls: 0},
		{
			name: "a wtmp logout wins over the journal", records: []testRecord{aliceIn, aliceOut}, journal: true, answers: journal,
			want: fmt.Sprintf("%s at %s until %s (00:15) from %s", value("alice"), value("Tue Mar 3 08:15"),
				value("08:30"), value("192.0.2.10")),
		},
		{
			name: "a crash isn't looked up", records: []testRecord{aliceIn, nextBoot}, journal: true, answers: journal,
			want: fmt.Sprintf("%s at %s until %s (01:47) from %s", value("alice"), value("Tue Mar 3 08:15"),
				value("crash"), value("192.0.2.10")),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := testSource(t, wtmpFile("amd64", tt.records...))
			src.bootTime = journalLogin + 3600 // after the login: gone without a look
			src.journal = tt.journal
			runner := &fakeRunner{answers: tt.answers}
			if got := lastLogin(context.Background(), runner.run, src); got != tt.want {
				t.Fatalf("lastLogin() = %q, want %q", got, tt.want)
			}
			if len(runner.calls) != tt.calls {
				t.Fatalf("lastLogin() ran %q, want %d calls", runner.calls, tt.calls)
			}
		})
	}
}

// A ut_line that isn't a relative path of plain parts is gone without a
// look at what it names.
func TestPhantomLineMustBePlain(t *testing.T) {
	dev := t.TempDir()
	if err := os.WriteFile(filepath.Join(dev, "x"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	me := uint32(os.Getuid())
	src := loginSource{
		procDir:   t.TempDir(),
		devDir:    filepath.Join(dev, "sub"),
		lookupUID: func(string) (uint32, bool) { return me, true },
	}
	if err := os.MkdirAll(filepath.Join(dev, "sub", "pts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dev, "sub", "pts", "3"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for line, want := range map[string]bool{
		"pts/3":       false,
		"pts//3":      false,
		"pts/./3":     false,
		"../x":        true,
		"pts/../../x": true,
		"/x":          true,
		"./pts/3":     true,
		".":           true,
		"":            true,
	} {
		rec := wtmpRecord{pid: 4999, line: line, user: "me", sec: 1}
		if got := src.phantom(rec); got != want {
			t.Errorf("phantom(line %q) = %v, want %v", line, got, want)
		}
	}
}

// A wtmp with another link isn't read: a hard link in /var/log could
// point the read at another file.
func TestLastLoginReadsOnlyAFileWithOneLink(t *testing.T) {
	src := testSource(t, wtmpFile("amd64", boot, aliceIn))
	if err := os.Link(src.wtmpPath, src.wtmpPath+".other"); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{answers: map[string]string{}}
	if got := lastLogin(context.Background(), runner.run, src); got != "No recent logins" || len(runner.calls) != 2 {
		t.Fatalf("lastLogin() = %q after %q, want the fallbacks", got, runner.calls)
	}
}

// A corrupt arm64 time doesn't wrap the duration around.
func TestDurationSaturates(t *testing.T) {
	login := wtmpLogin{record: wtmpRecord{user: "alice", sec: -1 << 62}, status: loginLoggedOut, end: 1 << 62}
	if got := formatLogin(login, false, time.UTC); strings.Contains(got, "(-") {
		t.Fatalf("formatLogin() = %q", got)
	}
}
