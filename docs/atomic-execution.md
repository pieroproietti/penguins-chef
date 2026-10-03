# Nuovo motore: operazioni verificabili

Questo branch esplora un nuovo modello, separato dalle ricette Wardrobe v2.
Il primo percorso eseguibile è LightDM su Debian, Arch e Fedora con systemd; il
traguardo successivo è Colibri completo, inclusi gli accessori.

## Contratto

1. Leggere una ricetta con schema rigoroso e selezionare un profilo completo.
2. Validare il profilo prima di modificare il sistema.
3. Aggiungere i file repository dichiarati, poi preparare il gestore pacchetti.
4. Controllare la disponibilità di tutti i pacchetti prima dell'installazione.
5. Controllare i pacchetti installati, installare quelli mancanti in una
   transazione, verificare l'esito di ogni pacchetto.
6. Confrontare, scrivere e verificare i file di configurazione.
7. Controllare, abilitare e verificare i servizi attraverso il backend init.

Ogni errore interrompe il piano. Le operazioni completate rimangono applicate:
"atomico" indica qui un'unità con risultato verificabile, non una transazione
con rollback dell'intero sistema. I file vengono sostituiti mediante rename
di un file temporaneo nella stessa directory. Una nuova esecuzione rilegge lo
stato reale di pacchetti, configurazioni e servizi; non si fida di un checkpoint.
La preparazione dei repository viene sempre ripetuta.

## Primo esperimento

```bash
# Questi comandi non eseguono nemmeno query al gestore pacchetti.
go run . apply examples/provision/lightdm.yaml --dry-run --family debian --init systemd
go run . apply examples/provision/lightdm.yaml --dry-run --family archlinux --init systemd
go run . apply examples/provision/lightdm.yaml --dry-run --family fedora --init systemd

# Prima fetta di Colibri: XFCE, LightDM, browser e integrazione SPICE.
go run . apply examples/provision/colibri.yaml --dry-run --family archlinux --init systemd

# Su una macchina di prova con la famiglia e systemd rilevati dall'host:
sudo tailor apply examples/provision/lightdm.yaml
```

`apply` non scarica Wardrobe e non seleziona fallback per altre distribuzioni.
Il piano mostra tutte le operazioni prima di eseguirle. Gli override della
famiglia e dell'init sono permessi soltanto nella simulazione. Il comando
`wear` continua a usare il percorso v2 esistente.

`colibri.yaml` è una prima fetta del desktop, non il porting completo del
costume Wardrobe: non include ancora rete, audio, branding, configurazioni
utente e gli accessori `base`/`eggs-dev`. I nomi dei pacchetti sono espliciti
per ciascuna famiglia; la loro disponibilità viene controllata all'esecuzione.

## Scelte iniziali

- Pacchetti e init sono dimensioni separate. I backend iniziali sono APT,
  pacman, DNF e systemd. Devuan con SysV/OpenRC viene rifiutato se la ricetta
  richiede servizi: il supporto APT non implica supporto dell'init.
- Su Debian la preparazione esegue `apt-get update --error-on=any`.
- Su Arch esegue **`pacman -Syu --noconfirm`: aggiorna anche il sistema**.
  Non viene mai eseguito `pacman -Sy` da solo, perché gli aggiornamenti
  parziali non sono supportati. Dopo un errore la vestizione si interrompe.
  Riferimento: https://wiki.archlinux.org/title/System_maintenance#Partial_upgrades_are_unsupported
- Su Fedora esegue `dnf --refresh makecache`, controlla la disponibilità con
  `dnf repoquery --available`, lo stato installato con `rpm -q` e installa
  con `dnf install -y`. Usa DNF disponibile nel PATH (anche DNF5); le vecchie
  installazioni DNF devono fornire `repoquery`. Usa i repository già configurati,
  senza aggiungerne altri e senza un aggiornamento generale del sistema.
- I repository già configurati vengono controllati attraverso la disponibilità
  dei pacchetti. L'aggiunta dichiarativa iniziale accetta solo file APT `.list`
  o `.sources` in `/etc/apt/sources.list.d/`. Le chiavi devono essere già
  predisposte; pacman usa i repository dell'host. L'aggiunta di repository
  pacman e la gestione delle chiavi sono ancora da progettare.
- Solo nomi concreti di pacchetti, niente gruppi pacman, AUR o traduzione
  automatica dei nomi. Niente comandi shell nelle ricette.
- I file gestiti sono UTF-8 YAML, scritti con permessi 0644. Destinazioni
  simboliche vengono rifiutate. Permessi personalizzati, ownership, backup
  e asset binari richiedono operazioni dedicate successive.
- I servizi sono **abilitati, non avviati**. Un display manager preesistente
  può causare un conflitto che interrompe l'esecuzione: non viene sostituito
  automaticamente. L'installazione dei pacchetti può comunque avviare servizi
  tramite gli script del gestore pacchetti.

## Verifica su macchine reali

## Prova Fedora

Su una VM Fedora tradizionale con systemd (non Silverblue/Kinoite), dalla
directory del progetto:

```bash
make test
go run . apply examples/provision/colibri.yaml --dry-run --family fedora --init systemd
make build
sudo /tmp/tailor-build-dir/tailor apply examples/provision/colibri.yaml
# Ripetere per verificare che pacchetti, file e servizi siano già applicati.
sudo /tmp/tailor-build-dir/tailor apply examples/provision/colibri.yaml
rpm -q lightdm lightdm-gtk xfce4-session
systemctl is-enabled lightdm.service
```

La ricetta abilita LightDM ma non cambia il target di avvio. Per la prova del
login grafico, su una VM senza altri display manager abilitati:

```bash
sudo systemctl set-default graphical.target
sudo reboot
```

Se GDM è già abilitato, il conflitto interrompe il piano: non viene disabilitato
automaticamente. La prova su Fedora reale resta da effettuare; i test simulati
non certificano il login grafico, la sessione XFCE o il comportamento SELinux.

Il test con runner simulato verifica ordine, fallimenti, postcondizioni e
riesecuzione. Non certifica il desktop su Debian, Arch o Fedora. Su VM pulite serve
verificare il piano, applicare la ricetta, rieseguirla, riavviare e controllare
il login grafico. Provare anche un repository senza il pacchetto richiesto e
un display manager già abilitato.

## Passi verso Colibri

1. Verificare LightDM su Debian, Arch e Fedora reali, fissando la gestione dei conflitti.
2. Aggiungere XFCE, rete e audio come funzioni separate del costume.
3. Introdurre composizione delle funzioni senza copie implicite dei profili.
4. Portare asset, configurazione utente, `base` ed `eggs-dev`.
5. Estendere repository e init sulla base dei casi incontrati.

La versione 1 nello schema è la versione del **prototipo**, non Wardrobe v3.

Desktop profiles explicitly set `default_target: graphical.target`. After enabling
services, Tailor checks `systemctl get-default`, runs `systemctl set-default`
only if needed, and verifies the result. This selects graphical boot for the
next restart without starting or isolating the desktop during apply. Omit
`default_target` to retain the current boot target; the field requires systemd
and a valid `.target` unit name.

## openSUSE Tumbleweed

Il backend `opensuse` usa `zypper --non-interactive refresh`, ricerca esatta
con output XML nei repository e installazione in una singola transazione.
La verifica dei pacchetti installati usa RPM. Non aggiunge repository esterni
né accetta automaticamente nuove chiavi GPG. Le ricette desktop puntano a
Tumbleweed con il servizio nativo `lightdm.service`; Leap 15.6 e sistemi
transactional richiedono un percorso separato e non sono validati.

```bash
tailor apply examples/provision/colibri.yaml --dry-run --family opensuse --init systemd
sudo tailor apply examples/provision/colibri.yaml
systemctl get-default
systemctl is-enabled lightdm.service
```

Il test reale deve verificare anche login Xfce dopo il riavvio e riapplicazione
senza reinstallazioni. L'avvio grafico non viene attivato durante `apply`.

Riferimenti: [Zypper](https://manpages.opensuse.org/Tumbleweed/zypper/zypper.8.en.html),
[LightDM su SUSE](https://packagehub.suse.com/packages/lightdm/1_32_0-bp160_1_1/).

Su Tumbleweed/Slowroll il profilo LightDM sostituisce il collegamento legacy
`display-manager.service` con `systemctl enable --force lightdm.service`.
Verifica sia l'abilitazione persistente sia che l'alias selezioni LightDM;
una seconda applicazione non ripete la modifica. Il servizio attivo non viene
avviato o fermato: il passaggio avviene al prossimo riavvio.

## Manjaro

Manjaro viene rilevata come famiglia `archlinux`: usa gli stessi profili completi
LightDM e Colibri e i repository Pacman della macchina. Non serve un backend
separato. La preparazione esegue `pacman -Syu --noconfirm`, quindi aggiorna anche
il sistema prima di installare i pacchetti mancanti.

```bash
tailor apply examples/provision/colibri.yaml --dry-run --family archlinux --init systemd
sudo tailor apply examples/provision/colibri.yaml
sudo tailor apply examples/provision/colibri.yaml
systemctl is-enabled lightdm.service
systemctl show --property=Id --value display-manager.service
systemctl get-default
```

I risultati attesi sono `enabled`, `lightdm.service` e `graphical.target`.
Il profilo Arch/Manjaro seleziona LightDM sostituendo l'eventuale alias di un
altro display manager con `systemctl enable --force lightdm.service`, poi
verifica l'alias. Non ferma il servizio attivo; riavviare per provare il login
Xfce. La verifica nella VM Manjaro resta da effettuare.

Riferimento: [installazione desktop Manjaro](https://wiki.manjaro.org/index.php?title=Install_Desktop_Environments%2Fen).

## Colibri: Whisker Menu e sysroot pubblico

La ricetta `examples/provision/colibri.yaml` dichiara
`sysroot: colibri/sysroot`, risolto rispetto alla directory della ricetta,
anche quando il comando viene eseguito da un'altra directory. Il sysroot
ripulito è versionato insieme agli esempi: non serve un checkout Wardrobe.

```bash
tailor apply examples/provision/colibri.yaml --dry-run
sudo tailor apply examples/provision/colibri.yaml
```

`rsync` deve essere disponibile. `--sysroot /percorso/sysroot` sostituisce la
sorgente dichiarata nella ricetta, se si desidera usare un costume locale.
Tutti i profili installano Whisker Menu e il plugin PulseAudio del pannello.

La copia avviene dopo la configurazione e prima dei servizi. Comprende file
nascosti, binari e collegamenti; conserva permessi, ACL e attributi estesi.
La proprietà dei file di sistema è normalizzata a root, indipendentemente
dall'utente che ha clonato Git. Non elimina file estranei. Confronta checksum
prima della copia e verifica l'assenza di differenze dopo. La directory sorgente
stessa non modifica gli attributi della radice `/`. Sorgenti mancanti o tipi
speciali di file vengono rifiutati prima dei pacchetti. La simulazione non
esegue rsync.

Gli asset pubblici includono le impostazioni Xfce, i file shell predefiniti,
la configurazione uinput e lo sfondo con i crediti originali. Sono esclusi
l'intero profilo Firefox, cache, cronologie, credenziali, stato dei monitor,
elenco delle applicazioni recenti e metadati EXIF dello sfondo. Il sysroot
locale originale non viene modificato.

`sysroot/etc/skel` configura i nuovi utenti attraverso `/etc/skel`; le home
degli utenti già esistenti non vengono sincronizzate automaticamente.
