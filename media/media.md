# 🎵 media.* – Audio/Video-Funktionen

**Voraussetzung:** FFmpeg muss installiert sein. Standardmäßig wird `ffmpeg` (Linux/Mac) bzw. `ffmpeg.exe` (Windows) über den System-PATH gesucht. Über die Umgebungsvariable `VBX_FFMPEG` kann stattdessen ein expliziter Pfad zur FFmpeg-ausführbaren Datei angegeben werden (z. B. wenn FFmpeg nicht im PATH liegt oder eine bestimmte Version verwendet werden soll):

```
set VBX_FFMPEG=C:\Tools\ffmpeg\bin\ffmpeg.exe
```

Alle Funktionen liefern bei Fehlern einen `ErrorVal` – prüfbar mit `IsError()`/`ErrorText()` oder abfangbar mit `Try/Catch`, je nachdem wie der Aufruf im Skript steht (siehe Abschnitt „Fehlerbehandlung" in der Hauptreferenz).

---

## media.IsValid(input)

Prüft, ob FFmpeg die Audio- und Videostreams einer Mediendatei vollständig verarbeiten kann.

| Parameter | Beschreibung |
|---|---|
| `input` | Pfad zur zu prüfenden Datei |

**Rückgabe:** `True`, wenn FFmpeg die Datei fehlerfrei verarbeiten konnte, sonst `False`.

```vbx
If media.IsValid("song.mp3") Then
    Print "Datei ist gültig"
End If
```

---

## media.GetBitrate(input)

Ermittelt die Audio-Bitrate einer Mediendatei.

| Parameter | Beschreibung |
|---|---|
| `input` | Pfad zur Mediendatei |

**Rückgabe:** Bitrate in kbit/s als Zahl, sonst `ErrorVal` (falls die Bitrate nicht ermittelt werden konnte).

```vbx
Print media.GetBitrate("song.mp3")   ' z. B. 320
```

---

## media.ToMP3(input, output, [bitrate], [trimSilence])

Konvertiert eine Audio- oder Videodatei nach MP3.

| Parameter | Beschreibung |
|---|---|
| `input` | Quelldatei (Audio oder Video) |
| `output` | Zielpfad der MP3-Datei |
| `bitrate` | Optional, Standard `192` (kbps). Muss größer als 0 sein. |
| `trimSilence` | Optional, Standard `False`. Entfernt Stille am Anfang/Ende (siehe `media.TrimSilence`). |

**Rückgabe:** `"OK"` bei Erfolg, sonst `ErrorVal` mit der FFmpeg-Fehlermeldung.

```vbx
Try
    media.ToMP3("video.mp4", "audio.mp3", 320, True)
    Print "Konvertiert"
Catch err
    Print "Fehler: " & ErrorText(err)
End Try
```

---

## media.TrimSilence(input, output)

Entfernt Stille am Anfang und Ende einer Audiodatei (Schwellwert -50 dB, mindestens 0,5 s).

| Parameter | Beschreibung |
|---|---|
| `input` | Quelldatei |
| `output` | Zieldatei (wird als MP3, 192 kbps, geschrieben) |

**Rückgabe:** `"OK"` bei Erfolg, sonst `ErrorVal`.

---

## media.Merge(files, output, [bitrate])

Fügt mehrere Audiodateien in der angegebenen Reihenfolge zu einer MP3-Datei zusammen.

| Parameter | Beschreibung |
|---|---|
| `files` | Array mit mindestens 2 Dateipfaden |
| `output` | Zielpfad der zusammengeführten MP3-Datei |
| `bitrate` | Optional. Ohne Angabe wird automatisch die höchste Bitrate der Quelldateien verwendet (siehe unten), damit das Ergebnis nie schlechter ist als die beste Quelldatei. |

**Rückgabe:** `"OK"` bei Erfolg, sonst `ErrorVal`.

```vbx
Dim parts = {"teil1.mp3", "teil2.mp3", "teil3.mp3"}
media.Merge(parts, "komplett.mp3")
```

**Automatische Bitrate-Ermittlung:** Wird `bitrate` weggelassen, prüft `Merge` die Bitrate jeder Quelldatei (intern wie `media.GetBitrate`) und verwendet die höchste gefundene. Kann bei keiner der Dateien eine Bitrate ermittelt werden, fällt `Merge` auf `192` kbps zurück und gibt dazu eine Warnung auf der Konsole aus – das Skript läuft trotzdem weiter, aber es lohnt sich, im Zweifel `bitrate` explizit anzugeben und neu auszuführen.

---

## Tags – Übersicht

Unterstützte kanonische Tag-Namen, mit deutschen und englischen Aliasen (Groß-/Kleinschreibung und Leerzeichen/Bindestrich egal):

| Kanonischer Name | Aliase |
|---|---|
| `artist` | interpret |
| `album_artist` | album artist, album-artist, album_interpret, album interpret, album-interpret |
| `album` | – |
| `title` | titel |
| `genre` | – |
| `date` | datum, jahr |
| `track` | tracknumber, track number, track-number, titelnummer |
| `disc` | discnumber, disc number, disc-number, disc_nummer, disc-nummer |
| `composer` | komponist |
| `comment` | kommentar |
| `copyright` | – |
| `publisher` | verlag |
| `description` | beschreibung |
| `language` | sprache |

---

## media.Tags([name|names])

Übersetzt einen oder mehrere Tag-Namen (deutsch oder englisch, per Alias-Tabelle oben) in den kanonischen internen Namen.

| Parameter | Beschreibung |
|---|---|
| `name` \| `names` | Optional. Einzelner Tag-Name (String) oder Array von Tag-Namen. Ohne Argument: alle 14 kanonischen Namen. |

**Rückgabe:** String (bei einzelnem Namen), Array (bei mehreren Namen oder ohne Argument). `ErrorVal`, falls ein Name nicht unterstützt wird.

```vbx
Print media.Tags("Interpret")     ' -> "artist"
Print media.Tags("Titelnummer")   ' -> "track"
```

---

## media.GetTag(input, [tag])

Liest einen einzelnen Tag oder alle Tags einer Mediendatei.

| Parameter | Beschreibung |
|---|---|
| `input` | Mediendatei |
| `tag` | Optional. Tag-Name (Alias oder kanonisch). Ohne Angabe: alle Tags als Map. |

**Rückgabe:** String (bei einzelnem Tag – leer, falls Tag nicht vorhanden), Map (alle Tags), `ErrorVal` bei FFmpeg-Fehler.

```vbx
Dim genre = media.GetTag("song.mp3", "genre")

For Each x In files
    Try
        If media.GetTag(x, "genre") <> "<Genre>" Then
            media.SetTag(x, "genre", "<Genre>")
        End If
    Catch err
        Print "Fehler bei " & x & ": " & ErrorText(err)
    End Try
Next
```

---

## media.GetTags(files, [tag])

Wie `media.GetTag`, aber für mehrere Dateien auf einmal.

| Parameter | Beschreibung |
|---|---|
| `files` | Array von Dateipfaden |
| `tag` | Optional. Tag-Name. Ohne Angabe: alle Tags pro Datei. |

**Rückgabe:** Array – ein Eintrag pro Datei (String bei einzelnem Tag, Map bei allen Tags). `ErrorVal`, sobald bei einer Datei ein FFmpeg-Fehler auftritt (Verarbeitung der restlichen Dateien wird dann abgebrochen).

---

## media.SetTag(file, tag, value) / media.SetTag(file, tags)

Setzt einen oder mehrere Tags einer Datei.

| Parameter | Beschreibung |
|---|---|
| `file` | Zieldatei |
| `tag`, `value` | Einzelner Tag-Name + Wert |
| `tags` | Alternativ: Map mit mehreren Tag/Wert-Paaren (ein einziger FFmpeg-Durchlauf) |

**Rückgabe:** `"OK"` bei Erfolg, sonst `ErrorVal`.

Intern: FFmpeg schreibt eine temporäre Datei, der Host ersetzt danach die Originaldatei. Schlägt der Ersetzungsschritt fehl, bleibt die Originaldatei unverändert und der Fehler wird gemeldet.

```vbx
media.SetTag("song.mp3", "genre", "<Genre>")

media.SetTag("song.mp3", {
    "artist": "Die drei ???",
    "genre": "<Genre>"
})
```

---

## media.SetTags(files, tag, value) / media.SetTags(files, tags)

Wie `media.SetTag`, aber für mehrere Dateien.

| Parameter | Beschreibung |
|---|---|
| `files` | Array von Dateipfaden |
| `tag`, `value` \| `tags` | Wie bei `media.SetTag` |

**Rückgabe:** `"OK"`, sobald alle Dateien verarbeitet wurden. `ErrorVal`, sobald eine Datei fehlschlägt (danach wird abgebrochen, bereits verarbeitete Dateien bleiben aber geändert).

---

## media.IsCover(file)

Prüft, ob eine MP3-Datei ein eingebettetes Cover hat.

| Parameter | Beschreibung |
|---|---|
| `file` | MP3-Datei (aktuell einziges unterstütztes Format) |

**Rückgabe:** `True`/`False`, sonst `ErrorVal` (z. B. falls die Datei kein MP3 ist).

```vbx
Dim files = folder.GetFiles("C:\Musik", "*.mp3", true, true)
Dim missingCover = array.Create()

For Each f In files
    Try
        If Not media.IsCover(f) Then
            missingCover = array.Add(missingCover, f)
        End If
    Catch err
        Print "Fehler bei " & f & ": " & ErrorText(err)
    End Try
Next

Print "Dateien ohne Cover: " & array.Count(missingCover)
```

---

## media.SetCover(file, jpg)

Setzt bzw. ersetzt das eingebettete Cover einer MP3-Datei.

| Parameter | Beschreibung |
|---|---|
| `file` | MP3-Datei (aktuell einziges unterstütztes Format) |
| `jpg` | JPG/JPEG-Datei als neues Cover |

**Anforderungen an das Cover:**
- Format: JPG/JPEG (an der Dateiendung erkannt)
- Maximal 1000 × 1000 Pixel
- Maximal 1 MB groß

**Rückgabe:** `"OK"` bei Erfolg, sonst `ErrorVal` (z. B. bei zu großem Cover, falschem Format oder falls die Zieldatei kein MP3 ist).

```vbx
Try
    media.SetCover("song.mp3", "cover.jpg")
Catch err
    Print "Cover konnte nicht gesetzt werden: " & ErrorText(err)
End Try
```

---

## media.GetCover(file, output)

Extrahiert das eingebettete Cover einer MP3-Datei als JPG.

| Parameter | Beschreibung |
|---|---|
| `file` | MP3-Datei |
| `output` | Zielpfad, muss auf `.jpg`/`.jpeg` enden |

**Rückgabe:** `"OK"` bei Erfolg, sonst `ErrorVal` (z. B. falls kein Cover eingebettet ist oder die Datei kein MP3 ist).

---

## Hinweise

- Bei `SetTag`/`SetTags`/`SetCover` entsteht kurzzeitig eine temporäre Zwischendatei (`<name>.vbx-tag-<timestamp><ext>`) im selben Verzeichnis wie die Originaldatei, bevor diese ersetzt wird. Bei einem Absturz mitten im Vorgang kann eine solche Datei zurückbleiben.
- Tag-Namen sind bei Ein- und Ausgabe immer case-insensitive.