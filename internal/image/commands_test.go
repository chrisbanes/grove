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

type fakeRunner struct {
	calls   []runnerCall
	outputs [][]byte
	errs    []error
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
	if err := SyncBase(r, "/src", "/dst"); err != nil {
		t.Fatalf("SyncBase() error = %v", err)
	}

	if len(r.calls) != 1 {
		t.Fatalf("expected 1 command call, got %d", len(r.calls))
	}
	call := r.calls[0]
	if call.name != "rsync" {
		t.Fatalf("expected rsync, got %q", call.name)
	}
	want := []string{"-a", "--delete", "--exclude", ".grove/", "/src/", "/dst/"}
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

