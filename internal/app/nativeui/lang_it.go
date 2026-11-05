package nativeui

// Italian (native addition: the web client has English and Chinese only).
// The web catalog in Italian is copy_it (copy_it.go); these are the texts
// that live outside it.

var welcome_it = []string{
	"Il tuo posto è proprio qui.",
	"Storia in singolo. Pubblico in multigiocatore.",
	"Tre, due, uno… si gira.",
	"Missione secondaria: condividi un po' di gioia.",
	"Snack facoltativi. Buona compagnia consigliata.",
	"E ora, la tua scena.",
	"Salva la partita. Resta un po'.",
	"Sbloccato: qualcuno che capisce.",
	"Sta per arrivare il momento clou. Porta gli snack.",
	"Resta fino ai titoli di coda. Potrebbe esserci una sorpresa.",
	"Certi momenti meritano un secondo pubblico.",
	"Tu guidi. Io porto la sedia pieghevole.",
	"Niente missione principale oggi. Andiamo a zonzo.",
	"Ogni grande momento ha bisogno di un testimone.",
	"Ho fatto una cosina. Vieni a vedere.",
	"Uno schermo. Un pubblico entusiasta.",
	"Cosa c'è nella scaletta, regista?",
	"Prossimo livello? Porta i tuoi amici.",
	"La gioia viaggia bene.",
	"Venti posti. Tante storie.",
}

var nativeCopy_it = map[string]string{
	"native.report":               "Salva report",
	"native.reportHint":           "Salva in Download la diagnostica di questa stanza: versione, capacità dell'App, stato della condivisione, dati di connessione. Niente password né link di invito.",
	"native.reportSaved":          "Report salvato: {path}",
	"native.reportFailed":         "Impossibile salvare il report; riprova",
	"native.reportShow":           "Mostra nella cartella",
	"native.chatSaved":            "Chat salvata: {path}",
	"native.chatFailed":           "Impossibile salvare la chat; riprova",
	"native.overlay.desktop":      "Chat sopra le altre app",
	"native.overlay.desktopHint":  "Nuovi messaggi e reazioni compaiono in un angolo dello schermo, sopra i giochi e le altre app. È una finestra trasparente separata che ignora il mouse: non tocca il gioco, quindi non riguarda l'anti-cheat, ed è nascosta dalla condivisione dello schermo. I giochi devono essere in schermo intero senza bordi o in finestra (lo schermo intero esclusivo la copre).",
	"native.overlay.screen":       "Schermo",
	"native.overlay.screenN":      "Schermo {n}",
	"native.overlay.primary":      "principale",
	"native.overlay.corner":       "Posizione",
	"native.overlay.top-left":     "In alto a sinistra",
	"native.overlay.top-right":    "In alto a destra",
	"native.overlay.bottom-left":  "In basso a sinistra",
	"native.overlay.bottom-right": "In basso a destra",
	"native.overlay.seconds":      "Tempo sullo schermo",
	"native.overlay.test":         "Mostra un messaggio di prova",
	"native.overlay.testMessage":  "Messaggio di prova: la chat compare qui",
	"native.join.browser":         "Si apre nel browser",
	"native.privacyHint":          "Immagine e suono vanno direttamente da questo PC agli spettatori, o passando per altri spettatori: questa App non li manda mai tramite un server multimediale.",
	"native.pipScreen":            "Il riquadro mobile funziona con una finestra condivisa, non con uno schermo intero",
}

var piikTvCopy_it = map[string]string{
	"send.label": "Invia URL all'app PIIK TV", "send.sentLabel": "Inviato alla TV",
	"send.hint": "Invia il link della stanza all'app PIIK TV", "send.noRoom": "Crea prima una stanza",
	"send.sentTo": "Inviato a {names}", "send.noTv": "Nessuna TV con l'app PIIK TV aperta su questa rete",
	"send.failed": "Invio non riuscito ({detail})", "send.appDown": "Invio non riuscito: l'app Piik su questo PC non risponde",
	"card.title": "Guarda sulla TV con l'app PIIK TV", "card.placeholder": "Link della stanza, anche di qualcun altro",
	"card.inputLabel": "Link della stanza da guardare sulla TV", "card.button": "Invia alla TV", "card.searching": "Cerco la TV…",
	"card.invalidExample": "Link non valido: deve essere il link di una stanza Piik, per esempio https://…/r/1234#v=…",
	"card.invalid":        "Link non valido: deve essere il link di una stanza Piik",
	"card.hint":           "Incolla il link di invito di qualsiasi stanza Piik: la TV con l'app PIIK TV aperta lo apre subito, anche senza iniziare a condividere da questo PC.",
}

// extraCatalogs holds web catalogs beyond English and Chinese ("it" is
// registered by copy_it_register.go).
var extraCatalogs = map[string]map[string]string{}
