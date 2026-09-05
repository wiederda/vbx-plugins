# 📦 tar.* – TAR- & TAR.GZ-Archivfunktionen

Dient zum Erstellen, Entpacken, Inspizieren und Bearbeiten von TAR- und komprimierten TAR.GZ-Archiven.
Schützt gegen Path-Traversal (Zip-Slip-Äquivalent) bei allen Entpack-Operationen.

---

## tar.Create(tarPath, files...)
- **Konkret:**
  Erstellt ein TAR-Archiv mit erhaltener Verzeichnisstruktur.
  Verzeichnisse werden rekursiv eingeschlossen.
  Bei leerem Ergebnis oder Fehler wird die Ausgabedatei automatisch gelöscht.
- **Parameter:**
  - `tarPath`: Zielpfad des Archivs.
  - `files...`: Dateipfade als einzelne Argumente oder als `ArrVal`.
- **Rückgabe:**
  `BoolVal`
  `true` wenn mindestens eine Datei erfolgreich archiviert wurde.

---

## tar.CreateFlat(tarPath, files...)
- **Konkret:**
  Erstellt ein TAR-Archiv ohne Ordnerstruktur – alle Dateien landen im Root des Archivs.
  Ansonsten identisch zu `tar.Create`.
- **Parameter:**
  - `tarPath`: Zielpfad des Archivs.
  - `files...`: Dateipfade als einzelne Argumente oder als `ArrVal`.
- **Rückgabe:**
  `BoolVal`

---

## tar.Extract(archive, dest)
- **Konkret:**
  Entpackt ein TAR-Archiv in das Zielverzeichnis.
  Erstellt fehlende Unterverzeichnisse automatisch.
  Einträge außerhalb des Zielverzeichnisses (Path-Traversal) werden still übersprungen.
- **Parameter:**
  - `archive`: Pfad zum TAR-Archiv.
  - `dest`: Zielverzeichnis.
- **Rückgabe:**
  `NullVal` bei Erfolg, `ErrorVal` bei Fehler.

---

## tar.List(path)
- **Konkret:**
  Gibt detaillierte Informationen über alle Einträge im Archiv zurück.
  Bei korruptem Archiv werden bisherige Einträge plus ein Fehler-Sentinel zurückgegeben.
- **Parameter:**
  - `path`: Pfad zum TAR-Archiv.
- **Rückgabe:**
  `ArrVal`
  Array von Maps, ein Eintrag pro Datei.

  | Schlüssel | Typ       | Inhalt                                  |
  |-----------|-----------|-----------------------------------------|
  | `Name`    | `StrVal`  | Pfad im Archiv                          |
  | `Size`    | `NumVal`  | Größe in Bytes                          |
  | `IsDir`   | `BoolVal` | `true` bei Verzeichniseinträgen         |
  | `ModTime` | `StrVal`  | Änderungsdatum (`YYYY-MM-DD HH:mm:ss`)  |

---

## tar.Exists(tarPath, file)
- **Konkret:**
  Prüft, ob eine bestimmte Datei im Archiv vorhanden ist, ohne es zu entpacken.
- **Parameter:**
  - `tarPath`: Pfad zum TAR-Archiv.
  - `file`: Gesuchter Eintragspfad (exakter Match).
- **Rückgabe:**
  `NumVal`
  `1` wenn gefunden, `0` wenn nicht gefunden oder Fehler.

---

## tar.Add(tarPath, files...)
- **Konkret:**
  Fügt weitere Dateien zu einem bestehenden TAR-Archiv hinzu (Append-Modus).
  Existiert das Archiv noch nicht, wird es neu erstellt.
  Einzelne Fehler beim Hinzufügen werden als Warnung geloggt; die Operation bricht nicht ab.
- **Hinweis:**
  Append ist nur für unkomprimierte TAR-Archive möglich. Für GZ-Archive `tar.GzCreate` verwenden.
- **Parameter:**
  - `tarPath`: Pfad zum TAR-Archiv.
  - `files...`: Hinzuzufügende Dateipfade.
- **Rückgabe:**
  `BoolVal`
  `true` wenn mindestens eine Datei erfolgreich hinzugefügt wurde.

---

## tar.ToGz(tarPath [, deleteSource])
- **Konkret:**
  Komprimiert ein vorhandenes `.tar`-Archiv zu einer `.tar.gz`-Datei.
  Das Original wird optional gelöscht.
- **Parameter:**
  - `tarPath`: Pfad zum bestehenden TAR-Archiv.
  - `deleteSource`: Optional. `BoolVal` – bei `true` wird das Original nach Erfolg gelöscht.
- **Rückgabe:**
  `StrVal`
  Pfad der erzeugten `.tar.gz`-Datei.

---

## tar.GzCreate(output, files...)
- **Konkret:**
  Erstellt ein komprimiertes `.tar.gz`-Archiv mit erhaltener Verzeichnisstruktur.
  Identisch zu `tar.Create`, jedoch mit Gzip-Komprimierung.
- **Parameter:**
  - `output`: Zielpfad des Archivs.
  - `files...`: Dateipfade als einzelne Argumente oder als `ArrVal`.
- **Rückgabe:**
  `BoolVal`

---

## tar.GzCreateFlat(output, files...)
- **Konkret:**
  Erstellt ein komprimiertes `.tar.gz`-Archiv ohne Ordnerstruktur.
  Identisch zu `tar.CreateFlat`, jedoch mit Gzip-Komprimierung.
- **Parameter:**
  - `output`: Zielpfad des Archivs.
  - `files...`: Dateipfade als einzelne Argumente oder als `ArrVal`.
- **Rückgabe:**
  `BoolVal`

---

## tar.GzExtract(archive, dest)
- **Konkret:**
  Entpackt ein `.tar.gz`-Archiv in das Zielverzeichnis.
  Identisch zu `tar.Extract`, jedoch mit Gzip-Dekomprimierung.
- **Parameter:**
  - `archive`: Pfad zum `.tar.gz`-Archiv.
  - `dest`: Zielverzeichnis.
- **Rückgabe:**
  `NullVal` bei Erfolg, `ErrorVal` bei Fehler.

---

## tar.GzList(path)
- **Konkret:**
  Gibt detaillierte Informationen über alle Einträge in einem `.tar.gz`-Archiv zurück.
  Identisch zu `tar.List`, jedoch mit Gzip-Dekomprimierung.
- **Parameter:**
  - `path`: Pfad zum `.tar.gz`-Archiv.
- **Rückgabe:**
  `ArrVal`
  Gleiche Map-Struktur wie `tar.List`.

---

## tar.GzExists(tarPath, file)
- **Konkret:**
  Prüft, ob eine Datei in einem `.tar.gz`-Archiv vorhanden ist, ohne es zu entpacken.
  Identisch zu `tar.Exists`, jedoch mit Gzip-Dekomprimierung.
- **Parameter:**
  - `tarPath`: Pfad zum `.tar.gz`-Archiv.
  - `file`: Gesuchter Eintragspfad (exakter Match).
- **Rückgabe:**
  `NumVal`
  `1` wenn gefunden, `0` wenn nicht gefunden oder Fehler.

---

## tar.GzIsValid(path)
- **Konkret:**
  Validiert, ob eine Datei ein echtes, unbeschädigtes Gzip-TAR-Archiv ist.
  Prüft Gzip-Header und liest den ersten TAR-Eintrag.
- **Parameter:**
  - `path`: Zu prüfende Datei.
- **Rückgabe:**
  `NumVal`
  `1` wenn gültig, `0` wenn ungültig, nicht lesbar oder kein Gzip-TAR.