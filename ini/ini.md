# ⚙️ ini.* – INI-Konfigurationsdatei-Funktionen

Dieses Modul wird als **Plugin** bereitgestellt. Weitere Informationen stehen in **Allgemein**.

Es dient zum Lesen, Schreiben und Verwalten von INI-Konfigurationsdateien.

`ini.Load` lädt eine INI-Datei in einen internen Arbeitsspeicher. Die weiteren Funktionen arbeiten mit diesem geladenen Zustand.

Schreiboperationen werden atomar ausgeführt (temporäre Datei + Rename). Der interne Zustand ist thread-sicher über `sync.RWMutex` geschützt.

Sektions- und Key-Namen sind **case-sensitiv**.

---

## ini.Load(filename)

### Konkret

Lädt eine INI-Datei in den Arbeitsspeicher.

Existiert die Datei nicht, wird ein leerer Zustand angelegt. Dies ist ein gültiger Zustand und kann anschließend mit `ini.Set` zum Erstellen einer neuen INI-Datei verwendet werden.

Kommentarzeilen (`; …` und `# …`) sowie Einträge außerhalb einer Sektion werden ignoriert.

### Parameter

* `filename`: Pfad zur INI-Datei.

### Rückgabe

* `NullVal` bei Erfolg
* `ErrorVal` bei einem Lesefehler

---

## ini.Get(section, key [, default])

### Konkret

Liest einen Wert aus einer Sektion.

Existiert der angegebene Key nicht, wird der angegebene Default-Wert zurückgegeben.

### Parameter

* `section`: Sektionsname.
* `key`: Schlüsselname.
* `default`: Optional. Rückgabewert, wenn der Key nicht gefunden wird. Standard: `""`.

### Rückgabe

`StrVal`

---

## ini.Set(section, key, value [, autosave])

### Konkret

Setzt einen Wert in einer Sektion.

Existiert die Sektion noch nicht, wird sie automatisch angelegt.

Mit `autosave = true` (Standard) wird die Änderung unmittelbar in die geladene INI-Datei geschrieben.

Mit `autosave = false` wird nur der interne Zustand aktualisiert. Die Änderungen müssen anschließend mit `ini.Save` gespeichert werden.

`ini.Load` muss zuvor aufgerufen worden sein, wenn die Änderung unmittelbar gespeichert werden soll.

### Parameter

* `section`: Sektionsname.
* `key`: Schlüsselname.
* `value`: Zu setzender Wert. Unterstützt werden String, Zahl und Bool.
* `autosave`: Optional. Bool-Wert. Standard: `true`.

### Rückgabe

* `NullVal` bei Erfolg
* `ErrorVal` bei einem Fehler

---

## ini.Save()

### Konkret

Schreibt die aktuellen Änderungen manuell in die geladene INI-Datei.

Nützlich nach `ini.Set` mit `autosave = false`.

Sektionen und Keys werden beim Schreiben alphabetisch sortiert.

### Rückgabe

* `NullVal` bei Erfolg
* `ErrorVal` bei einem Fehler

---

## ini.Exists(section, key)

### Konkret

Prüft, ob ein bestimmter Key in einer Sektion vorhanden ist.

Auch ein vorhandener Key mit einem leeren Wert gilt als vorhanden.

### Parameter

* `section`: Sektionsname.
* `key`: Schlüsselname.

### Rückgabe

`BoolVal`

---

## ini.Delete(section [, key])

### Konkret

Löscht einen einzelnen Key oder eine komplette Sektion.

Wird ein Key gelöscht und ist die Sektion anschließend leer, wird die Sektion ebenfalls entfernt.

Änderungen werden unmittelbar gespeichert.

### Parameter

* `section`: Sektionsname.
* `key`: Optional. Schlüsselname. Wird kein Key angegeben, wird die gesamte Sektion gelöscht.

### Rückgabe

* `NullVal` bei Erfolg
* `ErrorVal` bei einem Fehler

---

## ini.Sections()

### Konkret

Gibt alle Sektionsnamen des aktuell geladenen INI-Zustands zurück.

Die Sektionsnamen werden alphabetisch sortiert.

### Rückgabe

`ArrVal`

Array von `StrVal`-Einträgen.

---

## ini.Keys(section)

### Konkret

Gibt alle Keys einer Sektion des aktuell geladenen INI-Zustands zurück.

Die Keys werden alphabetisch sortiert.

Existiert die Sektion nicht, wird ein leeres Array zurückgegeben.

### Parameter

* `section`: Sektionsname.

### Rückgabe

`ArrVal`

Array von `StrVal`-Einträgen.
