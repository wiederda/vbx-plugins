# 📄 xml.* – XML-Funktionen

Dient zum Laden, Lesen, Schreiben und Validieren von XML-Dateien.
Arbeitet mit einem internen Dokumenten-Zustand – `xml.Load` muss vor allen anderen Operationen aufgerufen werden.
Pfade werden mit Punkt-Notation angegeben. Index-Zugriff auf gleichnamige Geschwisterknoten: `user[1]`.
Thread-sicher via `sync.RWMutex`.

---

## xml.Load(path)
- **Konkret:**
  Lädt eine XML-Datei in den Arbeitsspeicher.
  Gibt einen Fehler zurück wenn die Datei nicht lesbar oder das XML korrupt ist.
- **Parameter:**
  - `path`: Pfad zur XML-Datei.
- **Rückgabe:**
  `StrVal` (`"OK"`) bei Erfolg, `ErrorVal` bei Fehler.

---

## xml.Save([path])
- **Konkret:**
  Speichert die aktuelle XML-Struktur zurück in die Datei.
  Mit Pfadangabe wird der neue Pfad als Ziel gesetzt.
  Ohne Pfad wird der zuletzt geladene Pfad verwendet.
- **Parameter:**
  - `path`: Optional. Neuer Zielpfad.
- **Rückgabe:**
  `StrVal` (`"OK"`) bei Erfolg, `ErrorVal` bei Fehler.

---

## xml.Parse(xmlContent)
- **Konkret:**
  Prüft einen XML-String auf syntaktische Korrektheit ohne ihn zu laden.
  Liest den gesamten Token-Stream bis EOF.
- **Parameter:**
  - `xmlContent`: XML als String.
- **Rückgabe:**
  `BoolVal` (`true`) wenn valide, `ErrorVal` mit Fehlermeldung bei Syntaxfehler.

---

## xml.Get(xpath)
- **Konkret:**
  Liest den Textinhalt eines Knotens oder den Wert eines Attributs.
  Attributzugriff via `@attributname` am Ende des Pfades (z. B. `"root.user@id"`).
- **Parameter:**
  - `xpath`: Pfad zum Knoten oder Attribut.
- **Rückgabe:**
  `StrVal`
  Leerer String wenn Pfad nicht gefunden.

---

## xml.Set(xpath, value)
- **Konkret:**
  Setzt den Textinhalt eines Knotens oder den Wert eines Attributs.
  Fehlende Knoten entlang des Pfades werden automatisch angelegt.
  Attributzugriff via `@attributname` (z. B. `"root.user@id"`).
  Bestehender Content anderer Knoten wird nicht verändert.
- **Parameter:**
  - `xpath`: Pfad zum Knoten oder Attribut.
  - `value`: Zu setzender Wert.
- **Rückgabe:**
  `StrVal` (`"OK"`) bei Erfolg, `ErrorVal` bei Fehler.

---

## xml.Delete(xpath)
- **Konkret:**
  Löscht den angegebenen Knoten und alle seine Unterknoten.
- **Parameter:**
  - `xpath`: Pfad zum zu löschenden Knoten.
- **Rückgabe:**
  `StrVal` (`"OK"`) bei Erfolg, `ErrorVal` wenn nicht gefunden.

---

## xml.Count(xpath)
- **Konkret:**
  Zählt wie viele Geschwisterknoten mit demselben Namen unter dem Elternknoten existieren.
  Nützlich um die Anzahl gleichnamiger Elemente zu ermitteln bevor auf sie mit Index zugegriffen wird.
- **Parameter:**
  - `xpath`: Pfad zum zu zählenden Element.
- **Rückgabe:**
  `NumVal`

---

## xml.ToMap([xpath])
- **Konkret:**
  Wandelt den geladenen XML-Baum (oder einen Teilbaum ab `xpath`) in eine verschachtelte Map-Struktur um.
  Attribute werden unter dem Schlüssel `@attributname` abgelegt, Textinhalt unter `_text` (nur wenn der Knoten zusätzlich Kinder oder Attribute hat – reine Blattknoten liefern den Content direkt als String).
  Mehrere Geschwisterknoten mit demselben Namen werden zu einem Array gruppiert.
  Ist rein lesend und liefert einen Snapshot zum Zeitpunkt des Aufrufs – spätere `xml.Set`-Änderungen spiegeln sich nicht automatisch in der zurückgegebenen Map wider.
- **Parameter:**
  - `xpath`: Optional. Pfad zum Startknoten. Ohne Angabe wird der gesamte Baum ab Root konvertiert.
- **Rückgabe:**
  `MapVal` (verschachtelt) oder `ArrVal`/`StrVal` je nach Knotenstruktur.
  `ErrorVal` wenn kein XML geladen ist oder der Pfad nicht gefunden wurde.

---

## xml.Keys([xpath])
- **Konkret:**
  Gibt die Namen aller direkten Unterknoten eines Knotens zurück.
  Ohne Parameter: direkte Unterknoten des Root-Elements.
- **Parameter:**
  - `xpath`: Optional. Pfad zum Elternknoten.
- **Rückgabe:**
  `ArrVal`
  Array von `StrVal`-Einträgen (Knotennamen).

---

## Pfad-Syntax

```
root.child.subchild          ' Einfacher Pfad
root.user[0]                 ' Erster user-Knoten (0-basiert)
root.user[1]                 ' Zweiter user-Knoten
root.user@id                 ' Attribut 'id' des user-Knotens
root.users.user[2]@name      ' Attribut 'name' des dritten user-Knotens
```