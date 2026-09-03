# 📄 yaml.* – YAML-Funktionen

Dient zur Validierung, Abfrage und Manipulation von YAML-Strukturen.
Pfadangaben nutzen Punkt-Notation für Maps und numerische Indizes für Arrays.

---

## yaml.Parse(yamlContent)
- **Konkret:**
  Prüft, ob ein YAML-String syntaktisch valide ist.
  Gibt bei Fehler die Fehlermeldung mit Zeilenangabe zurück.
- **Parameter:**
  - `yamlContent`: YAML-String.
- **Rückgabe:**
  `BoolVal` (`true`) bei gültigem YAML.
  `StrVal` mit Präfix `"ERROR: ..."` bei Syntaxfehler.

---

## yaml.ParseAll(yamlContent)
- **Konkret:**
  Prüft den gesamten YAML-Stream inklusive mehrerer Dokumente (getrennt durch `---`).
  Bricht beim ersten fehlerhaften Dokument ab und gibt dessen Fehlermeldung zurück.
- **Parameter:**
  - `yamlContent`: YAML-Stream mit einem oder mehreren Dokumenten.
- **Rückgabe:**
  `BoolVal` (`true`) wenn der gesamte Stream valide ist.
  `StrVal` mit Präfix `"ERROR: ..."` beim ersten Fehler.

---

## yaml.Get(yaml, path)
- **Konkret:**
  Liest einen Wert aus einer YAML-Struktur anhand eines Pfades.
  Map-Schlüssel werden mit Punkten getrennt, Array-Indizes als Zahl angegeben.
- **Parameter:**
  - `yaml`: YAML-String.
  - `path`: Pfad zum gesuchten Wert (z. B. `"server.host"`, `"items.0"`).
- **Rückgabe:**
  Gefundener Wert als entsprechender `Value`-Typ.
  Leerer `Value` wenn der Pfad nicht existiert.

---

## yaml.Set(yaml, path, val)
- **Konkret:**
  Setzt einen Wert in einer YAML-Struktur und gibt das aktualisierte YAML zurück.
  Fehlende Zwischenpfade werden als Maps angelegt.
- **Parameter:**
  - `yaml`: YAML-String.
  - `path`: Pfad zum zu setzenden Wert (z. B. `"server.port"`, `"items.0"`).
  - `val`: Zu setzender Wert.
- **Rückgabe:**
  `StrVal` mit dem aktualisierten YAML-Dokument, `ErrorVal` bei Fehler.

---

## yaml.Stringify(value)
- **Konkret:**
  Konvertiert eine interne Map- oder Array-Struktur in einen formatierten YAML-String.
- **Parameter:**
  - `value`: `KindMap` oder `KindArr`.
- **Rückgabe:**
  `StrVal` (formatiertes YAML), `ErrorVal` bei Fehler.