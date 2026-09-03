# 💰 fin.* – Finanz- & Wissenschaftliche Funktionen

Dient zur Berechnung finanzmathematischer Kennzahlen sowie wissenschaftlicher Funktionen.

---

## fin.Fv(rate, nper, pmt [, pv])
- **Konkret:**
  Berechnet den Endwert (Future Value) einer Investition mit gleichbleibenden Zahlungen.
  Bei `rate = 0` wird linear gerechnet.
- **Parameter:**
  - `rate`: Zinssatz pro Periode.
  - `nper`: Anzahl der Perioden.
  - `pmt`: Zahlung pro Periode.
  - `pv`: Optional. Barwert (Standard: `0`).
- **Rückgabe:**
  `NumVal`

---

## fin.Pmt(rate, nper, pv)
- **Konkret:**
  Berechnet die periodische Rate (z. B. monatliche Kreditrate) für einen Kredit.
  Bei `rate = 0` wird linear gerechnet.
- **Parameter:**
  - `rate`: Zinssatz pro Periode.
  - `nper`: Anzahl der Perioden.
  - `pv`: Barwert (Kreditbetrag, positiv).
- **Rückgabe:**
  `NumVal`
  Negatives Vorzeichen = Auszahlung.

---

## fin.Npv(rate, values)
- **Konkret:**
  Berechnet den Kapitalwert (Net Present Value) einer Investition anhand zukünftiger Cashflows.
- **Parameter:**
  - `rate`: Diskontierungssatz pro Periode.
  - `values`: `ArrVal` mit Cashflows (chronologisch, ab Periode 1).
- **Rückgabe:**
  `NumVal`

---

## fin.Irr(values [, guess])
- **Konkret:**
  Berechnet den Internen Zinsfuß (Internal Rate of Return) mittels Newton-Raphson-Verfahren.
  Konvergiert in max. 100 Iterationen.
- **Parameter:**
  - `values`: `ArrVal` mit Cashflows (erster Wert = Anfangsinvestition, typischerweise negativ).
  - `guess`: Optional. Schätzwert als Startpunkt (Standard: `0.1`).
- **Rückgabe:**
  `NumVal`, `ErrorVal` wenn keine Konvergenz erreicht wird.

---

## fin.Fact(n)
- **Konkret:**
  Berechnet die Fakultät einer nicht-negativen Ganzzahl (`n!`).
- **Parameter:**
  - `n`: Nicht-negative Ganzzahl.
- **Rückgabe:**
  `NumVal`

---

## fin.Gamma(n)
- **Konkret:**
  Gibt den Wert der Gamma-Funktion zurück (Verallgemeinerung der Fakultät auf reelle Zahlen).
- **Rückgabe:**
  `NumVal`

---

## fin.Log10(n)
- **Konkret:**
  Berechnet den Zehnerlogarithmus (log₁₀).
- **Rückgabe:**
  `NumVal`

---

## fin.Log2(n)
- **Konkret:**
  Berechnet den Logarithmus zur Basis 2 (log₂).
- **Rückgabe:**
  `NumVal`

---

## fin.Hypot(x, y)
- **Konkret:**
  Berechnet die Länge der Hypotenuse eines rechtwinkligen Dreiecks (`√(x² + y²)`).
  Numerisch stabil (kein Overflow bei großen Werten).
- **Parameter:**
  - `x`: Erste Kathete.
  - `y`: Zweite Kathete.
- **Rückgabe:**
  `NumVal`

---

## fin.Remainder(x, y)
- **Konkret:**
  Berechnet den Rest der Division nach IEEE 754 (kann negativ sein).
  Unterscheidet sich von Modulo: Ergebnis liegt immer im Bereich `[-y/2, y/2]`.
- **Parameter:**
  - `x`: Dividend.
  - `y`: Divisor.
- **Rückgabe:**
  `NumVal`