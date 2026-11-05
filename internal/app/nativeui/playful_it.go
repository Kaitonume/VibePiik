package nativeui

import (
	"hash/fnv"
	"time"
)

// Italian title frames and waiting lines (free adaptations, like the web's
// "local cultural references are independent entries").
var titleFrames_it = map[string][]string{
	"hostActive": {
		"Schermo acceso",
		"Pixel in viaggio",
		"Immagini in movimento",
		"La piccola TV lavora",
		"Si va in onda",
		"I pixel devono scorrere",
		"Pixel a tutto gas",
		"Lascia cuocere",
		"La partita è aperta",
		"Si alza il sipario",
		"Schermo piccolo, momento grande",
		"Luci, motore, schermo",
		"Missione secondaria: condividere",
		"Ora di mostrare",
		"Mantieni la calma e condividi",
		"Pixel, uniamoci",
		"Proiezione in corso",
		"Condivido la trama",
		"Ecco il mio schermo",
	},
	"hostStarting": {
		"Si prepara il palco",
		"Raduno i pixel",
		"Preparo la scena d'apertura",
		"Quasi pronti per mostrare",
		"Un attimo prima del primo atto",
		"Missione secondaria: allestire il palco",
		"La piccola TV si scalda",
		"Prima mondiale in pixel",
		"Luci, motore... quasi",
		"Sistemo i titoli di testa",
		"Preparo l'attrazione principale",
		"Pixel in servizio",
		"Preparo lo schermo",
		"Allestisco questo episodio",
		"Preparo la grande rivelazione",
		"Srotolo il tappetino rosso",
		"Pronti a partire",
	},
	"hostReady": {
		"Il divano è pronto",
		"Pronti quando vuoi",
		"La piccola TV attende",
		"Telecomando alla mano",
		"Luci sistemate",
		"Il tuo canale ti aspetta",
		"Il tè è pronto",
		"Un posto attende la tua avventura",
		"Porta uno schermo, porta una storia",
		"Tutto pronto per mostrare",
		"Il tuo schermo, se accetti",
		"Il palco è tuo",
		"Missione principale: scegli uno schermo",
		"Un po' di spazio per l'avventura",
		"Pronti per la versione del regista",
		"La tua prima personale",
		"Posto per uno schermo libero",
		"Il divano è entrato nella sala d'attesa",
		"Tutta la stanza è un palcoscenico",
	},
	"hostIdle": {
		"La piccola TV riposa",
		"In attesa di uno schermo",
		"L'antenna fa un pisolino",
		"Lo schermo sogna",
		"Il telecomando è tra le nuvole",
		"Aspettando lo spettacolo",
		"Stanzetta tranquilla",
		"La piccola TV è AFK",
		"Missione secondaria: non fare niente",
		"La trama è ancora fuori campo",
		"Cinque minuti di pausa",
		"Pace, quiete, i pixel dopo",
		"Lo schermo può aspettare",
		"Un intervallo prima del primo atto",
		"Tra un'avventura e l'altra",
		"Risparmio energia per la prima",
		"Sogni in formato panoramico",
		"Vita da divano",
		"Un lungo riposo",
	},
}

var waitingLines_it = []string{
	"Avvicina una sedia comoda.",
	"La missione snack è facoltativa.",
	"Popcorn dolci o salati?",
	"Buon momento per sgranchirsi.",
	"Controllo idratazione.",
	"Il divano non scappa.",
	"Missione secondaria: mettiti comodo.",
	"Energia da prima fila, comodità da divano.",
	"Pixel piccoli, grandi personalità.",
	"Bottino leggendario: una sedia comoda.",
	"C'è una storia anche fuori dallo schermo.",
	"Il gatto prende il divano, la tastiera riposa.",
	"Sbatti le palpebre se vedi la piccola TV.",
	"Mantieni la calma e passa i popcorn.",
	"Ricordati di saccheggiare la ciotola degli snack.",
	"Inserire qui narrazione drammatica.",
	"Abilità passiva: sognare a occhi aperti.",
	"Una piccola pausa nell'azione.",
	"Pausa ballo? Anche seduti vale.",
	"Checkpoint raggiunto: il divano.",
}

func (u *UI) titleFrames(key string) []string {
	switch u.textLang() {
	case "zh":
		return titleFrames_zh[key]
	case "it":
		return titleFrames_it[key]
	}
	return titleFrames_en[key]
}

// titleDecoration is useDocumentTitle's decoration: one variation, drawn once
// per context (language, state, room) and kept while it lasts. ✦ mode has none.
func (u *UI) titleDecoration(key, context string) string {
	if u.Visual() {
		return ""
	}
	frames := u.titleFrames(key)
	if len(frames) == 0 {
		return ""
	}
	h := fnv.New32a()
	h.Write([]byte(u.Lang + ":" + key + ":" + context + ":" + titleSeed))
	return frames[int(h.Sum32()%uint32(len(frames)))]
}

// titleSeed changes the draw at each App start.
var titleSeed = time.Now().Format(time.RFC3339Nano)

// waitingCaption is WaitingCaption: a playful line beside a waiting status,
// changed every 15 s (TEXT_ROTATION_INTERVAL_MS); none in ✦ mode or with
// animations off.
func (u *UI) waitingCaption(now time.Time, salt int) string {
	if u.Visual() {
		return ""
	}
	lines := waitingLines_en
	switch u.textLang() {
	case "zh":
		lines = waitingLines_zh
	case "it":
		lines = waitingLines_it
	}
	if len(lines) == 0 {
		return ""
	}
	step := 0
	if !reducedMotion {
		step = int(now.Unix() / 15)
	}
	return lines[(step*7+salt)%len(lines)]
}
