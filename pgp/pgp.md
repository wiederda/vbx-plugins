# 🔑 pgp.* – PGP-Kryptografiefunktionen

Dient zur Erzeugung, Verwaltung und Verwendung von OpenPGP-Schlüsseln für Verschlüsselung und digitale Signaturen.
Schlüssel werden im ASCII-Armor-Format gespeichert und verarbeitet.
Alle Schreiboperationen sind atomar (temp-Datei + Rename). Private Keys werden mit Rechten `0600` gespeichert.

---

## pgp.SetupIdentity(folder, name, email [, password])
- **Konkret:**
  Erstellt ein neues PGP-Schlüsselpaar und speichert es im angegebenen Ordner.
  Erzeugt `id_pgp` (Private Key) und `id_pgp.pub` (Public Key).
  Ohne Passwort wird der Private Key unverschlüsselt gespeichert (Warnung auf Konsole).
- **Parameter:**
  - `folder`: Zielverzeichnis (wird automatisch erstellt mit Rechten `0700`).
  - `name`: Name des Schlüsselinhabers.
  - `email`: E-Mail-Adresse. Leerstring erzeugt `name@vbx.local`.
  - `password`: Optional. Passwort zum Schutz des Private Keys.
- **Rückgabe:**
  `StrVal` (`"OK"`) bei Erfolg, `ErrorVal` bei Fehler.

---

## pgp.Encrypt(msg, pubKeyPath)
- **Konkret:**
  Verschlüsselt eine Textnachricht mit einem PGP-Public-Key.
  Akzeptiert sowohl einen Armor-String als auch einen Dateipfad.
- **Parameter:**
  - `msg`: Zu verschlüsselnder Text.
  - `pubKeyPath`: Public Key als Armor-String oder Pfad zur Schlüsseldatei.
- **Rückgabe:**
  `StrVal` (ASCII-Armor-verschlüsselte Nachricht), `ErrorVal` bei Fehler.

---

## pgp.Decrypt(cipher, keyFolder [, password])
- **Konkret:**
  Entschlüsselt eine PGP-Nachricht mit dem Private Key aus einem Ordner.
  Liest `id_pgp` aus dem angegebenen Ordner.
- **Parameter:**
  - `cipher`: Verschlüsselte Nachricht als Armor-String.
  - `keyFolder`: Ordner der den Private Key `id_pgp` enthält.
  - `password`: Optional. Passwort des Private Keys.
- **Rückgabe:**
  `StrVal` (Klartext), `ErrorVal` bei Fehler oder falschem Passwort.

---

## pgp.Sign(msg, privKey [, password])
- **Konkret:**
  Erzeugt eine abgetrennte PGP-Signatur (Detached Signature) über eine Textnachricht.
  Führende und nachfolgende Whitespace-Zeichen werden vor dem Signieren entfernt.
- **Parameter:**
  - `msg`: Zu signierender Text.
  - `privKey`: Private Key als Armor-String oder Dateipfad.
  - `password`: Optional. Passwort des Private Keys.
- **Rückgabe:**
  `ArrVal`
  Format: `[OK, SignaturArmor, Msg]`

---

## pgp.SignFile(filePath, privKeyArmor [, password])
- **Konkret:**
  Erzeugt eine abgetrennte PGP-Signatur über eine Datei.
- **Parameter:**
  - `filePath`: Pfad zur zu signierenden Datei.
  - `privKeyArmor`: Private Key als Armor-String oder Dateipfad.
  - `password`: Optional. Passwort des Private Keys.
- **Rückgabe:**
  `ArrVal`
  Format: `[OK, SignaturArmor, Msg]`

---

## pgp.Verify(msg, sigArmor, pubKey)
- **Konkret:**
  Verifiziert einen Text gegen eine abgetrennte PGP-Signatur.
  Fehler werden in benutzerfreundliche Meldungen übersetzt (abgelaufen, falscher Schlüssel, Inhalt verändert etc.).
- **Parameter:**
  - `msg`: Ursprünglicher Text.
  - `sigArmor`: Signatur als Armor-String.
  - `pubKey`: Public Key als Armor-String oder Dateipfad.
- **Rückgabe:**
  `ArrVal`
  Format: `[OK, "Gültig", Msg]` bei Erfolg, `[false, "", Fehlermeldung]` bei Fehler.

---

## pgp.VerifyFile(filePath, sigArmor, pubKeyArmor)
- **Konkret:**
  Verifiziert eine Datei gegen eine abgetrennte PGP-Signatur.
  Gibt bei Erfolg den Namen des Unterzeichners zurück.
- **Parameter:**
  - `filePath`: Pfad zur verifizierten Datei.
  - `sigArmor`: Signatur als Armor-String.
  - `pubKeyArmor`: Public Key als Armor-String oder Dateipfad.
- **Rückgabe:**
  `ArrVal`
  Format: `[OK, "Gültig: <Name>", Msg]` bei Erfolg, `[false, "", Fehlermeldung]` bei Fehler.

---

## pgp.ChangePassword(keyFolder [, oldPass, newPass])
- **Konkret:**
  Ändert das Passwort eines Private Keys.
  Leerstring als `newPass` entfernt den Passwortschutz.
- **Parameter:**
  - `keyFolder`: Ordner der `id_pgp` enthält.
  - `oldPass`: Optional. Aktuelles Passwort (leer wenn unverschlüsselt).
  - `newPass`: Optional. Neues Passwort (leer = Passwort entfernen).
- **Rückgabe:**
  `StrVal` (`"OK"`) bei Erfolg, `ErrorVal` bei Fehler.

---

## pgp.ExportKey(privKeyArmor, path [, password])
- **Konkret:**
  Exportiert einen Private Key in eine Datei, optional mit neuem Passwort verschlüsselt.
  Datei wird mit Rechten `0600` gespeichert.
- **Parameter:**
  - `privKeyArmor`: Private Key als Armor-String oder Dateipfad.
  - `path`: Zielpfad für den Export.
  - `password`: Optional. Passwort für die exportierte Datei.
- **Rückgabe:**
  `StrVal` (`"OK"`) bei Erfolg, `ErrorVal` bei Fehler.

---

## pgp.ImportKey(path [, password])
- **Konkret:**
  Lädt einen Private Key aus einer Datei, entschlüsselt ihn und gibt ihn als Armor-String zurück.
  Der zurückgegebene String enthält den Key im Klartext – sollte so kurz wie möglich gehalten werden.
- **Parameter:**
  - `path`: Pfad zur Schlüsseldatei.
  - `password`: Optional. Passwort des Keys.
- **Rückgabe:**
  `StrVal` (entschlüsselter Armor-String), `ErrorVal` bei Fehler.

---

## pgp.GetPublicKey(pathOrKey)
- **Konkret:**
  Extrahiert den öffentlichen Schlüsselteil aus einem Private Key.
  Akzeptiert sowohl einen Armor-String als auch einen Dateipfad.
- **Parameter:**
  - `pathOrKey`: Private Key als Armor-String oder Dateipfad.
- **Rückgabe:**
  `StrVal` (Public Key als Armor-String), `ErrorVal` bei Fehler.