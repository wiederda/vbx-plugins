# 💰 fin.* – Finanz- & wissenschaftliche Funktionen

Dieses Modul wird als **Plugin** bereitgestellt. Weitere Informationen stehen in **Allgemein**.

Es dient zur Berechnung finanzmathematischer Kennzahlen sowie wissenschaftlicher Funktionen.

---

## fin.Fv(rate, nper, pmt [, pv])

* **Konkret:**

  Berechnet den Endwert (Future Value) einer Investition mit gleichbleibenden Zahlungen.

  Bei `rate = 0` wird linear gerechnet.

* **Parameter:**

  * `rate`: Zinssatz pro Periode.
  * `nper`: Anzahl der Perioden.
  * `pmt`: Zahlung pro Periode.
  * `pv`: Optional. Barwert (Standard: `0`).

* **Rückgabe:**

  `NumVal`

---

## fin.Pmt(rate, nper, pv)

* **Konkret:**

  Berechnet die periodische Rate (z. B. monatliche Kreditrate) für einen Kredit.

  Bei `rate = 0` wird linear gerechnet.

* **Parameter:**

  * `rate`: Zinssatz pro Periode.
  * `nper`: Anzahl der Perioden.
  * `pv`: Barwert (Kreditbetrag, positiv).

* **Rückgabe:**

  `NumVal`

  Bei positivem `pv` wird die Rate als negativer Wert zurückgegeben.

---

## fin.Npv(rate, values)

* **Konkret:**

  Berechnet den Kapitalwert (Net Present Value) einer Investition anhand zukünftiger Cashflows.

  Die Cashflows werden chronologisch ab **Periode 1** diskontiert.

* **Parameter:**

  * `rate`: Diskontierungssatz pro Periode.
  * `values`: `ArrVal` mit Cashflows, chronologisch ab Periode 1.

* **Rückgabe:**

  `NumVal`

---

## fin.Irr(values [, guess])

* **Konkret:**

  Berechnet den Internen Zinsfuß (Internal Rate of Return) mittels Newton-Raphson-Verfahren.

  Die Berechnung verwendet maximal 100 Iterationen.

* **Parameter:**

  * `values`: `ArrVal` mit Cashflows. Der erste Wert entspricht typischerweise der Anfangsinvestition und ist meist negativ.
  * `guess`: Optional. Startwert für die Berechnung (Standard: `0.1`).

* **Rückgabe:**

  `NumVal`

  `ErrorVal`, wenn innerhalb von 100 Iterationen keine Konvergenz erreicht wird.

---

## fin.Fact(n)

* **Konkret:**

  Berechnet die Fakultät einer nicht-negativen Ganzzahl (`n!`).

* **Parameter:**

  * `n`: Nicht-negative Ganzzahl.

* **Rückgabe:**

  `NumVal`

  `ErrorVal`, wenn `n` keine Ganzzahl oder negativ ist oder das Ergebnis zu groß wird.

---

## fin.Gamma(n)

* **Konkret:**

  Gibt den Wert der Gamma-Funktion zurück. Sie stellt eine Verallgemeinerung der Fakultät auf reelle Zahlen dar.

* **Parameter:**

  * `n`: Numerischer Wert.

* **Rückgabe:**

  `NumVal`

  `ErrorVal`, wenn kein gültiges numerisches Ergebnis berechnet werden kann.

---

## fin.Log10(n)

* **Konkret:**

  Berechnet den Zehnerlogarithmus (`log₁₀`).

* **Parameter:**

  * `n`: Positiver numerischer Wert.

* **Rückgabe:**

  `NumVal`

  `ErrorVal`, wenn `n` kleiner oder gleich `0` ist.

---

## fin.Log2(n)

* **Konkret:**

  Berechnet den Logarithmus zur Basis 2 (`log₂`).

* **Parameter:**

  * `n`: Positiver numerischer Wert.

* **Rückgabe:**

  `NumVal`

  `ErrorVal`, wenn `n` kleiner oder gleich `0` ist.

---

## fin.Hypot(x, y)

* **Konkret:**

  Berechnet die Länge der Hypotenuse eines rechtwinkligen Dreiecks (`√(x² + y²)`).

  Die Berechnung erfolgt numerisch stabil.

* **Parameter:**

  * `x`: Erste Kathete.
  * `y`: Zweite Kathete.

* **Rückgabe:**

  `NumVal`

---

## fin.Remainder(x, y)

* **Konkret:**

  Berechnet den Rest der Division nach IEEE 754.

  Das Ergebnis kann negativ sein und unterscheidet sich dadurch vom klassischen Modulo-Verhalten.

* **Parameter:**

  * `x`: Dividend.
  * `y`: Divisor.

* **Rückgabe:**

  `NumVal`

  `ErrorVal`, wenn `y = 0`.
