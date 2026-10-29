# Splash Bench

Finde auf deinem eigenen Mac heraus, ob sich **Splash** als zweiter Motor in **oMLX** lohnt – ohne eigene Entwicklung. Splash Bench misst dasselbe Modell einmal mit der oMLX-eigenen Engine und einmal mit Splash und zeigt dir, was auf deinem Mac schneller ist.

> *English:* Splash Bench compares oMLX's own engine with the Splash engine on local LLMs on Apple Silicon. The app, docs and issues are in German.

**Inoffizielles Projekt.** Splash ist ein Produkt von Inco AI, oMLX ein eigenständiges Open-Source-Projekt. Splash Bench ist mit keinem der beiden verbunden und liefert keins davon mit – beides installierst du selbst. Entstanden zur YouTube-Folge über Splash auf dem Kanal [@RalfKlemmer](https://www.youtube.com/@RalfKlemmer).

| Programm | Wozu |
|---|---|
| **Splash Bench.app** (`splashui`) | Vergleich im Browser starten, live mitverfolgen, Ergebnis als Diagramm – hell und dunkel |
| `splashcheck` | Diagnose: Ist alles da? Was fehlt? Welche Modelle passen in deinen RAM? |
| `splashbench` | A/B-Messung oMLX (MTP) gegen Splash mit drei echten Prompts, mehrere Läufe, Median |
| `splashdemo` | Live-Rennen für Bildschirm oder Video: derselbe Prompt erst auf oMLX, dann auf Splash |

Alle Programme sprechen nur mit dem **lokalen** oMLX-Server (Standard `http://localhost:8000`). Es gibt keine Telemetrie und keine Verbindung ins Internet.

Gemessen auf einem M5 Max (128 GB) am 29.10.2026 mit oMLX 0.7.3rc1 und Splash 1.5.0: Qwen3.8-27B mit Splash 1,33–1,87× schneller, Qwen3.6-35B-A3B nur 1,05–1,32× – dort ist oMLX mit MTP schon schnell, Splash bringt nur wenig. Deshalb: messen statt raten.

## Voraussetzungen

- Mac mit **Apple M3 oder neuer** und **macOS 26.4 oder neuer**
- [oMLX](https://github.com/jundot/omlx/releases) mit Splash-Unterstützung (externe Engine), läuft
- Splash ab 1.3.0: `brew install incoai/tap/splash`. Auf M1/M2 geht nur der inoffizielle Community-Port `splash-m1` (github.com/paperniuk/splash/releases), nicht von Inco AI und ohne deren Support

Was davon fehlt, zeigt dir die App nach dem Start selbst – mit installierten Versionen und dem passenden Befehl zum Kopieren.

## Installation

1. Unter [Releases](https://github.com/ralfklemmer/splash-bench/releases/latest) die Datei `Splash-Bench-<Version>.dmg` laden.
2. Das DMG öffnen und **Splash Bench** in den Ordner **Programme** ziehen.
3. Splash Bench per Doppelklick starten.

### „Splash Bench“ kann nicht geöffnet werden?

Die App ist **nicht von Apple notarisiert** (dafür bräuchte es ein kostenpflichtiges Apple-Entwicklerkonto). Beim ersten Start meldet macOS deshalb, dass es die App nicht auf Schadsoftware prüfen konnte. So startest du sie trotzdem – das ist nur einmal nötig:

1. Die Meldung mit **Fertig** schließen (nicht „In den Papierkorb legen“).
2. **Systemeinstellungen → Datenschutz & Sicherheit** öffnen und ganz nach unten scrollen.
3. Bei „Splash Bench wurde blockiert …“ auf **Trotzdem öffnen** klicken und mit Passwort oder Touch ID bestätigen.

Alternativ im Terminal die Download-Markierung entfernen und danach normal starten:

```bash
xattr -dr com.apple.quarantine "/Applications/Splash Bench.app"
```

Wer der heruntergeladenen App nicht traut, baut sie aus dem Quellcode (unten). Die SHA-256-Prüfsumme jedes DMG steht im Release.

Seit macOS 15 funktioniert der alte Trick „Rechtsklick → Öffnen“ nicht mehr; der Weg geht nur noch über die Systemeinstellungen.

## Aus dem Quellcode bauen

Du brauchst Go ab 1.27 (nur Standardbibliothek, keine weiteren Abhängigkeiten):

```bash
git clone https://github.com/ralfklemmer/splash-bench.git
cd splash-bench
go run ./cmd/makeapp          # → bin/Splash Bench.app
go build -o bin/ ./cmd/...    # die Kommandozeilen-Programme
go test ./...
```

Selbst gebaute Apps sind ad hoc signiert und starten auf dem eigenen Mac ohne Rückfrage.

## Fehler melden

In der App unter **Voraussetzungen** auf **Diagnose kopieren** klicken, dann auf **Problem melden** und die Diagnose ins Formular einfügen. Sie enthält Chip, Arbeitsspeicher, macOS-, Splash- und oMLX-Version und die Prüfergebnisse – aber nie deinen API-Key. Sicherheitslücken bitte nicht als Issue, siehe [SECURITY.md](SECURITY.md).

Lizenz: [Apache-2.0](LICENSE)

## API-Key

`splashbench` und `splashdemo` schalten über die Admin-API von oMLX die Engine um und brauchen dafür den API-Key. `splashcheck` läuft auch ohne Key, prüft dann aber nur lokal (ohne „Splash-Kompatibilität laut oMLX“).

Der Key steht in `~/.omlx/settings.json` unter `auth.api_key`. Übergib ihn als Umgebungsvariable, ohne ihn anzuzeigen:

```bash
export OMLX_API_KEY=$(jq -r .auth.api_key ~/.omlx/settings.json)
```

Die Programme senden den Key ausschließlich als `Authorization: Bearer …` an die angegebene `-base-url` – also an deinen lokalen oMLX. Sie geben ihn nie aus und schreiben ihn in keine Datei. Tipp für Aufnahmen: den Befehl oben nie im sichtbaren Terminal ausführen und den Key nie mit `echo` prüfen.

## splashui – Splash Bench im Browser

```bash
bin/splashui
bin/splashui -load results.json   # alte splashbench-Messung als „letzte Messung“ zeigen
```

Das Programm startet einen kleinen Server auf `http://127.0.0.1:8765` und öffnet den Browser. Dort wählst du Modelle, Aufgaben (Code, Fließtext, Übersetzung) und Durchläufe und startest den Vergleich. Während der Messung laufen oMLX und Splash als zwei Bahnen mit Live-tok/s gegeneinander; am Ende steht pro Modell der mittlere Faktor (geometrisches Mittel über die Aufgaben) mit Empfehlung und die Balken je Aufgabe, umschaltbar zwischen Geschwindigkeit und Wartezeit bis zum ersten Token.

#### Voraussetzungen – prüft die App selbst

Beim Start und auf Knopfdruck („Erneut prüfen“) prüft die App der Reihe nach. Die Statuszeile oben rechts zeigt das Ergebnis, ein Klick darauf öffnet die Liste. Fehlt etwas, öffnet sie sich von selbst, und der Start bleibt gesperrt. Zu jedem Punkt steht, was zu tun ist, wo es passt mit einem Befehl zum Kopieren.

| Prüfung | Voraussetzung |
|---|---|
| Chip | Apple M3 oder neuer. M1/M2 gelb: nur mit dem inoffiziellen Community-Port `splash-m1` |
| macOS | 26.4 oder neuer (rot, wenn älter; mit `splash-m1` nur gelb) |
| Splash installiert | `splash` ab 1.3.0 über Homebrew; gelb unter 1.4.0, weil Qwen3.8-Flash-Next 1.4.0 braucht. M1/M2: `splash-m1` |
| oMLX erreichbar | oMLX läuft; Adresse aus `~/.omlx/settings.json` (`server.host`, `server.port`). Solange oMLX noch Modelle lädt, sagt die App das. |
| Zugriff auf oMLX | API-Key aus `OMLX_API_KEY` oder `auth.api_key` in der Einstellungsdatei, falls oMLX einen verlangt |
| oMLX mit Splash-Unterstützung | eine oMLX-Version mit externer Engine (gemessen und geprüft mit 0.7.3rc1) |
| Splash-fähiges Modell | mindestens ein Zielmodell mit DFlash2-Draft, das oMLX für Splash freigibt. Fehlt der Draft, nennt oMLX ihn; die App zeigt dann den `hf download`-Befehl, auch für Familien, die nicht im Katalog stehen |
| Arbeitsspeicher | Schätzung je Modell mit Splash bei 32K Kontext gegen ca. 75 % des RAM. Modelle, die knapp werden, sind markiert und nicht vorausgewählt. |

Gelb heißt: Es geht, aber mit Einschränkung. Rot heißt: Ohne das läuft kein Vergleich.

| Flag | Standard | Bedeutung |
|---|---|---|
| `-base-url` | aus `~/.omlx/settings.json`, sonst `http://localhost:8000` | oMLX-Server |
| `-addr` | `127.0.0.1:8765` | Adresse der App; nur localhost erlaubt, bei belegtem Port ein freier |
| `-results` | `~/Library/Application Support/splashbench/results` | Ordner für gespeicherte Messungen (eine JSON-Datei pro Messung) |
| `-load` | – | Datei als „letzte Messung“ anbieten, auch `results.json` von `splashbench` |
| `-no-open` | aus | Browser nicht öffnen |
| `-idle-quit` | aus (in der App: 2 Minuten) | beenden, wenn so lange keine Seite offen ist und nichts misst |

Beenden: Knopf oben rechts in der Seite, Strg-C im Terminal oder einfach den Browser-Tab schließen (in der App).

### Splash Bench.app – per Doppelklick

```bash
go run ./cmd/makeapp                  # → bin/Splash Bench.app (Version „dev“)
go run ./cmd/makeapp -version 1.2.0   # so baut es die Release-Pipeline
```

Das Bundle in den Ordner „Programme“ ziehen und per Doppelklick starten. Die App öffnet die Seite im Standardbrowser und hat kein Dock-Symbol (die Seite ist das Fenster). Unterschiede zum Start im Terminal:

- Ausgaben landen in `~/Library/Logs/Splash Bench.log`.
- Homebrew- und Systemordner (`/opt/homebrew/bin`, `/usr/local/bin`, `/usr/sbin` …) werden ergänzt, damit `splash` und `sysctl` gefunden werden.
- Ein zweiter Doppelklick öffnet die laufende Instanz statt einer neuen.
- Ist zwei Minuten lang keine Seite offen und läuft keine Messung, beendet sich die App selbst.

Das Bundle ist nur ad hoc signiert: Auf dem Mac, auf dem es gebaut wurde, startet es ohne Rückfrage. Heruntergeladen blockiert Gatekeeper den ersten Start – siehe „Installation“.

- **Empfehlung:** „Splash lohnt sich“, wenn Splash bei mehr als der Hälfte der Aufgaben mindestens 1,15× so schnell ist; sonst „oMLX reicht“.
- **Messung:** dieselbe Logik wie `splashbench` (512 Tokens, Aufwärmlauf, Median), mit Wiederherstellung der Engine – auch bei Abbrechen, Fehler oder Strg-C im Terminal. Die Live-Zahl ist eine Schätzung aus den gestreamten Zeichen, geeicht mit der exakten Token-Zählung des oMLX-Laufs; die Ergebnisse nutzen immer die exakten Werte.
- **Modelle:** angeboten werden alle Modelle, die oMLX für Splash-fähig hält; die beiden aus dem Video sind vorausgewählt, sofern sie in den Arbeitsspeicher passen.
- **API-Key:** aus `OMLX_API_KEY`, sonst direkt aus `~/.omlx/settings.json`; ohne Key versucht es die App ohne (oMLX erlaubt das, wenn es nur lokal lauscht). Er bleibt im Programm und erreicht den Browser nie.
- **Sicherheit:** Der Server lauscht nur auf localhost, beantwortet nur Anfragen an localhost und nimmt Änderungen nur von der eigenen Seite an. Eine andere Webseite im Browser kann also keine Engine umschalten. Die Seite lädt nichts aus dem Internet (Systemschrift statt Webfonts).
- **Darstellung:** folgt der Systemeinstellung; der Knopf oben rechts wechselt zwischen hell und dunkel und merkt sich die Wahl.

## splashcheck – Diagnose

```bash
bin/splashcheck                # nur lokale Checks, wenn OMLX_API_KEY fehlt
bin/splashcheck -base-url http://localhost:8000
```

| Flag | Standard | Bedeutung |
|---|---|---|
| `-base-url` | `http://localhost:8000` | oMLX-Server |
| `-no-color` | aus | ohne Farben ausgeben (auch über `NO_COLOR`) |

Geprüft wird:

- Chip (`sysctl -n machdep.cpu.brand_string`), RAM (`hw.memsize`), macOS (`sw_vers -productVersion`)
- Chip: offiziell ab M3; auf M1/M2 Warnung mit Verweis auf den inoffiziellen Community-Port `splash-m1`. macOS ab 26.4
- Splash-Launcher: `splash --version` bzw. `splash-m1 --version` – mindestens 1.3.0, Mixed-Bit ab 1.3.1, Thinking-Budget ab 1.3.2, Qwen3.8-Flash-Next ab 1.4.0; auf M1/M2 wird `splash-m1` verlangt
- oMLX erreichbar (`/health`, ohne Key) und – mit Key – die Modellliste mit Splash-Kompatibilität und aktiver Engine
- Modellordner aus `~/.omlx/settings.json` (`model.model_dirs`, sonst `model.model_dir`)
- pro Katalog-Modell: Zielmodell und DFlash2-Draft vorhanden (Ordner mit `config.json`, bei GGUF die Dateien), unvollständige Downloads, und das **Splash-Komplettpaket** (`incoai/*-Splash`) als Warnung – es hat keine `config.json`, oMLX lädt es nicht. Nicht installierte Modelle sind nur ein Hinweis (`·`)
- Empfehlung nach RAM. Fertige `hf download …`-Befehle für die beiden gemessenen Modelle stehen im Block „Zum Kopieren“, für die ungemessenen nur als Hinweis beim Modell
- Katalog-Stand (Datum, Splash- und oMLX-Version), gegen den die Liste geprüft wurde

Markierungen: `✓` in Ordnung, `!` Warnung, `✗` Fehler. Exit-Code 0 ohne Fehler, 1 bei mindestens einem `✗`, 2 wenn der Check selbst nicht laufen konnte.

Beispiel (M5 Max, 128 GB, gekürzt):

```
Splash-Check

System
  ✓ Chip: Apple M5 Max
  ✓ Arbeitsspeicher: 128 GB (davon ca. 96 GB für Modelle nutzbar)
  ✓ macOS: 27.0.1

Splash
  ✓ splash 1.5.0 installiert – alle Funktionen (Mixed-Bit, Thinking-Budget, Qwen3.8-Flash-Next)

oMLX
  ✓ Server erreichbar: http://localhost:8000
  ✓ Modellordner: ~/.omlx/models/mlx-community
  ✓ oMLX kennt 17 Modelle

Modell Qwen3.8-27B
  ✓ Zielmodell Jundot/Qwen3.8-27B-oQ4e-mtp (~/.omlx/models/mlx-community/Jundot/Qwen3.8-27B-oQ4e-mtp)
  ✓ Draft (DFlash2) incoai/Qwen3.8-27B-DFlash2 (~/.omlx/models/mlx-community/incoai/Qwen3.8-27B-DFlash2)
  ! Splash-Komplettpaket incoai/Qwen3.8-27B-Splash gefunden (17,4 GB) – für oMLX unbrauchbar
      → Keine config.json: oMLX lädt es nicht, das Modell bleibt unsichtbar.
      → oMLX braucht Zielmodell + DFlash2-Draft. Das Paket nur für eigenständiges splash serve behalten.
  ✓ oMLX: Splash kompatibel, aktive Engine: Splash

Empfehlung nach Arbeitsspeicher
  ✓ Qwen3.8-27B: bis 256K Kontext (≈ 29 GB mit Splash)
      → Splash einschalten: 1,33–1,87× mehr tok/s gemessen
  ✓ Qwen3.6-35B-A3B: bis 256K Kontext (≈ 27 GB mit Splash)
      → oMLX (MTP) ist hier schon schnell, Splash bringt nur wenig: 1,05–1,32× gemessen

Ergebnis: 0 Fehler, 2 Warnungen
```

Fehlt etwas, erscheint am Ende ein Block „Zum Kopieren“, z. B.:

```
hf download incoai/Qwen3.8-27B-DFlash2 --local-dir ~/.omlx/models/mlx-community/incoai/Qwen3.8-27B-DFlash2
# danach oMLX neu starten (oder POST /admin/api/reload – entlädt alle Modelle)
```

### So rechnet die RAM-Empfehlung

Speicherbedarf mit Splash ≈ Gewichte + Draft/3 + 3 GB + KV-Cache (int8):

| Modell | 32K | 128K | 256K |
|---|---|---|---|
| Qwen3.8-27B (16,6 GB + Draft 3,8 GB) | ≈ 22 GB | ≈ 25 GB | ≈ 29 GB |
| Qwen3.6-35B-A3B (21,1 GB + Draft 1,0 GB) | ≈ 25 GB | ≈ 26 GB | ≈ 27 GB |

Als nutzbar gelten ca. 75 % des RAM (Standardgrenze von macOS für den GPU-Speicher); der Rest bleibt für System und Apps. Beispiele: 32 GB → nur 27B mit 32K, 36 GB → beide mit 128K, ab 48 GB → beide mit 256K. Mit 24 GB oder weniger passt keins der beiden Splash-Modelle.

## splashbench – A/B-Messung

```bash
bin/splashbench -models Qwen3.8-27B-oQ4e-mtp -repeats 3 -out results.json > report.md
```

| Flag | Standard | Bedeutung |
|---|---|---|
| `-base-url` | `http://localhost:8000` | oMLX-Server |
| `-models` | `Qwen3.6-35B-A3B-oQ4e-mtp,Qwen3.8-27B-oQ4e-mtp` | Modelle, kommagetrennt |
| `-repeats` | `3` | Messläufe pro Prompt und Engine |
| `-max-tokens` | `512` | maximale Ausgabelänge pro Lauf |
| `-thinking` | aus | Reasoning (`enable_thinking`) einschalten |
| `-out` | `results.json` | Rohmessungen als JSON |

Pro Modell und Engine: Engine setzen, entladen, warten bis entladen, kurz aufwärmen (Ladezeit zählt nicht), dann die Prompts `code`, `prose` und `translate` je `-repeats`-mal. Fortschritt läuft auf stderr, die Markdown-Tabelle (Median TTFT, Median tok/s, Faktor) auf stdout. Am Ende – auch bei Fehler oder Strg-C – wird die ursprüngliche Engine wiederhergestellt.

Beispiel (Auszug):

```
| Modell | Engine | Prompt | Läufe | TTFT (Median) | tok/s (Median) | Faktor vs. oMLX |
|---|---|---|---|---|---|---|
| Qwen3.8-27B-oQ4e-mtp | oMLX | code | 3 | 148ms | 72.2 | – |
| Qwen3.8-27B-oQ4e-mtp | splash | code | 3 | 66ms | 120.1 | 1.66× |
```

Dauer: pro Modell einige Minuten (zwei Ladevorgänge plus 18 Läufe).

## splashdemo – das Live-Rennen

```bash
bin/splashdemo
bin/splashdemo -model Qwen3.6-35B-A3B-oQ4e-mtp -max-tokens 400
```

| Flag | Standard | Bedeutung |
|---|---|---|
| `-base-url` | `http://localhost:8000` | oMLX-Server |
| `-model` | `Qwen3.8-27B-oQ4e-mtp` | Modell-ID in oMLX |
| `-prompt` | Go-Aufgabe `WordFrequency`/`TopN` mit Tests (ca. 400 Tokens Code) | Prompt für beide Engines |
| `-max-tokens` | `512` | maximale Ausgabelänge |
| `-plain` | aus | ohne Vollbild-Anzeige (für Logs, Pipes, schmale Fenster) |

Ablauf:

1. Aktuelle Engine des Modells merken.
2. oMLX-Engine setzen, entladen, warten bis entladen, kurz aufwärmen – das zählt nicht.
3. Prompt senden: Der Text läuft sichtbar durchs Terminal, oben steht eine feste Statuszeile mit Zeit, erstem Token und tok/s.
4. Dasselbe mit Splash; die Kopfzeile zeigt dabei das oMLX-Ergebnis zum Vergleich.
5. Gegenüberstellung, danach die ursprüngliche Engine zurück – auch bei Fehler und bei Strg-C (weitere Strg-C während des Zurücksetzens werden ignoriert).

Die Live-Zahl in der Statuszeile ist eine Schätzung aus den gestreamten Zeichen (`≈`). Splash schickt oft mehrere Tokens pro Chunk, Chunks zu zählen wäre deshalb falsch. Nach dem ersten Lauf wird die Schätzung mit der exakten Token-Zählung von oMLX geeicht. Die Gegenüberstellung am Ende nutzt immer die exakten Werte des Servers.

Beispiel-Gegenüberstellung (Testlauf mit `-max-tokens 120`, M5 Max):

```
Ergebnis Qwen3.8-27B-oQ4e-mtp

                 oMLX         Splash
  Erstes Token   201 ms       133 ms
  Generierung    86,4 tok/s   146,3 tok/s
  Tokens         120          120
  Dauer          1,6 s        0,9 s

  Splash generiert 1,69× so schnell wie oMLX.
Engine von Qwen3.8-27B-oQ4e-mtp wird auf Splash zurückgesetzt …
Engine zurückgesetzt: Splash
```

Ein einzelner Lauf schwankt; für belastbare Zahlen `splashbench` nehmen.

Für Aufnahmen: Terminal mit mindestens 30 Zeilen und 100 Spalten, große Schrift. Das Programm nutzt nur einfache ANSI-Sequenzen (Terminal.app, iTerm2, Ghostty).

## Aufbau des Codes

```
cmd/splashui, cmd/splashcheck, cmd/splashbench, cmd/splashdemo   main.go = Verdrahtung (Composition Root)
cmd/makeapp      baut Splash Bench.app
internal/check   Diagnose-Regeln (Versionen, Chip, RAM-Empfehlung, Modellordner), Systemprofil und Bericht
internal/bench   Messlogik, Runner mit Wiederherstellung der Engine und Live-Ereignissen, Vergleich und Empfehlung, Markdown-Bericht
internal/readiness  Voraussetzungen der App: Prüfungen mit Hinweis und Befehl, Modellauswahl mit Speicherschätzung
internal/compare Sitzung der App: ein Vergleich zur Zeit, Live-Stand, Abbrechen, letzte Messung
internal/store   Messungen als JSON-Dateien
internal/web     HTTP-Server der App, Server-Sent Events, eingebettete Seite (static/)
internal/appbundle  App-Bundle: Info.plist, Icon (im Code gezeichnet), Zusammenbau
internal/demo    Rennen oMLX vs. Splash, Live-Anzeige (Vollbild und einfach)
internal/omlx    Adapter für die Admin- und Chat-API von oMLX
```

Die Fachlogik hängt nur an kleinen Interfaces (Uhr, Dateisystem, Prozessaufrufe, oMLX-Server) und ist mit Fakes getestet – ohne Netz und ohne laufenden oMLX.
