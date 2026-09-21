# 🖼️ steg.* – Steganografie-Funktionen

Dieses Modul wird als **Plugin** bereitgestellt. Weitere Informationen stehen in **Allgemein**.

Es dient zum verdeckten Einbetten und Extrahieren von Daten in Bilddateien mittels **LSB-Steganografie** (Least Significant Bit).

Unterstützte Bildformate:

* BMP
* PNG

Die Nutzdaten werden nicht sequenziell im Bild abgelegt. Ihre Positionen werden anhand des Seeds über eine deterministische **AES-CTR-basierte Permutation** und einen Seed-abhängigen Stride verteilt. Zum Extrahieren muss daher derselbe Seed verwendet werden.

---

## steg.Inject(inPath, outPath, dataB64, seed)

### Konkret

Bettet Base64-kodierte Daten in ein Bild ein.

* Die Base64-Daten werden vor dem Einbetten dekodiert.
* BMP: Einbettung in die LSBs der Bild-/Pixelbytes.
* PNG: Einbettung in die LSBs des Rot-Kanals.
* Die Positionen werden anhand von Seed und Stride deterministisch verteilt.
* Der Payload beginnt mit einem internen `STEG`-Header und der Länge der Nutzdaten.
* Die Zieldatei wird über eine temporäre Datei geschrieben und anschließend atomar umbenannt.

### Parameter

* `inPath`: Quelldatei (BMP oder PNG).
* `outPath`: Zieldatei.
* `dataB64`: Einzubettende Daten als Base64-String.
* `seed`: Seed-String zur Bestimmung von Permutation und Stride.

### Rückgabe

`ArrVal`

Format:

```text
[OK, Seed, Msg]
```

* `OK`: `true` bei erfolgreichem Einbetten.
* `Seed`: verwendeter Seed.
* `Msg`: Fehlermeldung bei `OK = false`, sonst leer.

---

## steg.Extract(path, seed)

### Konkret

Extrahiert eingebettete Daten aus einem Bild.

Der beim Einbetten verwendete Seed muss identisch sein. Der Seed bestimmt die Positionen des Payloads und ermöglicht dadurch die korrekte Rekonstruktion der eingebetteten Daten.

Wird kein gültiger `STEG`-Payload gefunden, schlägt die Extraktion fehl.

### Parameter

* `path`: Quelldatei (BMP oder PNG).
* `seed`: Seed-String.

### Rückgabe

`ArrVal`

Format:

```text
[OK, DataBase64, Msg]
```

* `OK`: `true` bei erfolgreicher Extraktion.
* `DataBase64`: extrahierte Nutzdaten als Base64-String.
* `Msg`: Fehlermeldung bei `OK = false`, sonst leer.

---

## steg.GenerateSeed(pass, salt)

### Konkret

Erzeugt einen deterministischen Seed aus Passwort und Salt mittels **HMAC-SHA256**.

Gleiche Eingaben erzeugen immer denselben Seed.

Der erzeugte SHA-256-Wert wird als Hex-String zurückgegeben.

### Parameter

* `pass`: Passwort bzw. Schlüsselwert für die HMAC-Berechnung.
* `salt`: Salt-Wert.

### Rückgabe

`StrVal`

Ein hex-kodierter **64-Zeichen-Seed**.

---

## steg.GetCapacity(path, dataLen, seed)

### Konkret

Prüft, ob ein Bild groß genug für einen Payload der angegebenen Größe ist.

Die Berechnung berücksichtigt:

* den internen 8-Byte-Header,
* den vom Seed abhängigen Stride,
* die tatsächlich verfügbaren Bild-/Pixelbytes.

Zusätzlich wird ein Netto-Wert mit **35 % Sicherheitspuffer** berechnet.

### Hinweis

`dataLen` ist die **rohe Byte-Länge der Nutzdaten**, nicht die Länge der Base64-Darstellung.

Beispiel:

```text
Nutzdaten:      100 Bytes
Base64-Länge:   größer als 100 Bytes
dataLen:        100
```

### Parameter

* `path`: Zu prüfendes Bild (BMP oder PNG).
* `dataLen`: Geplante Größe der rohen Nutzdaten in Bytes.
* `seed`: Seed-String; beeinflusst den verwendeten Stride.

### Rückgabe

`ArrVal`

Format:

```text
[OK, NettoBytes, Msg, BruttoBytes]
```

* `OK`: Gibt an, ob die angegebene Datenmenge in das Bild passt.
* `NettoBytes`: verfügbare Kapazität mit 35 % Sicherheitspuffer.
* `Msg`: Fehlermeldung, falls die Prüfung fehlschlägt.
* `BruttoBytes`: maximale theoretische Rohkapazität ohne Sicherheitspuffer.

`NettoBytes` entspricht damit ungefähr **65 % der Bruttokapazität**.
