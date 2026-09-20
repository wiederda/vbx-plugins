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

## media.GetDuration(input)

Ermittelt die Dauer einer Mediendatei.

| Parameter | Beschreibung |
|---|---|
| `input` | Pfad zur Mediendatei |

**Rückgabe:** Dauer in Sekunden als Zahl (z. B. `754.56`), sonst `ErrorVal` (falls die Dauer nicht ermittelt werden konnte).

```vbx
Print media.GetDuration("song.mp3")   ' z. B. 213.4
```

---

## media.GetInfo(file)

Liefert Bitrate, Dauer und ob ein Cover eingebettet ist – in einem einzigen FFmpeg-Aufruf statt separater Aufrufe von `GetBitrate`, `GetDuration` und `IsCover`. Lohnt sich vor allem bei der Verarbeitung vieler Dateien (ein FFmpeg-Prozess statt drei pro Datei).

| Parameter | Beschreibung |
|---|---|
| `file` | Pfad zur Mediendatei |

**Rückgabe:** Map mit `bitrate` (Zahl, kbit/s), `duration` (Zahl, Sekunden), `hasCover` (Boolean). `ErrorVal`, falls Bitrate oder Dauer nicht ermittelt werden konnten.

```vbx
Dim files = folder.GetFiles("C:\Musik", "*.mp3", true, true)

For Each f In files
    Try
        Dim info = media.GetInfo(f)
        Print f & ": " & info["bitrate"] & " kbps, " & info["duration"] & "s, Cover: " & info["hasCover"]
    Catch err
        Print "Fehler bei " & f & ": " & ErrorText(err)
    End Try
Next
```

---

## media.ToMP3(input, output, [bitrate], [trimSilence], [silenceThresholdDB], [silenceDurationSec])

Konvertiert eine Audio- oder Videodatei nach MP3.

| Parameter | Beschreibung |
|---|---|
| `input` | Quelldatei (Audio oder Video) |
| `output` | Zielpfad der MP3-Datei |
| `bitrate` | Optional, Standard `192` (kbps). Muss größer als 0 sein. |
| `trimSilence` | Optional, Standard `False`. Entfernt Stille am Anfang/Ende (siehe `media.TrimSilence`). |
| `silenceThresholdDB` | Optional, Standard `-50`. Lautstärke-Schwellwert in dB, ab dem Audio als Stille gilt. Nur relevant, wenn `trimSilence=True`. |
| `silenceDurationSec` | Optional, Standard `0.3`. Mindestdauer der Stille in Sekunden, damit sie erkannt wird. Nur relevant, wenn `trimSilence=True`. |

**Rückgabe:** `"OK"` bei Erfolg, sonst `ErrorVal` mit der FFmpeg-Fehlermeldung.

```vbx
Try
    media.ToMP3("video.mp4", "audio.mp3", 320, True)
    Print "Konvertiert"
Catch err
    Print "Fehler: " & ErrorText(err)
End Try
```

**Hinweis zu `trimSilence`:** Die Standardwerte passen nicht auf jedes Material – bei Aufnahmen mit hörbarem Grundrauschen (z. B. Hörspiele, Sprachaufnahmen) erkennt `-50dB` echte Stille oft nicht zuverlässig, sodass am Anfang/Ende noch Reste stehen bleiben. Mit `media.AnalyzeSilence` lässt sich vorab prüfen, welcher Schwellwert zum jeweiligen Material passt, bevor tatsächlich geschnitten wird.

---

## media.TrimSilence(input, output)

Entfernt Stille am Anfang und Ende einer Audiodatei (Schwellwert -50 dB, mindestens 0,5 s).

| Parameter | Beschreibung |
|---|---|
| `input` | Quelldatei |
| `output` | Zieldatei (wird als MP3, 192 kbps, geschrieben) |

**Rückgabe:** `"OK"` bei Erfolg, sonst `ErrorVal`.

---

## media.AnalyzeSilence(file, [thresholdDB], [durationSec], [maxSeconds])

Diagnose-Funktion: erkennt Stille-Abschnitte im Material, **ohne** etwas zu schneiden. Dient zum Kalibrieren der Schwellwerte für `ToMP3`/`TrimSilence`, bevor tatsächlich geschnitten wird.

| Parameter | Beschreibung |
|---|---|
| `file` | Mediendatei |
| `thresholdDB` | Optional, Standard `-50`. Zu testender Lautstärke-Schwellwert in dB. |
| `durationSec` | Optional, Standard `0.3`. Mindestdauer der Stille in Sekunden. |
| `maxSeconds` | Optional, Standard `60`. Begrenzt die Analyse auf die ersten X Sekunden der Datei – bei langem Material reicht das zum Kalibrieren des Anfangs und spart Zeit gegenüber einer Analyse der kompletten Datei. |

**Rückgabe:** Array von Maps mit `start`, `end` und `duration` (jeweils in Sekunden, auf 0,1s gerundet) für jeden erkannten Stille-Abschnitt. `ErrorVal` bei FFmpeg-Fehler.

```vbx
Dim periods = media.AnalyzeSilence("hoerspiel.mp3", -35, 0.3, 30)

For Each p In periods
    Print "Stille von " & p["start"] & "s bis " & p["end"] & "s (Dauer: " & p["duration"] & "s)"
Next
```

Mehrere Schwellwerte (z. B. `-50`, `-40`, `-35`, `-30`) durchprobieren und vergleichen, bei welchem Wert der Anfangsbereich sauber erkannt wird, ohne schon in die eigentliche Aufnahme reinzuschneiden.

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

## media.CheckTags(file, tags)

Prüft, ob alle angegebenen Tags gesetzt und nicht leer sind.

| Parameter | Beschreibung |
|---|---|
| `file` | Mediendatei |
| `tags` | Array von Tag-Namen (Alias oder kanonisch), die geprüft werden sollen |

**Rückgabe:** Der Dateipfad selbst, sobald **einer** der angegebenen Tags fehlt oder leer ist (praktisch zum direkten Sammeln unvollständiger Dateien in ein Array); ein leerer String, wenn alle angegebenen Tags gesetzt sind. `ErrorVal` bei FFmpeg-Fehler oder nicht unterstütztem Tag-Namen.

```vbx
Dim files = folder.GetFiles("C:\Musik", "*.mp3", true, true)
Dim unvollstaendig = array.Create()

For Each f In files
    Dim missing = media.CheckTags(f, {"artist", "album", "genre"})
    If missing <> "" Then
        unvollstaendig = array.Add(unvollstaendig, missing)
    End If
Next

Print "Dateien mit fehlenden Tags: " & array.Count(unvollstaendig)
```

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