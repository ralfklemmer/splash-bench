package bench

// DefaultPrompts cover typical workloads. Speculative decoding gains depend
// on how predictable the output is, so code, free prose and a translation of
// given text are measured separately.
var DefaultPrompts = []Prompt{
	{
		Name: "code",
		Text: "Schreibe in Go ein vollständiges Paket `lru` mit einem threadsicheren LRU-Cache " +
			"mit Ablaufzeit pro Eintrag (TTL). Die Uhr soll per Interface injizierbar sein. " +
			"Füge tabellengetriebene Tests mit einer Fake-Uhr hinzu. Gib nur Code aus.",
	},
	{
		Name: "prose",
		Text: "Erkläre ausführlich und gut gegliedert, wie `git rebase` intern funktioniert, " +
			"worin es sich von `git merge` unterscheidet, wann ein Team welches Verfahren " +
			"nutzen sollte und welche Fallstricke es bei geteilten Branches gibt.",
	},
	{
		Name: "translate",
		Text: "Übersetze den folgenden Text vollständig ins Englische. Gib nur die Übersetzung aus.\n\n" +
			"Viele Teams messen ihre Produktivität an der Zahl abgeschlossener Tickets. " +
			"Das wirkt zunächst objektiv, führt aber oft dazu, dass große Aufgaben künstlich " +
			"zerteilt werden und niemand mehr auf das Ergebnis für die Nutzer schaut. " +
			"Besser ist es, den Durchfluss von der Idee bis zur Auslieferung zu betrachten: " +
			"Wie lange wartet eine Aufgabe, bevor jemand sie anfasst? Wie oft wird sie " +
			"zurückgereicht, weil Anforderungen unklar waren? Wie viel Arbeit ist gleichzeitig " +
			"in Bearbeitung? Wer diese Fragen ehrlich beantwortet, erkennt Engpässe, die keine " +
			"Ticketstatistik zeigt. Oft liegt das Problem nicht in der Geschwindigkeit einzelner " +
			"Entwickler, sondern in Übergaben zwischen Rollen, in langen Review-Schlangen und " +
			"in Abhängigkeiten zu anderen Teams. Ein Team, das weniger parallel beginnt und " +
			"mehr gemeinsam abschließt, liefert in der Regel schneller und mit weniger Fehlern. " +
			"Dazu gehört auch, Arbeit sichtbar zu machen, Wartezeiten offen anzusprechen und " +
			"Verbesserungen in kleinen Schritten auszuprobieren, statt auf die große " +
			"Reorganisation zu warten.",
	},
}
