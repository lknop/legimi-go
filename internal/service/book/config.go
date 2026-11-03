package book

import (
	"github.com/tp86/legimi-go/internal/api"
	"github.com/tp86/legimi-go/internal/options"
	"github.com/tp86/legimi-go/internal/service"
)

func DefaultService(
	sessionService service.Session,
	apiClient api.Client,
	bookDownloadPresenter service.DownloadPresenter,
	opts options.Download,
) service.Book {
	return defaultBookService{
		sessionService:    sessionService,
		client:            apiClient,
		downloadPresenter: bookDownloadPresenter,
		downloadDir:       opts.GetDownloadDir(),
	}
}
