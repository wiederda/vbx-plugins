# 📊 data.* – Konvertierungsfunktionen

Dieses Modul wird als **Plugin** bereitgestellt. Weitere Informationen stehen in **Allgemein**.

Es stellt Funktionen zur Umrechnung von Datenmengen, Leistungs- und Zeiteinheiten sowie zur Formatierung von Sekunden bereit.

---

## Datenmengen (SI, Basis 1000)

### data.ByteToKb(val)

**Konvertiert Byte in Kilobyte (Basis 1000).**

**Rückgabe:** `NumVal`

---

### data.KbToMb(val)

**Konvertiert Kilobyte in Megabyte (Basis 1000).**

**Rückgabe:** `NumVal`

---

### data.MbToGb(val)

**Konvertiert Megabyte in Gigabyte (Basis 1000).**

**Rückgabe:** `NumVal`

---

### data.GbToTb(val)

**Konvertiert Gigabyte in Terabyte (Basis 1000).**

**Rückgabe:** `NumVal`

---

## Datenmengen (Binär, Basis 1024)

### data.ByteToKiB(val)

**Konvertiert Byte in Kibibyte (Basis 1024).**

**Rückgabe:** `NumVal`

---

### data.KiBToMiB(val)

**Konvertiert Kibibyte in Mebibyte (Basis 1024).**

**Rückgabe:** `NumVal`

---

### data.MiBToGiB(val)

**Konvertiert Mebibyte in Gibibyte (Basis 1024).**

**Rückgabe:** `NumVal`

---

### data.GiBToTiB(val)

**Konvertiert Gibibyte in Tebibyte (Basis 1024).**

**Rückgabe:** `NumVal`

---

## Leistung

### data.WattToKilowatt(val)

**Konvertiert Watt in Kilowatt.**

**Rückgabe:** `NumVal`

---

### data.KilowattToWatt(val)

**Konvertiert Kilowatt in Watt.**

**Rückgabe:** `NumVal`

---

## Zeit

### data.MinutesToHours(val)

**Konvertiert Minuten in Stunden.**

**Rückgabe:** `NumVal`

---

### data.HoursToMinutes(val)

**Konvertiert Stunden in Minuten.**

**Rückgabe:** `NumVal`

---

### data.SecondsToDays(val)

**Konvertiert Sekunden in Tage.**

**Rückgabe:** `NumVal`

---

### data.DaysToSeconds(val)

**Konvertiert Tage in Sekunden.**

**Rückgabe:** `NumVal`

---

## Formatierung

### data.FormatSeconds(seconds)

**Formatiert eine Anzahl von Sekunden als Zeitangabe im Format `HH:MM:SS`.**

**Parameter:**

* `seconds` – Anzahl der Sekunden.

**Rückgabe:** `StrVal`

**Beispiel:**

```text
data.FormatSeconds(3661)
```

Ergebnis:

```text
01:01:01
```
