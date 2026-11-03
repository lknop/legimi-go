package presenter

import (
	"os"

	"github.com/tp86/legimi-go/internal/service"
)

func DefaultBookListPresenter() service.BookListPresenter {
	return defaultBookListPresenter{}
}

func JSONBookListPresenter() service.BookListPresenter {
	return jsonBookListPresenter{os.Stdout}
}

func DefaultBookDownloadPresenter() service.DownloadPresenter {
	return defaultBookDownloadPresenter{}
}
