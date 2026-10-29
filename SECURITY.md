# Sicherheit

## Lücke melden

Bitte **nicht** als öffentliches Issue, sondern vertraulich über [Security Advisories](https://github.com/ralfklemmer/splash-bench/security/advisories/new). Ich antworte in der Regel innerhalb einer Woche.

## Was Splash Bench schützt

Die App kann über die Admin-API von oMLX die Engine eines Modells umschalten und kennt dafür den oMLX-API-Key. Deshalb:

- Der Server der App lauscht nur auf `127.0.0.1` und beantwortet nur Anfragen, die an localhost adressiert sind (Schutz gegen DNS-Rebinding).
- Ändernde Anfragen nimmt er nur von der eigenen Seite an (Prüfung des `Origin`-Headers). Eine andere Webseite im Browser kann also keine Engine umschalten.
- Der API-Key bleibt im Programm. Er erreicht den Browser nie, steht in keiner Log-Zeile, keiner gespeicherten Messung und nicht in der kopierbaren Diagnose.
- Die Seite lädt nichts aus dem Internet; eine Content-Security-Policy erzwingt das.
- Die Programme sprechen nur mit dem eingestellten oMLX-Server, standardmäßig `http://localhost:8000`.

Eine Lücke ist alles, womit eine fremde Webseite, ein anderer Nutzer oder ein präpariertes Modell eine dieser Zusagen bricht.

## Downloads

Die DMGs sind ad hoc signiert, aber nicht von Apple notarisiert. Jedes Release enthält `SHA256SUMS.txt`; prüfen mit `shasum -a 256 -c SHA256SUMS.txt`. Wer ganz sicher gehen will, baut aus dem Quellcode (siehe README).
