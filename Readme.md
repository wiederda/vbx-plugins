# VBX Plugins

Hierbei handelt es sich um Erweiterungen für VBX. Diese können wie optionale Module über `#use` eingebunden werden.
Die `.wasm`-Dateien müssen im Unterordner `plugins` im VBX-Verzeichnis abgelegt werden.

## Konzept

Ein Plugin stellt zusätzliche Funktionen über einen eigenen Namespace bereit. Aktuell stehen folgende zur Verfügung:

| Plugin   | Namespace  | Beschreibung                                                                                     |
| -------- | ---------- | ------------------------------------------------------------------------------------------------- |
| `docker` | `docker.*` | Funktionen zur Verwaltung von Docker-Containern, Images, Docker Compose, Netzwerken und Volumes  |
| `fin`    | `fin.*`    | Finanz- und mathematische Funktionen, beispielsweise `Npv`, `Irr` und weitere Berechnungen       |
| `yaml`   | `yaml.*`   | Funktionen zum Arbeiten mit YAML-Daten                                                           |
| `rand`   | `rand.*`   | Funktionen zur Erzeugung von Zufallswerten                                                       |

## Verwendung

```vb
#use "docker"

result = docker.Ps()
```