package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const reservedPrefix = ".heimdrive-"

func cleanRel(raw string) (string, error) {
	if strings.ContainsRune(raw, 0) {
		return "", newAPIError(http.StatusBadRequest, "Ungültiger Pfad.")
	}
	raw = strings.ReplaceAll(raw, `\`, "/")
	cleaned := path.Clean("/" + raw)
	rel := strings.TrimPrefix(cleaned, "/")
	if rel == "." {
		rel = ""
	}
	return rel, nil
}

func (a *app) resolveExisting(rel string) (string, os.FileInfo, error) {
	abs := filepath.Join(a.root, filepath.FromSlash(rel))
	linkInfo, err := os.Lstat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, newAPIError(http.StatusNotFound, "Nicht gefunden.")
		}
		if os.IsPermission(err) {
			return "", nil, newAPIError(http.StatusForbidden, "Kein Zugriff.")
		}
		return "", nil, err
	}
	if linkInfo.Mode()&os.ModeSymlink != 0 {
		return "", nil, newAPIError(http.StatusForbidden, "Symbolische Links werden nicht unterstützt.")
	}
	realPath, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", nil, err
	}
	if !withinRoot(a.root, realPath) {
		return "", nil, newAPIError(http.StatusForbidden, "Pfad liegt außerhalb des Datenordners.")
	}
	info, err := os.Stat(realPath)
	if err != nil {
		return "", nil, err
	}
	return realPath, info, nil
}

func (a *app) resolveDir(rel string) (string, os.FileInfo, error) {
	realPath, info, err := a.resolveExisting(rel)
	if err != nil {
		return "", nil, err
	}
	if !info.IsDir() {
		return "", nil, newAPIError(http.StatusBadRequest, "Das ist kein Ordner.")
	}
	return realPath, info, nil
}

func withinRoot(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func safeFilename(name string) (string, error) {
	name = strings.ReplaceAll(name, `\`, "/")
	name = path.Base(name)
	if name == "" || name == "." || name == ".." || strings.ContainsRune(name, 0) {
		return "", newAPIError(http.StatusBadRequest, "Ungültiger Dateiname.")
	}
	if !utf8.ValidString(name) {
		return "", newAPIError(http.StatusBadRequest, "Dateiname ist kein gültiges UTF-8.")
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return "", newAPIError(http.StatusBadRequest, "Dateiname enthält ungültige Zeichen.")
		}
	}
	if len(name) > 200 {
		return "", newAPIError(http.StatusBadRequest, "Dateiname ist zu lang (maximal 200 Byte).")
	}
	if strings.HasPrefix(name, reservedPrefix) {
		return "", newAPIError(http.StatusBadRequest, "Der Name ist reserviert.")
	}
	return name, nil
}

func joinRel(base, name string) string {
	if base == "" {
		return name
	}
	return base + "/" + name
}

func splitName(name string) (string, string) {
	index := strings.LastIndexByte(name, '.')
	if index <= 0 {
		return name, ""
	}
	return name[:index], name[index:]
}

func contentDisposition(kind, filename string) string {
	fallback := strings.Map(func(r rune) rune {
		if r < 32 || r > 126 || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, filename)
	if fallback == "" {
		fallback = "download"
	}
	return fmt.Sprintf(`%s; filename="%s"; filename*=UTF-8''%s`, kind, fallback, url.PathEscape(filename))
}

func previewContentType(name string) (string, bool) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg":
		return "image/jpeg", true
	case ".png":
		return "image/png", true
	case ".gif":
		return "image/gif", true
	case ".webp":
		return "image/webp", true
	case ".bmp":
		return "image/bmp", true
	case ".avif":
		return "image/avif", true
	case ".mp4", ".m4v":
		return "video/mp4", true
	case ".webm":
		return "video/webm", true
	case ".mov":
		return "video/quicktime", true
	case ".mp3":
		return "audio/mpeg", true
	case ".m4a":
		return "audio/mp4", true
	case ".aac":
		return "audio/aac", true
	case ".ogg", ".oga":
		return "audio/ogg", true
	case ".wav":
		return "audio/wav", true
	case ".flac":
		return "audio/flac", true
	case ".pdf":
		return "application/pdf", true
	case ".txt", ".log", ".md", ".csv", ".json", ".xml", ".yaml", ".yml", ".toml", ".ini", ".conf":
		return "text/plain; charset=utf-8", true
	default:
		return "", false
	}
}
