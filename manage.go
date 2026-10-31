package main

import (
	"net/http"
	"os"
	"path/filepath"
)

type nameRequest struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

func (a *app) handleMkdir(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req nameRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeRequestError(w, err)
		return
	}
	parentRel, err := cleanRel(req.Path)
	if err != nil {
		writeRequestError(w, err)
		return
	}
	parent, _, err := a.resolveDir(parentRel)
	if err != nil {
		writeRequestError(w, err)
		return
	}
	name, err := safeFilename(req.Name)
	if err != nil {
		writeRequestError(w, err)
		return
	}
	target := filepath.Join(parent, name)
	if err := os.Mkdir(target, 0o750); err != nil {
		if os.IsExist(err) {
			writeAPIError(w, http.StatusConflict, "Ein Ordner oder eine Datei mit diesem Namen existiert bereits.")
			return
		}
		if os.IsPermission(err) {
			writeAPIError(w, http.StatusForbidden, "Kein Zugriff.")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "Ordner konnte nicht erstellt werden.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{
		"name": name,
		"path": joinRel(parentRel, name),
	})
}

func (a *app) handleRename(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req nameRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeRequestError(w, err)
		return
	}
	rel, err := cleanRel(req.Path)
	if err != nil {
		writeRequestError(w, err)
		return
	}
	if rel == "" {
		writeAPIError(w, http.StatusBadRequest, "Der Hauptordner kann nicht umbenannt werden.")
		return
	}
	source, _, err := a.resolveExisting(rel)
	if err != nil {
		writeRequestError(w, err)
		return
	}
	newName, err := safeFilename(req.Name)
	if err != nil {
		writeRequestError(w, err)
		return
	}

	parentRel := filepath.ToSlash(filepath.Dir(filepath.FromSlash(rel)))
	if parentRel == "." {
		parentRel = ""
	}
	parent, _, err := a.resolveDir(parentRel)
	if err != nil {
		writeRequestError(w, err)
		return
	}
	target := filepath.Join(parent, newName)
	if samePath(source, target) {
		writeJSON(w, http.StatusOK, map[string]string{"name": newName, "path": joinRel(parentRel, newName)})
		return
	}
	if _, err := os.Lstat(target); err == nil {
		writeAPIError(w, http.StatusConflict, "Ein Ordner oder eine Datei mit diesem Namen existiert bereits.")
		return
	} else if !os.IsNotExist(err) {
		writeAPIError(w, http.StatusInternalServerError, "Ziel konnte nicht geprüft werden.")
		return
	}
	if err := os.Rename(source, target); err != nil {
		if os.IsPermission(err) {
			writeAPIError(w, http.StatusForbidden, "Kein Zugriff.")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "Umbenennen fehlgeschlagen.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"name": newName,
		"path": joinRel(parentRel, newName),
	})
}

func (a *app) handleDelete(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodDelete) {
		return
	}
	rel, err := cleanRel(r.URL.Query().Get("path"))
	if err != nil {
		writeRequestError(w, err)
		return
	}
	if rel == "" {
		writeAPIError(w, http.StatusBadRequest, "Der Hauptordner kann nicht gelöscht werden.")
		return
	}
	target, _, err := a.resolveExisting(rel)
	if err != nil {
		writeRequestError(w, err)
		return
	}
	if err := os.RemoveAll(target); err != nil {
		if os.IsPermission(err) {
			writeAPIError(w, http.StatusForbidden, "Kein Zugriff.")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "Löschen fehlgeschlagen.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func samePath(left, right string) bool {
	leftAbs, errLeft := filepath.Abs(left)
	rightAbs, errRight := filepath.Abs(right)
	if errLeft != nil || errRight != nil {
		return left == right
	}
	return filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
}
