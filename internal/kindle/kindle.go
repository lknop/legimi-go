// Package kindle finds a Kindle connected by USB (in USB-drive mode) and puts books on it.
// Finding and mounting the Kindle works on Linux only, elsewhere its folder must be given.
package kindle

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Kindle struct {
	// Dir is where Kindle is mounted, books are in its documents folder
	Dir string
	// Device is block device name (e.g. "sdb1"), empty if not known
	Device string
}

// variables, so that tests can use fake system
var (
	sysBlockDir   = "/sys/block"
	sysClassBlock = "/sys/class/block"
	mountInfoFile = "/proc/self/mountinfo"
	mount         = func(device string) error {
		output, err := exec.Command("udisksctl", "mount", "--no-user-interaction", "-b", device).CombinedOutput()
		if err != nil {
			return fmt.Errorf("udisksctl couldn't mount %s: %v: %s", device, err, strings.TrimSpace(string(output)))
		}
		return nil
	}
)

// Find returns Kindle mounted in dir or, if dir is empty, finds connected Kindle and mounts it if needed.
func Find(dir string) (Kindle, error) {
	var kindle Kindle
	if dir != "" {
		kindle = Kindle{Dir: dir}
		if source, ok := mountSourceOf(dir); ok {
			kindle.Device = source
		}
	} else {
		device, err := findDevice()
		if err != nil {
			return kindle, err
		}
		kindle.Device = device
		mountPoint, ok := mountPointOf(device)
		if !ok {
			fmt.Printf("Mounting /dev/%s ...\n", device)
			if err := mount("/dev/" + device); err != nil {
				return kindle, fmt.Errorf("%v\nmount the Kindle yourself and give its folder with -kindle", err)
			}
			if mountPoint, ok = mountPointOf(device); !ok {
				return kindle, fmt.Errorf("couldn't find where /dev/%s was mounted, give Kindle's folder with -kindle", device)
			}
		}
		kindle.Dir = mountPoint
	}
	info, err := os.Stat(kindle.documents())
	if err != nil || !info.IsDir() {
		return kindle, fmt.Errorf("%s doesn't look like a Kindle (no documents folder)", kindle.Dir)
	}
	return kindle, nil
}

// findDevice returns name of block device with Kindle's file system: whole disk or its first partition.
func findDevice() (string, error) {
	vendors, _ := filepath.Glob(filepath.Join(sysBlockDir, "*", "device", "vendor"))
	for _, vendorFile := range vendors {
		vendor, err := os.ReadFile(vendorFile)
		if err != nil || !strings.EqualFold(strings.TrimSpace(string(vendor)), "kindle") {
			continue
		}
		diskDir := filepath.Dir(filepath.Dir(vendorFile))
		disk := filepath.Base(diskDir)
		if !hasSectors(disk) {
			return "", errors.New("the Kindle is connected but has no storage: unplug it and plug it in again")
		}
		partitions, _ := filepath.Glob(filepath.Join(diskDir, disk+"*", "partition"))
		if len(partitions) > 0 {
			return filepath.Base(filepath.Dir(partitions[0])), nil
		}
		return disk, nil
	}
	return "", errors.New("no Kindle found: connect it with a data cable and unlock it, or give its folder with -kindle")
}

func hasSectors(device string) bool {
	size, err := os.ReadFile(filepath.Join(sysClassBlock, device, "size"))
	return err == nil && strings.TrimSpace(string(size)) != "0"
}

type mountEntry struct {
	mountPoint string
	source     string
}

func mountEntries() []mountEntry {
	content, err := os.ReadFile(mountInfoFile)
	if err != nil {
		return nil
	}
	var entries []mountEntry
	for _, line := range strings.Split(string(content), "\n") {
		// see proc(5): ID parentID major:minor root mountPoint options [optional...] - type source superOptions
		fields := strings.Fields(line)
		separator := -1
		for i, field := range fields {
			if field == "-" {
				separator = i
				break
			}
		}
		if separator < 5 || len(fields) < separator+3 {
			continue
		}
		entries = append(entries, mountEntry{mountPoint: unescapeMountInfo(fields[4]), source: fields[separator+2]})
	}
	return entries
}

var octalEscape = regexp.MustCompile(`\\[0-7]{3}`)

func unescapeMountInfo(s string) string {
	return octalEscape.ReplaceAllStringFunc(s, func(escape string) string {
		value, _ := strconv.ParseUint(escape[1:], 8, 8)
		return string(rune(value))
	})
}

func mountPointOf(device string) (string, bool) {
	for _, entry := range mountEntries() {
		if entry.source == "/dev/"+device {
			return entry.mountPoint, true
		}
	}
	return "", false
}

func mountSourceOf(dir string) (string, bool) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	var found mountEntry
	for _, entry := range mountEntries() {
		// last matching entry wins, it is mounted on top of earlier ones
		if entry.mountPoint == dir {
			found = entry
		}
	}
	if !strings.HasPrefix(found.source, "/dev/") {
		return "", false
	}
	return strings.TrimPrefix(found.source, "/dev/"), true
}

func (k Kindle) documents() string {
	return filepath.Join(k.Dir, "documents")
}

// Connected tells if Kindle is still there. Kindle that leaves USB-drive mode stays on USB,
// but its disk has no sectors then, and writes not yet saved are lost.
func (k Kindle) Connected() bool {
	if _, err := os.Stat(k.documents()); err != nil {
		return false
	}
	return k.Device == "" || hasSectors(k.Device)
}

var bookFileName = regexp.MustCompile(`^([0-9]+)\.(azw|mobi)$`)

// BookIds returns ids of Legimi books on Kindle, saved as <id>.azw or <id>.mobi.
func (k Kindle) BookIds() ([]uint64, error) {
	entries, err := os.ReadDir(k.documents())
	if err != nil {
		return nil, err
	}
	var ids []uint64
	seen := make(map[uint64]bool)
	for _, entry := range entries {
		match := bookFileName.FindStringSubmatch(entry.Name())
		if match == nil || !entry.Type().IsRegular() {
			continue
		}
		id, err := strconv.ParseUint(match[1], 10, 64)
		if err != nil || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
}

// Install copies book file to Kindle as documents/<id>.azw, the name official Legimi app uses,
// so that new copy replaces old one and reading position is kept. Book is written under
// temporary name and synced to disk before renaming, so that Kindle never has half-written
// book under real name. Returns false if the same book is already there.
func (k Kindle) Install(bookFile string, id uint64) (bool, error) {
	destination := filepath.Join(k.documents(), fmt.Sprintf("%d.azw", id))
	same, err := sameContent(bookFile, destination)
	if err != nil {
		return false, err
	}
	if !same {
		partFile := destination + ".part"
		if err := copyAndSync(bookFile, partFile); err != nil {
			os.Remove(partFile)
			return false, err
		}
		if err := os.Rename(partFile, destination); err != nil {
			os.Remove(partFile)
			return false, err
		}
	}
	// older copy under name used by legimi-go download
	if err := os.Remove(filepath.Join(k.documents(), fmt.Sprintf("%d.mobi", id))); err != nil && !os.IsNotExist(err) {
		return false, err
	}
	return !same, syncDir(k.documents())
}

func sameContent(a, b string) (bool, error) {
	contentB, err := os.ReadFile(b)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	contentA, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	return bytes.Equal(contentA, contentB), nil
}

func copyAndSync(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(destination)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	return err
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
