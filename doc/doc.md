# 📄 doc.* – DOCX- & ODT-Dokumentfunktionen

Dieses Modul wird als **Plugin** bereitgestellt. Weitere Informationen stehen in **Allgemein**.

Es dient zum Lesen und Erzeugen von Word-Dokumenten (`.docx`) und OpenDocument-Texten (`.odt`).
Mit "doc" ist immer das moderne `.docx` gemeint, nicht das alte binäre `.doc`.

Beim Schreiben wird das Dokument Schritt für Schritt im Speicher aufgebaut (`New`, `Heading`, `Text`, `List`, `Table`, …) und erst mit `Save` in eine Datei geschrieben. Das Format ergibt sich aus der Dateiendung. Es sind keine Markdown-Kenntnisse nötig.

---

## doc.ReadText(pfad [, tabellenMarker] [, markdown])
- **Konkret:**
  Liest den Text eines DOCX- oder ODT-Dokuments.
  Jeder Absatz und jede Überschrift ist eine Zeile; Tabulatoren und Zeilenumbrüche innerhalb eines Absatzes bleiben erhalten.
  Kommentare und Fußnoten in ODT-Dateien werden übersprungen.
  Tabellen erscheinen standardmäßig nicht im Text. Mit `tabellenMarker` steht an ihrer Stelle eine Zeile `[Tabelle 1]`, `[Tabelle 2]`, … (nur Tabellen der obersten Ebene werden gezählt). Den Inhalt liefert `doc.Tables`.
  Mit `markdown` wird das Dokument stattdessen als Markdown ausgegeben (Überschriften, Listen, Tabellen); `tabellenMarker` hat dann keine Wirkung.
- **Hinweis:**
  Wer nur Markdown möchte, muss `tabellenMarker` trotzdem angeben: `doc.ReadText(pfad, False, True)`.
  Formatierungen (fett, kursiv, unterstrichen) erscheinen in der normalen Textausgabe nicht.
- **Parameter:**
  - `pfad`: Pfad zur `.docx`- oder `.odt`-Datei.
  - `tabellenMarker`: Optional. `BoolVal` – bei `true` werden Platzhalterzeilen `[Tabelle n]` eingefügt (Standard `false`).
  - `markdown`: Optional. `BoolVal` – bei `true` Ausgabe als Markdown (Standard `false`).
- **Rückgabe:**
  `StrVal`
  Dokumenttext, `ErrorVal` bei Fehler (Datei nicht lesbar, falsche Endung, kein gültiges Dokument).

---

## doc.Convert(quelle, ziel)
- **Konkret:**
  Konvertiert ein Dokument zwischen DOCX und ODT.
  Die Richtung ergibt sich aus den Dateiendungen von `quelle` und `ziel`.
- **Hinweis:**
  Vorerst nur für Dokumente gedacht, die mit diesem Plugin selbst erzeugt wurden.
- **Parameter:**
  - `quelle`: Pfad zur vorhandenen Datei (`.docx` oder `.odt`).
  - `ziel`: Pfad der neuen Datei (`.odt` bzw. `.docx`).
- **Rückgabe:**
  `BoolVal`
  `true` bei Erfolg, `ErrorVal` bei Fehler.

---

## doc.Tables(pfad)
- **Konkret:**
  Liest alle Tabellen eines DOCX- oder ODT-Dokuments.
  Zeilen mit weniger Zellen als die breiteste Zeile werden mit leeren Strings aufgefüllt, sodass jede Tabelle rechteckig ist.
  Mehrere Absätze in einer Zelle werden mit Zeilenumbruch verbunden.
  Verschachtelte Tabellen werden nicht separat geliefert.
- **Parameter:**
  - `pfad`: Pfad zur `.docx`- oder `.odt`-Datei.
- **Rückgabe:**
  `ArrVal`
  Ein 2D-Array aus `StrVal` pro Tabelle, in Dokumentreihenfolge. Ohne Tabellen ein leeres Array, `ErrorVal` bei Fehler.

---

## doc.New()
- **Konkret:**
  Beginnt ein neues, leeres Dokument.
  Ein vorher aufgebautes Dokument und die gesetzte Schrift werden verworfen (zurück auf Arial, 11 pt).
  Ein Dokument wird auch automatisch begonnen, wenn `Font`, `Heading`, `Text` usw. ohne vorheriges `New` aufgerufen werden.
- **Hinweis:**
  Nach `doc.Save` bleibt das Dokument im Speicher. Für ein zweites, unabhängiges Dokument zuerst `doc.New()` aufrufen.
- **Parameter:**
  Keine.
- **Rückgabe:**
  `BoolVal`
  Immer `true`.

---

## doc.Font(name [, size])
- **Konkret:**
  Setzt Schriftart und optional Schriftgröße für das gesamte aktuelle Dokument.
  Überschriften werden automatisch größer dargestellt (Ebene 1: 160 %, Ebene 2: 130 %, Ebene 3: 115 % der Grundgröße).
  Die Schrift gilt für das ganze Dokument, nicht erst ab dem Aufruf; bei mehreren Aufrufen zählt der zuletzt gesetzte Wert. Ohne `Font` gilt Arial 11 pt.
- **Hinweis:**
  Die Schrift wird nur referenziert, nicht eingebettet. Ist sie auf dem Zielrechner nicht installiert, ersetzt die Textverarbeitung sie.
- **Parameter:**
  - `name`: Schriftname, z. B. `"Calibri"`. Es gibt keine feste Liste: jeder Name wird akzeptiert, wirksam ist er aber nur, wenn die Schrift auf dem Zielrechner installiert ist (maximal 100 Zeichen; Anführungszeichen, `<`, `>` und `&` werden entfernt).
  - `size`: Optional. `NumVal` – Größe in pt, erlaubt 4 bis 200. Ohne Angabe bleibt die bisherige Größe.
- **Rückgabe:**
  `BoolVal`
  `true` bei Erfolg, `ErrorVal` bei leerem/zu langem Namen oder ungültiger Größe.

---

## doc.Heading(text, level [, underline])
- **Konkret:**
  Fügt eine Überschrift hinzu.
  Es werden die Formatvorlagen "Überschrift 1–3" verwendet, die Überschriften erscheinen daher in der Gliederung der Textverarbeitung.
  Überschriften sind immer fett; optional kann die ganze Überschrift unterstrichen werden.
- **Parameter:**
  - `text`: Text der Überschrift.
  - `level`: `NumVal` – Ebene 1, 2 oder 3 (nur ganze Zahlen; Standard 1 bei Auslassung).
  - `underline`: Optional. `BoolVal` – bei `true` wird die Überschrift unterstrichen (Standard `false`).
- **Rückgabe:**
  `BoolVal`
  `true` bei Erfolg, `ErrorVal` bei ungültiger Ebene oder falschem Parametertyp.

---

## doc.Text(text [, bold] [, italic] [, underline])
- **Konkret:**
  Fügt einen Absatz hinzu.
  Die Formatierung gilt für den ganzen Absatz; gemischte Formatierung innerhalb eines Absatzes ist nicht möglich.
  Zeilenumbrüche im Text werden zu Zeilenumbrüchen im Absatz, Tabulatoren zu Tabulatoren.
- **Hinweis:**
  Soll nur `italic` oder `underline` gesetzt werden, müssen die davorliegenden Parameter mit angegeben werden, z. B. `doc.Text("Hinweis", False, True)`.
- **Parameter:**
  - `text`: Absatztext. Zahlen und Booleans werden in Text umgewandelt.
  - `bold`: Optional. `BoolVal` – fett (Standard `false`).
  - `italic`: Optional. `BoolVal` – kursiv (Standard `false`).
  - `underline`: Optional. `BoolVal` – unterstrichen (Standard `false`).
- **Rückgabe:**
  `BoolVal`
  `true` bei Erfolg, `ErrorVal` bei falschem Parametertyp.

---

## doc.List(array [, numbered])
- **Konkret:**
  Fügt eine Aufzählung oder nummerierte Liste hinzu.
  Jedes Array-Element wird ein Listenpunkt.
  Jede nummerierte Liste beginnt wieder bei 1.
- **Parameter:**
  - `array`: `ArrVal` mit den Einträgen (mindestens einer). Zahlen und Booleans werden in Text umgewandelt.
  - `numbered`: Optional. `BoolVal` – bei `true` nummeriert, sonst Aufzählungspunkte (Standard `false`).
- **Rückgabe:**
  `BoolVal`
  `true` bei Erfolg, `ErrorVal` wenn kein Array übergeben wurde oder es leer ist.

---

## doc.Table(array [, header])
- **Konkret:**
  Fügt eine Tabelle mit Rahmenlinien hinzu; die Spaltenbreite wird gleichmäßig auf die Seitenbreite verteilt.
  Eingabe ist ein 2D-Array oder ein Array aus Arrays (jede Zeile ein Array). Ein einfaches eindimensionales Array wird als **eine** Zeile behandelt.
  Zeilen mit weniger Zellen als die breiteste Zeile werden mit leeren Zellen aufgefüllt.
  Nach der Tabelle folgt automatisch eine Leerzeile.
- **Hinweis:**
  `header` ist standardmäßig `true`. Eine Tabelle ohne Kopfzeile braucht ausdrücklich `False`.
- **Parameter:**
  - `array`: `ArrVal` – Tabellendaten.
  - `header`: Optional. `BoolVal` – erste Zeile ist Kopfzeile (fett, grau hinterlegt, im DOCX bei Seitenumbruch wiederholt). Standard `true`.
- **Rückgabe:**
  `BoolVal`
  `true` bei Erfolg, `ErrorVal` bei falschem Typ oder leerer Tabelle.

---

## doc.PageBreak()
- **Konkret:**
  Fügt einen Seitenumbruch ein. Der nächste Inhalt beginnt auf einer neuen Seite.
- **Parameter:**
  Keine. Mit Argumenten wird ein Fehler zurückgegeben.
- **Rückgabe:**
  `BoolVal`
  `true` bei Erfolg, `ErrorVal` bei übergebenen Argumenten.

---

## doc.Save(pfad)
- **Konkret:**
  Schreibt das aktuelle Dokument als `.docx` oder `.odt`; das Format ergibt sich aus der Endung.
  Eine vorhandene Datei wird überschrieben.
  Geschrieben wird atomar (erst temporäre Datei `<pfad>.tmp`, dann Umbenennen), damit bei einem Fehler keine halbfertige Datei entsteht.
  Das Dokument hat A4-Format mit Seitenrändern und deutscher Spracheinstellung.
- **Parameter:**
  - `pfad`: Zielpfad mit Endung `.docx` oder `.odt`. Der Zielordner muss existieren.
- **Rückgabe:**
  `BoolVal`
  `true` bei Erfolg, `ErrorVal` bei leerem Dokument, anderer Endung oder Schreibfehler.

---

## doc.Write(pfad, text)
- **Konkret:**
  Kurzform: Schreibt einen Text direkt als neues Dokument (Arial 11 pt), ohne `New`/`Text`/`Save`.
  Der Text wird an Leerzeilen in Absätze zerlegt; einzelne Zeilenumbrüche innerhalb eines Absatzes bleiben erhalten.
  Das aktuell aufgebaute Dokument bleibt unberührt.
- **Parameter:**
  - `pfad`: Zielpfad mit Endung `.docx` oder `.odt`.
  - `text`: Inhalt des Dokuments.
- **Rückgabe:**
  `BoolVal`
  `true` bei Erfolg, `ErrorVal` bei leerem Text, falscher Endung oder Schreibfehler.

---

## Nicht unterstützt
- Bilder, Links, Kopf-/Fußzeilen, Seitenzahlen, Inhaltsverzeichnis
- Textausrichtung, Farben, unterschiedliche Schriften pro Absatz
- Gemischte Formatierung innerhalb eines Absatzes
- Verschachtelte Listen und Tabellen
- Das alte binäre `.doc`-Format