package motd

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
)

// A gone login's end, from logind's own journal entries, for a wtmp login
// without a logout (every ended login on 26.04, whose sshd writes none)
// that last's is_phantom() finds gone (the maintainer, 2026-11-28). The
// rules are sb-rust's (its motd spec, "Last login, from wtmp", 5), so both
// print the same text.
//
// logind writes two catalogued entries per session with structured fields
// (systemd's src/login/logind-session.c, session_start() and
// session_finalize(), at v249 and v259), at LOG_INFO for every class but
// background, and the removal only for a session that started: "New
// session" and "Removed session", both with SESSION_ID, LEADER and
// journald's _BOOT_ID. An SSH login's LEADER is the PID sshd writes into
// its wtmp record on 22.04, 24.04 and 26.04. So the end is found by fields
// alone, never by a message's text:
//  1. of the "New session" entries with LEADER=<ut_pid>, the one nearest
//     the wtmp login, within a minute either way, gives the session's
//     SESSION_ID and _BOOT_ID;
//  2. the oldest "Removed session" entry with that SESSION_ID and
//     _BOOT_ID, not before the first, is the end.
//
// MESSAGE_ID, LEADER and SESSION_ID are fields any local user can write
// (logger --journald), and root reads the users' journals too, so both
// lookups also match two trusted fields, which journald sets from the
// writing process and no client can give: _SYSTEMD_UNIT=
// systemd-logind.service and _UID=0. A forged removal, or 32 forged new
// sessions burying the real one, then match nothing.
// https://www.freedesktop.org/software/systemd/man/latest/systemd.journal-fields.html
//
// Not sshd's "session closed" entry: sshd writes none when its session
// process is killed, where logind still removes the session once its last
// process ends; and it has no catalogue ID, so finding it would need its
// text. A session whose user left a process running ends when that
// process does.
// https://www.freedesktop.org/software/systemd/man/latest/journalctl.html
const (
	newSessionMessage     = "8d45620c1a4348dbb17410da57c60c66"
	removedSessionMessage = "3354939424b4456d9802ca8333ed424a"

	// journalNear is how far logind's "New session" may be from the wtmp
	// login: logind writes it during the PAM session, sshd the wtmp
	// record just after.
	journalNear = 60 * int64(time.Second/time.Microsecond)

	// journalPage is how many matching entries each lookup reads, newest
	// first: a leader's PID reused in older boots, a session's entries.
	journalPage = "32"

	// journalLimit bounds both lookups together: sb-go's 5 s for a child.
	journalLimit = 5 * time.Second
)

// logindTrusted are the trusted fields both lookups match: logind's own
// unit, as root.
var logindTrusted = []string{"_SYSTEMD_UNIT=systemd-logind.service", "_UID=0"}

// journalEntry is what a lookup reads of one entry.
type journalEntry struct {
	realtime int64 // __REALTIME_TIMESTAMP, microseconds
	session  string
	boot     string
}

// journalQuery runs journalctl for logind's entries (logindTrusted)
// matching every field=value match, as JSON lines with only the fields read here (journalctl always
// adds __CURSOR, __REALTIME_TIMESTAMP, __MONOTONIC_TIMESTAMP and
// _BOOT_ID).
func journalQuery(ctx context.Context, run commandRunner, matches ...string) []journalEntry {
	args := append([]string{"--no-pager", "-q", "-o", "json", "-n", journalPage,
		"--output-fields=SESSION_ID,_BOOT_ID"}, matches...)
	args = append(args, logindTrusted...)
	output := run(ctx, "journalctl", args...)
	if output == "Not available" {
		return nil
	}
	var entries []journalEntry
	for line := range strings.SplitSeq(output, "\n") {
		var fields map[string]json.RawMessage
		if json.Unmarshal([]byte(line), &fields) != nil {
			continue
		}
		realtime, err := strconv.ParseInt(journalString(fields["__REALTIME_TIMESTAMP"]), 10, 64)
		if err != nil {
			continue
		}
		entries = append(entries, journalEntry{
			realtime: realtime,
			session:  journalString(fields["SESSION_ID"]),
			boot:     journalString(fields["_BOOT_ID"]),
		})
	}
	return entries
}

// journalString is a field's text; "" for a missing field, or one that
// journalctl writes as an array (binary, or set more than once).
func journalString(raw json.RawMessage) string {
	var s string
	if raw == nil || json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}

// sessionEnd is when logind removed the session whose leader is `leader`,
// begun at second `login` by wtmp; false when the journal doesn't have
// both entries (rotated past them, unreadable, journalctl failing or too
// slow, or the session still open).
func sessionEnd(ctx context.Context, run commandRunner, leader int32, login int64) (int64, bool) {
	ctx, cancel := context.WithTimeout(ctx, journalLimit)
	defer cancel()

	// A corrupt 64-bit time whose microseconds don't fit isn't looked up.
	const micro = int64(time.Second / time.Microsecond)
	if login > math.MaxInt64/micro || login < math.MinInt64/micro {
		return 0, false
	}
	loginAt := login * micro
	var started *journalEntry
	var nearest int64
	news := journalQuery(ctx, run, "MESSAGE_ID="+newSessionMessage, "LEADER="+strconv.Itoa(int(leader)))
	for i := range news {
		distance := abs(saturatingSub(news[i].realtime, loginAt))
		if distance >= 0 && distance <= journalNear && (started == nil || distance < nearest) {
			started, nearest = &news[i], distance
		}
	}
	if started == nil || started.session == "" || started.boot == "" {
		return 0, false
	}

	var end int64
	found := false
	removed := journalQuery(ctx, run, "MESSAGE_ID="+removedSessionMessage,
		"SESSION_ID="+started.session, "_BOOT_ID="+started.boot)
	for _, e := range removed {
		if e.realtime >= started.realtime && (!found || e.realtime < end) {
			end, found = e.realtime, true
		}
	}
	if !found {
		return 0, false
	}
	return end / int64(time.Second/time.Microsecond), true
}
