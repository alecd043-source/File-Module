package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestServer(t *testing.T, cfg config) (*app, *httptest.Server) {
	t.Helper()
	if cfg.dataDir == "" {
		cfg.dataDir = t.TempDir()
	}
	if cfg.username == "" {
		cfg.username = "admin"
	}
	if cfg.password == "" {
		cfg.password = "secret"
	}
	if cfg.name == "" {
		cfg.name = "TestDrive"
	}
	application, err := newApp(cfg)
	if err != nil {
		t.Fatalf("newApp: %v", err)
	}
	server := httptest.NewServer(application.Handler())
	t.Cleanup(server.Close)
	return application, server
}

func loginClient(t *testing.T, application *app, server *httptest.Server) (*http.Client, string) {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.PostForm(server.URL+"/login", url.Values{
		"username": {application.cfg.username},
		"password": {application.cfg.password},
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status = %d, want %d", response.StatusCode, http.StatusSeeOther)
	}

	serverURL, _ := url.Parse(server.URL)
	for _, cookie := range jar.Cookies(serverURL) {
		if cookie.Name != sessionCookieName {
			continue
		}
		sess, ok := application.sessions.get(cookie.Value)
		if !ok {
			t.Fatal("session not found")
		}
		return client, sess.csrf
	}
	t.Fatal("session cookie missing")
	return nil, ""
}

func doRequest(t *testing.T, client *http.Client, method, target, csrf string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		t.Fatal(err)
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func uploadRequest(t *testing.T, client *http.Client, base, dir, filename, content, csrf string) *http.Response {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, base+"/api/upload?path="+url.QueryEscape(dir), &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-CSRF-Token", csrf)
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func TestLoginFilesAndManagement(t *testing.T) {
	application, server := newTestServer(t, config{})

	response := doRequest(t, http.DefaultClient, http.MethodGet, server.URL+"/api/list", "", nil)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list status = %d", response.StatusCode)
	}
	response.Body.Close()

	response, err := http.PostForm(server.URL+"/login", url.Values{"username": {"admin"}, "password": {"wrong"}})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong login status = %d", response.StatusCode)
	}
	response.Body.Close()

	client, csrf := loginClient(t, application, server)

	response = doRequest(t, client, http.MethodGet, server.URL+"/", "", nil)
	indexBody, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(string(indexBody), "TestDrive") || !strings.Contains(string(indexBody), csrf) {
		t.Fatalf("index status/body unexpected: %d", response.StatusCode)
	}

	response = uploadRequest(t, client, server.URL, "", "hello.txt", "Hallo Welt", "")
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("upload without csrf status = %d", response.StatusCode)
	}
	response.Body.Close()

	response = uploadRequest(t, client, server.URL, "", "hello.txt", "Hallo Welt", csrf)
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("upload status = %d: %s", response.StatusCode, body)
	}
	var first uploadResponse
	if err := json.NewDecoder(response.Body).Decode(&first); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if len(first.Saved) != 1 || first.Saved[0].Name != "hello.txt" || first.Saved[0].Renamed {
		t.Fatalf("unexpected upload result: %+v", first)
	}

	response = uploadRequest(t, client, server.URL, "", "hello.txt", "Noch einmal", csrf)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("duplicate upload status = %d", response.StatusCode)
	}
	var second uploadResponse
	if err := json.NewDecoder(response.Body).Decode(&second); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if len(second.Saved) != 1 || second.Saved[0].Name != "hello (1).txt" || !second.Saved[0].Renamed {
		t.Fatalf("unexpected duplicate result: %+v", second)
	}

	response = doRequest(t, client, http.MethodGet, server.URL+"/api/list?path=", "", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d", response.StatusCode)
	}
	var listing struct {
		Path    string      `json:"path"`
		Entries []fileEntry `json:"entries"`
	}
	if err := json.NewDecoder(response.Body).Decode(&listing); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if len(listing.Entries) != 2 {
		t.Fatalf("list entries = %+v", listing.Entries)
	}

	response = doRequest(t, client, http.MethodGet, server.URL+"/api/download?path=hello.txt", "", nil)
	downloadBody, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || string(downloadBody) != "Hallo Welt" {
		t.Fatalf("download status/body = %d %q", response.StatusCode, downloadBody)
	}
	if got := response.Header.Get("Content-Disposition"); !strings.Contains(got, "attachment") {
		t.Fatalf("download disposition = %q", got)
	}

	response = doRequest(t, client, http.MethodGet, server.URL+"/api/view?path=hello.txt", "", nil)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/plain") {
		t.Fatalf("view status/type = %d %q", response.StatusCode, response.Header.Get("Content-Type"))
	}

	response = doRequest(t, client, http.MethodPost, server.URL+"/api/mkdir", csrf, strings.NewReader(`{"path":"","name":"Unterordner"}`))
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("mkdir status = %d", response.StatusCode)
	}
	response.Body.Close()

	response = uploadRequest(t, client, server.URL, "Unterordner", "innen.txt", "innen", csrf)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("nested upload status = %d", response.StatusCode)
	}
	response.Body.Close()

	response = doRequest(t, client, http.MethodGet, server.URL+"/api/download?path=Unterordner", "", nil)
	zipBody, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !bytes.HasPrefix(zipBody, []byte("PK")) {
		t.Fatalf("zip download unexpected: %d", response.StatusCode)
	}

	response = doRequest(t, client, http.MethodPost, server.URL+"/api/rename", csrf, strings.NewReader(`{"path":"hello.txt","name":"welt.txt"}`))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("rename status = %d", response.StatusCode)
	}
	response.Body.Close()
	if _, err := os.Stat(filepath.Join(application.cfg.dataDir, "welt.txt")); err != nil {
		t.Fatalf("renamed file missing: %v", err)
	}

	response = doRequest(t, client, http.MethodDelete, server.URL+"/api/delete?path=welt.txt", csrf, nil)
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", response.StatusCode)
	}
	response.Body.Close()
	if _, err := os.Stat(filepath.Join(application.cfg.dataDir, "welt.txt")); !os.IsNotExist(err) {
		t.Fatalf("file still exists: %v", err)
	}

	response = doRequest(t, client, http.MethodDelete, server.URL+"/api/delete?path=", csrf, nil)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("root delete status = %d", response.StatusCode)
	}
	response.Body.Close()
}

func TestNoPathEscape(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "data")
	if err := os.Mkdir(root, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "outside-secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	application, server := newTestServer(t, config{dataDir: root})
	client, _ := loginClient(t, application, server)

	response := doRequest(t, client, http.MethodGet, server.URL+"/api/download?path="+url.QueryEscape("../outside-secret.txt"), "", nil)
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound || string(body) == "secret" {
		t.Fatalf("path traversal response = %d %q", response.StatusCode, body)
	}
}

func TestCleanRelAndSafeFilename(t *testing.T) {
	for input, want := range map[string]string{
		"":              "",
		"foo/bar":       "foo/bar",
		"/foo//bar/":    "foo/bar",
		`foo\bar`:       "foo/bar",
		"../../outside": "outside",
	} {
		got, err := cleanRel(input)
		if err != nil || got != want {
			t.Errorf("cleanRel(%q) = %q, %v; want %q", input, got, err, want)
		}
	}

	for input, want := range map[string]string{
		"hello.txt":         "hello.txt",
		"../../evil.txt":    "evil.txt",
		`a\b\file.txt`:      "file.txt",
		"Ordner mit Ümlaut": "Ordner mit Ümlaut",
	} {
		got, err := safeFilename(input)
		if err != nil || got != want {
			t.Errorf("safeFilename(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"", ".", "..", ".heimdrive-upload-x.part", strings.Repeat("a", 201), "a\x00b"} {
		if got, err := safeFilename(input); err == nil {
			t.Errorf("safeFilename(%q) = %q, want error", input, got)
		}
	}
}

func TestPreviewTypes(t *testing.T) {
	if contentType, ok := previewContentType("photo.JPG"); !ok || contentType != "image/jpeg" {
		t.Fatalf("jpg preview = %q %v", contentType, ok)
	}
	if contentType, ok := previewContentType("notes.md"); !ok || !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("md preview = %q %v", contentType, ok)
	}
	if _, ok := previewContentType("page.html"); ok {
		t.Fatal("html must not be served inline")
	}
	if _, ok := previewContentType("image.svg"); ok {
		t.Fatal("svg must not be served inline")
	}
}
