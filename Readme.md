# VBX Plugins

Hierbei handelt es sich um Erweiterungen für VBX. Diese können wie optionale Module über `#use` eingebunden werden.
Die `.wasm`-Dateien müssen im Unterordner `plugins` im VBX-Verzeichnis abgelegt werden.

## Konzept

Ein Plugin stellt zusätzliche Funktionen über einen eigenen Namespace bereit. Aktuell stehen folgende zur Verfügung:

| Plugin   | Namespace  | Beschreibung                                                                                     |
| -------- | ---------- | ------------------------------------------------------------------------------------------------- |
| `crypt`  | `crypt.*`  | Kryptografie- und Zufallsfunktionen (AES-GCM, HMAC, Bcrypt, Passwörter, GUIDs) — `Wipe`/`WipeString` liegen unter `string.*`, da sie technisch nicht als Plugin funktionieren können |                                                |
| `docker` | `docker.*` | Funktionen zur Verwaltung von Docker-Containern, Images, Docker Compose, Netzwerken und Volumes  |
| `fin`    | `fin.*`    | Finanz- und mathematische Funktionen, beispielsweise `Npv`, `Irr` und weitere Berechnungen       |
| `ini`    | `ini.*`    | Funktionen zum Arbeiten mit INI-Daten                                                            |
| `pgp`    | `pgp.*`    | PGP-Schlüsselerzeugung, Ver-/Entschlüsselung und digitale Signaturen                              |
| `pqc`    | `pqc.*`    | Post-Quanten-Kryptografie (ML-KEM-768, ML-DSA-65) für Schlüsselaustausch und Signaturen           |
| `rand`   | `rand.*`   | Funktionen zur Erzeugung von Zufallswerten                                                       |
| `steg`   | `steg.*`   | Verstecken und Extrahieren von Daten in Bilddateien (Steganografie)                              |
| `tar`    | `tar.*`    | Erstellen und Entpacken von TAR- und TAR.GZ-Archiven                                             | 
| `xml`    | `xml.*`    | Laden, Bearbeiten, Speichern und Abfragen von XML-Dokumenten                                     |
| `yaml`   | `yaml.*`   | Funktionen zum Arbeiten mit YAML-Daten                                                           |
| `zip`    | `zip.*`    | Erstellen, Entpacken und Auflisten von ZIP-Archiven                                              |




## Verwendung

```vb
#use "docker"

result = docker.Ps()
```