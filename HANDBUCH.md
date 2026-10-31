# Splash Bench – Handbuch

**Gleicher Mac, gleiches Modell, bis zu 1,9× mehr Tokens pro Sekunde.**
oMLX mit Splash einrichten und messen, ob es sich auf deinem Mac lohnt.

Ralf Klemmer · Stand: 29.10.2026 · gemessen mit oMLX 0.7.3rc1 und Splash 1.5.0

---

## Inhalt

1. Was du brauchst
2. Einrichten in 7 Schritten
3. Messen und entscheiden
4. Wenn es hakt
5. Ralfs Ergebnisse (M5 Max, 128 GB)
6. Für das Terminal
7. Quellen

---

## 1. Was du brauchst

- Mac mit **Apple M3 oder neuer** und **macOS 26.4 oder neuer**
- **32 GB RAM** oder mehr (Schätzung, siehe `vorlagen/modell-matrix.md`)
- ca. **25 GB freier Speicherplatz** pro Modell
- Internet für die Downloads

M1/M2: Das offizielle Splash läuft dort nicht. Es gibt einen inoffiziellen Community-Port `splash-m1` (github.com/paperniuk/splash/releases), nicht von Inco AI und ohne deren Support.

So arbeitet Splash: oMLX bleibt deine App auf Port 8000. Mit dem Schalter „Use external engine“ gibt oMLX ein Modell an Splash weiter. Splash rechnet mit einem kleinen Hilfsmodell, dem **Draft**, das mehrere Tokens vorausrät. Die Antwort bleibt gleich, sie kommt nur schneller.

---

## 2. Einrichten in 7 Schritten

Splash Bench prüft alles und sagt dir, was fehlt. **Splash Bench installiert und lädt selbst nichts.** Splash installierst du mit einem Befehl im Terminal, die Modelle lädst du im oMLX-Dashboard.

### Schritt 1 – oMLX installieren

1. oMLX von omlx.ai oder github.com/jundot/omlx laden, nach **Programme** ziehen, starten.
2. In der Menüleiste auf das oMLX-Symbol → **Open Web Dashboard**. Dort einen **API-Key** setzen.

✓ Kontrolle: Das Dashboard öffnet sich im Browser.

### Schritt 2 – Splash Bench installieren

1. Auf github.com/ralfklemmer/splash-bench/releases/latest die Datei `Splash-Bench-<Version>.dmg` laden.
2. DMG öffnen, **Splash Bench** in **Programme** ziehen, starten.
3. Meldet macOS, die App könne nicht geprüft werden: **Fertig** klicken → **Systemeinstellungen → Datenschutz & Sicherheit** → ganz unten **Trotzdem öffnen**. Das ist nur einmal nötig.

✓ Kontrolle: Splash Bench öffnet sich im Browser. Oben steht der Status, ein Klick darauf zeigt die **Voraussetzungen**.

Ab jetzt gilt nach jedem Schritt: In Splash Bench **Erneut prüfen** klicken.

### Schritt 3 – Homebrew installieren

Nur falls noch nicht vorhanden: Befehl von brew.sh kopieren, im Terminal ausführen und danach die angezeigten „Next steps“ ausführen.

✓ Kontrolle: `brew --version` gibt eine Version aus.

### Schritt 4 – Splash installieren

```sh
brew install incoai/tap/splash
```

Danach oMLX neu starten: Menüleiste → oMLX-Symbol → **Quit oMLX**, dann oMLX aus **Programme** wieder öffnen.

✓ Kontrolle: In Splash Bench ist „Splash installiert“ grün (mindestens Version 1.3.0). Im Dashboard unter **Settings → Integration Settings → External engines** steht Splash mit „Found automatically.“

### Schritt 5 – Modell und Draft herunterladen

Du brauchst **zwei** Downloads: das Modell und den passenden Draft. Welches Modell zu deinem RAM passt, zeigt Splash Bench unter **Empfehlungen**. Für die meisten Macs ab 32 GB ist es Qwen3.8-27B:

| Was | Repository ID | Größe |
|---|---|---|
| Modell | `Jundot/Qwen3.8-27B-oQ4e-mtp` | 16,6 GB |
| Draft | `incoai/Qwen3.8-27B-DFlash2` | 3,8 GB |

1. Im Dashboard **Models → Downloader** öffnen.
2. Die ID des Modells in **Repository ID** einfügen → **Download**. Das Feld **HF Token** bleibt leer.
3. Dasselbe für den Draft.
4. Warten, bis beide fertig sind (bei 100 Mbit/s rund eine halbe Stunde).

Nicht das „Splash“-Komplettpaket laden (`incoai/…-Splash`). oMLX sieht es nicht.

### Schritt 6 – oMLX die Modelle einlesen lassen

Im Dashboard **Models → Manager → Reload**. Das entlädt alle gerade geladenen Modelle, also nicht mitten in einem Chat.

✓ Kontrolle: Im Manager stehen das Modell **und** der Draft. Den Draft nie zum Chatten auswählen, aber auch nicht löschen – Splash braucht ihn im Hintergrund.

### Schritt 7 – Splash einschalten

**Models → Manager** → in der Zeile des Modells das Einstellungs-Symbol (Model settings) → **Advanced Settings** → **Use external engine** einschalten → Engine **Splash** → **Save**.

✓ Kontrolle: Splash Bench zeigt alle Punkte grün.

Für den Vergleich in Kapitel 3 musst du den Schalter nicht selbst umlegen. Splash Bench schaltet beim Messen selbst um und stellt danach alles zurück.

---

## 3. Messen und entscheiden

1. Alle anderen Programme schließen, Netzteil anschließen.
2. In Splash Bench Modell und Aufgaben wählen (`code`, `prose`, `translate`) → **Vergleich starten**.
3. Ergebnis ablesen: **Geschwindigkeit** (tok/s, mehr ist besser) und **Wartezeit** bis zum ersten Token.

**Entscheidungsregel:** Splash an, wenn es bei deiner Hauptarbeit (Code, Texte, Übersetzen) **mindestens 15 % mehr tok/s** bringt **und** die Wartezeit nicht schlechter wird. Sonst oMLX ohne Splash lassen.

Einzelläufe schwanken. Für eine Entscheidung 5 Durchläufe statt 3 einstellen.

---

## 4. Wenn es hakt

| Symptom | Lösung |
|---|---|
| Modell taucht in oMLX nicht auf | Du hast das Splash-Komplettpaket geladen. Im Finder ⇧⌘G → `~/.omlx/models` → den Ordner mit `-Splash` am Ende in den Papierkorb. Dann Modell **und** Draft getrennt laden (Schritt 5) und Reload (Schritt 6). |
| Splash fehlt unter External engines, oder der Schalter „Use external engine“ fehlt | Splash ist nicht installiert oder oMLX wurde danach nicht neu gestartet (Schritt 4). |
| Schalter ausgegraut, oder `Splash needs the … DFlash2 draft` | Draft fehlt oder oMLX hat ihn noch nicht eingelesen. Schritt 5 und 6 wiederholen. |
| Splash Bench: „antwortet kein oMLX“ | oMLX starten. Steht dort „oMLX startet noch“: kurz warten. |
| Splash Bench: „verlangt einen API-Key, es ist aber keiner hinterlegt“ | Im oMLX-Dashboard einen API-Key setzen (Schritt 1). |
| Splash Bench: „Kein Modell, das Splash bedienen kann“ | Modell und Draft fehlen oder sind noch nicht eingelesen. Schritt 5 und 6. |
| HTTP 409 „unload is pending“ | oMLX entlädt gerade ein Modell. Kurz warten, erneut versuchen. |
| Erste Antwort dauert lange | Normal: Das Modell wird geladen. Danach geht es schnell. |

Hilft nichts davon: In Splash Bench **Diagnose kopieren** und auf GitHub ein Issue anlegen. Der API-Key steht nicht in der Diagnose.

---

## 5. Ralfs Ergebnisse (M5 Max, 128 GB)

Median aus 3 Läufen, bis 512 Tokens, gemessen am 29.10.2026.

| Modell | Aufgabe | ohne Splash | mit Splash | Faktor |
|---|---|---|---|---|
| Qwen3.8-27B | code | 72 tok/s | 120 tok/s | **1,66×** |
| Qwen3.8-27B | prose | 50 tok/s | 67 tok/s | **1,33×** |
| Qwen3.8-27B | translate | 64 tok/s | 120 tok/s | **1,87×** |
| Qwen3.6-35B-A3B | code | 202 tok/s | 251 tok/s | 1,24× |
| Qwen3.6-35B-A3B | prose | 181 tok/s | 190 tok/s | 1,05× |
| Qwen3.6-35B-A3B | translate | 216 tok/s | 285 tok/s | 1,32× |

- **Qwen3.8-27B: Splash an.** Auch bei 131.072 Tokens Kontext +35 % und 8,6 GB weniger Speicher.
- **Qwen3.6-35B-A3B: bringt wenig.** oMLX ist hier ohne Splash schon schnell.
- Code profitiert am meisten, freier Text am wenigsten: Code kann der Draft besser vorhersagen.

Deine Zahlen werden anders sein. Deshalb selbst messen.

---

## 6. Für das Terminal

Das DMG enthält nur die App. Die Kommandozeilen-Programme `splashcheck`, `splashbench` und `splashdemo` liegen als eigenes Archiv im Release: `splash-bench-cli-<Version>-darwin-arm64.tar.gz`.

```sh
cd ~/Downloads
mkdir splash-bench-cli
tar -xzf splash-bench-cli-*-darwin-arm64.tar.gz -C splash-bench-cli
xattr -dr com.apple.quarantine splash-bench-cli   # sonst blockiert macOS die Programme
cd splash-bench-cli
```

Die Programme brauchen den API-Key als Umgebungsvariable:

```sh
export OMLX_API_KEY="dein-key"   # steht in ~/.omlx/settings.json unter auth.api_key
```

Alle Befehle aus diesem Ordner heraus:

```sh
./splashcheck                                         # Diagnose, wie „Voraussetzungen“ in der App
./splashbench -models Qwen3.8-27B-oQ4e-mtp -repeats 3 -out results.json
./splashdemo -model Qwen3.8-27B-oQ4e-mtp              # Live-Vergleich wie im Video
```

Modelle im Terminal statt im Downloader laden (Beispiel Qwen3.8-27B; den richtigen Ordner nennt `./splashcheck`):

```sh
brew install hf   # einmalig
hf download Jundot/Qwen3.8-27B-oQ4e-mtp --local-dir ~/.omlx/models/Jundot/Qwen3.8-27B-oQ4e-mtp
hf download incoai/Qwen3.8-27B-DFlash2  --local-dir ~/.omlx/models/incoai/Qwen3.8-27B-DFlash2
```

oMLX neu einlesen ohne Neustart (entlädt **alle** Modelle):

```sh
curl -X POST http://localhost:8000/admin/api/reload -H "Authorization: Bearer $OMLX_API_KEY"
```

Alle Flags stehen im `README.md`. Wer selbst baut (`go build -o bin/ ./cmd/...`), ruft die Programme als `bin/splashcheck` usw. auf.

---

## 7. Quellen

- oMLX: github.com/jundot/omlx (Splash-Integration in `engine/splash.py`)
- Splash: github.com/incoai/splash, Homebrew-Formel `incoai/tap/splash` 1.5.0
- splash-m1 für M1/M2: github.com/paperniuk/splash/releases
- Eigene Messungen vom 29.10.2026 mit Splash Bench und dem eingebauten oMLX-Benchmark (Läufe `bench-2b8b1a123d70`, `bench-25fa79b096ca`)
