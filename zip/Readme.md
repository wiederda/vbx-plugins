# 🗜️ zip.* – Archivfunktionen

Dieses Modul wird als **Plugin** bereitgestellt. Weitere Informationen stehen in **Allgemein**.

Es dient zum Erstellen, Entpacken und Inspizieren von ZIP-Archiven.
Unterstützt optionale Passwortverschlüsselung, Verzeichnisstruktur-Erhalt und atomares Schreiben.

---

## zip.Create(zipPath, files... [, password])
- **Konkret:**
  Erstellt ein ZIP-Archiv mit erhaltener Verzeichnisstruktur.
  Schreibt atomar: erst in eine `.tmp`-Datei, dann umbenennen.
  Überspringt nicht lesbare Dateien mit Warnung, bricht nicht ab.
- **Parameter:**
  - `zipPath`: Zielpfad des Archivs.
  - `files...`: Dateipfade oder Array von Pfaden. Verzeichnisse werden rekursiv eingeschlossen.
  - `password`: Optional. Passwort für Verschlüsselung.
- **Aufrufkonventionen:**
  - `zip.Create(out, [array], pass)`
  - `zip.Create(out, file1, file2, ..., pass)`
- **Rückgabe:**
  `BoolVal`
  `true` wenn mindestens eine Datei erfolgreich archiviert wurde.

---

## zip.CreateFlat(zipPath, files... [, password])
- **Konkret:**
  Erstellt ein ZIP-Archiv ohne Unterordner – alle Dateien landen auf der obersten Ebene.
  Ansonsten identisch zu `zip.Create`.
- **Parameter:**
  - `zipPath`: Zielpfad des Archivs.
  - `files...`: Dateipfade oder Array von Pfaden.
  - `password`: Optional. Passwort für Verschlüsselung.
- **Rückgabe:**
  `BoolVal`
  `true` wenn mindestens eine Datei erfolgreich archiviert wurde.

---

## zip.Extract(zipPath, dest [, password] [, stripComponents|"auto"])

- **Konkret:**
  Entpackt ein ZIP-Archiv in ein Zielverzeichnis. Erstellt das Zielverzeichnis und fehlende Unterverzeichnisse automatisch. Schützt gegen Zip-Slip: Pfade außerhalb des Zielverzeichnisses werden abgewiesen.
  Optional können führende Verzeichnisebenen aus den Archivpfaden entfernt werden. Dabei wird nie mehr entfernt, als alle Dateien im Archiv tatsächlich gemeinsam haben. Dadurch läuft ein zu hoher Wert nicht ins Leere und es werden keine Inhalte verschiedener Wurzelordner vermischt.
- **Parameter:**
  - `zipPath`: Pfad zum ZIP-Archiv.
  - `dest`: Zielverzeichnis.
  - `password`: Optional. Passwort für verschlüsselte Archive.
  - `stripComponents`: Optional. Nichtnegative Ganzzahl: Maximale Anzahl führender Pfadkomponenten, die von jedem Archiveintrag entfernt werden. Wird auf die gemeinsame Verzeichnistiefe aller Dateien im Archiv begrenzt.
  - `"auto"`: Optional, statt einer Zahl. Entfernt so viele Ebenen wie möglich, also die gesamte gemeinsame Verzeichnistiefe. Gibt es keinen gemeinsamen Ordner, wird nichts entfernt.
- **Aufrufkonventionen:**
  - `zip.Extract(zip, dest)`
  - `zip.Extract(zip, dest, password)`
  - `zip.Extract(zip, dest, stripComponents)`
  - `zip.Extract(zip, dest, "auto")`
  - `zip.Extract(zip, dest, password, stripComponents)`
  - `zip.Extract(zip, dest, password, "auto")`
- **Beispiele für `stripComponents`:**

  | Archivinhalt                 | `0` | `1`          | `2`          | `"auto"`     |
  |------------------------------|-----|--------------|--------------|--------------|
  | `Test.exe`                   | `Test.exe` | `Test.exe` | `Test.exe` | `Test.exe` |
  | `app-1.0/Test.exe`           | `app-1.0/Test.exe` | `Test.exe` | `Test.exe` | `Test.exe` |
  | `app-1.0/bin/Test.exe`       | `app-1.0/bin/Test.exe` | `bin/Test.exe` | `Test.exe` | `Test.exe` |
  | `a/x.txt` und `b/y.txt`      | unverändert | unverändert | unverändert | unverändert |

  Bei `stripComponents = 1` wird beispielsweise aus `test-0.6.1-windows-x64-portable/Test.exe` der Zielpfad `ziel/Test.exe`.
- **Hinweis:**
  Ein Passwort, das exakt `auto` lautet, wird als Modus interpretiert. In diesem Fall das Passwort mit einem zweiten Argument übergeben: `zip.Extract(z, d, "auto", 0)`.
- **Rückgabe:**
  `NullVal` bei Erfolg, `ErrorVal` bei Fehler.

- **Beispiel:**

```vbnet
  ' Entfernt den Wurzelordner, falls vorhanden – funktioniert für jedes ZIP
  zip.Extract("C:\test\app.zip", "C:\test\app", "auto")

  ' Mit Passwort und höchstens einer Ebene
  zip.Extract("archiv.zip", "ziel", "geheim", 1)
```

---

## zip.List(zipPath)
- **Konkret:**
  Gibt detaillierte Informationen über alle Einträge im Archiv zurück.
- **Parameter:**
  - `zipPath`: Pfad zum Archiv.
- **Rückgabe:**
  `ArrVal`
  Array von Maps, ein Eintrag pro Datei.
  Format je Eintrag:

  | Schlüssel | Typ      | Inhalt                          |
  |-----------|----------|---------------------------------|
  | `Name`    | `StrVal` | Pfad im Archiv                  |
  | `Size`    | `NumVal` | Unkomprimierte Größe in Bytes   |
  | `IsDir`   | `BoolVal`| `true` bei Verzeichniseinträgen |
  | `ModTime` | `StrVal` | Änderungsdatum (`YYYY-MM-DD HH:mm:ss`) |

- **Beispiel:**

```vbnet
  Dim entries = zip.List("archiv.zip")

  Dim i
  For i = 0 To array.Length(entries) - 1
      Print entries(i)["Name"] & " (" & entries(i)["Size"] & " bytes)"
  Next
```

---

## zip.ListNames(zipPath)
- **Konkret:**
  Gibt ein einfaches Array mit allen Dateinamen im Archiv zurück.
- **Parameter:**
  - `zipPath`: Pfad zum Archiv.
- **Rückgabe:**
  `ArrVal`
  Array von `StrVal`-Einträgen (Pfade innerhalb des Archivs).

- **Beispiel:**
```vbnet
  Dim names = zip.ListNames("archiv.zip")

  Dim i
  For i = 0 To array.Length(names) - 1
      Print names(i)
  Next
```

---

## zip.Exists(zipPath, entryName)
- **Konkret:**
  Prüft, ob eine bestimmte Datei im ZIP-Archiv existiert.
  Vergleicht case-sensitiv gegen den vollständigen Pfad im Archiv (Slash-Notation).
- **Parameter:**
  - `zipPath`: Pfad zum Archiv.
  - `entryName`: Pfad der gesuchten Datei innerhalb des Archivs (z. B. `"data/config.json"`).
- **Rückgabe:**
  `BoolVal`
  `true` wenn die Datei existiert, sonst `false`. `ErrorVal` nur wenn das Archiv selbst nicht geöffnet werden kann.