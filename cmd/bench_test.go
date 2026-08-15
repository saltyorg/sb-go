package cmd

import (
	"strings"
	"testing"
)

func TestEmbeddedBenchmarkHasNoExecutableDownloader(t *testing.T) {
	for _, forbidden := range []string{"--no-check-certificate", "dl.lamp.sh", "tar zxf speedtest", "./speedtest-cli/speedtest", "http://", "wget -qO- bench.sh | bash"} {
		if strings.Contains(benchmarkScript, forbidden) {
			t.Fatalf("embedded benchmark contains forbidden executable-download path %q", forbidden)
		}
	}
	if !strings.Contains(benchmarkScript, `"${SB_SPEEDTEST_BIN}"`) {
		t.Fatal("embedded benchmark does not use the verified Speedtest path")
	}
}
