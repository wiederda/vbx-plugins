# 🎲 rand.* – Zufallsfunktionen

Dient zur Erzeugung pseudozufälliger Zahlen, Booleans und Array-Elemente.
Nutzt `math/rand` mit einem zeitbasierten Seed. Für kryptografische Zwecke `crypt.*` verwenden.

---

## rand.Float()
- **Konkret:**
  Gibt eine Zufallszahl zwischen `0.0` und `1.0` zurück.
- **Rückgabe:**
  `NumVal`

---

## rand.Bool()
- **Konkret:**
  Gibt zufällig `true` oder `false` zurück (50/50).
- **Rückgabe:**
  `BoolVal`

---

## rand.Range(min, max)
- **Konkret:**
  Gibt eine zufällige Ganzzahl zwischen `min` und `max` zurück (beide Grenzen inklusive).
  Bei `max <= min` wird `min` zurückgegeben.
- **Parameter:**
  - `min`: Untere Grenze (inklusive).
  - `max`: Obere Grenze (inklusive).
- **Rückgabe:**
  `NumVal`

---

## rand.RangeFloat(min, max)
- **Konkret:**
  Gibt eine zufällige Fließkommazahl im Bereich `[min, max)` zurück.
  Bei `max <= min` wird `min` zurückgegeben.
- **Parameter:**
  - `min`: Untere Grenze.
  - `max`: Obere Grenze (exklusiv).
- **Rückgabe:**
  `NumVal`

---

## rand.Choice(array)
- **Konkret:**
  Wählt ein zufälliges Element aus einem Array aus.
- **Parameter:**
  - `array`: `ArrVal` mit mindestens einem Element.
- **Rückgabe:**
  Zufällig gewählter Wert aus dem Array, `ErrorVal` wenn das Array leer ist.

---

## rand.Seed([n])
- **Konkret:**
  Initialisiert den Zufallsgenerator mit einem Startwert.
  Gleicher Seed erzeugt immer dieselbe Zufallsfolge (reproduzierbar).
  Ohne Parameter wird der aktuelle Zeitstempel als Seed verwendet.
- **Parameter:**
  - `n`: Optional. Seed-Wert als Ganzzahl.
- **Rückgabe:**
  `NumVal` (verwendeter Seed, `0` bei automatischem Seed).