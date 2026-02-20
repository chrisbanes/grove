package image

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// Runner executes external commands.
type Runner interface {
	CombinedOutput(name string, args ...string) ([]byte, error)
	Stream(name string, args []string, onLine func(string)) error
}

type execRunner struct{}

func (execRunner) CombinedOutput(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	return cmd.CombinedOutput()
}

func (execRunner) Stream(name string, args []string, onLine func(string)) error {
	cmd := exec.Command(name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Split(scanCRLF)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			onLine(line)
		}
	}
	return cmd.Wait()
}

// scanCRLF is a bufio.SplitFunc that splits on both \r and \n.
// This is needed for rsync --info=progress2 which uses \r to overwrite lines.
func scanCRLF(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// AttachedVolume describes an attached disk image volume.
type AttachedVolume struct {
	Device     string
	MountPoint string
}

var (
	dictPattern         = regexp.MustCompile(`(?s)<dict>(.*?)</dict>`)
	keyStringPattern    = regexp.MustCompile(`(?s)<key>\s*([^<]+)\s*</key>\s*<string>\s*([^<]+)\s*</string>`)
	rsyncPercentPattern = regexp.MustCompile(`\s+(\d+)%`)
)

func parseRsyncPercent(line string) (int, bool) {
	m := rsyncPercentPattern.FindStringSubmatch(line)
	if m == nil {
		return -1, false
	}
	var pct int
	fmt.Sscanf(m[1], "%d", &pct)
	return pct, true
}

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

func SyncBaseWithProgress(r Runner, src, dst string, excludes []string, onPercent func(int)) error {
	if r == nil {
		r = execRunner{}
	}
	src = ensureTrailingSlash(src)
	dst = ensureTrailingSlash(dst)
	args := []string{
		"-a",
		"--delete",
		"--info=progress2",
		"--no-inc-recursive",
		"--exclude", ".grove/images/",
		"--exclude", ".grove/workspaces/",
		"--exclude", ".grove/shadows/",
		"--exclude", ".grove/mnt/",
	}
	for _, pattern := range excludes {
		args = append(args, "--exclude", pattern)
	}
	args = append(args, src, dst)
	return r.Stream("rsync", args, func(line string) {
		if onPercent == nil {
			return
		}
		if pct, ok := parseRsyncPercent(line); ok {
			onPercent(pct)
		}
	})
}

func SyncBase(r Runner, src, dst string, excludes []string) error {
	if r == nil {
		r = execRunner{}
	}
	src = ensureTrailingSlash(src)
	dst = ensureTrailingSlash(dst)
	args := []string{
		"-a",
		"--delete",
		"--exclude", ".grove/images/",
		"--exclude", ".grove/workspaces/",
		"--exclude", ".grove/shadows/",
		"--exclude", ".grove/mnt/",
	}
	for _, pattern := range excludes {
		args = append(args, "--exclude", pattern)
	}
	args = append(args, src, dst)
	return run(r, "rsync", args...)
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
