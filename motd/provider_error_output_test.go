package motd

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestProviderSummariesShowErrors(t *testing.T) {
	err := errors.New("timeout contacting instance")

	cases := []struct {
		name string
		got  string
	}{
		{name: "sabnzbd", got: formatSabnzbdSummary(SabnzbdInfo{Error: err})},
		{name: "nzbget", got: formatNzbgetSummary(NzbgetInfo{Error: err})},
		{name: "qbittorrent", got: formatQbittorrentSummary(qbittorrentInfo{Error: err})},
		{name: "rtorrent", got: formatRtorrentSummary(rtorrentInfo{Error: err})},
		{name: "plex", got: formatPlexStreamSummary(PlexStreamInfo{Error: err})},
		{name: "emby", got: formatStreamSummary(EmbyStreamInfo{Error: err})},
		{name: "jellyfin", got: formatJellyfinOutput([]JellyfinStreamInfo{{Name: "Jellyfin", Error: err}})},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(tc.got, "Error: timeout contacting instance") {
				t.Fatalf("expected error text in output, got %q", tc.got)
			}
		})
	}
}

func TestRunSectionProviderShowsTimeoutError(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	out := runSectionProvider(ctx, false, "Example info", func(context.Context, bool) string {
		return ""
	})

	if !strings.Contains(out, "Example info request ended early") {
		t.Fatalf("expected timeout error text, got %q", out)
	}
}
