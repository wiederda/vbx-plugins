# 🔐 vault.* – Passwort-Keystore

Dieses Modul wird als **Plugin** bereitgestellt. Weitere Informationen stehen in **Allgemein**.

Es dient zum Anlegen, Abrufen, Löschen und Auflisten verschlüsselter Einträge in einer lokalen Vault-Datei. Werte **und Namen** werden mit AES-256-GCM verschlüsselt, der Schlüssel wird aus dem Master-Passwort per Argon2id abgeleitet. Jeder Aufruf lädt bzw. speichert die Vault-Datei neu – es wird kein Zustand zwischen Aufrufen gehalten.

---

## vault.Add(datei, master, name, wert)

* **Konkret:**

  Legt einen verschlüsselten Eintrag an oder überschreibt einen bestehenden Eintrag mit gleichem Namen.

  Existiert `datei` noch nicht, wird eine neue Vault-Datei mit zufälligem Salt angelegt.

  Da `name` ebenfalls verschlüsselt gespeichert wird, muss zur Duplikat-Prüfung jeder bestehende Eintragsname mit dem angegebenen `master`-Passwort entschlüsselt werden. Bei vielen Einträgen ist `Add` dadurch geringfügig langsamer als ein reiner Neuanlage-Vorgang.

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

  Entschlüsselt und liefert den Wert eines Eintrags. Da Namen verschlüsselt gespeichert sind, wird intern jeder Eintragsname mit `master` entschlüsselt, bis der gesuchte `name` gefunden ist.

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

  `master` muss das korrekte Master-Passwort sein – da Namen verschlüsselt gespeichert sind, kann der zu löschende Eintrag nur über eine erfolgreiche Entschlüsselung mit dem richtigen Passwort gefunden werden.

* **Parameter:**

  * `datei`: Pfad zur Vault-Datei.
  * `master`: Master-Passwort.
  * `name`: Name des zu löschenden Eintrags.

* **Rückgabe:**

  `BoolVal(true)` bei Erfolg.

  `ErrorVal`, wenn der Eintrag nicht existiert, `master` falsch ist, oder die Datei nicht gespeichert werden kann.

---

## vault.List(datei, master)

* **Konkret:**

  Entschlüsselt und listet die Namen aller gespeicherten Einträge auf. Benötigt das Master-Passwort, da Eintragsnamen verschlüsselt gespeichert werden – wer die Datei z. B. in einem Texteditor öffnet, sieht dadurch keinerlei Namen oder Rückschlüsse auf den Inhalt, nur verschlüsselte Blöcke.

* **Parameter:**

  * `datei`: Pfad zur Vault-Datei.
  * `master`: Master-Passwort.

* **Rückgabe:**

  `ArrVal` mit den entschlüsselten Namen aller Einträge (leer, falls der Vault leer oder die Datei nicht vorhanden ist).

  `ErrorVal`, wenn die Datei nicht gelesen werden kann oder `master` falsch ist (Entschlüsselung eines Namens schlägt fehl).

---

## Sicherheitshinweise

* **Schlüsselableitung:** Argon2id (`time=1`, `memory=64 MiB`, `threads=4`), nicht das schwächere PBKDF2 der ursprünglichen CLI-Version.
* **Verschlüsselung:** AES-256-GCM (authentifiziert – jede Manipulation des Ciphertexts führt zu einem Entschlüsselungsfehler statt zu stillschweigend falschen Daten).
* **Metadatenschutz:** Sowohl Werte als auch Eintragsnamen werden verschlüsselt gespeichert. Wer die Vault-Datei ohne Master-Passwort öffnet (z. B. in einem Texteditor), sieht ausschließlich Base64-kodierte, verschlüsselte Blöcke – keine Klartext-Hinweise auf vorhandene Einträge.