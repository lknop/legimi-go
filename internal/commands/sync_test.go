package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tp86/legimi-go/internal/kindle"
	"github.com/tp86/legimi-go/internal/model"
)

type fakeShelf struct {
	books         []model.BookMetadata
	downloadsLeft uint32
}

func (s fakeShelf) ListBooks() ([]model.BookMetadata, error) {
	return s.books, nil
}

func (s fakeShelf) GetDownloadLimit() (model.DownloadLimit, error) {
	return model.DownloadLimit{Left: s.downloadsLeft, Max: 10}, nil
}

type fakeDownloader struct {
	downloaded []uint64
	failing    map[uint64]bool
}

func (d *fakeDownloader) DownloadBookTo(id uint64, dir string) (string, error) {
	if d.failing[id] {
		return "", errors.New("download failed")
	}
	d.downloaded = append(d.downloaded, id)
	path := filepath.Join(dir, fmt.Sprintf("%d.mobi", id))
	return path, os.WriteFile(path, []byte(fmt.Sprintf("book %d", id)), 0644)
}

var testShelf = fakeShelf{
	books: []model.BookMetadata{
		{Id: 1, Title: "Sent before", Downloaded: true},
		{Id: 2, Title: "New", Downloaded: false},
		{Id: 3, Title: "Also sent before", Downloaded: true},
		{Id: 4, Title: "Another new", Downloaded: false},
	},
	downloadsLeft: 1,
}

// setUpSync sets fake services and Kindle with given files in documents folder
func setUpSync(t *testing.T, shelf fakeShelf, files ...string) (*fakeDownloader, string) {
	t.Helper()
	kindleDir := t.TempDir()
	documents := filepath.Join(kindleDir, "documents")
	if err := os.MkdirAll(documents, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(documents, name), []byte("old"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	downloader := &fakeDownloader{}
	oldLister, oldLimit, oldDownloader, oldFind := BookLister, DownloadLimitGetter, BookFileDownloader, FindKindle
	t.Cleanup(func() {
		BookLister, DownloadLimitGetter, BookFileDownloader, FindKindle = oldLister, oldLimit, oldDownloader, oldFind
	})
	BookLister, DownloadLimitGetter, BookFileDownloader = shelf, shelf, downloader
	FindKindle = func(string) (kindle.Kindle, error) { return kindle.Kindle{Dir: kindleDir}, nil }
	return downloader, documents
}

func assertBook(t *testing.T, documents string, id uint64) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(documents, fmt.Sprintf("%d.azw", id)))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != fmt.Sprintf("book %d", id) {
		t.Errorf("%d.azw contains %q", id, content)
	}
}

func TestSyncGivenBooks(t *testing.T) {
	downloader, documents := setUpSync(t, testShelf, "1.mobi")

	err := runSync([]string{"1", "2", "1"})

	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(downloader.downloaded) != "[1 2]" {
		t.Errorf("downloaded %v, expected [1 2]", downloader.downloaded)
	}
	assertBook(t, documents, 1)
	assertBook(t, documents, 2)
	if _, err := os.Stat(filepath.Join(documents, "1.mobi")); !os.IsNotExist(err) {
		t.Error("old 1.mobi should be removed")
	}
}

func TestSyncRefresh(t *testing.T) {
	// 2 is on Kindle, but not listed as sent, 5 is not on shelf
	downloader, documents := setUpSync(t, testShelf, "1.azw", "2.azw", "3.mobi", "5.azw", "notes.txt")

	err := runSync([]string{"-refresh", "4"})

	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(downloader.downloaded) != "[4 1 3]" {
		t.Errorf("downloaded %v, expected [4 1 3]", downloader.downloaded)
	}
	assertBook(t, documents, 1)
	assertBook(t, documents, 3)
	assertBook(t, documents, 4)
	content, _ := os.ReadFile(filepath.Join(documents, "2.azw"))
	if string(content) != "old" {
		t.Error("2.azw should not be changed")
	}
}

func TestSyncFailsWhenNotEnoughDownloads(t *testing.T) {
	downloader, _ := setUpSync(t, testShelf)

	err := runSync([]string{"2", "4"})

	if err == nil || !strings.Contains(err.Error(), "only 1 downloads are left") {
		t.Errorf("expected error about downloads left, got %v", err)
	}
	if len(downloader.downloaded) != 0 {
		t.Errorf("nothing should be downloaded, downloaded %v", downloader.downloaded)
	}
}

func TestSyncFailsForBookNotOnShelf(t *testing.T) {
	downloader, _ := setUpSync(t, testShelf)

	err := runSync([]string{"1", "99"})

	if err == nil || !strings.Contains(err.Error(), "book 99 isn't on your Legimi shelf") {
		t.Errorf("expected error about book not on shelf, got %v", err)
	}
	if len(downloader.downloaded) != 0 {
		t.Errorf("nothing should be downloaded, downloaded %v", downloader.downloaded)
	}
}

func TestSyncContinuesAfterFailedDownload(t *testing.T) {
	downloader, documents := setUpSync(t, testShelf)
	downloader.failing = map[uint64]bool{1: true}

	err := runSync([]string{"1", "3"})

	if err == nil || !strings.Contains(err.Error(), "book 1 not downloaded") {
		t.Errorf("expected error about book 1, got %v", err)
	}
	assertBook(t, documents, 3)
	if _, err := os.Stat(filepath.Join(documents, "1.azw")); !os.IsNotExist(err) {
		t.Error("1.azw should not exist")
	}
}

func TestSyncWithoutBooks(t *testing.T) {
	setUpSync(t, testShelf)

	err := runSync(nil)

	if err == nil || !strings.Contains(err.Error(), "no book id provided") {
		t.Errorf("expected error about no book id, got %v", err)
	}
}

func TestSyncFailsWhenKindleDisconnects(t *testing.T) {
	downloader, documents := setUpSync(t, testShelf)
	kindleDir := filepath.Dir(documents)
	// Kindle disappears while first book is downloaded
	downloader.failing = nil
	BookFileDownloader = disconnectingDownloader{downloader, kindleDir}

	err := runSync([]string{"1", "3"})

	if err == nil || !strings.Contains(err.Error(), "left USB-drive mode") {
		t.Errorf("expected error about Kindle leaving USB-drive mode, got %v", err)
	}
	if fmt.Sprint(downloader.downloaded) != "[1]" {
		t.Errorf("downloaded %v, expected to stop after [1]", downloader.downloaded)
	}
}

type disconnectingDownloader struct {
	*fakeDownloader
	kindleDir string
}

func (d disconnectingDownloader) DownloadBookTo(id uint64, dir string) (string, error) {
	os.RemoveAll(d.kindleDir)
	return d.fakeDownloader.DownloadBookTo(id, dir)
}
