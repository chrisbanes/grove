package clone

import (
	"fmt"
	"os/exec"
	"runtime"
)

// APFSCloner performs CoW clones using macOS APFS cp -c.
type APFSCloner struct{}

func (c *APFSCloner) Clone(src, dst string) error {
	cmd := exec.Command("cp", "-c", "-R", src, dst)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("apfs clone failed: %w\n%s", err, out)
	}
	return nil
}

// isAPFS checks if the filesystem at path is APFS.
func isAPFS(path string) (bool, error) {
	if runtime.GOOS != "darwin" {
		return false, nil
	}
	// Use df to find the device for the volume containing path,
	// then diskutil to check its filesystem type.
	dfCmd := exec.Command("df", path)
	dfOut, err := dfCmd.Output()
	if err != nil {
		return false, fmt.Errorf("df failed: %w", err)
	}
	// df output: last line, first field is the device
	lines := splitLines(string(dfOut))
	if len(lines) < 2 {
		return false, fmt.Errorf("unexpected df output")
	}
	// Parse the mount point from the last column of the last line
	fields := splitFields(lines[len(lines)-1])
	if len(fields) < 1 {
		return false, fmt.Errorf("unexpected df output format")
	}
	mountPoint := fields[len(fields)-1]

	diskutilCmd := exec.Command("diskutil", "info", mountPoint)
	diskutilOut, err := diskutilCmd.Output()
	if err != nil {
		return false, fmt.Errorf("diskutil failed: %w", err)
	}
	return containsString(string(diskutilOut), "APFS"), nil
}
