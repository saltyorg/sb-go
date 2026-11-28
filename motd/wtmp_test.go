package motd

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The record types of glibc's <utmp.h>, as the tests write them.
const (
	testRunLvl      = 1
	testBootTime    = 2
	testLoginProc   = 6
	testUserProcess = 7
	testDeadProcess = 8
)

// testRecord is one invented wtmp entry.
type testRecord struct {
	typ  int16
	pid  int32
	line string
	user string
	host string
	at   time.Time
}

// encode writes the record in glibc's struct utmp layout for the
// architecture, at the offsets glibc's headers give (offsetof() on 22.04,
// 24.04 and 26.04): amd64 has __WORDSIZE_TIME64_COMPAT32 and a 32-bit
// ut_tv.tv_sec at 340 in 384 bytes; arm64 has a 64-bit struct timeval at
// 344 in 400 bytes.
func (r testRecord) encode(arch string) []byte {
	size, secAt := 384, 340
	if arch == "arm64" {
		size, secAt = 400, 344
	}
	b := make([]byte, size)
	binary.LittleEndian.PutUint16(b[0:], uint16(r.typ))
	binary.LittleEndian.PutUint32(b[4:], uint32(r.pid))
	copy(b[8:8+32], r.line)
	copy(b[44:44+32], r.user)
	copy(b[76:76+256], r.host)
	if arch == "arm64" {
		binary.LittleEndian.PutUint64(b[secAt:], uint64(r.at.Unix()))
	} else {
		binary.LittleEndian.PutUint32(b[secAt:], uint32(r.at.Unix()))
	}
	return b
}

func wtmpFile(arch string, records ...testRecord) []byte {
	var buf bytes.Buffer
	for _, r := range records {
		buf.Write(r.encode(arch))
	}
	return buf.Bytes()
}

func at(day, hour, minute int) time.Time {
	return time.Date(2026, time.March, day, hour, minute, 0, 0, time.UTC)
}

var (
	boot = testRecord{typ: testBootTime, line: "~", user: "reboot", host: "6.8.0-1-generic", at: at(3, 7, 0)}
	// systemd's utmp_put_shutdown() zeroes the record: ut_pid is 0, so
	// only its user marks the shutdown.
	shutdown = testRecord{typ: testRunLvl, line: "~", user: "shutdown", host: "6.8.0-1-generic", at: at(3, 9, 0)}
	// A change to run level 6 from N: the run level is ut_pid's low byte.
	runlevel6 = testRecord{typ: testRunLvl, pid: '6' | 'N'<<8, line: "~", user: "runlevel", host: "6.8.0-1-generic", at: at(3, 9, 0)}
	aliceIn   = testRecord{typ: testUserProcess, pid: 4100, line: "pts/0", user: "alice", host: "192.0.2.10", at: at(3, 8, 15)}
	aliceOut  = testRecord{typ: testDeadProcess, pid: 4100, line: "pts/0", at: at(3, 8, 30)}
	bobIn     = testRecord{typ: testUserProcess, pid: 4200, line: "pts/1", user: "bob", host: "198.51.100.7", at: at(3, 8, 40)}
	nextBoot  = testRecord{typ: testBootTime, line: "~", user: "reboot", host: "6.8.0-1-generic", at: at(3, 10, 2)}
	longIPv6  = testRecord{typ: testUserProcess, pid: 4300, line: "pts/2", user: "averyverylongusernamethatfills32", host: "2001:db8:85a3::8a2e:370:7334", at: at(4, 6, 5)}
	gettyLine = testRecord{typ: testLoginProc, pid: 900, line: "tty1", user: "LOGIN", at: at(3, 8, 50)}
)

func value(s string) string { return ValueStyle.Render(s) }

func TestLastDuration(t *testing.T) {
	tests := map[int64]string{
		0:                       "00:00",
		59:                      "00:00",
		15 * 60:                 "00:15",
		3*3600 + 5*60:           "03:05",
		2*86400 + 3600 + 25*60:  "2+01:25",
		86400:                   "1+00:00",
		-3 * 60:                 "-00:03",
		-(2*3600 + 4*60):        "-2:04",
		-(86400 + 3600 + 2*60):  "-1+01:02",
		23*3600 + 59*60 + 59:    "23:59",
		40*86400 + 7*3600 + 120: "40+07:02",
	}
	for secs, want := range tests {
		if got := lastDuration(secs); got != want {
			t.Errorf("lastDuration(%d) = %q, want %q", secs, got, want)
		}
	}
}

func TestNewestLogin(t *testing.T) {
	alive := func(wtmpRecord) bool { return false }
	gone := func(wtmpRecord) bool { return true }
	tests := []struct {
		name    string
		records []testRecord
		phantom func(wtmpRecord) bool
		share   bool
		want    string
		found   bool
	}{
		{
			name:    "a login without a logout whose session is alive",
			records: []testRecord{boot, aliceIn},
			phantom: alive,
			want:    fmt.Sprintf("%s at %s (still logged in) from %s", value("alice"), value("Tue Mar 3 08:15"), value("192.0.2.10")),
			found:   true,
		},
		{
			name:    "a login without a logout whose session is gone",
			records: []testRecord{boot, aliceIn},
			phantom: gone,
			want:    fmt.Sprintf("%s at %s from %s", value("alice"), value("Tue Mar 3 08:15"), value("192.0.2.10")),
			found:   true,
		},
		{
			name:    "a login and its logout",
			records: []testRecord{boot, aliceIn, aliceOut},
			phantom: alive,
			want: fmt.Sprintf("%s at %s until %s (00:15) from %s", value("alice"), value("Tue Mar 3 08:15"),
				value("08:30"), value("192.0.2.10")),
			found: true,
		},
		{
			name:    "the newest login wins over an older one on another line",
			records: []testRecord{boot, aliceIn, bobIn, aliceOut},
			phantom: alive,
			want:    fmt.Sprintf("%s at %s (still logged in) from %s", value("bob"), value("Tue Mar 3 08:40"), value("198.51.100.7")),
			found:   true,
		},
		{
			name:    "a reboot after the login without a logout reads as a crash",
			records: []testRecord{boot, aliceIn, nextBoot},
			phantom: alive,
			want: fmt.Sprintf("%s at %s until %s (01:47) from %s", value("alice"), value("Tue Mar 3 08:15"),
				value("crash"), value("192.0.2.10")),
			found: true,
		},
		{
			name:    "a shutdown after the login without a logout reads as down",
			records: []testRecord{boot, aliceIn, shutdown, nextBoot},
			phantom: alive,
			want: fmt.Sprintf("%s at %s until %s (00:45) from %s", value("alice"), value("Tue Mar 3 08:15"),
				value("down"), value("192.0.2.10")),
			found: true,
		},
		{
			name:    "a run level 6 record after the login without a logout reads as down",
			records: []testRecord{boot, aliceIn, runlevel6, nextBoot},
			phantom: alive,
			want: fmt.Sprintf("%s at %s until %s (00:45) from %s", value("alice"), value("Tue Mar 3 08:15"),
				value("down"), value("192.0.2.10")),
			found: true,
		},
		{
			name: "the earliest logout on the line closes the login",
			records: []testRecord{boot, aliceIn, aliceOut,
				{typ: testDeadProcess, pid: 4100, line: "pts/0", at: at(3, 8, 50)}},
			phantom: alive,
			want: fmt.Sprintf("%s at %s until %s (00:15) from %s", value("alice"), value("Tue Mar 3 08:15"),
				value("08:30"), value("192.0.2.10")),
			found: true,
		},
		{
			name: "a reboot drops the logouts after it",
			records: []testRecord{boot, aliceIn, nextBoot,
				{typ: testDeadProcess, pid: 4100, line: "pts/0", at: at(3, 10, 30)}},
			phantom: alive,
			want: fmt.Sprintf("%s at %s until %s (01:47) from %s", value("alice"), value("Tue Mar 3 08:15"),
				value("crash"), value("192.0.2.10")),
			found: true,
		},
		{
			name: "control and format characters and bytes that aren't UTF-8 are dropped",
			records: []testRecord{boot, {typ: testUserProcess, pid: 4100, line: "pts/0",
				user: "al\x1b]0;x\x07\x1b[2J\xc2\x9bice\xff", host: "192.0.2.10\u202e\u200b", at: at(3, 8, 15)}},
			phantom: alive,
			want:    fmt.Sprintf("%s at %s (still logged in) from %s", value("al]0;x[2Jice"), value("Tue Mar 3 08:15"), value("192.0.2.10")),
			found:   true,
		},
		{
			name:    "a logout before the next reboot still closes the login",
			records: []testRecord{boot, aliceIn, aliceOut, nextBoot},
			phantom: alive,
			want: fmt.Sprintf("%s at %s until %s (00:15) from %s", value("alice"), value("Tue Mar 3 08:15"),
				value("08:30"), value("192.0.2.10")),
			found: true,
		},
		{
			name:    "a getty's LOGIN entry is no login",
			records: []testRecord{boot, aliceIn, aliceOut, gettyLine},
			phantom: alive,
			want: fmt.Sprintf("%s at %s until %s (00:15) from %s", value("alice"), value("Tue Mar 3 08:15"),
				value("08:30"), value("192.0.2.10")),
			found: true,
		},
		{
			name: "a user entry with an empty user is a logout",
			records: []testRecord{boot, aliceIn,
				{typ: testUserProcess, pid: 4100, line: "pts/0", at: at(3, 8, 20)}},
			phantom: alive,
			want: fmt.Sprintf("%s at %s until %s (00:05) from %s", value("alice"), value("Tue Mar 3 08:15"),
				value("08:20"), value("192.0.2.10")),
			found: true,
		},
		{
			name:    "a long user and an IPv6 source show whole",
			records: []testRecord{boot, longIPv6},
			phantom: alive,
			want: fmt.Sprintf("%s at %s (still logged in) from %s", value("averyverylongusernamethatfills32"),
				value("Wed Mar 4 06:05"), value("2001:db8:85a3::8a2e:370:7334")),
			found: true,
		},
		{
			name: "a console login has no source",
			records: []testRecord{boot,
				{typ: testUserProcess, pid: 950, line: "tty1", user: "root", at: at(3, 8, 55)}},
			phantom: alive,
			want:    fmt.Sprintf("%s at %s (still logged in) from %s", value("root"), value("Tue Mar 3 08:55"), value("local")),
			found:   true,
		},
		{
			name:    "share mode masks the source",
			records: []testRecord{boot, aliceIn},
			phantom: alive,
			share:   true,
			want:    fmt.Sprintf("%s at %s (still logged in) from %s", value("alice"), value("Tue Mar 3 08:15"), value("xxx.xxx.xxx.xxx")),
			found:   true,
		},
		{
			name:    "only a reboot",
			records: []testRecord{boot},
			phantom: alive,
		},
		{
			name:    "an empty file",
			phantom: alive,
		},
	}
	for _, arch := range []string{"amd64", "arm64"} {
		layout := utmpLayouts[arch]
		for _, tt := range tests {
			t.Run(arch+"/"+tt.name, func(t *testing.T) {
				data := wtmpFile(arch, tt.records...)
				login, found, err := newestLogin(bytes.NewReader(data), int64(len(data)), layout, tt.phantom)
				if err != nil {
					t.Fatalf("newestLogin() error = %v", err)
				}
				if found != tt.found {
					t.Fatalf("newestLogin() found = %v, want %v", found, tt.found)
				}
				if !found {
					return
				}
				if got := formatLogin(login, tt.share, time.UTC); got != tt.want {
					t.Fatalf("formatLogin() = %q, want %q", got, tt.want)
				}
			})
		}
	}
}

// A file cut inside its last record reads the whole records before it,
// as glibc's own reader does.
func TestNewestLoginIgnoresATruncatedTail(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		data := wtmpFile(arch, boot, aliceIn, aliceOut)
		data = data[:len(data)-100]
		login, found, err := newestLogin(bytes.NewReader(data), int64(len(data)), utmpLayouts[arch], func(wtmpRecord) bool { return true })
		if err != nil || !found {
			t.Fatalf("%s: newestLogin() = %v, %v", arch, found, err)
		}
		want := fmt.Sprintf("%s at %s from %s", value("alice"), value("Tue Mar 3 08:15"), value("192.0.2.10"))
		if got := formatLogin(login, false, time.UTC); got != want {
			t.Fatalf("%s: formatLogin() = %q, want %q", arch, got, want)
		}
	}
}

// Many records are read back to front across reads of several records.
func TestNewestLoginReadsAcrossChunks(t *testing.T) {
	records := []testRecord{boot, aliceIn}
	for i := range 300 {
		records = append(records, testRecord{typ: testBootTime, line: "~", user: "reboot", at: at(3, 9, 0).Add(time.Duration(i) * time.Second)})
	}
	data := wtmpFile("amd64", records...)
	login, found, err := newestLogin(bytes.NewReader(data), int64(len(data)), utmpLayouts["amd64"], func(wtmpRecord) bool { return false })
	if err != nil || !found || login.record.user != "alice" || login.status != loginCrash {
		t.Fatalf("newestLogin() = %+v, %v, %v", login, found, err)
	}
}

func TestPhantom(t *testing.T) {
	dir := t.TempDir()
	proc := filepath.Join(dir, "proc")
	dev := filepath.Join(dir, "dev")
	if err := os.MkdirAll(filepath.Join(proc, "4100"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proc, "4100", "loginuid"), []byte("1000"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dev, "pts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dev, "pts", "7"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	me := uint32(os.Getuid())
	uids := map[string]uint32{"alice": 1000, "bob": 1001, "me": me}
	src := loginSource{
		bootTime: at(3, 7, 0).Unix(),
		procDir:  proc,
		devDir:   dev,
		lookupUID: func(user string) (uint32, bool) {
			uid, ok := uids[user]
			return uid, ok
		},
	}
	tests := []struct {
		name string
		rec  wtmpRecord
		want bool
	}{
		{name: "its process has the user's login uid", rec: wtmpRecord{pid: 4100, line: "pts/0", user: "alice", sec: at(3, 8, 15).Unix()}, want: false},
		{name: "its process has another login uid", rec: wtmpRecord{pid: 4100, line: "pts/0", user: "bob", sec: at(3, 8, 15).Unix()}, want: true},
		{name: "from before the boot", rec: wtmpRecord{pid: 4100, line: "pts/0", user: "alice", sec: at(2, 8, 15).Unix()}, want: true},
		{name: "an unknown user", rec: wtmpRecord{pid: 4100, line: "pts/0", user: "nobody-here", sec: at(3, 8, 15).Unix()}, want: true},
		{name: "no process, its terminal is the user's", rec: wtmpRecord{pid: 4999, line: "pts/7", user: "me", sec: at(3, 8, 15).Unix()}, want: false},
		{name: "no process, its terminal is another user's", rec: wtmpRecord{pid: 4999, line: "pts/7", user: "alice", sec: at(3, 8, 15).Unix()}, want: me != 1000},
		{name: "no process and no terminal", rec: wtmpRecord{pid: 4999, line: "ssh", user: "alice", sec: at(3, 8, 15).Unix()}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := src.phantom(tt.rec); got != tt.want {
				t.Fatalf("phantom() = %v, want %v", got, tt.want)
			}
		})
	}
}

func testSource(t *testing.T, data []byte) loginSource {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wtmp")
	if data != nil {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return loginSource{
		wtmpPath:  path,
		layout:    utmpLayouts["amd64"],
		layoutOK:  true,
		bootTime:  at(3, 7, 0).Unix(),
		procDir:   t.TempDir(),
		devDir:    t.TempDir(),
		lookupUID: func(string) (uint32, bool) { return 0, false },
		loc:       time.UTC,
	}
}

func TestLastLoginReadsWtmp(t *testing.T) {
	runner := &fakeRunner{answers: map[string]string{}}
	src := testSource(t, wtmpFile("amd64", boot, aliceIn, aliceOut))
	want := fmt.Sprintf("%s at %s until %s (00:15) from %s", value("alice"), value("Tue Mar 3 08:15"),
		value("08:30"), value("192.0.2.10"))
	if got := lastLogin(context.Background(), runner.run, src); got != want {
		t.Fatalf("lastLogin() = %q, want %q", got, want)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("lastLogin() ran %q, want nothing", runner.calls)
	}
}

func TestLastLoginWithoutLoginsInWtmp(t *testing.T) {
	for name, data := range map[string][]byte{"empty": {}, "only a reboot": wtmpFile("amd64", boot)} {
		runner := &fakeRunner{answers: map[string]string{}}
		if got := lastLogin(context.Background(), runner.run, testSource(t, data)); got != "No recent logins" {
			t.Fatalf("%s: lastLogin() = %q, want No recent logins", name, got)
		}
		if len(runner.calls) != 0 {
			t.Fatalf("%s: lastLogin() ran %q, want nothing", name, runner.calls)
		}
	}
}

func TestLastLoginFallsBackWhenWtmpCantBeRead(t *testing.T) {
	tests := []struct {
		name    string
		answers map[string]string
		want    string
		calls   []string
	}{
		{
			name:    "nothing answers",
			answers: map[string]string{},
			want:    "No recent logins",
			calls:   []string{"lastlog -u root", "who -u"},
		},
		{
			name: "a lastlog line",
			answers: map[string]string{
				"lastlog -u root": "Username         Port     From             Latest\n" +
					"root             pts/0    192.0.2.10       Tue Mar  3 08:15:02 +0000 2026",
			},
			want:  fmt.Sprintf("%s %s", value("root"), value("Tue Mar 3 08:15:02 +0000 2026")),
			calls: []string{"lastlog -u root"},
		},
		{
			name: "root never logged in falls through to who",
			answers: map[string]string{
				"lastlog -u root": "Username         Port     From             Latest\n" +
					"root                                       **Never logged in**",
				"who -u": "alice    pts/0        2026-03-03 08:15   .          4100 (192.0.2.10)",
			},
			want:  fmt.Sprintf("%s at %s (still logged in)", value("alice"), value("2026-03-03 08:15")),
			calls: []string{"lastlog -u root", "who -u"},
		},
		{
			name: "who's C-locale date",
			answers: map[string]string{
				"who -u": "alice    pts/0        Mar  3 08:15   .          4100 (192.0.2.10)",
			},
			want:  fmt.Sprintf("%s at %s (still logged in)", value("alice"), value("Mar 3 08:15")),
			calls: []string{"lastlog -u root", "who -u"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeRunner{answers: tt.answers}
			src := testSource(t, nil) // no file at the path
			if got := lastLogin(context.Background(), runner.run, src); got != tt.want {
				t.Fatalf("lastLogin() = %q, want %q", got, tt.want)
			}
			if strings.Join(runner.calls, "\n") != strings.Join(tt.calls, "\n") {
				t.Fatalf("lastLogin() ran %q, want %q", runner.calls, tt.calls)
			}
		})
	}
}

// A FIFO or a symlink at the path isn't read: opening a FIFO would block
// the login, and a symlink could point anywhere.
func TestLastLoginReadsOnlyARegularFile(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.WriteFile(real, wtmpFile("amd64", boot, aliceIn), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{link, fifo, dir} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			runner := &fakeRunner{answers: map[string]string{}}
			src := testSource(t, nil)
			src.wtmpPath = path
			done := make(chan string, 1)
			go func() { done <- lastLogin(context.Background(), runner.run, src) }()
			select {
			case got := <-done:
				if got != "No recent logins" || len(runner.calls) != 2 {
					t.Fatalf("lastLogin() = %q after %q, want the fallbacks", got, runner.calls)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("lastLogin() blocked")
			}
		})
	}
}

func TestLastLoginFallsBackOnAnUnknownLayout(t *testing.T) {
	runner := &fakeRunner{answers: map[string]string{}}
	src := testSource(t, wtmpFile("amd64", boot, aliceIn))
	src.layoutOK = false
	if got := lastLogin(context.Background(), runner.run, src); got != "No recent logins" || len(runner.calls) != 2 {
		t.Fatalf("lastLogin() = %q after %q", got, runner.calls)
	}
}
