package main

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

type fileEntry struct {
	Name     string `json:"name"`
	IsDir    bool   `json:"isDir"`
	Size     int64  `json:"size"`
	Modified string `json:"modified,omitempty"`
}

func (a *app) handleList(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	rel, err := cleanRel(r.URL.Query().Get("path"))
	if err != nil {
		writeRequestError(w, err)
		return
	}
	dir, _, err := a.resolveDir(rel)
	if err != nil {
		writeRequestError(w, err)
		return
	}

	f, err := os.Open(dir)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Ordner konnte nicht gelesen werden.")
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	out := bufio.NewWriterSize(w, 32<<10)
	pathJSON, _ := json.Marshal(rel)
	_, _ = out.WriteString(`{"path":`)
	_, _ = out.Write(pathJSON)
	_, _ = out.WriteString(`,"entries":[`)

	first := true
	for {
		entries, readErr := f.ReadDir(256)
		for _, de := range entries {
			if de.Type()&os.ModeSymlink != 0 || isHiddenName(de.Name()) || !utf8.ValidString(de.Name()) {
				continue
			}
			entryInfo, err := de.Info()
			if err != nil {
				continue
			}
			if !entryInfo.IsDir() && !entryInfo.Mode().IsRegular() {
				continue
			}
			entry := fileEntry{
				Name:     de.Name(),
				IsDir:    entryInfo.IsDir(),
				Size:     entryInfo.Size(),
				Modified: entryInfo.ModTime().UTC().Format(time.RFC3339),
			}
			encoded, err := json.Marshal(entry)
			if err != nil {
				continue
			}
			if !first {
				_ = out.WriteByte(',')
			}
			first = false
			_, _ = out.Write(encoded)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			log.Printf("Ordner lesen %q: %v", rel, readErr)
			break
		}
	}
	_, _ = out.WriteString(`]}`)
	if err := out.Flush(); err != nil && !strings.Contains(err.Error(), "broken pipe") {
		log.Printf("Liste senden: %v", err)
	}
}
