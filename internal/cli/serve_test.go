package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServeListenRequiresTLS(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "8443 without certs", args: []string{"--listen", ":8443", "--db", "sync.sqlite"}, wantErr: "requires --tls-cert and --tls-key"},
		{name: "443 without certs", args: []string{"--listen", "127.0.0.1:443", "--db", "sync.sqlite"}, wantErr: "requires --tls-cert and --tls-key"},
		{name: "bare 8443", args: []string{"--listen", "8443", "--db", "sync.sqlite"}, wantErr: "requires --tls-cert and --tls-key"},
		{name: "cert without key", args: []string{"--listen", ":8080", "--tls-cert", "cert.pem", "--db", "sync.sqlite"}, wantErr: "must be set together"},
		{name: "key without cert", args: []string{"--listen", ":8080", "--tls-key", "key.pem", "--db", "sync.sqlite"}, wantErr: "must be set together"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseServeArgs(tc.args)
			if err == nil {
				t.Fatal("parseServeArgs() error = nil, want TLS validation error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("parseServeArgs() error = %q, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestServeListenAllowsCleartextHTTP(t *testing.T) {
	t.Parallel()
	opts, err := parseServeArgs([]string{"--listen", ":8080", "--db", "sync.sqlite"})
	if err != nil {
		t.Fatalf("parseServeArgs() error = %v", err)
	}
	if opts.listen != ":8080" {
		t.Fatalf("listen = %q, want :8080", opts.listen)
	}
	opts, err = parseServeArgs([]string{"--listen", ":8443", "--tls-cert", "cert.pem", "--tls-key", "key.pem", "--db", "sync.sqlite"})
	if err != nil {
		t.Fatalf("parseServeArgs() with TLS files error = %v", err)
	}
	if opts.tlsCert != "cert.pem" || opts.tlsKey != "key.pem" {
		t.Fatalf("tls files = %q %q, want cert.pem key.pem", opts.tlsCert, opts.tlsKey)
	}
}

func TestSyncServerDockerfileBindsHTTP8080(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "cli", "docker", "sync-server", "Dockerfile")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	body := string(data)
	if strings.Contains(body, "8443") {
		t.Fatal("sync-server Dockerfile still mentions 8443; bind cleartext HTTP on :8080")
	}
	if !strings.Contains(body, `EXPOSE 8080`) {
		t.Fatal("sync-server Dockerfile must EXPOSE 8080")
	}
	if !strings.Contains(body, `":8080"`) {
		t.Fatal("sync-server Dockerfile must listen on :8080")
	}
}
