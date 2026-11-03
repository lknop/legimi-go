package session

import (
	"github.com/tp86/legimi-go/internal/api"
	"github.com/tp86/legimi-go/internal/model"
	"github.com/tp86/legimi-go/internal/service"
)

type defaultSessionService struct {
	accountService service.Account
	client         api.Client
	// last session received, reused for download limit
	last *model.Session
}

func (ss defaultSessionService) GetSession() (string, error) {
	login, password := ss.accountService.GetCredentials()
	kindleId := ss.accountService.GetKindleId()
	var session model.Session
	err := ss.client.Exchange(model.NewGetSessionRequest(login, password, kindleId), &session)
	if err != nil {
		return "", err
	}
	*ss.last = session
	return session.Id, nil
}

func (ss defaultSessionService) GetDownloadLimit() (model.DownloadLimit, error) {
	if ss.last.Id == "" {
		if _, err := ss.GetSession(); err != nil {
			return model.DownloadLimit{}, err
		}
	}
	return ss.last.DownloadLimit(), nil
}
