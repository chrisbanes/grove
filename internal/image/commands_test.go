package image

import (
	"errors"
	"strings"
	"testing"
)

type runnerCall struct {
	name string
	args []string
}

type streamCall struct {
	name string
	args []string
}

type fakeRunner struct {
	calls       []runnerCall
	outputs     [][]byte
	errs        []error
	streamCalls []streamCall
	streamLines []string
	streamErr   error
}

func (f *fakeRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, runnerCall{name: name, args: append([]string(nil), args...)})
	if len(f.outputs) == 0 {
		return nil, nil
	}
	out := f.outputs[0]
	f.outputs = f.outputs[1:]
	var err error
	if len(f.errs) > 0 {
		err = f.errs[0]
		f.errs = f.errs[1:]
	}
	return out, err
}

func (f *fakeRunner) Stream(name string, args []string, onLine func(string)) error {
	f.streamCalls = append(f.streamCalls, streamCall{name: name, args: append([]string(nil), args...)})
	for _, line := range f.streamLines {
		onLine(line)
	}
	return f.streamErr
}

func TestCreateSparseBundle_UsesExpectedCommand(t *testing.T) {
	r := &fakeRunner{}
	if err := CreateSparseBundle(r, "/tmp/base.sparsebundle", "grove-base", 20); err != nil {
		t.Fatalf("CreateSparseBundle() error = %v", err)
	}

	if len(r.calls) != 1 {
		t.Fatalf("expected 1 command call, got %d", len(r.calls))
	}
	call := r.calls[0]
	if call.name != "hdiutil" {
		t.Fatalf("expected hdiutil, got %q", call.name)
	}

	want := []string{"create", "-type", "SPARSEBUNDLE", "-fs", "APFS", "-size", "20g", "-volname", "grove-base", "/tmp/base.sparsebundle"}
	if strings.Join(call.args, " ") != strings.Join(want, " ") {
		t.Fatalf("unexpected args\nwant: %v\ngot:  %v", want, call.args)
	}
}

func TestAttachWithShadow_ParsesMountedDevice(t *testing.T) {
	r := &fakeRunner{
		outputs: [][]byte{[]byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>system-entities</key>
  <array>
    <dict>
      <key>dev-entry</key>
      <string>/dev/disk4</string>
    </dict>
    <dict>
      <key>dev-entry</key>
      <string>/dev/disk4s1</string>
      <key>mount-point</key>
      <string>/tmp/ws</string>
    </dict>
  </array>
</dict>
</plist>`)},
	}

	vol, err := AttachWithShadow(r, "/tmp/base.sparsebundle", "/tmp/ws.shadow", "/tmp/ws")
	if err != nil {
		t.Fatalf("AttachWithShadow() error = %v", err)
	}
	if vol.Device != "/dev/disk4s1" {
		t.Fatalf("expected device /dev/disk4s1, got %q", vol.Device)
	}
	if vol.MountPoint != "/tmp/ws" {
		t.Fatalf("expected mount-point /tmp/ws, got %q", vol.MountPoint)
	}

	if len(r.calls) != 1 {
		t.Fatalf("expected 1 command call, got %d", len(r.calls))
	}
	call := r.calls[0]
	want := []string{"attach", "/tmp/base.sparsebundle", "-shadow", "/tmp/ws.shadow", "-mountpoint", "/tmp/ws", "-nobrowse", "-plist"}
	if call.name != "hdiutil" || strings.Join(call.args, " ") != strings.Join(want, " ") {
		t.Fatalf("unexpected call: %s %v", call.name, call.args)
	}
}

func TestAttachWithShadow_MissingDeviceOrMountPoint(t *testing.T) {
	r := &fakeRunner{
		outputs: [][]byte{[]byte(`<plist version="1.0"><dict></dict></plist>`)},
	}
	_, err := AttachWithShadow(r, "/tmp/base.sparsebundle", "/tmp/ws.shadow", "/tmp/ws")
	if err == nil {
		t.Fatal("expected error for missing device/mount-point")
	}
}

func TestDetach_UsesExpectedCommand(t *testing.T) {
	r := &fakeRunner{}
	if err := Detach(r, "/dev/disk4s1"); err != nil {
		t.Fatalf("Detach() error = %v", err)
	}

	if len(r.calls) != 1 {
		t.Fatalf("expected 1 command call, got %d", len(r.calls))
	}
	call := r.calls[0]
	if call.name != "hdiutil" {
		t.Fatalf("expected hdiutil, got %q", call.name)
	}
	want := []string{"detach", "/dev/disk4s1"}
	if strings.Join(call.args, " ") != strings.Join(want, " ") {
		t.Fatalf("unexpected args\nwant: %v\ngot:  %v", want, call.args)
	}
}

func TestSyncBase_UsesExpectedCommand(t *testing.T) {
	r := &fakeRunner{}
	if err := SyncBase(r, "/src", "/dst", nil); err != nil {
		t.Fatalf("SyncBase() error = %v", err)
	}

	if len(r.calls) != 1 {
		t.Fatalf("expected 1 command call, got %d", len(r.calls))
	}
	call := r.calls[0]
	if call.name != "rsync" {
		t.Fatalf("expected rsync, got %q", call.name)
	}
	want := []string{
		"-a",
		"--delete",
		"--exclude", ".grove/images/",
		"--exclude", ".grove/workspaces/",
		"--exclude", ".grove/shadows/",
		"--exclude", ".grove/mnt/",
		"/src/",
		"/dst/",
	}
	if strings.Join(call.args, " ") != strings.Join(want, " ") {
		t.Fatalf("unexpected args\nwant: %v\ngot:  %v", want, call.args)
	}
}

func TestCreateSparseBundle_PropagatesCommandError(t *testing.T) {
	r := &fakeRunner{
		outputs: [][]byte{[]byte("boom")},
		errs:    []error{errors.New("exit 1")},
	}
	err := CreateSparseBundle(r, "/tmp/base.sparsebundle", "grove-base", 20)
	if err == nil {
		t.Fatal("expected command error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected stderr/stdout in error, got %v", err)
	}
}

func TestParseRsyncPercent(t *testing.T) {
	tests := []struct {
		line string
		want int
		ok   bool
	}{
		{"    458,588,160   6%  109.38MB/s    0:01:02", 6, true},
		{"  1,234,567,890  99%   50.00MB/s    0:00:01", 99, true},
		{"              0   0%    0.00kB/s    0:00:00", 0, true},
		{"  1,234,567,890 100%   50.00MB/s    0:00:01 (xfr#1, to-chk=0/100)", 100, true},
		{"sending incremental file list", -1, false},
		{"", -1, false},
	}
	for _, tt := range tests {
		got, ok := parseRsyncPercent(tt.line)
		if ok != tt.ok {
			t.Errorf("parseRsyncPercent(%q) ok = %v, want %v", tt.line, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Errorf("parseRsyncPercent(%q) = %d, want %d", tt.line, got, tt.want)
		}
	}
}

func TestSyncBaseWithProgress_CallsRsyncWithProgressFlags(t *testing.T) {
	r := &fakeRunner{
		streamLines: []string{
			"sending incremental file list",
			"    458,588,160   6%  109.38MB/s    0:01:02",
			"  4,585,881,600  60%  109.38MB/s    0:01:02",
			"  7,643,136,000 100%  109.38MB/s    0:01:02 (xfr#1, to-chk=0/100)",
		},
	}

	var percents []int
	onProgress := func(pct int) {
		percents = append(percents, pct)
	}

	if err := SyncBaseWithProgress(r, "/src", "/dst", nil, onProgress); err != nil {
		t.Fatalf("SyncBaseWithProgress() error = %v", err)
	}

	if len(r.streamCalls) != 1 {
		t.Fatalf("expected 1 stream call, got %d", len(r.streamCalls))
	}
	call := r.streamCalls[0]
	if call.name != "rsync" {
		t.Fatalf("expected rsync, got %q", call.name)
	}
	argsStr := strings.Join(call.args, " ")
	if !strings.Contains(argsStr, "--info=progress2") {
		t.Fatalf("expected --info=progress2 in args, got %v", call.args)
	}
	if !strings.Contains(argsStr, "--no-inc-recursive") {
		t.Fatalf("expected --no-inc-recursive in args, got %v", call.args)
	}

	if len(percents) != 3 {
		t.Fatalf("expected 3 progress callbacks, got %d: %v", len(percents), percents)
	}
	if percents[0] != 6 || percents[1] != 60 || percents[2] != 100 {
		t.Fatalf("unexpected percents: %v", percents)
	}
}

func TestSyncBaseWithProgress_NilRunnerUsesDefault(t *testing.T) {
	// Just verifying it doesn't panic when runner is nil — will fail
	// with a real rsync error since paths don't exist, which is fine.
	_ = SyncBaseWithProgress(nil, "/nonexistent/src", "/nonexistent/dst", nil, nil)
}

func TestSyncBase_WithExcludes(t *testing.T) {
	r := &fakeRunner{}
	if err := SyncBase(r, "/src", "/dst", []string{"node_modules", "*.lock"}); err != nil {
		t.Fatalf("SyncBase() error = %v", err)
	}

	if len(r.calls) != 1 {
		t.Fatalf("expected 1 command call, got %d", len(r.calls))
	}
	call := r.calls[0]
	if call.name != "rsync" {
		t.Fatalf("expected rsync, got %q", call.name)
	}
	want := []string{
		"-a",
		"--delete",
		"--exclude", ".grove/images/",
		"--exclude", ".grove/workspaces/",
		"--exclude", ".grove/shadows/",
		"--exclude", ".grove/mnt/",
		"--exclude", "node_modules",
		"--exclude", "*.lock",
		"/src/",
		"/dst/",
	}
	if strings.Join(call.args, " ") != strings.Join(want, " ") {
		t.Fatalf("unexpected args\nwant: %v\ngot:  %v", want, call.args)
	}
}

func TestSyncBaseWithProgress_WithExcludes(t *testing.T) {
	r := &fakeRunner{
		streamLines: []string{
			"  7,643,136,000 100%  109.38MB/s    0:01:02 (xfr#1, to-chk=0/100)",
		},
	}

	if err := SyncBaseWithProgress(r, "/src", "/dst", []string{"__pycache__"}, nil); err != nil {
		t.Fatalf("SyncBaseWithProgress() error = %v", err)
	}

	if len(r.streamCalls) != 1 {
		t.Fatalf("expected 1 stream call, got %d", len(r.streamCalls))
	}
	argsStr := strings.Join(r.streamCalls[0].args, " ")
	if !strings.Contains(argsStr, "--exclude __pycache__") {
		t.Fatalf("expected user exclude in args, got %v", r.streamCalls[0].args)
	}
}

func TestSyncBase_NilExcludes(t *testing.T) {
	r := &fakeRunner{}
	if err := SyncBase(r, "/src", "/dst", nil); err != nil {
		t.Fatalf("SyncBase() error = %v", err)
	}
	call := r.calls[0]
	want := []string{
		"-a",
		"--delete",
		"--exclude", ".grove/images/",
		"--exclude", ".grove/workspaces/",
		"--exclude", ".grove/shadows/",
		"--exclude", ".grove/mnt/",
		"/src/",
		"/dst/",
	}
	if strings.Join(call.args, " ") != strings.Join(want, " ") {
		t.Fatalf("unexpected args\nwant: %v\ngot:  %v", want, call.args)
	}
}

func TestExecRunner_StreamCallsOnLine(t *testing.T) {
	r := execRunner{}
	var lines []string
	err := r.Stream("echo", []string{"hello"}, func(line string) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if len(lines) == 0 {
		t.Fatal("expected at least one line from echo")
	}
	if lines[0] != "hello" {
		t.Fatalf("expected 'hello', got %q", lines[0])
	}
}
