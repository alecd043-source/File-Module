package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultName = "HeimDrive"

type config struct {
	addr              string
	dataDir           string
	username          string
	password          string
	passwordGenerated bool
	passwordFile      string
	tlsCert           string
	tlsKey            string
	name              string
	maxUpload         int64
}

func parseConfig(args []string) (config, error) {
	maxUpload, err := envInt64("HEIMDRIVE_MAX_UPLOAD", 0)
	if err != nil {
		return config{}, err
	}

	cfg := config{
		addr:         envString("HEIMDRIVE_ADDR", ":8080"),
		dataDir:      envString("HEIMDRIVE_DATA", "./data"),
		username:     envString("HEIMDRIVE_USER", "admin"),
		password:     os.Getenv("HEIMDRIVE_PASSWORD"),
		passwordFile: os.Getenv("HEIMDRIVE_PASSWORD_FILE"),
		tlsCert:      os.Getenv("HEIMDRIVE_TLS_CERT"),
		tlsKey:       os.Getenv("HEIMDRIVE_TLS_KEY"),
		name:         envString("HEIMDRIVE_NAME", defaultName),
		maxUpload:    maxUpload,
	}

	fs := flag.NewFlagSet("heimdrive", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&cfg.addr, "addr", cfg.addr, "Listen-Adresse, z. B. :8080 oder 127.0.0.1:8080")
	fs.StringVar(&cfg.dataDir, "data", cfg.dataDir, "Ordner für die Dateien")
	fs.StringVar(&cfg.username, "user", cfg.username, "Benutzername für die Anmeldung")
	fs.StringVar(&cfg.password, "password", cfg.password, "Passwort (besser HEIMDRIVE_PASSWORD nutzen)")
	fs.StringVar(&cfg.passwordFile, "password-file", cfg.passwordFile, "Datei, die das Passwort enthält")
	fs.StringVar(&cfg.tlsCert, "tls-cert", cfg.tlsCert, "TLS-Zertifikat (optional)")
	fs.StringVar(&cfg.tlsKey, "tls-key", cfg.tlsKey, "TLS-Schlüssel (optional)")
	fs.StringVar(&cfg.name, "name", cfg.name, "Anzeigename")
	fs.Int64Var(&cfg.maxUpload, "max-upload", cfg.maxUpload, "Maximale Upload-Größe pro Anfrage in Bytes (0 = unbegrenzt)")
	showVersion := fs.Bool("version", false, "Version anzeigen")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if *showVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	cfg.name = strings.TrimSpace(cfg.name)
	if cfg.name == "" {
		cfg.name = defaultName
	}
	cfg.username = strings.TrimSpace(cfg.username)
	if cfg.username == "" {
		return config{}, errors.New("Benutzername darf nicht leer sein")
	}
	if cfg.maxUpload < 0 {
		return config{}, errors.New("max-upload darf nicht negativ sein")
	}
	if (cfg.tlsCert == "") != (cfg.tlsKey == "") {
		return config{}, errors.New("tls-cert und tls-key müssen gemeinsam angegeben werden")
	}

	if cfg.passwordFile != "" {
		content, err := os.ReadFile(cfg.passwordFile)
		if err != nil {
			return config{}, fmt.Errorf("Passwortdatei lesen: %w", err)
		}
		cfg.password = strings.TrimRight(string(content), "\r\n")
		if cfg.password == "" {
			return config{}, errors.New("Passwortdatei ist leer")
		}
	}
	if cfg.password == "" {
		password, err := randomToken(18)
		if err != nil {
			return config{}, err
		}
		cfg.password = password
		cfg.passwordGenerated = true
	}

	absDir, err := filepath.Abs(cfg.dataDir)
	if err != nil {
		return config{}, fmt.Errorf("Datenordner: %w", err)
	}
	if err := os.MkdirAll(absDir, 0o750); err != nil {
		return config{}, fmt.Errorf("Datenordner anlegen: %w", err)
	}
	info, err := os.Stat(absDir)
	if err != nil {
		return config{}, fmt.Errorf("Datenordner prüfen: %w", err)
	}
	if !info.IsDir() {
		return config{}, errors.New("Datenordner ist kein Ordner")
	}
	cfg.dataDir = absDir

	return cfg, nil
}

func envString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt64(key string, fallback int64) (int64, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s ist keine ganze Zahl: %w", key, err)
	}
	return value, nil
}

func randomToken(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("Zufallswert erzeugen: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
