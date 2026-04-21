package stack

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aprakasa/gow/internal/testmock"
)

var errGeneric = errors.New("mock error")

func writeOutputMock(t *testing.T, body string) string {
	t.Helper()
	return testmock.WriteMock(t, "printf '%s' '"+body+"'")
}

func TestShellRunner_Run_Success(t *testing.T) {
	r := NewShellRunner()
	mock := testmock.WriteMock(t, "exit 0")
	if err := r.Run(ctx, mock); err != nil {
		t.Fatalf("Run() = %v, want nil", err)
	}
}

func TestShellRunner_Run_Failure(t *testing.T) {
	r := NewShellRunner()
	mock := testmock.WriteMock(t, "echo 'something failed' >&2; exit 1")
	err := r.Run(ctx, mock)
	if err == nil {
		t.Fatal("Run() should return error on non-zero exit")
	}
	if !strings.Contains(err.Error(), "something failed") {
		t.Errorf("error should contain stderr output, got %q", err.Error())
	}
}

func TestShellRunner_Output_Success(t *testing.T) {
	r := NewShellRunner()
	mock := writeOutputMock(t, "hello world")
	got, err := r.Output(ctx, mock)
	if err != nil {
		t.Fatalf("Output() = %v", err)
	}
	if got != "hello world" {
		t.Errorf("Output() = %q, want %q", got, "hello world")
	}
}

func TestShellRunner_Output_Failure(t *testing.T) {
	r := NewShellRunner()
	mock := testmock.WriteMock(t, "exit 1")
	_, err := r.Output(ctx, mock)
	if err == nil {
		t.Fatal("Output() should return error on non-zero exit")
	}
}

func TestShellRunner_Stream_WritesStdout(t *testing.T) {
	r := NewShellRunner()
	mock := testmock.WriteMock(t, "printf '%s' 'streamed'")
	var out, errOut bytes.Buffer
	if err := r.Stream(ctx, nil, &out, &errOut, mock); err != nil {
		t.Fatalf("Stream() = %v", err)
	}
	if out.String() != "streamed" {
		t.Errorf("stdout = %q, want %q", out.String(), "streamed")
	}
}

func TestShellRunner_Stream_WritesStderr(t *testing.T) {
	r := NewShellRunner()
	mock := testmock.WriteMock(t, "printf '%s' 'oops' >&2")
	var out, errOut bytes.Buffer
	if err := r.Stream(ctx, nil, &out, &errOut, mock); err != nil {
		t.Fatalf("Stream() = %v", err)
	}
	if errOut.String() != "oops" {
		t.Errorf("stderr = %q, want %q", errOut.String(), "oops")
	}
}

func TestShellRunner_Stream_ReadsStdin(t *testing.T) {
	r := NewShellRunner()
	mock := testmock.WriteMock(t, "cat")
	var out, errOut bytes.Buffer
	in := strings.NewReader("piped-in")
	if err := r.Stream(ctx, in, &out, &errOut, mock); err != nil {
		t.Fatalf("Stream() = %v", err)
	}
	if out.String() != "piped-in" {
		t.Errorf("stdout = %q, want %q", out.String(), "piped-in")
	}
}

func TestShellRunner_Stream_ReturnsErrOnNonZero(t *testing.T) {
	r := NewShellRunner()
	mock := testmock.WriteMock(t, "exit 2")
	var out, errOut bytes.Buffer
	err := r.Stream(ctx, nil, &out, &errOut, mock)
	if err == nil {
		t.Fatal("Stream() should return error on non-zero exit")
	}
}

func TestShellRunner_Run_PassesArgs(t *testing.T) {
	r := NewShellRunner()
	dir := t.TempDir()
	mock := testmock.WriteArgMock(t, dir)

	if err := r.Run(ctx, mock, "hello", "world"); err != nil {
		t.Fatalf("Run() = %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "args")) //nolint:gosec // test reads from temp dir
	if !strings.Contains(string(got), "hello") || !strings.Contains(string(got), "world") {
		t.Errorf("args = %q, want 'hello world'", string(got))
	}
}
