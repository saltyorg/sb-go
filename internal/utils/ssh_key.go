package utils

import "strings"

var authorizedKeyTypes = map[string]struct{}{
	"sk-ecdsa-sha2-nistp256@openssh.com":          {},
	"sk-ecdsa-sha2-nistp256-cert-v01@openssh.com": {},
	"webauthn-sk-ecdsa-sha2-nistp256@openssh.com": {},
	"ecdsa-sha2-nistp256":                         {},
	"ecdsa-sha2-nistp256-cert-v01@openssh.com":    {},
	"ecdsa-sha2-nistp384":                         {},
	"ecdsa-sha2-nistp384-cert-v01@openssh.com":    {},
	"ecdsa-sha2-nistp521":                         {},
	"ecdsa-sha2-nistp521-cert-v01@openssh.com":    {},
	"sk-ssh-ed25519@openssh.com":                  {},
	"sk-ssh-ed25519-cert-v01@openssh.com":         {},
	"ssh-ed25519":                                 {},
	"ssh-ed25519-cert-v01@openssh.com":            {},
	"ssh-dss":                                     {},
	"ssh-rsa":                                     {},
	"ssh-xmss@openssh.com":                        {},
	"ssh-xmss-cert-v01@openssh.com":               {},
	"rsa-sha2-256":                                {},
	"rsa-sha2-512":                                {},
	"ssh-rsa-cert-v01@openssh.com":                {},
	"rsa-sha2-256-cert-v01@openssh.com":           {},
	"rsa-sha2-512-cert-v01@openssh.com":           {},
	"ssh-dss-cert-v01@openssh.com":                {},
}

// IsValidAuthorizedKeyLine checks a single authorized_keys line for a supported key type.
func IsValidAuthorizedKeyLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}

	fields := strings.Fields(trimmed)
	if len(fields) < 2 {
		return false
	}

	for i, field := range fields {
		if _, ok := authorizedKeyTypes[field]; ok {
			return i+1 < len(fields)
		}
	}

	return false
}

// IsValidAuthorizedKeyOrURL validates a key string or supported key source URL.
func IsValidAuthorizedKeyOrURL(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return true
	}

	if strings.HasPrefix(trimmed, "http") || strings.HasPrefix(trimmed, "file") {
		return true
	}

	lines := strings.Split(trimmed, "\n")
	foundKey := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		foundKey = true
		if !IsValidAuthorizedKeyLine(line) {
			return false
		}
	}

	return foundKey
}
