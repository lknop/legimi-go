package presenter

import (
	"encoding/json"
	"io"

	"github.com/tp86/legimi-go/internal/model"
)

type jsonBookListPresenter struct {
	w io.Writer
}

type jsonBook struct {
	Id         uint64 `json:"id"`
	Title      string `json:"title"`
	Author     string `json:"author"`
	Downloaded bool   `json:"downloaded"`
}

type jsonBookList struct {
	DownloadsLeft uint32     `json:"downloadsLeft"`
	MaxDownloads  uint32     `json:"maxDownloads"`
	Books         []jsonBook `json:"books"`
}

func (p jsonBookListPresenter) Present(bookList []model.BookMetadata, downloadLimit model.DownloadLimit) {
	list := jsonBookList{
		DownloadsLeft: downloadLimit.Left,
		MaxDownloads:  downloadLimit.Max,
		Books:         make([]jsonBook, len(bookList)),
	}
	for i, book := range bookList {
		list.Books[i] = jsonBook{Id: book.Id, Title: book.Title, Author: book.Author, Downloaded: book.Downloaded}
	}
	encoder := json.NewEncoder(p.w)
	encoder.SetIndent("", "  ")
	encoder.Encode(list)
}
