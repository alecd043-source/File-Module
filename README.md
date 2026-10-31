# HeimDrive

HeimDrive ist eine kleine, schnelle Web-App für Dateien im eigenen Netzwerk. Auf dem Linux-Server läuft ein einzelnes Go-Programm; Handy, Tablet und PC öffnen die Oberfläche einfach im Browser. Dateien lassen sich hochladen, herunterladen, ansehen, umbenennen, löschen und als Ordner-ZIP herunterladen.

Die App ist bewusst kein Cloud-Sync-Client. Sie ist ein privater Dateiserver – ohne Kontoanbieter, Datenbank oder Hintergrunddienste.

## Funktionen

- Mehrfach-Upload per Dateiauswahl oder Drag & Drop, mit Fortschritt und Abbrechen
- Ordner erstellen, Dateien und Ordner umbenennen und löschen
- Download von Dateien und ganzen Ordnern als ZIP
- Vorschau für Bilder, Videos, Audio, PDF und Textdateien
- Mobile-first Oberfläche, funktioniert auch als Home-Screen-App
- Login mit Passwort, Sitzungen und CSRF-Schutz
- Optional HTTPS
- Ein einziges statisches Binary, keine externen Laufzeitabhängigkeiten
- Sehr geringer RAM-Verbrauch durch Streaming statt Dateien im Arbeitsspeicher

## Schnellstart auf Linux

Voraussetzung: Linux mit systemd. Das passende Binary liegt bereits im Ordner `dist/`.

```sh
# amd64 (normale PCs/Server)
sudo sh scripts/install.sh dist/heimdrive-linux-amd64

# arm64 (z. B. Raspberry Pi mit 64-Bit-System)
sudo sh scripts/install.sh dist/heimdrive-linux-arm64

# i386 (Debian 12 32-Bit, x86)
sudo sh scripts/install.sh dist/heimdrive-linux-386
```

Das i386-Binary wird mit `GO386=softfloat` gebaut und läuft dadurch auch auf älteren 32-Bit-CPUs ohne SSE2, etwa Athlon XP oder Pentium III.

Das Installationsskript:

1. legt den Systembenutzer `heimdrive` an,
2. installiert das Binary nach `/usr/local/bin/heimdrive`,
3. erzeugt `/etc/heimdrive.env` mit einem zufälligen Passwort,
4. richtet den systemd-Dienst ein und startet ihn,
5. zeigt die Adresse und die Zugangsdaten an.

Danach im Browser auf dem Handy öffnen:

```text
http://<IP-des-Servers>:8080
```

Benutzer ist standardmäßig `admin`. Das erzeugte Passwort steht in `/etc/heimdrive.env` und wird bei der Installation angezeigt.

### Firewall

Falls die Firewall Port 8080 blockiert, nur das lokale Netz freigeben. Beispiel mit ufw:

```sh
sudo ufw allow from 192.168.0.0/16 to any port 8080 proto tcp
```

Die App sollte **nicht** direkt aus dem Internet erreichbar sein. Für Zugriff von unterwegs besser ein VPN wie WireGuard verwenden.

## Ohne Installation starten

Das Binary kann auch direkt ausgeführt werden. Ohne Passwort erzeugt HeimDrive beim Start eines und zeigt es im Terminal an.

```sh
chmod +x dist/heimdrive-linux-amd64
./dist/heimdrive-linux-amd64 -data ./meine-dateien -addr :8080
```

Dauerhaftes Passwort setzen:

```sh
HEIMDRIVE_PASSWORD='ein-langes-passwort' \
  ./dist/heimdrive-linux-amd64 -data ./meine-dateien -addr :8080
```

Die App sucht die Dateien im angegebenen Datenordner. Der Ordner wird automatisch angelegt.

## Selbst bauen

Voraussetzung: Go 1.22 oder neuer.

```sh
sh scripts/build-linux.sh
```

Ergebnis:

```text
dist/heimdrive-linux-amd64
dist/heimdrive-linux-arm64
dist/heimdrive-linux-386
```

Alternativ mit Make:

```sh
make test
make build-linux
```

Unter Windows lässt sich das Linux-Binary ebenfalls cross-compilieren:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/build-linux.ps1
```

Tests:

```sh
go test ./...
```

## Konfiguration

Alles lässt sich über Umgebungsvariablen oder Kommandozeilenparameter setzen. Die Umgebungsvariablen sind für den systemd-Betrieb gedacht.

| Umgebungsvariable | Parameter | Standard | Bedeutung |
|---|---|---|---|
| `HEIMDRIVE_ADDR` | `-addr` | `:8080` | Listen-Adresse, z. B. `:8080` oder `127.0.0.1:8080` |
| `HEIMDRIVE_DATA` | `-data` | `./data` | Ordner für die Dateien |
| `HEIMDRIVE_USER` | `-user` | `admin` | Benutzername |
| `HEIMDRIVE_PASSWORD` | `-password` | zufällig | Passwort |
| `HEIMDRIVE_PASSWORD_FILE` | `-password-file` | – | Datei mit dem Passwort; hat Vorrang |
| `HEIMDRIVE_NAME` | `-name` | `HeimDrive` | Anzeigename |
| `HEIMDRIVE_MAX_UPLOAD` | `-max-upload` | `0` | Max. Upload pro Anfrage in Bytes; `0` = unbegrenzt |
| `HEIMDRIVE_TLS_CERT` | `-tls-cert` | – | TLS-Zertifikat |
| `HEIMDRIVE_TLS_KEY` | `-tls-key` | – | TLS-Schlüssel |

Beispiel für höchstens 5 GiB pro Upload:

```sh
HEIMDRIVE_MAX_UPLOAD=5368709120
```

### HTTPS

Mit eigenem Zertifikat:

```sh
./dist/heimdrive-linux-amd64 \
  -addr :8443 \
  -tls-cert /etc/heimdrive/fullchain.pem \
  -tls-key /etc/heimdrive/privkey.pem
```

Bei einem selbst ausgestellten Zertifikat warnt der Browser. Für ein vertrauenswürdiges Zertifikat im LAN kann man z. B. eine lokale CA oder einen Reverse Proxy verwenden. Wichtig: Die App nur im vertrauenswürdigen Netz betreiben, wenn sie über HTTP läuft.

## systemd

Nach der Installation:

```sh
systemctl status heimdrive
journalctl -u heimdrive -f
sudo systemctl restart heimdrive
```

Konfiguration ändern:

```sh
sudo editor /etc/heimdrive.env
sudo systemctl restart heimdrive
```

Die Unit liegt unter `/etc/systemd/system/heimdrive.service`, die Konfiguration unter `/etc/heimdrive.env` (Modus `600`).

### Update

```sh
sudo install -m 0755 dist/heimdrive-linux-amd64 /usr/local/bin/heimdrive
sudo systemctl restart heimdrive
```

Die Dateien bleiben in `/var/lib/heimdrive` erhalten.

### Deinstallation

```sh
sudo systemctl disable --now heimdrive
sudo rm /etc/systemd/system/heimdrive.service /usr/local/bin/heimdrive
sudo rm /etc/heimdrive.env
sudo userdel heimdrive
# Nur wenn auch alle Dateien gelöscht werden sollen:
# sudo rm -rf /var/lib/heimdrive
```

## Warum so wenig RAM?

HeimDrive ist für kleine Server und Raspberry Pis gedacht. Deshalb:

- **Streaming statt Einlesen:** Uploads werden direkt auf die Platte geschrieben, Downloads per `io.Copy` bzw. `http.ServeContent` gestreamt. Eine 10-GB-Datei wird nicht in den RAM geladen.
- **Keine Datenbank:** Dateien und Ordner sind das Dateisystem. Sitzungen liegen nur klein im Speicher.
- **Keine Bildverarbeitung:** Es werden keine Thumbnails oder Indizes im Hintergrund erzeugt.
- **Ein statisches Go-Binary:** Kein Node.js, PHP, Python oder Datenbankserver im Hintergrund.
- **Niedriger GC-Zielwert:** Ohne gesetzte `GOGC`-Variable startet die App mit `GOGC=40`.
- **systemd-Limits:** Die mitgelieferte Unit setzt `GOGC=30`, `GOMEMLIMIT=96MiB`, `MemoryHigh=64M` und `MemoryMax=128M`.

Im Leerlauf liegt der Verbrauch typischerweise im einstelligen bis niedrigen zweistelligen MB-Bereich. Im Test unter Debian (amd64, ohne Last) waren es etwa **8 MB RSS**. Die genauen Werte hängen von Go-Version, Kernel und Last ab. Prüfen mit:

```sh
systemctl status heimdrive
# oder
ps -o pid,rss,cmd -C heimdrive
```

## Verhalten und Grenzen

- Bei einem Namenskonflikt wird die neue Datei automatisch umbenannt, z. B. `foto.jpg` → `foto (1).jpg`. Es werden keine vorhandenen Dateien überschrieben.
- Löschen ist endgültig; es gibt keinen Papierkorb.
- Symbolische Links im Datenordner werden aus Sicherheitsgründen nicht angezeigt oder ausgeliefert.
- Es gibt einen Benutzer und keine Freigabe-Rechte pro Ordner.
- Kein automatischer Sync, keine Versionierung und keine Wiederaufnahme abgebrochener Uploads.
- Dateien werden nicht verschlüsselt gespeichert. Datenträgerverschlüsselung (LUKS) oder ein verschlüsseltes Backup sind Aufgabe des Servers.

## Projektstruktur

```text
.
├── main.go              Start, Netzwerk, HTTP-Server
├── config.go            Konfiguration und Datenordner
├── app.go               Routing, Templates, Sicherheitsheader
├── auth.go              Login, Sitzungen, CSRF, Rate-Limit
├── paths.go             Sichere Pfade und Dateinamen
├── list.go              Gestreamte Ordnerliste
├── upload.go            Gestreamter Upload
├── download.go          Download, ZIP und Vorschau
├── manage.go            Ordner, Umbenennen, Löschen
├── web/                 Eingebettete Oberfläche
├── packaging/           systemd-Unit
├── scripts/             Build- und Installationsskripte
└── dist/                Fertige Linux-Binaries
```
