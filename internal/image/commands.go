package image

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// Runner executes external commands.
type Runner interface {
	CombinedOutput(name string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	return cmd.CombinedOutput()
}

// AttachedVolume describes an attached disk image volume.
type AttachedVolume struct {
	Device     string
	MountPoint string
}

var (
	dictPattern      = regexp.MustCompile(`(?s)<dict>(.*?)</dict>`)
	keyStringPattern = regexp.MustCompile(`(?s)<key>\s*([^<]+)\s*</key>\s*<string>\s*([^<]+)\s*</string>`)
)

func CreateSparseBundle(r Runner, path, volName string, sizeGB int) error {
	if r == nil {
		r = execRunner{}
	}
	args := []string{
		"create",
		"-type", "SPARSEBUNDLE",
		"-fs", "APFS",
		"-size", fmt.Sprintf("%dg", sizeGB),
		"-volname", volName,
		path,
	}
	return run(r, "hdiutil", args...)
}

func AttachWithShadow(r Runner, basePath, shadowPath, mountPoint string) (*AttachedVolume, error) {
	if r == nil {
		r = execRunner{}
	}
	args := []string{
		"attach",
		basePath,
		"-shadow", shadowPath,
		"-mountpoint", mountPoint,
		"-nobrowse",
		"-plist",
	}
	out, err := r.CombinedOutput("hdiutil", args...)
	if err != nil {
		return nil, fmt.Errorf("hdiutil attach failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	vol, err := parseAttachedVolume(out)
	if err != nil {
		return nil, fmt.Errorf("parse attach output: %w", err)
	}
	return vol, nil
}

func Attach(r Runner, basePath, mountPoint string) (*AttachedVolume, error) {
	if r == nil {
		r = execRunner{}
	}
	args := []string{
		"attach",
		basePath,
		"-mountpoint", mountPoint,
		"-nobrowse",
		"-plist",
	}
	out, err := r.CombinedOutput("hdiutil", args...)
	if err != nil {
		return nil, fmt.Errorf("hdiutil attach failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	vol, err := parseAttachedVolume(out)
	if err != nil {
		return nil, fmt.Errorf("parse attach output: %w", err)
	}
	return vol, nil
}

func Detach(r Runner, device string) error {
	if r == nil {
		r = execRunner{}
	}
	return run(r, "hdiutil", "detach", device)
}

func SyncBase(r Runner, src, dst string) error {
	if r == nil {
		r = execRunner{}
	}
	src = ensureTrailingSlash(src)
	dst = ensureTrailingSlash(dst)
	return run(r, "rsync", "-a", "--delete", "--exclude", ".grove/", src, dst)
}

func run(r Runner, name string, args ...string) error {
	out, err := r.CombinedOutput(name, args...)
	if err != nil {
		return fmt.Errorf("%s %s failed: %w\n%s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func ensureTrailingSlash(path string) string {
	if strings.HasSuffix(path, "/") {
		return path
	}
	return path + "/"
}

func parseAttachedVolume(plist []byte) (*AttachedVolume, error) {
	dicts := dictPattern.FindAllSubmatch(plist, -1)
	for _, dictMatch := range dicts {
		kv := map[string]string{}
		for _, pair := range keyStringPattern.FindAllSubmatch(dictMatch[1], -1) {
			kv[string(pair[1])] = string(pair[2])
		}
		dev := kv["dev-entry"]
		mount := kv["mount-point"]
		if dev != "" && mount != "" {
			return &AttachedVolume{
				Device:     strings.TrimSpace(dev),
				MountPoint: strings.TrimSpace(mount),
			}, nil
		}
	}
	return nil, fmt.Errorf("missing dev-entry or mount-point in attach plist")
}
