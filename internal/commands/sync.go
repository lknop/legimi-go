package commands

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/tp86/legimi-go/internal/kindle"
	"github.com/tp86/legimi-go/internal/model"
	"github.com/tp86/legimi-go/internal/usecase"
)

var (
	BookFileDownloader usecase.BookFileDownloader
	// variable, so that tests can use fake Kindle
	FindKindle = kindle.Find
)

func syncBooks() error {
	return runSync(flag.Args()[1:])
}

var errKindleDisconnected = errors.New("the Kindle left USB-drive mode before all books were saved: " +
	"unplug it, plug it in again and run sync again, so that incomplete books are sent again")

func runSync(args []string) error {
	flags := flag.NewFlagSet("sync", flag.ContinueOnError)
	refresh := flags.Bool("refresh", false, "also download again books already on Kindle, which renews their loan")
	if err := flags.Parse(args); err != nil {
		return err
	}
	ids, err := parseBookIds(flags.Args())
	if err != nil {
		return err
	}
	if len(ids) == 0 && !*refresh {
		return fmt.Errorf("no book id provided (or use -refresh)")
	}

	k, err := FindKindle(Options.kindleDir)
	if err != nil {
		return err
	}
	fmt.Printf("Kindle: %s\n", k.Dir)

	bookList, err := BookLister.ListBooks()
	if err != nil {
		return err
	}
	shelf := make(map[uint64]model.BookMetadata, len(bookList))
	for _, book := range bookList {
		shelf[book.Id] = book
	}
	for _, id := range ids {
		if _, ok := shelf[id]; !ok {
			return fmt.Errorf("book %d isn't on your Legimi shelf", id)
		}
	}
	if *refresh {
		onKindle, err := k.BookIds()
		if err != nil {
			return err
		}
		for _, id := range onKindle {
			book, ok := shelf[id]
			switch {
			case !ok:
				fmt.Printf("Note: book %d isn't on your shelf any more (loan ended?), leaving it as it is.\n", id)
			case !book.Downloaded:
				fmt.Printf("Note: Legimi doesn't list book %d as sent to this Kindle, not refreshing it, so that no download is used.\n", id)
			default:
				ids = append(ids, id)
			}
		}
	}
	ids = unique(ids)

	newBooks := 0
	for _, id := range ids {
		if !shelf[id].Downloaded {
			newBooks++
		}
	}
	downloadLimit, err := DownloadLimitGetter.GetDownloadLimit()
	if err != nil {
		return err
	}
	if newBooks > int(downloadLimit.Left) {
		return fmt.Errorf("%d of these books are new, but only %d downloads are left", newBooks, downloadLimit.Left)
	}
	if len(ids) == 0 {
		fmt.Println("Nothing to download.")
		return nil
	}

	tempDir, err := os.MkdirTemp("", "legimi-go-sync")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	errs := make([]error, 0)
	copied := 0
	for _, id := range ids {
		bookFile, err := BookFileDownloader.DownloadBookTo(id, tempDir)
		if err != nil {
			errs = append(errs, fmt.Errorf("book %d not downloaded: %w", id, err))
			continue
		}
		changed, err := k.Install(bookFile, id)
		os.Remove(bookFile)
		if !k.Connected() {
			return errors.Join(append(errs, errKindleDisconnected)...)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("book %d not copied to Kindle: %w", id, err))
			continue
		}
		if changed {
			fmt.Printf("  copied %d.azw  %s\n", id, shelf[id].Title)
			copied++
		} else {
			fmt.Printf("  %d.azw is up to date\n", id)
		}
	}
	fmt.Printf("Copied %d book(s) to the Kindle. Eject it before unplugging.\n", copied)
	return errors.Join(errs...)
}

func unique(ids []uint64) []uint64 {
	seen := make(map[uint64]bool, len(ids))
	result := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}
