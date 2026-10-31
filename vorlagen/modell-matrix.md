# Modell-Matrix: Was passt zu welchem RAM?

Gilt für oMLX mit Splash als externer Engine, auf einem Mac mit **M3 oder neuer** und **macOS 26.4 oder neuer**. Die Matrix deckt die beiden gemessenen Modelle **Qwen3.8-27B** und **Qwen3.6-35B-A3B** ab. Seit Splash 1.4 und oMLX 0.7.3rc1 kommen Qwen3.8-Flash-Next (≈ 66 GB bei 32K), Ornith-1.5-122B (≈ 69 GB) und Flint (≈ 27 GB) dazu. Sie sind nicht gemessen, Download-Befehle zeigt Splash Bench unter „Empfehlungen“.

> **Gemessen ist nur die 128-GB-Stufe** (M5 Max, macOS 27.0.1, gemessen am 29.10.2026 mit oMLX 0.7.3rc1 und Splash 1.5.0). Alle anderen Stufen sind aus der Speicherformel abgeleitet – **Schätzungen**. Prüfe auf deinem Mac mit Splash Bench oder `splashcheck` und miss selbst.

## Grundlage: Speicherbedarf

Formel: **Gewichte + Draft/3 + 3 GB + KV-Cache (int8)**

| Modell | Gewichte | Draft | 32K | 128K | 256K |
|---|---|---|---|---|---|
| Qwen3.8-27B (`Jundot/Qwen3.8-27B-oQ4e-mtp` + `incoai/Qwen3.8-27B-DFlash2`) | 16,6 GB | 3,8 GB | ≈ 22 GB | ≈ 25 GB | ≈ 29 GB |
| Qwen3.6-35B-A3B (`Jundot/Qwen3.6-35B-A3B-oQ4e-mtp` + `incoai/Qwen3.6-35B-A3B-DFlash2`) | 21,1 GB | 1,0 GB | ≈ 25 GB | ≈ 26 GB | ≈ 27 GB |

Gegenprobe (gemessen): 27B, 131.072 Prompt-Tokens → Splash-Prozess 25,3 GB Spitzenspeicher.

Der Bedarf in der Tabelle ist nur das Modell. macOS, oMLX selbst und deine anderen Apps brauchen zusätzlich Speicher. Wie viel, hängt von deinem Alltag ab – das ist nicht gemessen. Die Matrix rechnet wie Splash Bench und `splashcheck` mit der Annahme, dass etwa **75 % des RAM** für das Modell nutzbar sind (Standard-Anteil, den macOS der GPU überlässt; nicht gemessen, offener Punkt).

## Matrix nach RAM-Stufe

Legende: **ja** = passt mit Reserve · **knapp** = passt rechnerisch, wenig Luft für andere Apps · **nein** = passt nicht sinnvoll

| RAM | 27B · 32K | 27B · 128K | 27B · 256K | 35B-A3B · 32K | 35B-A3B · 128K | 35B-A3B · 256K | Status |
|---|---|---|---|---|---|---|---|
| 16 GB | nein | nein | nein | nein | nein | nein | Schätzung |
| 24 GB | nein | nein | nein | nein | nein | nein | Schätzung |
| 32 GB | knapp | nein | nein | nein | nein | nein | Schätzung |
| 36 GB | ja | knapp | nein | knapp | knapp | nein | Schätzung |
| 48 GB | ja | ja | ja | ja | ja | ja | Schätzung |
| 64 GB | ja | ja | ja | ja | ja | ja | Schätzung |
| 96 GB | ja | ja | ja | ja | ja | ja | Schätzung |
| 128 GB | ja | ja | ja | ja | ja | ja | **gemessen** |

Hinweise:

- 16 und 24 GB: Schon die Gewichte (16,6 bzw. 21,1 GB) lassen keinen Platz für Draft, KV-Cache und System.
- Ab 96 GB passen beide Modelle gleichzeitig (rechnerisch ≈ 29 + 27 GB bei 256K). Nicht gemessen.
- Bei 35B-A3B wächst der KV-Cache mit dem Kontext deutlich langsamer als bei 27B.

## Welches Modell nehmen?

| Wenn du … | dann | weil (gemessen, M5 Max) |
|---|---|---|
| viel Code schreibst | Qwen3.8-27B **mit Splash** | 72,2 → 120,1 tok/s (1,66×), TTFT 148 → 66 ms |
| viel Prosa erzeugst | Qwen3.8-27B **mit Splash** | 50,4 → 67,0 tok/s (1,33×) |
| Texte übersetzt | Qwen3.8-27B **mit Splash** | 64,0 → 119,7 tok/s (1,87×), TTFT 163 → 86 ms |
| mit langem Kontext arbeitest (Agenten, große Codebasen) | Qwen3.8-27B **mit Splash** | 131K: Einlesen +40 %, Generierung +35 %, 25,3 statt 33,9 GB Spitzenspeicher; trotzdem 207 s (knapp 3,5 min) bis zum ersten Token |
| maximale Geschwindigkeit willst und der RAM reicht | Qwen3.6-35B-A3B (oMLX/MTP), Splash kann an, bringt aber nur wenig | 181–216 tok/s mit oMLX (MTP); Splash bringt 1,05–1,32×, vor allem bei Code und Übersetzung |

Entscheidungsregel für deine eigene Messung: **≥ 15 % mehr tok/s auf deinem Hauptworkload und TTFT nicht schlechter → Splash an.**

## Hilf mit, die Schätzungen zu ersetzen

Miss mit Splash Bench (oder `splashbench`), trag die Werte in `messvorlage.csv` ein und schick sie als GitHub-Issue („Idee oder Wunsch“, Feld „Eigene Messwerte“). Mit genug Messungen von 32-, 36-, 48- und 64-GB-Macs wird aus „Schätzung“ hier „gemessen“.
