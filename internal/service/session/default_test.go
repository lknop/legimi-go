package session

import (
	"testing"

	"github.com/tp86/legimi-go/internal/api/protocol"
	"github.com/tp86/legimi-go/internal/model"
)

type fakeAccountService struct{}

func (fakeAccountService) GetCredentials() (string, string) { return "login", "password" }
func (fakeAccountService) GetKindleId() uint64              { return 1 }

type countingClient struct {
	exchanges int
}

func (c *countingClient) Exchange(request protocol.Request, response protocol.Response) error {
	c.exchanges++
	response.(*model.Session).Id = "session-id"
	return nil
}

func TestDownloadLimitReusesLastSession(t *testing.T) {
	client := &countingClient{}
	ss := DefaultService(fakeAccountService{}, client)

	if _, err := ss.GetSession(); err != nil {
		t.Fatal(err)
	}
	if _, err := ss.GetDownloadLimit(); err != nil {
		t.Fatal(err)
	}

	if client.exchanges != 1 {
		t.Errorf("expected 1 exchange with server, got %d", client.exchanges)
	}
}

func TestDownloadLimitGetsSession(t *testing.T) {
	client := &countingClient{}
	ss := DefaultService(fakeAccountService{}, client)

	if _, err := ss.GetDownloadLimit(); err != nil {
		t.Fatal(err)
	}

	if client.exchanges != 1 {
		t.Errorf("expected 1 exchange with server, got %d", client.exchanges)
	}
}
