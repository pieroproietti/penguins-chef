# Nuovo motore: operazioni verificabili

Questo branch esplora un nuovo modello, separato dalle ricette Wardrobe v2.
Il primo percorso eseguibile è LightDM su Debian e Arch con systemd; il
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
  pacman e systemd. Devuan con SysV/OpenRC viene rifiutato se la ricetta
  richiede servizi: il supporto APT non implica supporto dell'init.
- Su Debian la preparazione esegue `apt-get update --error-on=any`.
- Su Arch esegue **`pacman -Syu --noconfirm`: aggiorna anche il sistema**.
  Non viene mai eseguito `pacman -Sy` da solo, perché gli aggiornamenti
  parziali non sono supportati. Dopo un errore la vestizione si interrompe.
  Riferimento: https://wiki.archlinux.org/title/System_maintenance#Partial_upgrades_are_unsupported
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

Il test con runner simulato verifica ordine, fallimenti, postcondizioni e
riesecuzione. Non certifica il desktop su Debian o Arch. Su VM pulite serve
verificare il piano, applicare la ricetta, rieseguirla, riavviare e controllare
il login grafico. Provare anche un repository senza il pacchetto richiesto e
un display manager già abilitato.

## Passi verso Colibri

1. Verificare LightDM su Debian e Arch reali, fissando la gestione dei conflitti.
2. Aggiungere XFCE, rete e audio come funzioni separate del costume.
3. Introdurre composizione delle funzioni senza copie implicite dei profili.
4. Portare asset, configurazione utente, `base` ed `eggs-dev`.
5. Estendere repository e init sulla base dei casi incontrati.

La versione 1 nello schema è la versione del **prototipo**, non Wardrobe v3.
