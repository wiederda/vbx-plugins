# 🎲 rand.* – Zufallsfunktionen

Dieses Modul wird als **Plugin** bereitgestellt. Weitere Informationen stehen in **Allgemein**.

Es dient zur Erzeugung pseudozufälliger Zahlen, Booleans und zufällig ausgewählter Array-Elemente.
Für kryptografische Zwecke `crypt.*` verwenden.

---

## rand.Float()

* **Konkret:**

  Gibt eine Zufallszahl im Bereich `[0.0, 1.0)` zurück.

* **Rückgabe:**

  `NumVal`

---

## rand.Bool()

* **Konkret:**

  Gibt zufällig `true` oder `false` zurück.

* **Rückgabe:**

  `BoolVal`

---

## rand.Range(min, max)

* **Konkret:**

  Gibt eine zufällige Ganzzahl zwischen `min` und `max` zurück (beide Grenzen inklusive).

  Bei `min = max` wird `min` zurückgegeben.

* **Parameter:**

  * `min`: Untere Grenze (inklusive).
  * `max`: Obere Grenze (inklusive).

* **Rückgabe:**

  `NumVal`

  `ErrorVal`, wenn `max < min` ist oder ein Argument kein numerischer Wert ist.

---

## rand.RangeFloat(min, max)

* **Konkret:**

  Gibt eine zufällige Fließkommazahl im Bereich `[min, max)` zurück.

  Bei `min = max` wird `min` zurückgegeben.

* **Parameter:**

  * `min`: Untere Grenze (inklusive).
  * `max`: Obere Grenze (exklusiv).

* **Rückgabe:**

  `NumVal`

  `ErrorVal`, wenn `max < min` ist oder ein Argument kein numerischer Wert ist.

---

## rand.Choice(array)

* **Konkret:**

  Wählt ein zufälliges Element aus einem Array aus.

* **Parameter:**

  * `array`: `ArrVal` mit mindestens einem Element.

* **Rückgabe:**

  Zufällig gewählter Wert aus dem Array.

  `ErrorVal`, wenn das Argument kein Array ist oder das Array leer ist.

---

## rand.Seed([n])

* **Konkret:**

  Initialisiert den Zufallsgenerator mit einem Startwert.

  Derselbe Seed erzeugt immer dieselbe Zufallsfolge und ermöglicht damit reproduzierbare Ergebnisse.

  Ohne Parameter wird der Zufallsgenerator mit dem aktuellen Zeitstempel initialisiert.

* **Parameter:**

  * `n`: Optional. Numerischer Seed-Wert.

* **Rückgabe:**

  `NumVal`

  Der verwendete Seed. Bei automatischer Initialisierung ohne Parameter wird `0` zurückgegeben.
