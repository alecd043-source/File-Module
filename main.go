package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"
)

var version = "dev"

func main() {
	log.SetFlags(log.Ldate | log.Ltime)

	cfg, err := parseConfig(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		log.Fatalf("Konfiguration: %v", err)
	}

	// A lower GC target keeps the idle footprint small. GOGC always wins.
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(40)
	}

	application, err := newApp(cfg)
	if err != nil {
		log.Fatalf("App starten: %v", err)
	}

	listener, err := net.Listen("tcp", cfg.addr)
	if err != nil {
		log.Fatalf("Port öffnen: %v", err)
	}
	defer listener.Close()

	scheme := "http"
	if cfg.tlsCert != "" {
		scheme = "https"
	}

	if cfg.passwordGenerated {
		fmt.Fprintf(os.Stderr, "\nKein Passwort gesetzt – für diesen Start wurde eines erzeugt:\n")
		fmt.Fprintf(os.Stderr, "  Benutzer: %s\n", cfg.username)
		fmt.Fprintf(os.Stderr, "  Passwort: %s\n", cfg.password)
		fmt.Fprintf(os.Stderr, "  Dauerhaft setzen mit HEIMDRIVE_PASSWORD oder --password-file.\n\n")
	}
	if cfg.tlsCert == "" {
		log.Printf("Hinweis: HTTP ohne Verschlüsselung. Nur in einem vertrauenswürdigen WLAN verwenden.")
	}

	urls := serverURLs(cfg, listener.Addr(), scheme)
	log.Printf("%s %s läuft. Datenordner: %s", cfg.name, version, cfg.dataDir)
	for _, url := range urls {
		log.Printf("Im Browser öffnen: %s", url)
	}

	srv := &http.Server{
		Handler:           application.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    64 << 10,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("Herunterfahren: %v", err)
		}
	}()

	if cfg.tlsCert != "" {
		err = srv.ServeTLS(listener, cfg.tlsCert, cfg.tlsKey)
	} else {
		err = srv.Serve(listener)
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Server: %v", err)
	}
}

func serverURLs(cfg config, addr net.Addr, scheme string) []string {
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		return []string{scheme + "://localhost"}
	}
	port := fmt.Sprint(tcpAddr.Port)
	host, _, err := net.SplitHostPort(cfg.addr)
	if err != nil {
		host = ""
	}

	if host != "" && host != "0.0.0.0" && host != "::" {
		return []string{scheme + "://" + joinHostPort(host, port)}
	}

	seen := make(map[string]bool)
	var urls []string
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil || ipNet.IP.IsLoopback() {
				continue
			}
			ip := ipNet.IP.String()
			if !seen[ip] {
				seen[ip] = true
				urls = append(urls, scheme+"://"+joinHostPort(ip, port))
			}
		}
	}
	if len(urls) == 0 {
		urls = append(urls, scheme+"://localhost:"+port)
	}
	return urls
}

func joinHostPort(host, port string) string {
	if strings.Contains(host, ":") {
		return "[" + strings.Trim(host, "[]") + "]:" + port
	}
	return host + ":" + port
}
