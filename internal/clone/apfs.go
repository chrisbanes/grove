package clone

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

func (c *APFSCloner) CloneWithProgress(src, dst string, onProgress ProgressFunc) error {
	total, err := countEntries(src)
	if err != nil {
		total = 0
	}
	if onProgress != nil {
		onProgress(ProgressEvent{Total: total, Phase: "scan"})
	}

	cmd := exec.Command("cp", "-c", "-R", "-v", src, dst)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("apfs clone failed: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("apfs clone failed: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("apfs clone failed: %w", err)
	}

	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		copied int

		stdoutBuf bytes.Buffer
		stderrBuf bytes.Buffer
	)

	scan := func(r io.Reader, out *bytes.Buffer) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			out.WriteString(line)
			out.WriteByte('\n')
			if !parseCPVerboseLine(line) {
				continue
			}
			mu.Lock()
			copied++
			current := copied
			mu.Unlock()
			if onProgress != nil {
				onProgress(ProgressEvent{Copied: current, Total: total, Phase: "clone"})
			}
		}
	}

	wg.Add(2)
	go scan(stdout, &stdoutBuf)
	go scan(stderr, &stderrBuf)

	waitErr := cmd.Wait()
	wg.Wait()

	if waitErr != nil {
		return fmt.Errorf("apfs clone failed: %w\n%s%s", waitErr, stdoutBuf.String(), stderrBuf.String())
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

func parseCPVerboseLine(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}
	return strings.Contains(line, " -> ")
}

func mapClonePercent(copied, total, min, max int) int {
	if max < min {
		min, max = max, min
	}
	if total <= 0 {
		return min
	}
	if copied < 0 {
		copied = 0
	}
	if copied > total {
		copied = total
	}
	span := max - min
	return min + (copied*span)/total
}

func countEntries(root string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(_ string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
}
