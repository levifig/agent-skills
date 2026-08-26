package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/levifig/loaf/internal/syncserver"
)

type serveOptions struct {
	listen string
	dbPath string
}

func (r Runner) runServe(args []string, out io.Writer) error {
	if len(args) == 0 || isHelpArg(args) {
		writeServeHelp(out)
		return nil
	}
	opts, err := parseServeArgs(args)
	if err != nil {
		return err
	}
	dbPath, err := resolveServeDBPath(opts.dbPath, r.StateHome)
	if err != nil {
		return err
	}
	store, err := syncserver.OpenStore(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	srv := syncserver.NewServer(store)
	httpSrv := &http.Server{
		Addr:              opts.listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	fmt.Fprintf(out, "loaf serve listening on %s (db: %s)\n", opts.listen, dbPath)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpSrv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func writeServeHelp(out io.Writer) {
	writeUsageHelp(out, "loaf serve", "Run the self-hostable sync relay (opaque blobs + token auth).",
		"--listen <addr>   Listen address (default :8080)",
		"--db <path>       Sync server SQLite path (default $XDG_DATA_HOME/loaf/sync.sqlite)")
}

func parseServeArgs(args []string) (serveOptions, error) {
	opts := serveOptions{listen: ":8080"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--listen":
			if i+1 >= len(args) {
				return serveOptions{}, fmt.Errorf("serve --listen requires a value")
			}
			opts.listen = strings.TrimSpace(args[i+1])
			i++
		case "--db":
			if i+1 >= len(args) {
				return serveOptions{}, fmt.Errorf("serve --db requires a value")
			}
			opts.dbPath = strings.TrimSpace(args[i+1])
			i++
		default:
			return serveOptions{}, fmt.Errorf("serve: unknown option %q", args[i])
		}
	}
	if opts.listen == "" {
		return serveOptions{}, fmt.Errorf("serve --listen cannot be empty")
	}
	return opts, nil
}

func resolveServeDBPath(explicit, stateHome string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return filepath.Clean(explicit), nil
	}
	home := strings.TrimSpace(stateHome)
	if home == "" {
		return "", fmt.Errorf("serve --db is required when state home is unset")
	}
	return filepath.Join(home, "sync.sqlite"), nil
}
