package presenter

import (
	"bytes"
	"testing"

	"github.com/tp86/legimi-go/internal/model"
)

func TestJSONBookListPresenter(t *testing.T) {
	var buf bytes.Buffer
	books := []model.BookMetadata{
		{Id: 123, Title: `Title with "quotes"`, Author: "Author", Version: 5, Downloaded: true, NextPage: "x"},
		{Id: 456, Title: "Other", Author: "Someone"},
	}

	jsonBookListPresenter{&buf}.Present(books, model.DownloadLimit{Left: 7, Max: 10})

	expected := `{
  "downloadsLeft": 7,
  "maxDownloads": 10,
  "books": [
    {
      "id": 123,
      "title": "Title with \"quotes\"",
      "author": "Author",
      "downloaded": true
    },
    {
      "id": 456,
      "title": "Other",
      "author": "Someone",
      "downloaded": false
    }
  ]
}
`
	if buf.String() != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", buf.String(), expected)
	}
}

func TestJSONBookListPresenterEmptyList(t *testing.T) {
	var buf bytes.Buffer

	jsonBookListPresenter{&buf}.Present(nil, model.DownloadLimit{})

	expected := "{\n  \"downloadsLeft\": 0,\n  \"maxDownloads\": 0,\n  \"books\": []\n}\n"
	if buf.String() != expected {
		t.Errorf("got:\n%s\nexpected:\n%s", buf.String(), expected)
	}
}
