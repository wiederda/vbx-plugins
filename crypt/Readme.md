# 🔐 crypt.* – Kryptografie- & Zufallsfunktionen

Dieses Modul wird als **Plugin** bereitgestellt. Weitere Informationen stehen in **Allgemein**.

Es dient zur kryptografisch sicheren Erzeugung von Zufallswerten, Passwörtern und GUIDs sowie zur AES-256-GCM-Verschlüsselung von Text und Dateien.
Alle Zufallsoperationen nutzen `crypto/rand` (kein `math/rand`).

---

## crypt.HMAC(s, key [, algo])
- **Konkret:**
  Erzeugt einen HMAC (Hash-based Message Authentication Code).
  Geeignet für API-Authentifizierung, Webhook-Validierung und Signaturen.
- **Parameter:**
  - `s`: Zu signierender String.
  - `key`: Geheimer Schlüssel.
  - `algo`: Optional. Hash-Algorithmus (Standard: `"sha256"`). Unterstützt: `"sha256"`, `"sha512"`, `"sha1"`, `"md5"`.
- **Rückgabe:**
  `StrVal` (Hex-String), `ErrorVal` bei unbekanntem Algorithmus.

---

## crypt.Bcrypt(pass [, cost])
- **Konkret:**
  Erzeugt einen sicheren Bcrypt-Hash eines Passworts.
  Bcrypt ist absichtlich langsam und damit sicher für Passwort-Hashing.
  Der gleiche Hash sieht bei jedem Aufruf anders aus (integriertes Salt).
- **Parameter:**
  - `pass`: Zu hashendes Passwort.
  - `cost`: Optional. Kosten-Faktor (Standard: `10`, gültig: `4`–`31`). Höher = langsamer = sicherer.
- **Rückgabe:**
  `StrVal` (Bcrypt-Hash), `ErrorVal` bei Fehler.

---

## crypt.BcryptVerify(pass, hash)
- **Konkret:**
  Prüft ob ein Klartext-Passwort zu einem Bcrypt-Hash passt.
- **Parameter:**
  - `pass`: Klartext-Passwort.
  - `hash`: Bcrypt-Hash (aus `hash.Bcrypt`).
- **Rückgabe:**
  `BoolVal` (`true` wenn Passwort korrekt)

---

## crypt.GUID()
- **Konkret:**
  Generiert eine eindeutige GUID (Version 4, RFC 4122).
  Setzt die Versions- und Variant-Bits korrekt.
- **Rückgabe:**
  `ArrVal`
  Format: `[OK, GUID, Msg]`
  Beispiel: `"550e8400-e29b-41d4-a716-446655440000"`.

---

## crypt.RNGCryptoProvider([length])
- **Konkret:**
  Erzeugt kryptografisch sichere Zufallsbytes und gibt sie als Hex-String zurück.
- **Parameter:**
  - `length`: Optional. Anzahl Bytes (Standard: 16).
- **Rückgabe:**
  `ArrVal`
  Format: `[OK, HexString, Msg]`

---

## crypt.RandomString(length)
- **Konkret:**
  Erzeugt eine kryptografisch zufällige alphanumerische Zeichenfolge (`a-z`, `A-Z`, `0-9`).
  Minimum: 10 Zeichen (kleinere Werte werden auf 10 aufgerundet).
- **Parameter:**
  - `length`: Gewünschte Länge (Minimum: 10).
- **Rückgabe:**
  `StrVal`

---

## crypt.RandomPassword(len [, num, low, up, spec, head])
- **Konkret:**
  Generiert ein kryptografisch zufälliges Passwort mit konfigurierbarer Zeichensatz-Zusammensetzung.
  Stellt sicher, dass jede aktivierte Zeichenklasse mindestens einmal enthalten ist.
  Minimum: 10 Zeichen. Fallback: Kleinbuchstaben, falls kein Charset aktiviert.
- **Parameter:**
  - `len`: Passwortlänge (Minimum: 10).
  - `num`: Optional. `BoolVal` – Ziffern einschließen.
  - `low`: Optional. `BoolVal` – Kleinbuchstaben einschließen.
  - `up`: Optional. `BoolVal` – Großbuchstaben einschließen.
  - `spec`: Optional. `BoolVal` – Sonderzeichen einschließen.
  - `head`: Optional. `BoolVal` – erstes Zeichen erzwungener Buchstabe.
- **Rückgabe:**
  `StrVal`

---

## crypt.AESEncrypt(text, pass)
- **Konkret:**
  Verschlüsselt einen String mit AES-256-GCM.
  Der Schlüssel wird aus dem Passwort via SHA-256 abgeleitet.
  Nonce wird kryptografisch zufällig erzeugt und dem Ciphertext vorangestellt.
  Ausgabe ist Base64-kodiert.
- **Parameter:**
  - `text`: Zu verschlüsselnder Klartext.
  - `pass`: Passwort (wird intern zu 256-Bit-Key gehasht).
- **Rückgabe:**
  `ArrVal`
  Format: `[OK, CipherBase64, Msg]`

---

## crypt.AESDecrypt(cipher, pass)
- **Konkret:**
  Entschlüsselt AES-256-GCM-verschlüsselte Daten (erzeugt von `crypt.AESEncrypt`).
  Extrahiert Nonce automatisch aus den Rohdaten.
  Schlägt bei falschem Passwort oder korrupten Daten fehl.
- **Parameter:**
  - `cipher`: Base64-kodierter Ciphertext.
  - `pass`: Passwort (muss identisch mit dem bei der Verschlüsselung sein).
- **Rückgabe:**
  `ArrVal`
  Format: `[OK, Plaintext, Msg]`

  ---

## crypt.BytesToString(byteArray)
- **Konkret:**
  Wandelt ein Byte-Array (0–255-Werte) in einen UTF-8-String um.
- **Parameter:**
  - `byteArray`: Ein Array vom Typ `KindArr` mit Byte-Werten (0–255).
- **Rückgabe:**
  `StrVal` bei Erfolg, `ErrorVal` bei falschem Argumenttyp oder Nicht-Zahlen-Werten im Array.
- **Hinweis:**
  Der erzeugte String ist ab diesem Moment eine eigene Speicherkopie und **nicht** mehr über `string.Wipe` löschbar (Wipe wirkt nur auf das Byte-Array). Für kurzlebige Verwendung gedacht – direkt vor dem eigentlichen Gebrauch aufrufen und danach mit `string.WipeString` löschen.

---

## crypt.CheckPassword(password [, minLength] [, maxLength] [, requireUpper] [, requireLower] [, requireDigit] [, requireSpecial])
- **Konkret:**
  Prüft ein vom Nutzer gewähltes Passwort gegen konfigurierbare Regeln, statt es blind zu übernehmen. Gibt eine Map mit Detail-Ergebnissen zurück (nicht nur ein Gesamturteil), damit dem Nutzer konkret gesagt werden kann, was fehlt.
- **Parameter:**
  - `password`: Das zu prüfende Passwort.
  - `minLength`: Optional. Mindestlänge in Zeichen (Unicode-Runes, nicht Bytes). Standard: `8`.
  - `maxLength`: Optional. Maximallänge in Zeichen. Standard: `128`.
  - `requireUpper`: Optional. Großbuchstabe erforderlich. Standard: `true`.
  - `requireLower`: Optional. Kleinbuchstabe erforderlich. Standard: `true`.
  - `requireDigit`: Optional. Ziffer erforderlich. Standard: `true`.
  - `requireSpecial`: Optional. Sonderzeichen erforderlich. Standard: `true`.
- **Rückgabe:**
  `Value` vom Typ `KindMap` mit den Feldern:
  - `Valid` (`BoolVal`): Gesamtergebnis.
  - `Length` (`NumVal`): Tatsächliche Zeichenlänge (Runes).
  - `HasUpper`, `HasLower`, `HasDigit`, `HasSpecial` (`BoolVal`): Einzelergebnisse.
  - `Errors` (`KindArr` von `StrVal`): Liste der nicht erfüllten Anforderungen als lesbare deutsche Meldungen.
- **Hinweis:**
  Längenprüfung erfolgt Rune-basiert (nicht Byte-basiert), damit Passwörter mit Umlauten nicht fälschlich als zu lang gewertet werden. Sonderzeichen werden über Unicode-Kategorien (`unicode.IsPunct`/`IsSymbol`) erkannt, nicht über eine feste Zeichenliste.

**Beispiel:**
```vb
#use crypt
Dim UserPw = "hallo123"
Dim Check = crypt.CheckPassword(UserPw, 10)

If Check["Valid"] = false Then
    Print "Passwort ungueltig:"
    For Each Err In Check["Errors"]
        Print "  - " & Err
    Next
Else
    reg.WriteProtectedValue(TestRoot, TestPath, TestName, UserPw)
End If
```

---

## crypt.AESEncryptFile(sourcePath, destPath, pass [, deleteSource])
- **Konkret:**
  Verschlüsselt eine Datei mit AES-256-GCM und speichert das Ergebnis unter `destPath`. Im Gegensatz zu `reg.WriteProtectedValue` (DPAPI) plattformunabhängig – nicht auf Windows beschränkt, keine Bindung an einen bestimmten Benutzer/Rechner. Für Weitergabe an andere Personen/Systeme mit expliziter Schlüsselverwaltung stehen weiterhin PGP/PQC zur Verfügung.
- **Parameter:**
  - `sourcePath`: Pfad zur zu verschlüsselnden Datei.
  - `destPath`: Zielpfad für die verschlüsselte Datei.
  - `pass`: Passwort, aus dem der AES-Schlüssel abgeleitet wird (SHA-256).
  - `deleteSource`: Optional. Löscht die Original-Datei nach erfolgreicher Verschlüsselung. Standard: `false`.
- **Rückgabe:**
  `Value` vom Typ `KindArr` mit `[OK (BoolVal), Msg (StrVal)]`.
- **Hinweis:**
  Format: `nonce + ciphertext`, Base64-frei (Rohbytes werden direkt in die Zieldatei geschrieben). Schreibvorgang ist atomar (`.tmp` + Rename). Die komplette Datei wird beim Verschlüsseln einmal in den RAM geladen (kein Streaming) – für Configs/Dokumente unproblematisch, bei sehr großen Dateien (mehrere GB) ungeeignet.
  Löschen der Originaldatei erfolgt **ausschließlich nach erfolgreichem Schreiben** der verschlüsselten Datei. Schlägt nur das Löschen fehl, wird das als Teilerfolg gemeldet (`OK, aber Original konnte nicht gelöscht werden: ...`), nicht als kompletter Fehler.
  `os.Remove` löscht nur den Verzeichniseintrag, überschreibt aber nicht den Dateiinhalt auf der Platte selbst (kein sicheres Löschen/Shredding).
  **Kein Salt/keine KDF:** Der Schlüssel wird direkt per SHA-256 aus dem Passwort abgeleitet, ohne Salt oder Iterationen (kein PBKDF2/scrypt/Argon2). Für stark zufällige Passwörter (z. B. via `crypt.RandomPassword`) unkritisch, bei potenziell schwachen, nutzergewählten Passwörtern (siehe `crypt.CheckPassword`) ist Offline-Brute-Force leichter möglich als mit einer echten Passwort-KDF.

---

## crypt.AESDecryptFile(sourcePath, destPath, pass [, deleteSource])
- **Konkret:**
  Entschlüsselt eine mit `crypt.AESEncryptFile` verschlüsselte Datei und speichert das Ergebnis unter `destPath`.
- **Parameter:**
  - `sourcePath`: Pfad zur verschlüsselten Datei.
  - `destPath`: Zielpfad für die entschlüsselte Datei.
  - `pass`: Passwort, das beim Verschlüsseln verwendet wurde.
  - `deleteSource`: Optional. Löscht die verschlüsselte Quelldatei nach erfolgreicher Entschlüsselung. Standard: `false`.
- **Rückgabe:**
  `Value` vom Typ `KindArr` mit `[OK (BoolVal), Msg (StrVal)]`.
- **Hinweis:**
  Bei falschem Passwort oder beschädigter/manipulierter Datei schlägt `gcm.Open` fehl (GCM erkennt das dank Authenticated Encryption zuverlässig) – Rückgabe dann `[false, "Entschlüsselung fehlgeschlagen (falsches Passwort oder Datei beschädigt): ..."]`.
  Der entschlüsselte Inhalt wird nach dem Schreiben aus dem Speicher genullt (`plain[i] = 0` in einem `defer`), analog zu `string.Wipe`.
  Gleiche Lösch-Semantik wie bei `crypt.AESEncryptFile`: Löschen erfolgt erst nach erfolgreichem Schreiben, Fehler beim Löschen wird als Teilerfolg gemeldet, kein sicheres Shredding.

**Beispiel:**
```vb
#use crypt
Dim EncResult = crypt.AESEncryptFile("C:\Daten\geheim.txt", "C:\Daten\geheim.enc", "MeinSicheresPasswort", true)
If EncResult(0) = false Then
    Print "Verschlüsselung fehlgeschlagen: " & EncResult(1)
End If

Dim DecResult = crypt.AESDecryptFile("C:\Daten\geheim.enc", "C:\Daten\entschluesselt.txt", "MeinSicheresPasswort")
If DecResult(0) = false Then
    Print "Entschlüsselung fehlgeschlagen: " & DecResult(1)
End If
```

---

## crypt.AESDecryptFileToString(sourcePath, pass)
- **Konkret:**
  Entschlüsselt eine mit `crypt.AESEncryptFile` verschlüsselte Datei direkt im Speicher und gibt den Inhalt zurück. Im Gegensatz zu `crypt.AESDecryptFile` wird zu keinem Zeitpunkt eine Klartext-Version auf die Platte geschrieben – die Datei bleibt verschlüsselt liegen, nur der Inhalt wird kurzzeitig im RAM verfügbar gemacht. Sinnvoll z. B. für verschlüsselte Config- oder Textdateien, die nur eingelesen, aber nicht dauerhaft als Klartext-Datei benötigt werden.
- **Parameter:**
  - `sourcePath`: Pfad zur verschlüsselten Datei.
  - `pass`: Passwort, das beim Verschlüsseln verwendet wurde.
- **Rückgabe:**
  `Value` vom Typ `KindArr` mit `[OK (BoolVal), Content (StrVal), Msg (StrVal)]`.
- **Hinweis:**
  Der rohe entschlüsselte Byte-Buffer wird intern genullt, sobald die (technisch unvermeidliche) String-Kopie für die Rückgabe erzeugt wurde. Der zurückgegebene `Content`-String selbst bleibt danach eine normale Speicherkopie – für konsequente Speicher-Hygiene nach Gebrauch mit `string.WipeString(Content)` behandeln (siehe Lifecycle unten).
  Bei falschem Passwort oder beschädigter/manipulierter Datei liefert `gcm.Open` einen Fehler zurück (GCM erkennt das zuverlässig dank Authenticated Encryption).

**Beispiel:**
```vb
#use crypt
Dim Result = crypt.AESDecryptFileToString("C:\Daten\geheim.enc", "MeinSicheresPasswort")
If Result(0) = false Then
    Print "Fehler: " & Result(2)
Else
    Dim Content = Result(1)
    Print "Inhalt: " & Content

    ' Nach Gebrauch aufräumen
    string.WipeString(Content)
End If
```

---

## Empfohlener Lifecycle für sensible Werte

Um einen per DPAPI gespeicherten Wert möglichst spurenarm zu verwenden:

```vb
#use reg,crypt
Dim SecretBytes = reg.ReadProtectedValueBytes(TestRoot, TestPath, TestName)
Dim SecretStr = crypt.BytesToString(SecretBytes)

' ... SecretStr sofort und einmalig verwenden (z.B. für einen API-Call) ...
' KEINE Verkettung oder Kopie von SecretStr vor dem Wipe!

string.Wipe(SecretBytes)       ' Byte-Array nullen
string.WipeString(SecretStr)   ' String-Backing-Array nullen
```

**Was damit erreicht wird:**
Beide Speicherkopien (Rohbytes und der daraus erzeugte String) werden aktiv überschrieben, statt unkontrolliert im Speicher bis zur GC-Freigabe liegen zu bleiben.

**Was damit *nicht* erreicht wird:**
Vollständiger Schutz gegen einen Memory-Dump *während* der kurzen Nutzung von `SecretStr` ist technisch nicht möglich, solange der Wert überhaupt als normaler String verwendet wird (z. B. für einen HTTP-Header). Der Ansatz minimiert das Zeitfenster, in dem der Klartext im Speicher steht, ersetzt aber keine dedizierte Secure-Memory-Lösung. Für die meisten Zwecke (Schutz gegen Registry-Auslesen durch andere Nutzer/Prozesse, Schutz gegen nachträgliche Speicheranalyse nach Programmende) ist das aber ausreichend.