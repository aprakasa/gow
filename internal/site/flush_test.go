package site

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/aprakasa/gow/internal/state"
)

func TestFlush_WP_NoCache_RunsOnlyCacheFlush(t *testing.T) {
	rr := &recordingRunner{}
	m, _ := setupManagerWithRunner(t, rr)
	if err := m.store.Add(state.Site{Name: "blog.test", Type: "wp", PHPVersion: "83", Preset: "standard"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}

	if err := m.Flush(context.Background(), "blog.test"); err != nil {
		t.Fatalf("Flush() = %v", err)
	}

	sawCacheFlush, sawLSCachePurge := false, false
	for _, cmd := range rr.commands {
		joined := strings.Join(cmd, " ")
		if strings.Contains(joined, "cache flush") {
			sawCacheFlush = true
		}
		if strings.Contains(joined, "litespeed-purge") {
			sawLSCachePurge = true
		}
	}
	if !sawCacheFlush {
		t.Error("expected `wp cache flush` call")
	}
	if sawLSCachePurge {
		t.Error("should not call litespeed-purge when CacheMode is empty")
	}
}

func TestFlush_WP_LSCache_RunsBoth(t *testing.T) {
	rr := &recordingRunner{}
	m, _ := setupManagerWithRunner(t, rr)
	if err := m.store.Add(state.Site{Name: "blog.test", Type: "wp", PHPVersion: "83", Preset: "standard", CacheMode: "lscache"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}

	if err := m.Flush(context.Background(), "blog.test"); err != nil {
		t.Fatalf("Flush() = %v", err)
	}

	sawCacheFlush, sawLSCachePurge := false, false
	for _, cmd := range rr.commands {
		joined := strings.Join(cmd, " ")
		if strings.Contains(joined, "cache flush") {
			sawCacheFlush = true
		}
		if strings.Contains(joined, "litespeed-purge all") {
			sawLSCachePurge = true
		}
	}
	if !sawCacheFlush {
		t.Error("expected `wp cache flush` call")
	}
	if !sawLSCachePurge {
		t.Error("expected `wp litespeed-purge all` call for lscache sites")
	}
}

func TestFlush_Multisite_AddsURL(t *testing.T) {
	rr := &recordingRunner{}
	m, _ := setupManagerWithRunner(t, rr)
	if err := m.store.Add(state.Site{Name: "net.test", Type: "wp", PHPVersion: "83", Preset: "standard", Multisite: "subdirectory"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}

	if err := m.Flush(context.Background(), "net.test"); err != nil {
		t.Fatalf("Flush() = %v", err)
	}

	for _, cmd := range rr.commands {
		joined := strings.Join(cmd, " ")
		if strings.Contains(joined, "cache flush") {
			if !strings.Contains(joined, "--url=net.test") {
				t.Errorf("multisite flush missing --url, got: %s", joined)
			}
		}
	}
}

func TestFlush_HTMLSite_Errors(t *testing.T) {
	m, _ := setupManagerWithRunner(t, &recordingRunner{})
	if err := m.store.Add(state.Site{Name: "static.test", Type: "html"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}

	err := m.Flush(context.Background(), "static.test")
	if err == nil {
		t.Fatal("expected error for html site")
	}
	if !strings.Contains(err.Error(), "html") {
		t.Errorf("error = %q, want mention of 'html'", err.Error())
	}
}

func TestFlush_SiteNotFound_Errors(t *testing.T) {
	m, _ := setupManagerWithRunner(t, &recordingRunner{})
	err := m.Flush(context.Background(), "nope.test")
	if err == nil {
		t.Fatal("expected error for missing site")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

// selectiveFailRunner fails only when the command+args contain all substrings
// in failOn. Used to simulate litespeed-purge failing on non-lscache sites.
type selectiveFailRunner struct {
	recordingRunner
	failOn []string
	failed error
}

func (r *selectiveFailRunner) Run(ctx context.Context, name string, args ...string) error {
	_ = r.recordingRunner.Run(ctx, name, args...)
	joined := name + " " + strings.Join(args, " ")
	for _, sub := range r.failOn {
		if !strings.Contains(joined, sub) {
			return nil
		}
	}
	return r.failed
}

func (r *selectiveFailRunner) Stream(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, name string, args ...string) error {
	return r.recordingRunner.Stream(ctx, stdin, stdout, stderr, name, args...)
}

func TestFlush_LSCachePurgeFailure_IsWarning(t *testing.T) {
	rr := &selectiveFailRunner{
		failOn: []string{"litespeed-purge"},
		failed: context.DeadlineExceeded, // any error
	}
	m, _ := setupManagerWithRunner(t, rr)
	if err := m.store.Add(state.Site{Name: "blog.test", Type: "wp", PHPVersion: "83", Preset: "standard", CacheMode: "lscache"}); err != nil {
		t.Fatalf("store.Add: %v", err)
	}

	if err := m.Flush(context.Background(), "blog.test"); err != nil {
		t.Fatalf("Flush() should succeed despite litespeed-purge failure, got %v", err)
	}
}
