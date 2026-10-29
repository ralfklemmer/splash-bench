# Splash Bench – Handbuch

**Gleicher Mac, gleiches Modell, bis zu 1,9× mehr Tokens pro Sekunde.**
Von null bis zur eigenen Messung: oMLX mit Splash einrichten, richtig messen und entscheiden, ob sich Splash auf deinem Mac lohnt.

Ralf Klemmer · Stand: 29.10.2026 · gemessen am 29.10.2026 mit oMLX 0.7.3rc1 und Splash 1.5.0

---

## Inhalt

1. Was Splash ist – in fünf Sätzen
2. Voraussetzungen
3. Welches Modell passt zu deinem RAM?
4. Setup in 4 Schritten
5. Externe Engine einschalten und Splash-Log lesen
6. Einstellungen: was mit Splash gilt und was nicht mehr
7. Messen: eingebauter oMLX-Benchmark und splashbench
8. Die 4-Schritte-Methode für deine Hardware
9. Die 5 Fallstricke – mit Lösung
10. Referenzergebnisse: M5 Max, 128 GB
11. Kurzreferenz
12. Offene Punkte in diesem Handbuch
13. Quellen

Am einfachsten geht alles mit der App **Splash Bench**: Sie prüft die Voraussetzungen, zeigt fehlende Downloads als Befehl zum Kopieren und misst oMLX gegen Splash. Für das Terminal gibt es dieselben Abläufe als `splashcheck`, `splashbench` und `splashdemo`. Installation und alle Flags beschreibt `README.md`. Dieses Handbuch nennt nur die Flags, die du für die Abläufe brauchst.

---

## 1. Was Splash ist – in fünf Sätzen

- **oMLX** ist eine Mac-App mit Server (Port 8000, OpenAI-kompatible API). Sie bringt MLX mit und verwaltet Modelle unter `~/.omlx/models`.
- **Splash** (Inco AI) ist eine eigenständige Inferenz-Engine. oMLX kann sie als **externe Engine** nutzen: oMLX startet dann selbst `splash serve --offline` auf einem freien Loopback-Port und leitet Anfragen weiter.
- Splash nutzt **Speculative Decoding** mit einem kleinen Draft-Modell (**DFlash2**): Der Draft rät mehrere Tokens, das Zielmodell prüft sie in einem Durchgang. Gleiche Antwort, schneller.
- Ohne Splash nutzt oMLX sein eigenes Speculative Decoding, **MTP** (Multi-Token-Prediction). Das ist die Baseline, gegen die du misst.
- Splash läuft offiziell nur auf **M3 oder neuer** mit **macOS 26.4 oder neuer** und nur für bestimmte Modellfamilien. In oMLX 0.7.3rc1 sind es fünf: **Qwen3.8-27B**, **Qwen3.6-35B-A3B**, Qwen3.8-Flash-Next, Ornith-1.5-122B und Flint. Gemessen sind die ersten beiden.

Architektur:

```
Dein Programm ──► oMLX (Port 8000) ──┬──► MLX-Engine (MTP)        ──► ~/.omlx/models/...
                                      └──► Splash (Loopback-Port)  ──► Zielmodell + DFlash2-Draft
```

**Das Ergebnis vorweg:** Bei Qwen3.8-27B bringt Splash 1,3- bis 1,9-mal mehr Tokens pro Sekunde. Bei Qwen3.6-35B-A3B nur wenig (1,05- bis 1,3-mal) – oMLX (MTP) ist dort schon schnell.

---

## 2. Voraussetzungen

| Was | Anforderung | Prüfen mit |
|---|---|---|
| Mac | **Apple M3 oder neuer**. M1/M2 nur mit dem inoffiziellen Community-Port `splash-m1` (nicht von Inco AI, ohne Support) | Splash Bench / `splashcheck` |
| RAM | ab ca. 32 GB für 27B mit 32K Kontext (Schätzung, siehe Kapitel 3) | Splash Bench / `splashcheck` |
| Speicherplatz | 27B: 16,6 GB + 3,8 GB Draft = 20,4 GB · 35B-A3B: 21,1 GB + 1,0 GB Draft = 22,1 GB, dazu der SSD-Cache von Splash (Standard: so groß wie der oMLX-SSD-Cache, bei Ralf 92 GB; einstellbar, Kapitel 6) | Finder / `df -h ~` |
| macOS | **26.4 oder neuer** (die Homebrew-Formel bricht sonst ab); gemessen mit macOS 27.0.1 | `sw_vers -productVersion` |
| oMLX | installiert und gestartet, Server auf Port 8000 (Kapitel 4, Vorab) | Splash Bench / `splashcheck` |
| Splash | mindestens 1.3.0; für Mixed-Bit-Modelle 1.3.1, für ein Thinking-Budget 1.3.2, für Qwen3.8-Flash-Next 1.4.0, für Ornith-1.5-122B und Flint 1.4.1. Ist Splash zu alt, graut oMLX den Schalter aus und nennt den Grund. | `splash --version` |
| Homebrew | für die Installation von Splash | `brew --version` |
| Hugging Face CLI | Befehl `hf` für die Downloads (alternativ der Download in der oMLX-Admin-Oberfläche) | `hf --help` |
| oMLX-API-Key | für Splash Bench, `splashbench`, `splashdemo` und die Admin-API; `splashcheck` läuft auch ohne | Splash Bench liest ihn selbst aus `~/.omlx/settings.json`; im Terminal Umgebungsvariable `OMLX_API_KEY` |

API-Key für die Sitzung im Terminal setzen (die App braucht das nicht):

```sh
export OMLX_API_KEY="dein-key"
```

> Den Key nie in Skripte, Screenshots oder Messdateien schreiben. Die Programme lesen ihn aus der Umgebungsvariable (die App sonst aus `~/.omlx/settings.json`), schicken ihn nur an deinen oMLX-Server (`-base-url`, Standard `http://localhost:8000`) und zeigen ihn nirgends an.

**Erster Schritt immer:** Splash Bench starten und die Voraussetzungen ansehen – oder im Terminal:

```sh
splashcheck
```

Beide zeigen Chip, RAM, macOS, Splash-Version, ob oMLX erreichbar ist, welche Zielmodelle und Drafts im Modellordner liegen, ob oMLX Splash für das Modell für kompatibel hält, welche Modelle zu deinem RAM passen – und gibt dir fertige `hf download`-Befehle für alles, was fehlt.

---

## 3. Welches Modell passt zu deinem RAM?

### Die zwei gemessenen Modelle

| | Qwen3.8-27B | Qwen3.6-35B-A3B |
|---|---|---|
| Zielmodell (MLX, 4 Bit) | `Jundot/Qwen3.8-27B-oQ4e-mtp` (16,6 GB) | `Jundot/Qwen3.6-35B-A3B-oQ4e-mtp` (21,1 GB) |
| Draft (DFlash2) | `incoai/Qwen3.8-27B-DFlash2` (3,8 GB) | `incoai/Qwen3.6-35B-A3B-DFlash2` (1,0 GB) |
| Gewinn durch Splash (gemessen, M5 Max) | 1,33–1,87× | 1,05–1,32× |
| Empfehlung | **Splash an** | kann an, bringt aber nur wenig – oMLX (MTP) ist hier schon schnell |

### Neue Modellfamilien (nicht gemessen)

Seit Splash 1.4 und oMLX 0.7.3rc1 kann oMLX drei weitere Familien an Splash geben. Gemessen ist davon nichts. Die Speicherwerte sind Schätzungen mit derselben Formel, der KV-Cache ist aus der Zahl der Full-Attention-Schichten hochgerechnet.

| Familie | Zielmodell | Draft (DFlash2) | Splash ab | ≈ Bedarf bei 32K |
|---|---|---|---|---|
| Qwen3.8-Flash-Next | GGUF `ISTA-DASLab/Qwen3.8-Flash-Next-GSQ-RCO-GGUF`, Variante `Q2_0` (61,8 GB) + Vision-Projektor (0,8 GB) | `incoai/Qwen3.8-Flash-Next-DFlash2` (1,1 GB) | 1.4.0 | ≈ 66 GB |
| Ornith-1.5-122B | MLX `ornith-ai/Ornith-1.5-122B-MLX-4bit` (64,9 GB) | `incoai/Ornith-1.5-122B-DFlash2` (1,7 GB) | 1.4.1 | ≈ 69 GB |
| Flint | GGUF `unsloth/flint-GGUF`, Datei `flint-UD-Q4_K_XL.gguf` (23,3 GB), nur Text | `incoai/Flint-DFlash2` (0,8 GB) | 1.4.1 | ≈ 27 GB |

Flash-Next und Flint lädt Splash **nur als GGUF**, nicht als MLX. oMLX liest GGUF-Dateien direkt aus dem Modellordner und benennt das Modell nach der Datei. `splashcheck` und die App zeigen die passenden `hf download`-Befehle, laden aber nichts ungefragt.

Der Katalog in Splash Bench ist von Hand gepflegt (Stand 29.10.2026). Was oMLX tatsächlich an Splash gibt, entscheidet oMLX selbst. Die App zeigt deshalb auch Modelle, die nicht im Katalog stehen, und übernimmt den fehlenden Draft direkt aus der oMLX-Antwort.

### Speicherbedarf mit Splash

Faustformel: **Gewichte + Draft/3 + 3 GB + KV-Cache (int8)**

| Kontext | Qwen3.8-27B | Qwen3.6-35B-A3B |
|---|---|---|
| 32K | ≈ 22 GB | ≈ 25 GB |
| 128K | ≈ 25 GB | ≈ 26 GB |
| 256K | ≈ 29 GB | ≈ 27 GB |

Gegenprobe aus der Messung: 27B mit 131.072 Prompt-Tokens → Splash-Prozess **25,3 GB** Spitzenspeicher. Die Formel passt.

Der KV-Cache des 35B-A3B wächst mit dem Kontext deutlich langsamer als beim 27B. Ab 256K braucht das größere Modell sogar weniger Speicher.

### RAM-Stufen

Nur die 128-GB-Stufe ist gemessen. Alles andere ist aus der Formel abgeleitet – **Schätzung**. macOS und deine anderen Apps brauchen zusätzlich Speicher. Die Tabelle rechnet wie `splashcheck` mit etwa 75 % des RAM als nutzbar für das Modell (Annahme, nicht gemessen).

| RAM | Qwen3.8-27B | Qwen3.6-35B-A3B | Status |
|---|---|---|---|
| 16–24 GB | nein | nein | Schätzung |
| 32 GB | 32K, knapp | nein, zu knapp | Schätzung |
| 36 GB | 32K; 128K knapp | bis 256K, knapp | Schätzung |
| 48 GB | bis 256K | bis 256K | Schätzung |
| 64 GB | bis 256K, komfortabel | bis 256K, komfortabel | Schätzung |
| 96–128 GB | bis 256K, beide parallel möglich | bis 256K | **128 GB gemessen** (M5 Max) |

Die ausführliche Matrix mit Begründung steht in `vorlagen/modell-matrix.md`.

---

## 4. Setup in 4 Schritten

### Vorab – oMLX und Homebrew

MLX musst du nicht separat installieren: Die oMLX-App bringt MLX und ihre eigene Python-Umgebung im App-Bundle mit.

1. **oMLX** von omlx.ai oder GitHub (`jundot/omlx`) laden und nach `/Applications` ziehen. Beim ersten Start legt oMLX `~/.omlx` an, mit dem Modellordner `~/.omlx/models` und der Konfiguration `~/.omlx/settings.json`.
2. **Admin-Oberfläche** öffnen: `http://localhost:8000/admin`. Dort einen **API-Key** setzen. Admin-API, Benchmark, Splash Bench sowie `splashbench` und `splashdemo` brauchen ihn.
3. **Homebrew** installieren, falls noch nicht vorhanden: Befehl von brew.sh kopieren.

### Schritt 1 – Splash installieren

```sh
brew install incoai/tap/splash
splash --version
```

Erwartet: **1.3.0 oder neuer** (gemessen mit 1.5.0). Das offizielle Splash läuft nur auf **M3 oder neuer** mit **macOS 26.4 oder neuer**. Für **M1/M2** gibt es den inoffiziellen Community-Port `splash-m1` (Releases unter github.com/paperniuk/splash). Er stammt nicht von Inco AI, und Inco unterstützt ihn nicht. Ohne Homebrew legt dessen `install-m1.sh` das Programm nach `~/.local/bin`; oMLX sucht dort und in den Homebrew-Ordnern.

Danach **oMLX neu starten**, damit es Splash findet. Erst dann erscheint unter Model Settings → Advanced der Schalter „Use external engine“.

### Schritt 2 – Zielmodell **und** Draft laden

Wichtig: Du brauchst **zwei** Ordner – Zielmodell und Draft. **Nicht** das Splash-Komplettpaket (`incoai/Qwen3.8-27B-Splash` u. a.) – das ist nur für eigenständiges `splash serve` und für oMLX unsichtbar (Fallstrick 1).

Qwen3.8-27B:

```sh
hf download Jundot/Qwen3.8-27B-oQ4e-mtp \
  --local-dir ~/.omlx/models/Jundot/Qwen3.8-27B-oQ4e-mtp

hf download incoai/Qwen3.8-27B-DFlash2 \
  --local-dir ~/.omlx/models/incoai/Qwen3.8-27B-DFlash2
```

Qwen3.6-35B-A3B:

```sh
hf download Jundot/Qwen3.6-35B-A3B-oQ4e-mtp \
  --local-dir ~/.omlx/models/Jundot/Qwen3.6-35B-A3B-oQ4e-mtp

hf download incoai/Qwen3.6-35B-A3B-DFlash2 \
  --local-dir ~/.omlx/models/incoai/Qwen3.6-35B-A3B-DFlash2
```

Abkürzung: Splash Bench (unter „Empfehlungen“) und `splashcheck` geben dir genau die Befehle aus, die bei dir noch fehlen.

Welcher Modellordner gilt, steht in `~/.omlx/settings.json` unter `model.model_dirs` (bei Ralf `~/.omlx/models/mlx-community`). Statt `hf` kannst du die Modelle auch in der oMLX-Admin-Oberfläche über den eingebauten Hugging-Face-Download laden.

Modelle mit 8 Bit (z. B. `Qwen3.8-27B-8bit`) sind nicht getestet. Splash prüft die Bitbreiten selbst.

### Schritt 3 – oMLX die Modelle neu einlesen lassen

Entweder oMLX neu starten oder über die Admin-API:

```sh
curl -X POST http://localhost:8000/admin/api/reload \
  -H "Authorization: Bearer $OMLX_API_KEY"
```

Achtung: **Reload entlädt alle Modelle** (Fallstrick 5). Nicht mitten in laufender Arbeit.

### Schritt 4 – Externe Engine einschalten

oMLX → **Model Settings** → Modell wählen → **Advanced** → **„Use external engine“** einschalten.

Ist der Schalter **ausgegraut**, nennt oMLX den Grund. Meist fehlt der Draft oder oMLX hat noch nicht neu eingelesen (Fallstrick 2).

Kontrolle:

```sh
splashcheck
```

---

## 5. Externe Engine einschalten und Splash-Log lesen

### Was beim Einschalten passiert

1. Du schaltest „Use external engine“ ein.
2. Bei der nächsten Anfrage startet oMLX selbst `splash serve --offline` auf einem **freien Loopback-Port**. Port 8000 bleibt bei oMLX – du musst nichts umkonfigurieren (Fallstrick 3).
3. oMLX leitet die Anfragen an Splash weiter. Dein Programm spricht weiter mit `http://localhost:8000`.
4. Die erste Anfrage lädt das Modell. Die erste Messung ist deshalb langsam – immer aufwärmen (macht `splashbench` automatisch).

Schaltest du zwischen den Engines um, entlädt oMLX das Modell asynchron. Kommt in dieser Zeit eine Anfrage, antwortet oMLX mit **HTTP 409 „unload is pending“** – warten, dann erneut (Fallstrick 4).

### Splash-Log lesen

oMLX schreibt pro Modell ein Log nach `~/.omlx/logs/splash/` (Dateiname `omlx--<Modell-ID>-<hash>.log`). Nach dem Start zeigt die `Ready`-Zeile den eingeplanten Kontext, danach steht pro Anfrage eine `Done`-Zeile:

```
17:16:47 Ready · omlx/Qwen3.8-27B-oQ4e-mtp-e5133ef4 · context 256K · http://127.0.0.1:52180
17:16:52 Done · input 74 · cached 64 · output 512 · TTFT 0.3s · 120.1 tok/s · 4.5 tok/cycle
```

Live mitlesen: `tail -f ~/.omlx/logs/splash/*.log`

Die wichtigste Zahl im Splash-Log ist die **Draft-Trefferquote: wie viele Tokens pro Zyklus das Zielmodell vom Draft übernimmt**.

| Workload | Tokens pro Zyklus (gemessen) | Bedeutung |
|---|---|---|
| Code | ca. 4,5 | Draft rät gut → großer Gewinn |
| Prosa | ca. 3 | Draft rät schlechter → kleinerer Gewinn |

Faustregel: Je vorhersagbarer der Text (Code, Übersetzung eines vorgegebenen Texts), desto mehr Tokens pro Zyklus, desto mehr bringt Splash. Freie Prosa profitiert am wenigsten.

Fehlermeldung, die du kennen solltest:

```
Splash needs the … DFlash2 draft in a model directory
```

→ Draft fehlt oder oMLX hat nicht neu eingelesen. Siehe Fallstrick 2.

---

## 6. Einstellungen: was mit Splash gilt und was nicht mehr

| Einstellung | oMLX-Engine (MLX) | Externe Engine (Splash) |
|---|---|---|
| Speculative Decoding | MTP (oMLX-eigen) | DFlash2-Draft (Splash) – MTP greift nicht |
| Schalter | „Use external engine“ aus | „Use external engine“ an (Model Settings → Advanced) |
| Port | 8000 | oMLX bleibt auf 8000, Splash auf freiem Loopback-Port |
| Modellordner | Zielmodell | Zielmodell **und** Draft-Ordner |
| Unterstützte Modelle | alle, die oMLX kann | nur Qwen3.8-27B und Qwen3.6-35B-A3B |
| ANE/GPU-Aufteilung (ANE-Tuning) | wirkt | gilt für die oMLX-Engine, nicht für Splash |
| KV-Cache | oMLX-Einstellungen | int8 (Grundlage der Speicherformel) |
| Spitzenspeicher 27B @ 131K (gemessen) | 33,9 GB (MLX) | 25,3 GB (Splash-Prozess) |

**Konsequenz für Vergleiche:** Wenn du an oMLX-Einstellungen drehst oder das ANE-Tuning laufen lässt (siehe Kapitel 8, Schritt 3), verbesserst du die Baseline – nicht Splash. Deshalb erst oMLX gut einstellen, dann Splash dagegen messen. Sonst vergleichst du Splash mit einem schlecht eingestellten oMLX.

### Was du für Splash einstellen kannst

Diese Einstellungen gibt oMLX an Splash weiter. Global stehen sie in `~/.omlx/settings.json` unter `integrations`, das Kontextlimit pro Modell in den Model Settings.

| Einstellung | Standard | Wann ändern |
|---|---|---|
| `max_context_window` (pro Modell) | leer: Splash passt den Kontext an den Speicher an (höchstens 256K) | kleiner setzen, wenn ein zweites Modell gleichzeitig geladen bleiben soll |
| `splash_kv_format` | `int8` | `bf16` nur bei vermuteten Genauigkeitsproblemen mit langem Kontext; kostet doppelten KV-Speicher |
| `splash_ane_prefill` | an | aus, wenn ein Vergleich zeigt, dass der Prefill nur auf der GPU schneller ist |
| `splash_ssd_cache_gb` | leer: so groß wie der oMLX-SSD-Cache (bei Ralf 92 GB) | kleiner, wenn die SSD knapp ist |
| `splash_max_memory_gb` | leer: oMLX rechnet das Budget selbst | nur bei Speicherkonflikten mit anderen Programmen |

**Mit Splash wirken nicht:** `mtp_enabled`, `qwen35_ane_prefill_*`, TurboQuant-KV, DFlash und SpecPrefill. Das sind In-Process-Optionen der oMLX-Engine; sie greifen wieder, sobald Splash aus ist. So steht es im oMLX-Code (`engine/splash.py`), gemessen ist es nicht.

---

## 7. Messen

Zwei Werkzeuge, zwei Fragen:

| | eingebauter oMLX-Benchmark | Splash Bench / `splashbench` |
|---|---|---|
| Frage | Wie verhält sich das Modell bei **langem Kontext**? | Was bringt Splash bei **meinen typischen Aufgaben**? |
| Prompt | synthetisch, z. B. 131.072 Prompt-Tokens, 128 Ausgabe-Tokens | echte Prompts: `code`, `prose`, `translate` |
| Ergebnis | Prompt Processing, Token Generation, TTFT, Spitzenspeicher | TTFT und tok/s je Engine und Prompt, Median, Faktor |
| Engine-Umschaltung | von Hand | automatisch, stellt danach den Ausgangszustand wieder her |

### Die Kennzahlen

| Kennzahl | Bedeutung | Spürst du als … |
|---|---|---|
| **tok/s (Generierung)** | Tokens pro Sekunde beim Schreiben der Antwort | wie schnell der Text fließt |
| **TTFT** (Time to First Token) | Zeit bis zum ersten Token | wie lange du nach dem Absenden wartest |
| **Prompt Processing** | Tokens pro Sekunde beim Einlesen des Prompts | Wartezeit bei langem Kontext (Agenten, große Dateien) |
| **Spitzenspeicher** | maximaler RAM-Verbrauch | ob es auf deinem Mac überhaupt passt |

### 7.1 Eingebauter oMLX-Benchmark (langer Kontext)

Für diesen Vergleich musst du nichts herunterladen oder programmieren. Der Benchmark hat einen eigenen Splash-Pfad; du misst dasselbe Modell einmal mit Splash aus und einmal mit Splash an.

1. Engine wählen (externe Engine an oder aus, Kapitel 4, Schritt 4).
2. Benchmark in oMLX starten, Modell wählen, Prompt-Länge einstellen. Möglich sind 1.024, 4.096, 8.192, 16.384, 32.768, 65.536, 131.072 und 200.000 Tokens. Die Generierungslänge ist standardmäßig 128 Tokens. Als Textkorpus stehen `code_python`, `code_mixed`, `novel_en`, `novel_ja` und `novel_ko` zur Wahl.
3. Werte notieren: Prompt Processing, Token Generation, TTFT, Spitzenspeicher → `vorlagen/messvorlage.csv`.
4. Engine umschalten, warten bis entladen (sonst 409), gleichen Lauf wiederholen.

Dasselbe per API, z. B. für 131K Kontext:

```sh
curl -s -X POST http://localhost:8000/admin/api/bench/start \
  -H "Authorization: Bearer $OMLX_API_KEY" -H "Content-Type: application/json" \
  -d '{"model_id":"Qwen3.8-27B-oQ4e-mtp","prompt_lengths":[131072]}'
# Ergebnis: GET /admin/api/bench/<bench_id>/results
```

Grenzen des eingebauten Benchmarks:

- Er misst **synthetischen Korpustext**. Wie viel Speculative Decoding bringt, hängt aber stark vom Inhalt ab: Beim 35B-A3B brachte Splash bei Prosa praktisch nichts (1,05×), bei Übersetzungen 1,32×. Für typische Aufgaben deshalb `splashbench` (7.2).
- Pro Start gibt es **einen einzigen Lauf** ohne Wiederholungen. Unterschiede unter etwa 10 % sind nicht belastbar.
- oMLX lädt Splash-Läufe **nicht** zu omlx.ai hoch (`skipped_reason: splash`). Community-Vergleiche dort zeigen nur oMLX-Läufe; einen öffentlichen Splash-Vergleich gibt es nur über eigene Tabellen.

### 7.2 splashbench (typische Aufgaben)

```sh
export OMLX_API_KEY="dein-key"

splashbench \
  -models Qwen3.8-27B-oQ4e-mtp \
  -repeats 3 \
  -max-tokens 512 \
  -out results.json
```

Was passiert:

1. Für jedes Modell: Engine auf oMLX stellen, aufwärmen, je Prompt (`code`, `prose`, `translate`) `-repeats` Läufe.
2. Engine auf Splash stellen, aufwärmen, gleiche Läufe.
3. Ausgangszustand der Engine wiederherstellen.
4. Rohwerte nach `-out` (JSON), Zusammenfassung als Markdown-Tabelle auf der Konsole: TTFT (Median), tok/s (Median), Faktor vs. oMLX.

Wichtige Flags (vollständig in `README.md`):

| Flag | Bedeutung | Referenzlauf |
|---|---|---|
| `-models` | Modelle, kommagetrennt (Ordnername wie in oMLX) | beide Modelle |
| `-repeats` | Läufe pro Prompt und Engine | 3 |
| `-max-tokens` | maximale Ausgabelänge | 512 |
| `-thinking` | Reasoning einschalten | aus |
| `-out` | Datei für Rohmessungen | `results.json` |

Tipps:

- Während der Messung nichts anderes auf dem Mac laufen lassen (Build, Videocall, Browser mit vielen Tabs).
- Netzteil anschließen, andere Modelle entladen, Benachrichtigungen aus.
- Einzelläufe streuen stark, besonders TTFT. Für eine Entscheidung lieber `-repeats 5` als 3.
- Mit Reasoning (`-thinking`) misst du einen anderen Workload. Nicht mit Läufen ohne Reasoning vergleichen.

### 7.3 Ergebnisse festhalten

Trag jede Messung in `vorlagen/messvorlage.csv` ein (Semikolon als Trenner, Dezimalkomma – öffnet sich direkt in Numbers und deutschem Excel). Die Zeilen mit „BEISPIEL“ sind Ralfs Werte; lösch sie oder lass sie zum Vergleich stehen. Eine Zeile = ein Modell × eine Engine × ein Workload.

---

## 8. Die 4-Schritte-Methode für deine Hardware

Ziel: In ca. 20 Minuten wissen, ob Splash auf **deinem** Mac für **deine** Arbeit schneller ist – ohne eigene Entwicklung.

### Schritt 1 – Hardware bestimmen

```sh
splashcheck
```

Notiere Chip, RAM, macOS, oMLX- und Splash-Version in der ersten Zeile der Messvorlage. Wähle das Modell nach RAM-Stufe (Kapitel 3).

### Schritt 2 – Community-Bestwerte nachschlagen

In der oMLX-Suche nach **„optimal settings“** für deinen Chip und dein Modell suchen. Das zeigt dir, was oMLX allein auf ähnlicher Hardware schafft.

Beispiel: Qwen3.8-27B auf M5 Max mit oMLX bis **98,5 tok/s** Generierung (64-GB-Gerät, `code_mixed`, 4K Kontext). Ralfs eigene oMLX-Baseline lag bei 72,5 tok/s (`code`). Lehre: Die Baseline ist oft noch nicht ausgereizt.

### Schritt 3 – ANE einmessen (faire oMLX-Baseline)

oMLX hat einen eingebauten **ANE-Tuning-Lauf**. Er stellt die Aufteilung zwischen Neural Engine (ANE) und GPU für die oMLX-Engine ein, **ohne gespeicherte Einstellungen zu ändern**.

1. Externe Engine **aus** (der Lauf gilt für die oMLX-Engine, nicht für Splash).
2. ANE-Tuning starten:

   ```sh
   curl -X POST http://localhost:8000/admin/api/bench/ane-tune/start \
     -H "Authorization: Bearer $OMLX_API_KEY"
   ```

3. Danach die Baseline messen (Splash Bench, `splashbench` oder eingebauter Benchmark) und mit den Community-Bestwerten aus Schritt 2 vergleichen.
4. Liegst du deutlich darunter: oMLX-Einstellungen in Richtung der Community-Bestwerte bringen und erneut messen, bis die Werte stabil sind.

Ergebnis: eine **faire Baseline**. Erst gegen die misst du Splash.

### Schritt 4 – Splash messen und entscheiden

Messmatrix pro Modell:

| Dimension | Werte | Warum |
|---|---|---|
| Engine | oMLX (faire Baseline aus Schritt 3), Splash | die eigentliche Frage |
| Inhalt | `code`, `prose`, `translate`, dein typischer Prompt | Speculative Decoding hängt vom Inhalt ab |
| Kontext | kurz (unter 1K), 32K, 131K | bei langem Kontext dominiert der Prefill |
| Splash-ANE-Prefill | an, aus | wirkt nur beim Prefill (Kapitel 6) |
| Wiederholungen | mindestens 3, Median | ein Einzellauf schwankte bis zu 20 % |

Kurze Prompts misst Splash Bench (oder `splashbench`), 32K und 131K der eingebaute Benchmark (7.1).

1. In Splash Bench den Vergleich für dein Modell starten (oder `splashbench`; beide vergleichen automatisch gegen die Baseline).
2. Bestimme deinen **Hauptworkload**: Was machst du überwiegend – Code, Prosa, Übersetzen, Agent mit langem Kontext?
3. Entscheidungsregel anwenden:

> **Splash an, wenn: ≥ 15 % mehr tok/s auf deinem Hauptworkload UND TTFT nicht schlechter.**
> Sonst: oMLX lassen.

Angewendet auf Ralfs Messungen:

| Modell | Workload | tok/s | TTFT | Entscheidung |
|---|---|---|---|---|
| 27B | code | +66 % | 148 → 66 ms (besser) | **Splash an** |
| 27B | prose | +33 % | 150 → 155 ms (gleich, innerhalb der Streuung) | **Splash an** |
| 27B | translate | +87 % | 163 → 86 ms (besser) | **Splash an** |
| 27B | 131K Kontext | +35 % | 289,3 → 207,2 s (besser) | **Splash an** |
| 35B-A3B | code | +24 % | 47 → 22 ms (besser) | kann an, Gewinn klein |
| 35B-A3B | prose | +5 % | 47 → 38 ms (besser) | praktisch gleich – Regel nicht erfüllt |
| 35B-A3B | translate | +32 % | 49 → 35 ms (besser) | kann an, Gewinn klein |

Liegt ein TTFT-Unterschied innerhalb der Streuung deiner Einzelläufe, miss mit mehr Läufen nach, bevor du entscheidest.

---

## 9. Die 5 Fallstricke – mit Lösung

| # | Symptom | Ursache | Lösung |
|---|---|---|---|
| 1 | Modell taucht in oMLX nicht auf | Splash-**Komplettpaket** geladen (`incoai/Qwen3.8-27B-Splash` u. a., 17–20 GB). Es hat keine `config.json` und ist nur für eigenständiges `splash serve`. | Komplettpaket aus `~/.omlx/models` entfernen. Zielmodell **und** Draft getrennt laden (Kapitel 4, Schritt 2). Neu einlesen. |
| 2 | `Splash needs the … DFlash2 draft in a model directory` oder Schalter ausgegraut | Draft fehlt oder oMLX hat ihn noch nicht eingelesen. Der `draft/`-Ordner im Komplettpaket zählt nicht. | Draft nach `~/.omlx/models/incoai/<Modell>-DFlash2` laden, oMLX neu einlesen (Neustart oder `POST /admin/api/reload`), `splashcheck` |
| 3 | Sorge: Splash blockiert Port 8000 | – | Unbegründet. oMLX startet Splash auf einem freien Loopback-Port. Nichts tun. |
| 4 | HTTP 409 „unload is pending“ | oMLX entlädt das Modell asynchron (z. B. nach Engine-Wechsel) | Warten und erneut senden. `splashbench` wartet selbst. |
| 5 | Nach Reload sind alle Modelle weg / erste Anfrage dauert lang | `POST /admin/api/reload` entlädt **alle** Modelle | Reload nur, wenn nichts läuft. Danach aufwärmen, bevor du misst. |

Ralfs eigener Fehler beim Einrichten war Nr. 1: 17 GB heruntergeladen, Modell unsichtbar. Richtig sind Zielmodell + Draft.

Ein weiterer Punkt betrifft die Auswertung, nicht die Technik: Ein Vergleichswert von omlx.ai stammt aus einem anderen Lauf als deine eigene Splash-Messung. Für eine Entscheidung beide Engines direkt nacheinander auf demselben Mac messen.

---

## 10. Referenzergebnisse: M5 Max, 128 GB

Hardware und Software: M5 Max, 128 GB, macOS 27.0.1, oMLX 0.7.3rc1, Splash 1.5.0. Gemessen am 29.10.2026.

### Kurze Prompts (splashbench)

Median aus 3 Läufen, bis 512 Tokens, Reasoning aus. Generierung in tok/s, TTFT in ms.

| Modell | Prompt | oMLX (MTP) tok/s | Splash tok/s | Faktor | TTFT oMLX | TTFT Splash |
|---|---|---|---|---|---|---|
| Qwen3.8-27B | code | 72,2 | 120,1 | **1,66×** | 148 | 66 |
| Qwen3.8-27B | prose | 50,4 | 67,0 | **1,33×** | 150 | 155 |
| Qwen3.8-27B | translate | 64,0 | 119,7 | **1,87×** | 163 | 86 |
| Qwen3.6-35B-A3B | code | 202,2 | 251,4 | 1,24× | 47 | 22 |
| Qwen3.6-35B-A3B | prose | 181,1 | 190,3 | 1,05× | 47 | 38 |
| Qwen3.6-35B-A3B | translate | 215,6 | 285,3 | 1,32× | 49 | 35 |

Beim 35B-A3B ist Splash überall etwas schneller, aber nur wenig; bei Prosa praktisch gleich. oMLX mit MTP liefert hier schon rund 200 tok/s, da bleibt wenig Luft, und bei Prosa rät der Draft schlechter.

### Langer Kontext (eingebauter oMLX-Benchmark, Qwen3.8-27B)

131.072 Prompt-Tokens, 128 Ausgabe-Tokens, Korpus `code_python`, je ein eigener Lauf pro Engine.

| Kennzahl | oMLX mit MTP | Splash | Unterschied |
|---|---|---|---|
| Prompt Processing | 453,1 tok/s | 632,6 tok/s | +40 % |
| Token Generation | 52,2 tok/s | 70,6 tok/s | +35 % |
| Zeit bis erstes Token | 289,3 s (knapp 5 min) | 207,2 s (knapp 3,5 min) | −82 s |
| Spitzenspeicher | 33,9 GB (MLX) | 25,3 GB (Splash-Prozess) | −8,6 GB |

### Was man daraus lernt

- **27B: Splash an.** 1,3- bis 1,9-mal mehr Tokens pro Sekunde bei gleicher Antwort. Auch das erste Token kommt meist früher. Bei langem Kontext +35 % beim Schreiben und weniger Speicher.
- **35B-A3B: kann an, bringt aber nur wenig.** oMLX (MTP) ist bei diesem MoE-Modell schon schnell; Splash ist 1,05- bis 1,3-mal so schnell, vor allem bei Code und Übersetzung, bei Prosa praktisch gleich.
- **Langer Kontext bleibt Geduldssache.** Splash liest 40 % schneller ein und spart rund 80 Sekunden bis zum ersten Token – trotzdem wartest du bei 131K noch knapp 3,5 Minuten. Das Einlesen bleibt der Flaschenhals. Wiederholt sich der Kontext (Agent, gleiche Codebasis), hilft der SSD-Prefix-Cache von Splash.
- **Erst oMLX einstellen, dann vergleichen.** Die Community schafft mit oMLX allein bis 98,5 tok/s beim 27B auf M5 Max.
- **Messen statt raten.** Dein Chip, dein RAM, deine Prompts – die Zahlen oben sind ein Startpunkt, kein Versprechen.

---

## 11. Kurzreferenz

```sh
# Diagnose
splashcheck

# Splash
brew install incoai/tap/splash
splash --version                      # >= 1.3.0

# Modelle (27B)
hf download Jundot/Qwen3.8-27B-oQ4e-mtp --local-dir ~/.omlx/models/Jundot/Qwen3.8-27B-oQ4e-mtp
hf download incoai/Qwen3.8-27B-DFlash2  --local-dir ~/.omlx/models/incoai/Qwen3.8-27B-DFlash2

# Neu einlesen (entlädt ALLE Modelle)
curl -X POST http://localhost:8000/admin/api/reload -H "Authorization: Bearer $OMLX_API_KEY"

# Schalter: Model Settings → Advanced → „Use external engine“

# Messen
splashbench -models Qwen3.8-27B-oQ4e-mtp -repeats 3 -max-tokens 512 -out results.json

# Live-Vergleich wie im Video
splashdemo -model Qwen3.8-27B-oQ4e-mtp
```

**Entscheidungsregel:** ≥ 15 % mehr tok/s auf dem Hauptworkload **und** TTFT nicht schlechter → Splash an.

---

## 12. Offene Punkte in diesem Handbuch

Diese Punkte sind im Projekt-Brief nicht belegt und vor der Veröffentlichung zu klären:

- **oMLX installieren:** genauen Download-Weg und Signierung der App vor der Aufnahme prüfen.
- **Hugging Face CLI `hf`:** Installationsweg fehlt.
- **M1/M2:** `splash-m1` ist nicht getestet; Bezugsquelle siehe Kapitel 4, Schritt 1.
- **Eingebauter oMLX-Benchmark:** Menüpfad in der App fehlt (die Optionen stehen in Kapitel 7.1).
- **ANE-Tuning (Methode Schritt 3):** Der Body braucht `model_id`. Laut oMLX-Code sind optional `sequence_length` (Standard 2048) und `repeats` (Standard 2) sowie Schalter für CPU- und ANE-Anteile. Ergebnis per `GET /admin/api/bench/ane-tune/<tuning_id>/results`. Offen sind die Laufzeit und ob es einen Knopf in der App gibt.
- **RAM-Stufen unter 128 GB:** nur geschätzt. Die 75-%-Annahme für den nutzbaren RAM (aus Splash Bench und `splashcheck`) ist nicht gemessen und nicht im Brief belegt; Community-Messungen von kleineren Macs wären die beste Bestätigung.
- **Lizenzen** von Qwen-Modellen, Drafts und Splash: nicht geprüft. Splash Bench verteilt keine Modelle und kein Splash; es gibt nur die Download-Befehle aus. Lizenzen und Nutzungsbedingungen bitte bei den Anbietern auf Hugging Face und bei Inco AI nachlesen.

---

## 13. Quellen

- oMLX: GitHub `jundot/omlx`. Splash-Integration gelesen in `/Applications/oMLX.app/Contents/Resources/omlx/engine/splash.py` und `settings.py`, oMLX 0.7.1rc1.
- Splash: GitHub `incoai/splash`, Homebrew-Formel `incoai/tap/splash` 1.5.0.
- splash-m1 für M1/M2: github.com/paperniuk/splash/releases.
- omlx.ai-Benchmark Qwen3.8-27B-oQ4e-mtp, M5 Max 128 GB: https://omlx.ai/benchmarks/performance/9qwu2bjm
- Eigene Messungen vom 29.10.2026 (oMLX 0.7.3rc1, Splash 1.5.0): Splash Bench (Rohdaten im Episodenordner, nicht in diesem Repo) und eingebauter oMLX-Benchmark, Läufe `bench-2b8b1a123d70` und `bench-25fa79b096ca`.
