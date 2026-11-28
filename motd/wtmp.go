package motd

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"
)

// Last login reads /var/log/wtmp itself, as util-linux's last does,
// because a default 26.04 install has neither last (now in the wtmpdb
// package) nor lastlog, while sshd still appends glibc struct utmp records
// to /var/log/wtmp on 22.04, 24.04 and 26.04.
//
// Which record wins and how a login reads follow util-linux 2.39.3's
// login-utils/last.c, process_wtmp_file(), is_phantom() and list():
// https://github.com/util-linux/util-linux/blob/v2.39.3/login-utils/last.c
// The file is read from the end; the newest login is shown with sb-go's
// texts for last's columns.
const wtmpPath = "/var/log/wtmp"

// utmpLayout is where glibc's struct utmp keeps the fields read here.
// ut_type, ut_pid, ut_line, ut_user and ut_host sit at the same offsets
// on every 64-bit Linux; ut_tv does not. amd64 defines
// __WORDSIZE_TIME64_COMPAT32, so ut_session and ut_tv are 32-bit fields
// and a record is 384 bytes, ut_tv.tv_sec at 340 (int32_t up to glibc
// 2.39, __uint32_t from 2.40); arm64 doesn't, so ut_tv is a 64-bit
// struct timeval at 344 and a record is 400 bytes. Checked with offsetof()
// and sizeof against glibc 2.35, 2.39 and 2.43 (Ubuntu 22.04, 24.04,
// 26.04), arm64 under QEMU; glibc's bits/utmp.h:
// https://sourceware.org/git/?p=glibc.git;a=blob;f=bits/utmp.h
type utmpLayout struct {
	size  int
	secAt int
	sec64 bool
}

const (
	utPidAt    = 4
	utLineAt   = 8
	utLineSize = 32
	utUserAt   = 44
	utUserSize = 32
	utHostAt   = 76
	utHostSize = 256
)

var utmpLayouts = map[string]utmpLayout{
	"amd64": {size: 384, secAt: 340},
	"arm64": {size: 400, secAt: 344, sec64: true},
}

// The record types of glibc's <utmp.h>, and last.c's own SHUTDOWN_TIME.
const (
	utRunLvl       = 1
	utBootTime     = 2
	utNewTime      = 3
	utOldTime      = 4
	utUserProcess  = 7
	utDeadProcess  = 8
	utShutdownTime = 254
)

// wtmpRecord is the part of a struct utmp that last reads.
type wtmpRecord struct {
	typ  int16
	pid  int32
	line string
	user string
	host string
	sec  int64
}

func decodeUtmp(b []byte, layout utmpLayout) wtmpRecord {
	rec := wtmpRecord{
		typ:  int16(binary.LittleEndian.Uint16(b[0:])),
		pid:  int32(binary.LittleEndian.Uint32(b[utPidAt:])),
		line: cField(b[utLineAt : utLineAt+utLineSize]),
		user: cField(b[utUserAt : utUserAt+utUserSize]),
		host: cField(b[utHostAt : utHostAt+utHostSize]),
	}
	if layout.sec64 {
		rec.sec = int64(binary.LittleEndian.Uint64(b[layout.secAt:]))
	} else {
		rec.sec = int64(binary.LittleEndian.Uint32(b[layout.secAt:]))
	}
	return rec
}

// cField is a fixed-size C string field up to its first NUL.
func cField(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

// The ways last ends a login's line.
type loginStatus int

const (
	loginLoggedOut loginStatus = iota // a logout record: "- HH:MM (duration)"
	loginStillIn                      // "still logged in"
	loginGone                         // "gone - no logout"
	loginCrash                        // a reboot came first: "- crash (duration)"
	loginDown                         // a shutdown came first: "- down (duration)"
)

// wtmpLogin is the newest login and how it ended (end is the logout,
// boot or shutdown time).
type wtmpLogin struct {
	record wtmpRecord
	status loginStatus
	end    int64
}

// wtmpChunk is how many records are read at once from the end.
const wtmpChunk = 64

// newestLogin walks the records from the newest, as last.c's
// process_wtmp_file() does, to the first login. Whole records are read
// from the start of the file, so a torn last write is passed over, as
// glibc's getutent() does. phantom is is_phantom(): whether a login
// without a logout has no live session.
func newestLogin(r io.ReaderAt, size int64, layout utmpLayout, phantom func(wtmpRecord) bool) (wtmpLogin, bool, error) {
	recSize := int64(layout.size)
	count := size / recSize
	var (
		lastboot int64
		whydown  loginStatus
		logouts  = map[string]int64{}
		buf      = make([]byte, wtmpChunk*layout.size)
	)
	for end := count; end > 0; {
		start := max(end-wtmpChunk, 0)
		chunk := buf[:(end-start)*recSize]
		if _, err := r.ReadAt(chunk, start*recSize); err != nil {
			return wtmpLogin{}, false, err
		}
		for i := end - start - 1; i >= 0; i-- {
			rec := decodeUtmp(chunk[i*recSize:(i+1)*recSize], layout)
			normalizeUtmp(&rec)
			down := false
			switch rec.typ {
			case utShutdownTime, utBootTime:
				down = true
			case utRunLvl:
				// The run level is ut_pid's low byte (last.c's `ut_pid & 255`).
				if x := rec.pid & 0xff; x == '0' || x == '6' {
					down = true
					rec.typ = utShutdownTime
				}
			case utUserProcess:
				if logout, ok := logouts[rec.line]; ok {
					return wtmpLogin{record: rec, status: loginLoggedOut, end: logout}, true, nil
				}
				if lastboot != 0 {
					return wtmpLogin{record: rec, status: whydown, end: lastboot}, true, nil
				}
				if phantom(rec) {
					return wtmpLogin{record: rec, status: loginGone}, true, nil
				}
				return wtmpLogin{record: rec, status: loginStillIn}, true, nil
			case utDeadProcess:
				// Walking back, the last one kept for a line is the
				// earliest after its login: last.c's first match.
				if rec.line != "" {
					logouts[rec.line] = rec.sec
				}
			}
			if down {
				lastboot = rec.sec
				whydown = loginCrash
				if rec.typ == utShutdownTime {
					whydown = loginDown
				}
				clear(logouts)
			}
		}
		end = start
	}
	return wtmpLogin{}, false, nil
}

// normalizeUtmp sets ut_type as last.c does before its switch: "~" lines
// are reboots, shutdowns and run levels; otherwise a record with a user
// and a line is a login unless its user is a getty's LOGIN, one without a
// user is a logout, and "date" records are clock changes.
func normalizeUtmp(rec *wtmpRecord) {
	if strings.HasPrefix(rec.line, "~") {
		switch {
		case strings.HasPrefix(rec.user, "shutdown"):
			rec.typ = utShutdownTime
		case strings.HasPrefix(rec.user, "reboot"):
			rec.typ = utBootTime
		case strings.HasPrefix(rec.user, "runlevel"):
			rec.typ = utRunLvl
		}
		return
	}
	if rec.typ != utDeadProcess && rec.user != "" && rec.line != "" && !strings.HasPrefix(rec.user, "LOGIN") {
		rec.typ = utUserProcess
	}
	if rec.user == "" {
		rec.typ = utDeadProcess
	}
	if strings.HasPrefix(rec.user, "date") {
		switch {
		case strings.HasPrefix(rec.line, "|"):
			rec.typ = utOldTime
		case strings.HasPrefix(rec.line, "{"):
			rec.typ = utNewTime
		}
	}
}

// loginSource is where Last login reads from; the tests point it at
// their own files.
type loginSource struct {
	wtmpPath  string
	layout    utmpLayout
	layoutOK  bool
	bootTime  int64
	procDir   string
	devDir    string
	lookupUID func(user string) (uint32, bool)
	loc       *time.Location
}

func systemLoginSource() loginSource {
	layout, ok := utmpLayouts[runtime.GOARCH]
	return loginSource{
		wtmpPath:  wtmpPath,
		layout:    layout,
		layoutOK:  ok,
		bootTime:  bootTime("/proc/stat"),
		procDir:   "/proc",
		devDir:    "/dev",
		lookupUID: lookupUID,
		loc:       time.Local,
	}
}

// bootTime is the kernel's btime, the boot is_phantom() compares with;
// 0 when it can't be read.
func bootTime(path string) int64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if value, ok := strings.CutPrefix(line, "btime "); ok {
			if n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
				return n
			}
		}
	}
	return 0
}

// lookupUID finds the user's UID. sb-go is built with CGO off, so os/user
// reads /etc/passwd only: a user known only through NSS (LDAP, sssd)
// isn't found, and a login of theirs without a logout reads as gone.
func lookupUID(name string) (uint32, bool) {
	u, err := user.Lookup(name)
	if err != nil {
		return 0, false
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(uid), true
}

// readNewestLogin opens the wtmp file; an error means it can't be read
// and the older fallbacks run.
func (src loginSource) readNewestLogin() (wtmpLogin, bool, error) {
	if !src.layoutOK {
		return wtmpLogin{}, false, fmt.Errorf("no struct utmp layout for %s", runtime.GOARCH)
	}
	// No symlink is followed, and opening a FIFO or device doesn't block:
	// only a regular file is read.
	f, err := os.OpenFile(src.wtmpPath, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return wtmpLogin{}, false, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return wtmpLogin{}, false, err
	}
	if !info.Mode().IsRegular() {
		return wtmpLogin{}, false, fmt.Errorf("%s is not a regular file", src.wtmpPath)
	}
	return newestLogin(f, info.Size(), src.layout, src.phantom)
}

// phantom is last.c's is_phantom(): a login without a logout has no live
// session when it is older than the boot, its user is unknown, its
// process's login UID isn't the user's, or, without that process, its
// terminal isn't the user's.
func (src loginSource) phantom(rec wtmpRecord) bool {
	if rec.sec < src.bootTime {
		return true
	}
	uid, ok := src.lookupUID(rec.user)
	if !ok {
		return true
	}
	if data, err := os.ReadFile(filepath.Join(src.procDir, strconv.Itoa(int(rec.pid)), "loginuid")); err == nil {
		loginUID, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 32)
		return err != nil || uint32(loginUID) != uid
	}
	info, err := os.Stat(filepath.Join(src.devDir, rec.line))
	if err != nil {
		return true
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	return !ok || st.Uid != uid
}

// formatLogin writes the login in sb-go's texts for last's columns:
//   - "user at Day Mon D HH:MM (still logged in) from IP";
//   - "user at ... until HH:MM (duration) from IP" after a logout, and
//     "until crash" or "until down" (with the time to the reboot or
//     shutdown) for last's "- crash" and "- down";
//   - "user at ... from IP" when its session is gone without a logout
//     (last's "gone - no logout"; there is no logout time).
//
// A login without a source is "local".
func formatLogin(login wtmpLogin, share bool, loc *time.Location) string {
	rec := login.record
	user := ValueStyle.Render(printable(rec.user))
	from := printable(rec.host)
	if from == "" {
		from = "local"
	}
	if share {
		from = obscureIP(from)
	}
	coloredFrom := ValueStyle.Render(from)
	coloredAt := ValueStyle.Render(time.Unix(rec.sec, 0).In(loc).Format("Mon Jan 2 15:04"))

	switch login.status {
	case loginStillIn:
		return fmt.Sprintf("%s at %s (still logged in) from %s", user, coloredAt, coloredFrom)
	case loginGone:
		return fmt.Sprintf("%s at %s from %s", user, coloredAt, coloredFrom)
	}
	until := time.Unix(login.end, 0).In(loc).Format("15:04")
	switch login.status {
	case loginCrash:
		until = "crash"
	case loginDown:
		until = "down"
	}
	return fmt.Sprintf("%s at %s until %s (%s) from %s",
		user, coloredAt, ValueStyle.Render(until), lastDuration(login.end-rec.sec), coloredFrom)
}

// lastDuration is last.c's list() duration without its parentheses:
// "D+HH:MM" over a day, "HH:MM" under it.
func lastDuration(secs int64) string {
	mins := (secs / 60) % 60
	hours := (secs / 3600) % 24
	days := secs / 86400
	switch {
	case days != 0:
		return fmt.Sprintf("%d+%02d:%02d", days, abs(hours), abs(mins))
	case hours != 0:
		return fmt.Sprintf("%02d:%02d", hours, abs(mins))
	case secs >= 0:
		return fmt.Sprintf("%02d:%02d", hours, mins)
	default:
		return fmt.Sprintf("-00:%02d", abs(mins))
	}
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// printable keeps a wtmp field from steering root's terminal at login:
// it drops control characters (C0, DEL and C1, so no escape sequence
// survives), Unicode format characters (category Cf, such as U+202E,
// which would show the text reversed) and bytes that aren't UTF-8.
func printable(s string) string {
	var b strings.Builder
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		s = s[size:]
		if (r == utf8.RuneError && size == 1) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
