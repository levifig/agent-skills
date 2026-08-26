package project

import (
	"net/url"
	"regexp"
	"strings"
)

var scpLikeRemote = regexp.MustCompile(`^([^@/]+@)?([^:/]+):([^/].+)$`)

// NormalizeRemoteURL converges ssh/https/port/.git variants of one remote to a
// canonical comparison key. Cross-host mirror pairs do not converge unless
// explicitly attached elsewhere.
func NormalizeRemoteURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	host, path, err := parseRemoteHostPath(raw)
	if err != nil {
		return "", err
	}
	if host == "" {
		return "", nil
	}
	path = trimGitSuffix(cleanRemotePath(path))
	key := strings.ToLower(host) + path
	return strings.TrimSuffix(key, "/"), nil
}

func parseRemoteHostPath(raw string) (host, path string, err error) {
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "", "", err
		}
		host = parsed.Hostname()
		port := parsed.Port()
		if port != "" && port != "22" && port != "443" && port != "80" {
			host = host + ":" + port
		}
		return host, parsed.Path, nil
	}
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "ssh://")
	if strings.HasPrefix(trimmed, "git@") {
		trimmed = trimmed[len("git@"):]
	} else if match := scpLikeRemote.FindStringSubmatch(trimmed); len(match) == 4 {
		return match[2], "/" + match[3], nil
	}
	parts := strings.SplitN(trimmed, ":", 2)
	if len(parts) == 2 && !strings.Contains(parts[0], "/") {
		return parts[0], "/" + parts[1], nil
	}
	host, path, ok := strings.Cut(strings.TrimPrefix(trimmed, "/"), "/")
	if !ok {
		return trimmed, "", nil
	}
	return host, "/" + path, nil
}

func cleanRemotePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func trimGitSuffix(path string) string {
	path = strings.TrimSuffix(path, "/"); return strings.TrimSuffix(path, ".git")
}
