<h1 align="center">VibePiik</h1>
<p align="center"><strong>Piik v2.5.3 per Windows, con il supporto a PIIK TV e una condivisione a 60 fps più fluida.</strong></p>
<p align="center"><a href="./README.md">English</a> · Italiano · <a href="./README.zh-CN.md">简体中文</a></p>

VibePiik è una versione modificata di [Piik](https://github.com/TNTcraftHIM/Piik)
v2.5.3 (revisione `f2f9d27`) per Windows x64. Aggiunge le funzioni che la
collegano a **PIIK TV**, un'app per Android TV che riproduce le stanze Piik sul
televisore, e alcune correzioni che rendono più fluida la condivisione a 60 fps.

> VibePiik non è una versione ufficiale di Piik e non è supportata dagli
> sviluppatori di Piik. Segnala i problemi di VibePiik qui, non al progetto Piik.

## Differenze rispetto a Piik v2.5.3

### Guardare sulla TV
- Tasto **«Manda URL all'app PIIK TV»** nell'intestazione della stanza: manda il
  link della stanza, con l'invito, a tutte le app PIIK TV aperte nella rete di
  casa. L'App trova da sola le TV (porta TCP 8790): non serve scrivere indirizzi.
- Riquadro **«Guarda sulla TV con l'app PIIK TV»** nella pagina iniziale
  dell'App: incolla il link di qualsiasi stanza Piik, anche di qualcun altro, e
  mandalo alla TV, anche prima di avviare la condivisione.
- Entrambi rispondono solo alle pagine di Piik aperte su questo PC (loopback,
  stessa origine): mai ai visitatori che arrivano dal link pubblico, mai ad altri
  dispositivi della rete.
- L'App raggiunge solo le TV della stessa rete locale: ogni rete IPv4 privata del
  PC, per intero fino a una /22 (1022 indirizzi, come usano alcuni router mesh),
  altrimenti la /24 attorno al PC. In una rete di casa la ricerca dura meno di un
  secondo.

### Una finestra nativa, senza browser (`piik-app.exe`)
- La pagina iniziale e la stanza dell'App sono disegnate **in modo nativo** (Go +
  [Gio](https://gioui.org)) in una finestra VibePiik, con lo stesso aspetto delle
  pagine web di Piik: niente browser e niente WebView. Durante la condivisione usa
  circa 150–250 MB e molto meno dell'1% della CPU, il che conta mentre giochi.
- C'è tutto quello che fa la pagina dell'host di Piik: schermi e finestre da
  condividere, qualità (preimpostazioni o valori scritti), suono della sorgente ed
  esclusione di un'app, microfono del commento, codice e password della stanza,
  inviti, controllo remoto, voce della stanza, chat e reazioni, dettagli e
  topologia della connessione, aiuto e report di diagnostica.
- Lingue: **中 / EN / IT / ✦** in alto a destra (cinese, inglese, italiano e la
  modalità visiva di Piik). Al primo avvio segue Windows; per tutte le altre
  lingue usa l'inglese. In modalità ✦ i suggerimenti restano nella lingua scelta
  prima.
- Chiudendo la finestra si chiude l'App, come `q` nella console. Dimensione e
  posizione vengono ricordate (`%LOCALAPPDATA%\VibePiik`).
- Nella console, **[o] Open in browser** sposta l'App nel browser di sistema. Se
  c'è una condivisione in corso la finestra viene in primo piano e chiede prima
  conferma: la condivisione si ferma e si riavvia dal browser. **[w] Open in
  window** riporta la finestra. `piik-app.exe -browser` parte dal browser come
  Piik.
- **Join a room** apre la pagina dello spettatore nel browser.

### Gli extra della finestra nativa
- **Chat sullo schermo**: i messaggi scorrono sull'immagine della TV, e la **chat
  sopra gli altri programmi** mostra messaggi e reazioni in un angolo di uno
  schermo a scelta, sopra un gioco. È una finestra separata, trasparente ai clic,
  che non prende mai il fuoco e non tocca il processo del gioco (niente
  iniezione, niente hook grafici): è sicura con gli anti-cheat e non compare
  nelle catture dello schermo. Il gioco deve essere in finestra o a schermo
  intero senza bordi.
- **Pannelli staccabili**: Interactions e Remote control si aprono in finestre
  proprie sempre in primo piano. I pannelli si spostano e ridimensionano con il
  mouse o, selezionati, con le frecce.
- **Anteprima locale**: pausa, adatta / riempi / allunga, modalità cinema, schermo
  intero e picture-in-picture (finestre condivise). Condividendo uno schermo, il
  tasto ⚡ sotto la TV passa a un'**anteprima nitida** a 30 o 60 fps invece di
  quella leggera aggiornata ogni 3 s (usa più CPU).
- **Voce della stanza**: microfono e volume, volume generale e di ogni persona,
  riduzione di rumore ed eco di Windows e una **soglia del microfono (noise
  gate)** regolabile sull'indicatore del livello. Nella voce della stanza una
  condivisione dello schermo esclude il suono di VibePiik stesso, così nessuno
  sente tornare la propria voce (Windows può escludere una sola applicazione: la
  tua scelta torna quando esci dalla voce).
- I fumetti animati di Piik, le frasi nel titolo e quelle d'attesa, il divano
  che aggiunge file per le stanze grandi e la navigazione da tastiera.

### Aggiornamenti
- L'App non propone mai le versioni ufficiali di Piik: installarne una
  toglierebbe VibePiik. L'avviso di aggiornamento resta spento finché non potrà
  puntare alle versioni di VibePiik. (Le pagine aperte nel browser mantengono
  l'avviso di Piik.)

### Niente blocchi di 3 secondi per chi guarda in Wi-Fi (`piik-app.exe`)
- Gli encoder hardware AMD mettono un'unità SVC *prefix NAL* (tipo 14) prima di
  ogni fetta. I decoder AVC la ignorano, ma veniva spedita come pacchetto RTP a sé
  di 5 byte all'inizio di ogni fotogramma. Quando il Wi-Fi consegnava quel
  pacchetto dopo la fetta che chiude il fotogramma, il buffer H.264 di libwebrtc
  completava il fotogramma senza di lui e teneva il pacchetto in ritardo come un
  fotogramma che non finisce mai: tutti i fotogrammi successivi aspettavano un
  keyframe, circa 3 s dopo. Ora le unità SVC (tipi 14, 15, 20) vengono tolte prima
  di creare i pacchetti. Misurato su una TV in Wi-Fi con 15 s fermi e 15 s in
  movimento: 6 blocchi in 120 s prima, nessuno in 240 s dopo.

### Cattura più fluida (`runtime/native/piik-capture.exe`)
- **Nessun fotogramma catturato viene scartato.** La griglia di cadenza che
  buttava i fotogrammi arrivati "in anticipo" (fotogrammi veri del video,
  irregolari su un desktop ad alta frequenza) è sostituita da un limitatore a
  gettoni: al massimo 60 fotogrammi al secondo in media, con una scorta di 2. Un
  fotogramma arrivato troppo presto aspetta il gettone successivo invece di
  essere buttato.
- **GOP di 30 s.** Niente keyframe periodico ogni 2 s (ognuno pesa circa otto
  fotogrammi normali e arrivava in ritardo): i keyframe arrivano quando un viewer
  li chiede.
- **Encoder hardware AMD.** Windows può elencare per primo l'encoder H.264 della
  grafica integrata, che si bloccava catturando da una scheda dedicata: ora viene
  provato per primo l'encoder il cui ID PCI corrisponde all'adattatore scelto. Il
  profilo Constrained Baseline viene richiesto esplicitamente (con ripiego su
  Baseline), perché l'encoder AMD non lo dichiarava.
- **Schermo fermo sempre nitido.** Windows non manda fotogrammi finché
  l'immagine condivisa non cambia: il server dava per spento ogni livello di
  qualità dopo 2 s e gli spettatori chiedevano un keyframe dopo 3 s, così uno
  schermo fermo continuava a saltare tra 1080p e 540p, con ogni fotogramma un
  keyframe. Ora l'ultima immagine viene ripetuta una volta al secondo (come fa
  WebRTC nella condivisione schermo), come fotogramma differenziale di poche
  decine di byte.
- Diagnostica facoltativa: variabile d'ambiente `PIIK_CAPTURE_DEBUG=1`
  (contatori di cadenza) o `PIIK_CAPTURE_DEBUG=2` (una riga di log per fotogramma).

### Impostazioni di partenza
- Le nuove condivisioni partono con **High FPS**: 1080p, 60 fps, **12 Mbps**
  (Piik: 8 Mbps), preferenza dell'immagine bilanciata, codec video **H.264**
  (Piik: automatico). Si può cambiare tutto durante la condivisione.

### Lingue
- La finestra nativa è in cinese, inglese e italiano. I testi di PIIK TV la seguono
  in inglese e cinese, e sono in italiano con Windows in italiano. Le pagine
  aperte nel browser sono quelle di Piik (inglese e cinese, secondo il browser).

## Scaricare e avviare

1. Scarica `VibePiik-windows-amd64.zip` dalla pagina Releases.
2. Estrailo dove vuoi e avvia `piik-app.exe`, esattamente come il pacchetto
   ufficiale di Piik. Non si installa niente. Tieni VibePiik in una cartella sua:
   non mescolare i suoi file con una cartella di Piik ufficiale.

I file non sono firmati digitalmente (non lo sono nemmeno quelli ufficiali di
Piik), quindi al primo avvio Windows SmartScreen può chiedere conferma.

Per tutto il resto funziona come Piik: vedi il [README originale](./README.md#about-piik)
e la [documentazione di Piik](https://piik.tv/docs/?lang=en).

## Compatibilità

- Stesso protocollo delle stanze di Piik v2.5.3: chi guarda vede le stanze di
  VibePiik esattamente come quelle di Piik.
- L'App si presenta come Piik v2.5.3 a stanze e siti. Installare una versione
  ufficiale di Piik sostituisce le modifiche di VibePiik.
- PIIK TV e il suo ricevitore di link sono facoltativi: senza TV, VibePiik si
  comporta come Piik con le correzioni e le impostazioni descritte sopra.

## Compilare dai sorgenti

Vedi la sezione *Building from source* del [README](./README.md#building-from-source).

## Licenza

MIT, come Piik: vedi [LICENSE](./LICENSE) (Copyright (c) 2026 TNTcraft and Piik
contributors). Le modifiche di VibePiik sono rilasciate con la stessa licenza.
