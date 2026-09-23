# 🔳 qr.* – QR-Code-Funktionen

Dieses Modul wird als **Plugin** bereitgestellt. Weitere Informationen stehen in **Allgemein**.

Es dient zum Erstellen und Lesen von QR-Codes als PNG-Dateien.

---

## qr.Create(text, ausgabepfad [, size])

* **Konkret:**

  Erstellt einen QR-Code aus `text` und speichert ihn als PNG-Datei unter `ausgabepfad`.

  Die Bildgröße lässt sich über `size` steuern.

* **Parameter:**

  * `text`: Inhalt, der als QR-Code kodiert werden soll.
  * `ausgabepfad`: Zielpfad der PNG-Datei.
  * `size`: Optional. Breite/Höhe des QR-Codes in Pixeln (Standard: `256`).

* **Rückgabe:**

  `BoolVal(true)` bei Erfolg.

  `ErrorVal`, wenn die Datei nicht geschrieben werden kann oder `size` kleiner oder gleich `0` ist.

---

## qr.Read(pfad)

* **Konkret:**

  Liest den ersten QR-Code aus einer PNG-Datei und gibt seinen Inhalt als Text zurück.

  Enthält das Bild mehrere QR-Codes, wird nur der erste zurückgegeben – siehe `qr.ReadAll` für alle Treffer.

* **Parameter:**

  * `pfad`: Pfad zu einer PNG-Datei mit mindestens einem QR-Code.

* **Rückgabe:**

  `StrVal` mit dem erkannten Inhalt.

  `ErrorVal`, wenn die Datei nicht geöffnet oder dekodiert werden kann, oder kein QR-Code gefunden wird.

---

## qr.ReadAll(pfad)

* **Konkret:**

  Liest alle QR-Codes aus einer PNG-Datei und gibt ihre Inhalte als Array zurück.

* **Parameter:**

  * `pfad`: Pfad zu einer PNG-Datei mit einem oder mehreren QR-Codes.

* **Rückgabe:**

  `ArrVal` mit den Textinhalten aller gefundenen QR-Codes (leer, falls keiner gefunden wurde).

  `ErrorVal`, wenn die Datei nicht geöffnet oder dekodiert werden kann.