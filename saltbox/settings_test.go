package saltbox

import "testing"

func TestRcloneRemoteListedUsesNamesOnly(t *testing.T) {
	output := []byte("media:\nbackup:\n")
	found, count := rcloneRemoteListed(output, "media")
	if !found || count != 2 {
		t.Fatalf("found = %t, count = %d", found, count)
	}
	if found, _ := rcloneRemoteListed(output, "secret-token"); found {
		t.Fatal("matched a value that was not a remote name")
	}
}
