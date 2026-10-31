package main

import (
	"archive/zip"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func (a *app) handleDownload(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	rel, err := cleanRel(r.URL.Query().Get("path"))
	if err != nil {
		writeRequestError(w, err)
		return
	}
	realPath, info, err := a.resolveExisting(rel)
	if err != nil {
		writeRequestError(w, err)
		return
	}
	if info.IsDir() {
		a.downloadZip(w, r, realPath, rel)
		return
	}
	if !info.Mode().IsRegular() {
		writeAPIError(w, http.StatusBadRequest, "Das ist keine Datei.")
		return
	}

	file, err := os.Open(realPath)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Datei konnte nicht geöffnet werden.")
		return
	}
	defer file.Close()

	name := path.Base(rel)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", contentDisposition("attachment", name))
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, name, info.ModTime(), file)
}

func (a *app) handleView(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	rel, err := cleanRel(r.URL.Query().Get("path"))
	if err != nil || rel == "" {
		writeAPIError(w, http.StatusBadRequest, "Ungültiger Pfad.")
		return
	}
	realPath, info, err := a.resolveExisting(rel)
	if err != nil {
		writeRequestError(w, err)
		return
	}
	if !info.Mode().IsRegular() {
		writeAPIError(w, http.StatusBadRequest, "Das ist keine Datei.")
		return
	}
	name := path.Base(rel)
	contentType, ok := previewContentType(name)
	if !ok {
		writeAPIError(w, http.StatusUnsupportedMediaType, "Für diesen Dateityp gibt es keine Vorschau.")
		return
	}
	file, err := os.Open(realPath)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Datei konnte nicht geöffnet werden.")
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", contentDisposition("inline", name))
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'self'")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeContent(w, r, name, info.ModTime(), file)
}

func (a *app) downloadZip(w http.ResponseWriter, r *http.Request, baseDir, rel string) {
	baseName := path.Base(rel)
	if rel == "" || baseName == "." || baseName == "/" {
		baseName = a.cfg.name + "-Dateien"
	}
	zipName := baseName + ".zip"

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", contentDisposition("attachment", zipName))
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)

	zw := zip.NewWriter(w)
	walkErr := filepath.WalkDir(baseDir, func(filePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if r.Context().Err() != nil {
			return r.Context().Err()
		}
		if filePath == baseDir {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 || isHiddenName(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(baseDir, filePath)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		info, err := d.Info()
		if err != nil {
			return err
		}

		if d.IsDir() {
			header := &zip.FileHeader{Name: name + "/", Method: zip.Store}
			header.SetMode(info.Mode())
			header.SetModTime(info.ModTime())
			_, err = zw.CreateHeader(header)
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(info.Mode())
		header.SetModTime(info.ModTime())
		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		file, err := os.Open(filePath)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if walkErr != nil && !strings.Contains(walkErr.Error(), "broken pipe") {
		log.Printf("ZIP für %q: %v", rel, walkErr)
	}
	if err := zw.Close(); err != nil && !strings.Contains(err.Error(), "broken pipe") {
		log.Printf("ZIP abschließen: %v", err)
	}
}
