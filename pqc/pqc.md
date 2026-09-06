# 🔐 pqc.* – Post-Quantum-Kryptografiefunktionen

Dient zur quantensicheren Verschlüsselung und digitalen Signierung.
Nutzt **ML-KEM-768** (CRYSTALS-Kyber) für Schlüsselaustausch und **ML-DSA-65** (CRYSTALS-Dilithium) für Signaturen.
Alle Schlüssel werden als Base64-Strings übergeben und zurückgegeben.
Private Keys auf der Festplatte werden mit AES-256-GCM + Argon2id verschlüsselt.

---

## Schlüsselpaar-Erzeugung (im Arbeitsspeicher)

## pqc.GenerateKeyPair()
- **Konkret:**
  Erzeugt ein ML-KEM-768 Schlüsselpaar für Verschlüsselung und Schlüsselaustausch.
- **Rückgabe:**
  `ArrVal` mit `[PublicKeyB64, PrivateKeyB64]` bei Erfolg, `ErrorVal` bei Fehler.

---

## pqc.GenerateSigKeyPair()
- **Konkret:**
  Erzeugt ein ML-DSA-65 Schlüsselpaar für digitale Signaturen.
- **Rückgabe:**
  `ArrVal` mit `[PublicKeyB64, PrivateKeyB64]` bei Erfolg, `ErrorVal` bei Fehler.

---

## Schlüsselpaar-Erzeugung (auf Festplatte)

## pqc.SetupIdentity(folder [, encrypt])
- **Konkret:**
  Erzeugt ein ML-KEM-768 Schlüsselpaar und speichert es im angegebenen Ordner.
  Erzeugt `id_pqc` (Private Key) und `id_pqc.pub` (Public Key).
  Mit `encrypt = true` (Standard) wird der Private Key mit einem zufälligen Passwort verschlüsselt.
  Das Passwort wird in `id_pqc.key` gespeichert – diese Datei sollte nach dem Notieren gelöscht werden.
  Ohne Verschlüsselung erscheint eine Sicherheitswarnung auf der Konsole.
- **Parameter:**
  - `folder`: Zielverzeichnis (wird mit Rechten `0700` erstellt).
  - `encrypt`: Optional. `BoolVal` (Standard: `true`).
- **Rückgabe:**
  `StrVal` (`"OK"`) bei Erfolg, `ErrorVal` bei Fehler.

---

## pqc.SetupSignIdentity(folder [, encrypt])
- **Konkret:**
  Erzeugt ein ML-DSA-65 Schlüsselpaar und speichert es im angegebenen Ordner.
  Erzeugt `id_sig` (Private Key) und `id_sig.pub` (Public Key).
  Sonst identisch zu `pqc.SetupIdentity`.
- **Parameter:**
  - `folder`: Zielverzeichnis.
  - `encrypt`: Optional. `BoolVal` (Standard: `true`).
- **Rückgabe:**
  `StrVal` (`"OK"`) bei Erfolg, `ErrorVal` bei Fehler.

---

## Verschlüsselung (ML-KEM)

## pqc.Encapsulate(pubKeyB64)
- **Konkret:**
  Erzeugt ein zufälliges Shared Secret und kapselt es für einen ML-KEM Public Key.
  Das Shared Secret kann als symmetrischer Schlüssel (z. B. für AES) genutzt werden.
- **Parameter:**
  - `pubKeyB64`: ML-KEM Public Key als Base64-String.
- **Rückgabe:**
  `ArrVal` mit `[CiphertextB64, SharedSecretB64]` bei Erfolg, `ErrorVal` bei Fehler.

---

## pqc.Decapsulate(ciphertextB64, privKeyB64)
- **Konkret:**
  Entkapselt ein Shared Secret mit dem zugehörigen ML-KEM Private Key.
  Das Ergebnis ist dasselbe Shared Secret wie bei `pqc.Encapsulate`.
- **Parameter:**
  - `ciphertextB64`: Ciphertext aus `pqc.Encapsulate`.
  - `privKeyB64`: ML-KEM Private Key als Base64-String.
- **Rückgabe:**
  `StrVal` (SharedSecretB64) bei Erfolg, `ErrorVal` bei Fehler.

---

## Signaturen (ML-DSA)

## pqc.Sign(msg, privKeyB64)
- **Konkret:**
  Signiert eine Textnachricht mit ML-DSA-65.
- **Parameter:**
  - `msg`: Zu signierende Nachricht.
  - `privKeyB64`: ML-DSA Private Key als Base64-String.
- **Rückgabe:**
  `StrVal` (SignaturB64) bei Erfolg, `ErrorVal` bei Fehler.

---

## pqc.Verify(msg, sigB64, pubKeyB64)
- **Konkret:**
  Verifiziert eine ML-DSA-65 Signatur gegen eine Textnachricht.
- **Parameter:**
  - `msg`: Ursprüngliche Nachricht.
  - `sigB64`: Signatur als Base64-String.
  - `pubKeyB64`: ML-DSA Public Key als Base64-String.
- **Rückgabe:**
  `BoolVal(true)` bei gültiger Signatur, `ErrorVal` bei ungültiger Signatur oder Fehler.

---

## pqc.SignFile(filePath, privKeyB64)
- **Konkret:**
  Signiert eine Datei über ihren SHA-256-Hash mit ML-DSA-65.
- **Parameter:**
  - `filePath`: Pfad zur zu signierenden Datei.
  - `privKeyB64`: ML-DSA Private Key als Base64-String.
- **Rückgabe:**
  `StrVal` (SignaturB64) bei Erfolg, `ErrorVal` bei Fehler.

---

## pqc.VerifyFile(filePath, sigB64, pubKeyB64)
- **Konkret:**
  Verifiziert eine Datei gegen eine ML-DSA-65 Signatur.
- **Parameter:**
  - `filePath`: Pfad zur verifizierten Datei.
  - `sigB64`: Signatur als Base64-String.
  - `pubKeyB64`: ML-DSA Public Key als Base64-String.
- **Rückgabe:**
  `BoolVal(true)` bei gültiger Signatur, `ErrorVal` bei Fehler.

---

## Schlüsselverwaltung

## pqc.ExportKey(privKeyB64, path [, password])
- **Konkret:**
  Exportiert einen ML-KEM Private Key verschlüsselt in eine Datei (AES-256-GCM + Argon2id).
  Ohne Passwort wird der Key im Klartext gespeichert.
  Mit leerem Passwort: automatisch generiertes Passwort wird zurückgegeben.
- **Parameter:**
  - `privKeyB64`: ML-KEM Private Key als Base64-String.
  - `path`: Zielpfad.
  - `password`: Optional. Passwort. Leerstring = automatisch generiert.
- **Rückgabe:**
  `StrVal` (verwendetes Passwort oder `""` bei Klartext-Export), `ErrorVal` bei Fehler.

---

## pqc.ImportKey(path [, password])
- **Konkret:**
  Lädt einen ML-KEM Private Key aus einer Datei.
  Mit Passwort: entschlüsselt den Key. Ohne Passwort: liest die Datei als Klartext.
- **Parameter:**
  - `path`: Pfad zur Schlüsseldatei.
  - `password`: Optional. Passwort für verschlüsselte Dateien.
- **Rückgabe:**
  `StrVal` (Private Key als Base64-String), `ErrorVal` bei Fehler.

---

## pqc.ExportSignKey(privKeyB64, path [, password])
- **Konkret:**
  Exportiert einen ML-DSA Private Key verschlüsselt in eine Datei.
  Identisch zu `pqc.ExportKey`, aber für ML-DSA-Schlüssel.
- **Parameter:**
  - `privKeyB64`: ML-DSA Private Key als Base64-String.
  - `path`: Zielpfad.
  - `password`: Optional. Passwort.
- **Rückgabe:**
  `StrVal` (verwendetes Passwort), `ErrorVal` bei Fehler.

---

## pqc.ImportSignKey(path [, password])
- **Konkret:**
  Lädt einen ML-DSA Private Key aus einer Datei.
  Identisch zu `pqc.ImportKey`, aber für ML-DSA-Schlüssel.
- **Parameter:**
  - `path`: Pfad zur Schlüsseldatei.
  - `password`: Optional.
- **Rückgabe:**
  `StrVal` (Private Key als Base64-String), `ErrorVal` bei Fehler.