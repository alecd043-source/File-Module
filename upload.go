package main

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

type uploadedFile struct {
	Name     string `json:"name"`
	Original string `json:"original"`
	Size     int64  `json:"size"`
	Renamed  bool   `json:"renamed"`
}

type uploadResponse struct {
	Saved []uploadedFile `json:"saved"`
}

func (a *app) handleUpload(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
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

	if a.cfg.maxUpload > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, a.cfg.maxUpload)
	}
	reader, err := r.MultipartReader()
	if err != nil {
		writeAPIError(w, http.StatusUnsupportedMediaType, "Es wurden keine Dateien gefunden.")
		return
	}

	var saved []uploadedFile
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeUploadReadError(w, err)
			return
		}
		if part.FileName() == "" {
			_ = part.Close()
			continue
		}
		name, err := safeFilename(part.FileName())
		if err != nil {
			_ = part.Close()
			writeRequestError(w, err)
			return
		}

		tmp, err := os.CreateTemp(dir, reservedPrefix+"upload-*.part")
		if err != nil {
			_ = part.Close()
			writeAPIError(w, http.StatusInternalServerError, "Temporäre Datei konnte nicht angelegt werden.")
			return
		}
		tmpName := tmp.Name()
		size, copyErr := io.Copy(tmp, part)
		closeErr := tmp.Close()
		_ = part.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(tmpName)
			if copyErr != nil {
				writeUploadReadError(w, copyErr)
			} else {
				writeAPIError(w, http.StatusInternalServerError, "Datei konnte nicht gespeichert werden.")
			}
			return
		}

		finalName, err := a.finishUpload(dir, name, tmpName)
		if err != nil {
			_ = os.Remove(tmpName)
			writeRequestError(w, err)
			return
		}
		saved = append(saved, uploadedFile{
			Name:     finalName,
			Original: name,
			Size:     size,
			Renamed:  finalName != name,
		})
	}

	if len(saved) == 0 {
		writeAPIError(w, http.StatusBadRequest, "Es wurden keine Dateien ausgewählt.")
		return
	}
	writeJSON(w, http.StatusOK, uploadResponse{Saved: saved})
}

func writeUploadReadError(w http.ResponseWriter, err error) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		writeAPIError(w, http.StatusRequestEntityTooLarge, "Die Datei ist größer als erlaubt.")
		return
	}
	writeAPIError(w, http.StatusBadRequest, "Upload wurde abgebrochen oder konnte nicht gelesen werden.")
}

func (a *app) finishUpload(dir, desiredName, tempPath string) (string, error) {
	a.uploadMu.Lock()
	defer a.uploadMu.Unlock()

	finalName, target, err := availableName(dir, desiredName)
	if err != nil {
		return "", err
	}
	if err := os.Chmod(tempPath, 0o640); err != nil {
		return "", err
	}
	if err := os.Rename(tempPath, target); err != nil {
		return "", err
	}
	return finalName, nil
}

func availableName(dir, desired string) (string, string, error) {
	base, ext := splitName(desired)
	for i := 0; i < 10_000; i++ {
		name := desired
		if i > 0 {
			name = base + " (" + strconv.Itoa(i) + ")" + ext
		}
		if len(name) > 255 {
			continue
		}
		target := filepath.Join(dir, name)
		_, err := os.Lstat(target)
		if err == nil {
			continue
		}
		if !os.IsNotExist(err) {
			return "", "", err
		}
		return name, target, nil
	}
	return "", "", newAPIError(http.StatusConflict, "Es konnte kein freier Dateiname gefunden werden.")
}
