package motd

import (
	"net/url"
	"regexp"
	"strings"
)

const (
	redactedCredential             = "<redacted>"
	credentialRedactionPlaceholder = "\x00sb-motd-credential\x00"
)

var (
	sensitiveQueryCredentialPattern = regexp.MustCompile(`(?i)([?&](?:api[-_]?key|access[-_]?token|token)=)[^&#\s"']+`)
	urlUserinfoPasswordPattern      = regexp.MustCompile(`(?i)(https?://[^:/@\s"']+:)[^/@\s"']+(@)`)
)

type censoredProviderError struct {
	cause   error
	message string
}

func (e *censoredProviderError) Error() string {
	return e.message
}

func (e *censoredProviderError) Unwrap() error {
	return e.cause
}

func censorProviderError(err error, credentials ...string) error {
	if err == nil {
		return nil
	}

	original := err.Error()
	message := censorProviderText(original, credentials...)
	if message == original {
		return err
	}

	return &censoredProviderError{cause: err, message: message}
}

func censorProviderText(message string, credentials ...string) string {
	seen := make(map[string]struct{}, len(credentials)*3)
	replacements := make([]string, 0, len(credentials)*6)
	for _, credential := range credentials {
		if credential == "" {
			continue
		}

		variants := []string{
			url.QueryEscape(credential),
			url.PathEscape(credential),
			credential,
		}
		for _, variant := range variants {
			if variant == "" {
				continue
			}
			if _, ok := seen[variant]; ok {
				continue
			}
			seen[variant] = struct{}{}
			replacements = append(replacements, variant, credentialRedactionPlaceholder)
		}
	}
	if len(replacements) > 0 {
		message = strings.NewReplacer(replacements...).Replace(message)
	}

	message = sensitiveQueryCredentialPattern.ReplaceAllString(message, `${1}`+credentialRedactionPlaceholder)
	message = urlUserinfoPasswordPattern.ReplaceAllString(message, `${1}`+credentialRedactionPlaceholder+`${2}`)

	return strings.ReplaceAll(message, credentialRedactionPlaceholder, redactedCredential)
}
