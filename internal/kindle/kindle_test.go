package kindle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

// fakeSystem sets up fake /sys and /proc/self/mountinfo, returns directory for mount points
func fakeSystem(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	oldBlock, oldClass, oldMountInfo, oldMount := sysBlockDir, sysClassBlock, mountInfoFile, mount
	t.Cleanup(func() { sysBlockDir, sysClassBlock, mountInfoFile, mount = oldBlock, oldClass, oldMountInfo, oldMount })
	sysBlockDir = filepath.Join(root, "sys", "block")
	sysClassBlock = filepath.Join(root, "sys", "class", "block")
	mountInfoFile = filepath.Join(root, "mountinfo")
	mount = func(string) error { return errors.New("mount not expected") }
	writeFile(t, mountInfoFile, "22 1 8:1 / / rw,relatime shared:1 - ext4 /dev/sda1 rw\n")
	addDisk(t, "sda", "ATA", "1000", "sda1")
	mountDir := filepath.Join(root, "media")
	if err := os.MkdirAll(mountDir, 0755); err != nil {
		t.Fatal(err)
	}
	return mountDir
}

func addDisk(t *testing.T, disk, vendor, size string, partitions ...string) {
	t.Helper()
	writeFile(t, filepath.Join(sysBlockDir, disk, "device", "vendor"), vendor+"  \n")
	writeFile(t, filepath.Join(sysClassBlock, disk, "size"), size+"\n")
	for i, partition := range partitions {
		writeFile(t, filepath.Join(sysBlockDir, disk, partition, "partition"), fmt.Sprint(i+1))
		writeFile(t, filepath.Join(sysClassBlock, partition, "size"), size+"\n")
	}
}

func addMount(t *testing.T, device, mountPoint string) {
	t.Helper()
	f, err := os.OpenFile(mountInfoFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	escaped := strings.ReplaceAll(mountPoint, " ", `\040`)
	fmt.Fprintf(f, "99 22 8:17 / %s rw,nosuid shared:2 - vfat /dev/%s rw,uid=1000\n", escaped, device)
	if err := os.MkdirAll(filepath.Join(mountPoint, "documents"), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestFindMountedKindle(t *testing.T) {
	media := fakeSystem(t)
	addDisk(t, "sdb", "Kindle", "6000000")
	mountPoint := filepath.Join(media, "My Kindle")
	addMount(t, "sdb", mountPoint)

	kindle, err := Find("")

	if err != nil {
		t.Fatal(err)
	}
	if kindle.Dir != mountPoint || kindle.Device != "sdb" {
		t.Errorf("found %+v, expected Kindle on sdb mounted in %s", kindle, mountPoint)
	}
	if !kindle.Connected() {
		t.Error("Kindle should be connected")
	}
}

func TestFindMountsKindlePartition(t *testing.T) {
	media := fakeSystem(t)
	addDisk(t, "sdc", "Kindle", "6000000", "sdc1")
	mountPoint := filepath.Join(media, "Kindle")
	var mounted string
	mount = func(device string) error {
		mounted = device
		addMount(t, "sdc1", mountPoint)
		return nil
	}

	kindle, err := Find("")

	if err != nil {
		t.Fatal(err)
	}
	if mounted != "/dev/sdc1" {
		t.Errorf("mounted %q, expected /dev/sdc1", mounted)
	}
	if kindle.Dir != mountPoint || kindle.Device != "sdc1" {
		t.Errorf("found %+v, expected Kindle on sdc1 mounted in %s", kindle, mountPoint)
	}
}

func TestFindFailsWithoutKindle(t *testing.T) {
	fakeSystem(t)

	_, err := Find("")

	if err == nil || !strings.Contains(err.Error(), "no Kindle found") {
		t.Errorf("expected 'no Kindle found' error, got %v", err)
	}
}

func TestFindFailsWhenKindleHasNoStorage(t *testing.T) {
	fakeSystem(t)
	addDisk(t, "sdb", "Kindle", "0")

	_, err := Find("")

	if err == nil || !strings.Contains(err.Error(), "no storage") {
		t.Errorf("expected 'no storage' error, got %v", err)
	}
}

func TestFindGivenDir(t *testing.T) {
	media := fakeSystem(t)
	addDisk(t, "sdb", "Kindle", "6000000")
	mountPoint := filepath.Join(media, "Kindle")
	addMount(t, "sdb", mountPoint)

	kindle, err := Find(mountPoint)

	if err != nil {
		t.Fatal(err)
	}
	if kindle.Dir != mountPoint || kindle.Device != "sdb" {
		t.Errorf("found %+v, expected Kindle on sdb mounted in %s", kindle, mountPoint)
	}
}

func TestFindGivenDirWithoutDocuments(t *testing.T) {
	fakeSystem(t)

	_, err := Find(t.TempDir())

	if err == nil || !strings.Contains(err.Error(), "doesn't look like a Kindle") {
		t.Errorf("expected 'doesn't look like a Kindle' error, got %v", err)
	}
}

func TestConnectedDetectsKindleLeavingUSBDriveMode(t *testing.T) {
	media := fakeSystem(t)
	addDisk(t, "sdb", "Kindle", "6000000")
	addMount(t, "sdb", filepath.Join(media, "Kindle"))
	kindle, err := Find("")
	if err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(sysClassBlock, "sdb", "size"), "0\n")

	if kindle.Connected() {
		t.Error("Kindle with no sectors should not be connected")
	}
}

func newKindle(t *testing.T) Kindle {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "documents"), 0755); err != nil {
		t.Fatal(err)
	}
	return Kindle{Dir: dir}
}

func TestBookIds(t *testing.T) {
	kindle := newKindle(t)
	for _, name := range []string{"123.azw", "123.mobi", "456.mobi", "789.azw.part", "notes.txt", "12a.azw", "123.sdr/x"} {
		writeFile(t, filepath.Join(kindle.documents(), name), "x")
	}

	ids, err := kindle.BookIds()

	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(ids) != "[123 456]" {
		t.Errorf("got ids %v, expected [123 456]", ids)
	}
}

func TestInstall(t *testing.T) {
	kindle := newKindle(t)
	book := filepath.Join(t.TempDir(), "123.mobi")
	writeFile(t, book, "new book")
	writeFile(t, filepath.Join(kindle.documents(), "123.azw"), "old book")
	writeFile(t, filepath.Join(kindle.documents(), "123.mobi"), "old copy")

	changed, err := kindle.Install(book, 123)

	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("expected book to be changed")
	}
	if content := readFile(t, filepath.Join(kindle.documents(), "123.azw")); content != "new book" {
		t.Errorf("123.azw contains %q", content)
	}
	for _, name := range []string{"123.mobi", "123.azw.part"} {
		if _, err := os.Stat(filepath.Join(kindle.documents(), name)); !os.IsNotExist(err) {
			t.Errorf("%s should not exist", name)
		}
	}
}

func TestInstallSameBook(t *testing.T) {
	kindle := newKindle(t)
	book := filepath.Join(t.TempDir(), "123.mobi")
	writeFile(t, book, "book")
	writeFile(t, filepath.Join(kindle.documents(), "123.azw"), "book")

	changed, err := kindle.Install(book, 123)

	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("same book should not be changed")
	}
}
