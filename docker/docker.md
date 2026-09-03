# 🐳 docker.* – Docker-Funktionen

Dient zur Verwaltung von Docker-Containern, Images, Compose-Projekten und Systemressourcen.
Erfordert eine installierte und laufende Docker-Installation.
Alle Funktionen rufen intern `docker`-CLI-Befehle auf.

---

## Container

## docker.List([all])
- **Konkret:**
  Gibt alle laufenden Container zurück.
  Mit `all = true` werden auch gestoppte Container eingeschlossen.
- **Parameter:**
  - `all`: Optional. `BoolVal` (Standard: `false`).
- **Rückgabe:**
  `ArrVal`
  Array von `StrVal`-Einträgen. Format je Zeile: `ID|Name|Image|Status|Ports`.

---

## docker.Run(image [, name, options, command])
- **Konkret:**
  Startet einen neuen Container aus einem Image im Hintergrund (`-d`).
  Ist das Image lokal nicht vorhanden, wird es automatisch gepullt.
- **Parameter:**
  - `image`: Image-Name inkl. optionalem Tag (z. B. `"nginx:latest"`).
  - `name`: Optional. Container-Name (`--name`).
  - `options`: Optional. `ArrVal` mit Docker-Optionen (z. B. `{"-p", "8080:80", "-v", "/data:/data"}`).
  - `command`: Optional. Befehl der im Container ausgeführt wird.
- **Rückgabe:**
  `StrVal` (Container-ID) bei Erfolg, `"error: ..."` bei Fehler.

---

## docker.Login(registry, username, token)
- **Konkret:**
  Authentifiziert an einer Docker Registry via `--password-stdin`.
- **Parameter:**
  - `registry`: Registry-URL (z. B. `"registry.example.com"`).
  - `username`: Benutzername.
  - `token`: Passwort oder Access Token.
- **Rückgabe:**
  `StrVal` (`"ok"`) bei Erfolg, `"error: ..."` bei Fehler.

---

## docker.Logout(registry)
- **Konkret:**
  Meldet sich von einer Docker Registry ab.
- **Parameter:**
  - `registry`: Registry-URL.
- **Rückgabe:**
  `StrVal` bei Erfolg, `"error: ..."` bei Fehler.

---

## docker.Start(name)
- **Konkret:**
  Startet einen gestoppten Container.
- **Parameter:**
  - `name`: Container-Name oder ID.
- **Rückgabe:**
  `StrVal` (`"OK"`) bei Erfolg, `"error: ..."` bei Fehler.

---

## docker.Stop(name [, timeout])
- **Konkret:**
  Stoppt einen laufenden Container.
- **Parameter:**
  - `name`: Container-Name oder ID.
  - `timeout`: Optional. Wartezeit in Sekunden (Standard: `10`).
- **Rückgabe:**
  `StrVal` (`"OK"`) bei Erfolg, `"error: ..."` bei Fehler.

---

## docker.Restart(name [, timeout])
- **Konkret:**
  Startet einen Container neu.
- **Parameter:**
  - `name`: Container-Name oder ID.
  - `timeout`: Optional. Wartezeit in Sekunden (Standard: `10`).
- **Rückgabe:**
  `StrVal` (`"OK"`) bei Erfolg, `"error: ..."` bei Fehler.

---

## docker.Remove(name [, force])
- **Konkret:**
  Entfernt einen Container.
  Mit `force = true` wird auch ein laufender Container gelöscht.
- **Parameter:**
  - `name`: Container-Name oder ID.
  - `force`: Optional. `BoolVal` (Standard: `false`).
- **Rückgabe:**
  `StrVal` (`"OK"`) bei Erfolg, `"error: ..."` bei Fehler.

---

## docker.Status(name)
- **Konkret:**
  Gibt den aktuellen Zustand eines Containers zurück.
- **Parameter:**
  - `name`: Container-Name oder ID.
- **Rückgabe:**
  `StrVal`
  Mögliche Werte: `"running"`, `"exited"`, `"paused"`, `"restarting"` etc.

---

## docker.IsRunning(name)
- **Konkret:**
  Prüft ob ein Container aktuell läuft.
- **Parameter:**
  - `name`: Container-Name oder ID.
- **Rückgabe:**
  `BoolVal`

---

## docker.Inspect(name)
- **Konkret:**
  Gibt die vollständigen Metadaten eines Containers als JSON-String zurück.
- **Parameter:**
  - `name`: Container-Name oder ID.
- **Rückgabe:**
  `StrVal` (JSON), `"error: ..."` bei Fehler.

---

## docker.Logs(name [, lines])
- **Konkret:**
  Gibt die letzten Logzeilen eines Containers zurück.
- **Parameter:**
  - `name`: Container-Name oder ID.
  - `lines`: Optional. Anzahl Zeilen (Standard: `50`).
- **Rückgabe:**
  `StrVal`, `"error: ..."` bei Fehler.

---

## docker.Exec(name, cmd [, args...])
- **Konkret:**
  Führt einen Befehl in einem laufenden Container aus.
- **Parameter:**
  - `name`: Container-Name oder ID.
  - `cmd`: Auszuführender Befehl.
  - `args...`: Optional. Weitere Argumente.
- **Rückgabe:**
  `StrVal` (stdout), `"error: ..."` bei Fehler.

---

## docker.Stats([name])
- **Konkret:**
  Gibt CPU- und RAM-Nutzung zurück.
  Ohne Name: alle laufenden Container.
- **Parameter:**
  - `name`: Optional. Container-Name oder ID.
- **Rückgabe:**
  `ArrVal`
  Format je Zeile: `Name|CPU%|MemUsage|Mem%`.

---

## docker.ExportCompose(name [, outputPath])
- **Konkret:**
  Generiert eine `docker-compose.yml` aus einem laufenden Container (Ports, Volumes, Umgebungsvariablen, Labels, Restart-Policy).
  Ohne Pfad: gibt das YAML als String zurück.
  Mit Pfad: schreibt direkt in die Datei.
- **Parameter:**
  - `name`: Container-Name oder ID.
  - `outputPath`: Optional. Zielpfad für die YAML-Datei.
- **Rückgabe:**
  `StrVal` (YAML oder `"OK"`), `"error: ..."` bei Fehler.

---

## Images

## docker.ImageList()
- **Konkret:**
  Gibt alle lokal vorhandenen Docker-Images zurück.
- **Rückgabe:**
  `ArrVal`
  Format je Zeile: `Repository|Tag|ID|Size`.

---

## docker.ImagePull(image)
- **Konkret:**
  Lädt ein Image von Docker Hub oder einer Registry.
- **Parameter:**
  - `image`: Image-Name inkl. optionalem Tag (z. B. `"nginx:latest"`).
- **Rückgabe:**
  `StrVal` (`"OK"`) bei Erfolg, `"error: ..."` bei Fehler.

---

## docker.ImageRemove(image [, force])
- **Konkret:**
  Löscht ein lokales Image.
- **Parameter:**
  - `image`: Image-Name oder ID.
  - `force`: Optional. `BoolVal` (Standard: `false`).
- **Rückgabe:**
  `StrVal` (`"OK"`) bei Erfolg, `"error: ..."` bei Fehler.

---

## docker.ImageBuild(path, tag [, dockerfile])
- **Konkret:**
  Baut ein Image aus einem Dockerfile.
- **Parameter:**
  - `path`: Build-Kontext-Verzeichnis.
  - `tag`: Name und Tag des Images (z. B. `"myapp:1.0"`).
  - `dockerfile`: Optional. Pfad zum Dockerfile (Standard: `"Dockerfile"`).
- **Rückgabe:**
  `StrVal` (`"OK"`) bei Erfolg, `"error: ..."` bei Fehler.

---

## docker.ImagePrune()
- **Konkret:**
  Löscht alle ungenutzten (dangling) Images.
- **Rückgabe:**
  `StrVal` (`"OK"`) bei Erfolg, `"error: ..."` bei Fehler.

---

## docker.ImageRemoveAll()
- **Konkret:**
  Löscht alle ungenutzten Images, auch nicht-dangling.
  Container müssen gestoppt sein.
- **Rückgabe:**
  `StrVal` (`"OK"`) bei Erfolg, `"error: ..."` bei Fehler.

---

## Compose

## docker.ComposeUp(path [, detach])
- **Konkret:**
  Startet alle Dienste einer Compose-Datei.
- **Parameter:**
  - `path`: Pfad zur `docker-compose.yml`.
  - `detach`: Optional. `BoolVal` – Hintergrundmodus (Standard: `true`).
- **Rückgabe:**
  `StrVal` (`"OK"`) bei Erfolg, `"error: ..."` bei Fehler.

---

## docker.ComposeDown(path [, removeVolumes])
- **Konkret:**
  Stoppt und entfernt alle Dienste einer Compose-Datei.
  Mit `removeVolumes = true` werden auch die zugehörigen Volumes gelöscht.
- **Parameter:**
  - `path`: Pfad zur `docker-compose.yml`.
  - `removeVolumes`: Optional. `BoolVal` (Standard: `false`).
- **Rückgabe:**
  `StrVal` (`"OK"`) bei Erfolg, `"error: ..."` bei Fehler.

---

## docker.ComposePull(path)
- **Konkret:**
  Aktualisiert alle Images einer Compose-Datei.
- **Parameter:**
  - `path`: Pfad zur `docker-compose.yml`.
- **Rückgabe:**
  `StrVal` (`"OK"`) bei Erfolg, `"error: ..."` bei Fehler.

---

## docker.ComposeRestart(path)
- **Konkret:**
  Startet alle Dienste einer Compose-Datei neu.
- **Parameter:**
  - `path`: Pfad zur `docker-compose.yml`.
- **Rückgabe:**
  `StrVal` (`"OK"`) bei Erfolg, `"error: ..."` bei Fehler.

---

## docker.ComposeLogs(path [, lines])
- **Konkret:**
  Gibt die Logs aller Dienste einer Compose-Datei zurück.
- **Parameter:**
  - `path`: Pfad zur `docker-compose.yml`.
  - `lines`: Optional. Anzahl Zeilen pro Dienst (Standard: `50`).
- **Rückgabe:**
  `StrVal`, `"error: ..."` bei Fehler.

---

## System

## docker.IsInstalled()
- **Konkret:**
  Prüft ob Docker installiert und der Daemon erreichbar ist.
- **Rückgabe:**
  `BoolVal`

---

## docker.Version()
- **Konkret:**
  Gibt die installierte Docker Server-Version zurück.
- **Rückgabe:**
  `StrVal`, `"error: ..."` wenn Docker nicht verfügbar.

---

## docker.SystemPrune([all])
- **Konkret:**
  Räumt ungenutzte Ressourcen auf (gestoppte Container, Netzwerke, dangling Images, Build-Cache).
  Mit `all = true` werden auch alle ungenutzten Images entfernt.
- **Parameter:**
  - `all`: Optional. `BoolVal` (Standard: `false`).
- **Rückgabe:**
  `StrVal` (`"OK"`) bei Erfolg, `"error: ..."` bei Fehler.

---

## docker.NetworkList()
- **Konkret:**
  Gibt alle Docker-Netzwerke zurück.
- **Rückgabe:**
  `ArrVal`
  Format je Zeile: `ID|Name|Driver|Scope`.

---

## docker.VolumeList()
- **Konkret:**
  Gibt alle Docker-Volumes zurück.
- **Rückgabe:**
  `ArrVal`
  Format je Zeile: `Name|Driver`.