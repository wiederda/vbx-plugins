# 🔐 vault.* – Passwort-Keystore

Dieses Modul wird als **Plugin** bereitgestellt. Weitere Informationen stehen in **Allgemein**.

Es dient zum Anlegen, Abrufen, Löschen und Auflisten verschlüsselter Einträge in einer lokalen Vault-Datei. Werte werden mit AES-256-GCM verschlüsselt, der Schlüssel wird aus dem Master-Passwort per Argon2id abgeleitet. Jeder Aufruf lädt bzw. speichert die Vault-Datei neu – es wird kein Zustand zwischen Aufrufen gehalten.

---

## vault.Add(datei, master, name, wert)

* **Konkret:**

  Legt einen verschlüsselten Eintrag an oder überschreibt einen bestehenden Eintrag mit gleichem Namen.

  Existiert `datei` noch nicht, wird eine neue Vault-Datei mit zufälligem Salt angelegt.

* **Parameter:**

  * `datei`: Pfad zur Vault-Datei (JSON).
  * `master`: Master-Passwort.
  * `name`: Name des Eintrags.
  * `wert`: Der zu verschlüsselnde Wert.

* **Rückgabe:**

  `BoolVal(true)` bei Erfolg.

  `ErrorVal`, wenn die Datei nicht gelesen/geschrieben werden kann oder die Verschlüsselung fehlschlägt.

---

## vault.Get(datei, master, name)

* **Konkret:**

  Entschlüsselt und liefert den Wert eines Eintrags.

* **Parameter:**

  * `datei`: Pfad zur Vault-Datei.
  * `master`: Master-Passwort.
  * `name`: Name des Eintrags.

* **Rückgabe:**

  `StrVal` mit dem entschlüsselten Wert.

  `ErrorVal`, wenn der Eintrag nicht existiert, die Datei nicht gelesen werden kann, oder das Master-Passwort falsch ist (Entschlüsselung schlägt fehl).

---

## vault.Delete(datei, master, name)

* **Konkret:**

  Entfernt einen Eintrag dauerhaft aus dem Vault.

  `master` wird zur Bestätigung verlangt, auch wenn das Löschen selbst keine Entschlüsselung erfordert.

* **Parameter:**

  * `datei`: Pfad zur Vault-Datei.
  * `master`: Master-Passwort.
  * `name`: Name des zu löschenden Eintrags.

* **Rückgabe:**

  `BoolVal(true)` bei Erfolg.

  `ErrorVal`, wenn der Eintrag nicht existiert oder die Datei nicht gespeichert werden kann.

---

## vault.List(datei)

* **Konkret:**

  Listet die Namen aller gespeicherten Einträge auf, ohne sie zu entschlüsseln. Benötigt daher kein Master-Passwort.

* **Parameter:**

  * `datei`: Pfad zur Vault-Datei.

* **Rückgabe:**

  `ArrVal` mit den Namen aller Einträge (leer, falls der Vault leer oder die Datei nicht vorhanden ist).

  `ErrorVal`, wenn die vorhandene Datei nicht gelesen werden kann.

---

## Sicherheitshinweise

* **Schlüsselableitung:** Argon2id (`time=1`, `memory=64 MiB`, `threads=4`), nicht das schwächere PBKDF2 der ursprünglichen CLI-Version.
* **Verschlüsselung:** AES-256-GCM (authentifiziert – jede Manipulation des Ciphertexts führt zu einem Entschlüsselungsfehler statt zu stillschweigend falschen Daten).