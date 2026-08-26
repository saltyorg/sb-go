package motd

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

const credentialCanary = "MOTD_CREDENTIAL_CANARY"

type credentialRoundTripper func(*http.Request) (*http.Response, error)

func (f credentialRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func installCredentialTransport(t *testing.T, roundTrip credentialRoundTripper) {
	t.Helper()

	original := http.DefaultTransport
	http.DefaultTransport = roundTrip
	t.Cleanup(func() {
		http.DefaultTransport = original
	})
}

func TestEmbyUsesHeaderAndKeepsTokenOutOfErrors(t *testing.T) {
	var request *http.Request
	installCredentialTransport(t, func(req *http.Request) (*http.Response, error) {
		request = req.Clone(req.Context())
		return nil, errors.New("synthetic connection failure")
	})

	_, err := getEmbyStreamInfo(t.Context(), EmbyInstance{
		URL:   "http://emby.test",
		Token: credentialCanary,
	})
	if err == nil {
		t.Fatal("getEmbyStreamInfo() error = nil, want transport error")
	}
	if request == nil {
		t.Fatal("getEmbyStreamInfo() did not issue a request")
	}
	if got := request.URL.Query().Get("api_key"); got != "" {
		t.Error("Emby request unexpectedly contains api_key query parameter")
	}
	if got := request.Header.Get("X-Emby-Token"); got != credentialCanary {
		t.Error("Emby request did not send the token in X-Emby-Token")
	}
	if strings.Contains(err.Error(), credentialCanary) {
		t.Error("Emby transport error exposed the configured token")
	}
}

func TestSabnzbdCensorsRequiredQueryCredentialInErrors(t *testing.T) {
	var request *http.Request
	installCredentialTransport(t, func(req *http.Request) (*http.Response, error) {
		request = req.Clone(req.Context())
		return nil, errors.New("synthetic connection failure")
	})

	_, err := getSabnzbdQueueInfo(t.Context(), AppInstance{
		URL:    "http://sabnzbd.test",
		APIKey: credentialCanary,
	})
	if err == nil {
		t.Fatal("getSabnzbdQueueInfo() error = nil, want transport error")
	}
	if request == nil {
		t.Fatal("getSabnzbdQueueInfo() did not issue a request")
	}
	if got := request.URL.Query().Get("apikey"); got != credentialCanary {
		t.Error("SABnzbd request did not retain its required apikey query parameter")
	}
	if strings.Contains(err.Error(), credentialCanary) {
		t.Error("SABnzbd transport error exposed the configured API key")
	}
	if !strings.Contains(err.Error(), "<redacted>") {
		t.Errorf("SABnzbd transport error did not mark the credential as redacted: %q", err)
	}
}

func TestNzbgetUsesBasicAuthWithoutURLUserinfo(t *testing.T) {
	var request *http.Request
	installCredentialTransport(t, func(req *http.Request) (*http.Response, error) {
		request = req.Clone(req.Context())
		return nil, errors.New("synthetic connection failure")
	})

	var target struct{}
	err := callNzbgetAPI(t.Context(), UserPassAppInstance{
		URL:      "http://nzbget.test",
		User:     "motd-user",
		Password: credentialCanary,
	}, "status", &target)
	if err == nil {
		t.Fatal("callNzbgetAPI() error = nil, want transport error")
	}
	if request == nil {
		t.Fatal("callNzbgetAPI() did not issue a request")
	}
	if request.URL.User != nil {
		t.Error("NZBGet request URL unexpectedly contains userinfo")
	}
	user, password, ok := request.BasicAuth()
	if !ok || user != "motd-user" || password != credentialCanary {
		t.Error("NZBGet request did not use the configured HTTP Basic credentials")
	}
	if strings.Contains(err.Error(), credentialCanary) {
		t.Error("NZBGet transport error exposed the configured password")
	}
}

func TestProviderSummariesCensorCredentialQueryParameters(t *testing.T) {
	for _, parameter := range []string{"apikey", "api_key", "api-key", "token", "access_token", "access-token"} {
		t.Run(parameter, func(t *testing.T) {
			err := fmt.Errorf("Get \"http://service.test/api?mode=queue&%s=%s\": connection refused", parameter, credentialCanary)
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
					if strings.Contains(tc.got, credentialCanary) {
						t.Errorf("provider summary exposed a credential query value: %q", tc.got)
					}
					if !strings.Contains(tc.got, "connection refused") {
						t.Errorf("provider summary lost the useful transport cause: %q", tc.got)
					}
				})
			}
		})
	}
}

func TestCensorProviderErrorRedactsConfiguredCredentialAndPreservesCause(t *testing.T) {
	credential := "token with/+symbols"
	variants := []string{
		credential,
		url.QueryEscape(credential),
		url.PathEscape(credential),
	}

	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			cause := fmt.Errorf("provider echoed credential %s", variant)
			got := censorProviderError(cause, credential)
			if strings.Contains(got.Error(), variant) {
				t.Error("censorProviderError() exposed a configured credential")
			}
			if !strings.Contains(got.Error(), redactedCredential) {
				t.Errorf("censorProviderError() = %q, want redaction marker", got)
			}
			if !errors.Is(got, cause) {
				t.Error("censorProviderError() did not preserve the original error chain")
			}
		})
	}
}

func TestFormatProviderErrorCensorsURLPassword(t *testing.T) {
	got := formatProviderError(errors.New(`Get "http://motd-user:secret-password@service.test/api": connection refused`))
	if strings.Contains(got, "secret-password") {
		t.Errorf("formatProviderError() exposed URL userinfo: %q", got)
	}
	if !strings.Contains(got, redactedCredential) {
		t.Errorf("formatProviderError() = %q, want redaction marker", got)
	}
}
