'use strict';

// Interface locale de gh cohorte, organisée comme GitHub Classroom.
//
// Un groupe rassemble des étudiants ; un travail est distribué à ce groupe, un
// dépôt par étudiant. Le groupe n'existe que dans le fichier local : sur GitHub,
// ce sont les noms de dépôts — « préfixe-travail-compte » — qui portent tout.
//
// Tout le contenu variable passe par textContent : un nom de dépôt ou de
// personne ne peut pas devenir du balisage.

// ------------------------------------------------------------------ outillage

const $ = (id) => document.getElementById(id);

function el(tag, attributs = {}, ...enfants) {
  const noeud = document.createElement(tag);
  for (const [cle, valeur] of Object.entries(attributs)) {
    if (valeur === null || valeur === undefined || valeur === false) continue;
    if (cle === 'classe') noeud.className = valeur;
    else if (cle === 'texte') noeud.textContent = valeur;
    else if (cle.startsWith('on')) noeud.addEventListener(cle.slice(2), valeur);
    else if (valeur === true) noeud.setAttribute(cle, '');
    else noeud.setAttribute(cle, valeur);
  }
  for (const enfant of enfants.flat()) {
    if (enfant === null || enfant === undefined) continue;
    noeud.append(enfant);
  }
  return noeud;
}

function vider(noeud) {
  while (noeud.firstChild) noeud.firstChild.remove();
}

// plageDeCases donne aux listes de cases ce que le terminal accepte déjà sous
// la forme « 2-5 » : on coche une case, puis maj + clic sur une autre, et tout
// ce qui les sépare prend l'état de la seconde. Sans cela, trente dépôts se
// cochent en trente clics.
//
// L'écoute est posée sur le conteneur, jamais sur les cases : les listes se
// redessinent à chaque chargement, et celles d'hier ont disparu. Le conteneur
// est rendu, pour se poser dans un arbre en cours de construction.
function plageDeCases(conteneur) {
  let ancre = null;

  // Maj + clic étend aussi la sélection de texte du navigateur, qui surlignerait
  // la liste au passage. Ce n'est pas l'appui qui coche, c'est le clic : le
  // refuser ne coûte que le surlignage.
  conteneur.addEventListener('mousedown', (evenement) => {
    if (evenement.shiftKey) evenement.preventDefault();
  });

  conteneur.addEventListener('click', (evenement) => {
    const cible = evenement.target;
    if (!cible.matches('input[type="checkbox"]')) return;
    const cases = [...conteneur.querySelectorAll('input[type="checkbox"]')];
    const arrivee = cases.indexOf(cible);
    const depart = cases.indexOf(ancre);
    ancre = cible;
    if (!evenement.shiftKey || depart < 0 || depart === arrivee) return;

    // Le clic vient de basculer la case visée ; les autres prennent son état,
    // retenu avant la boucle. Certaines listes remettent en effet leurs cases
    // d'aplomb à chaque « change » : relire la case visée en cours de route
    // rendrait ce qu'elle valait avant le clic. Pour la même raison elle est
    // réannoncée avec les autres plutôt que laissée au navigateur, qui la
    // déclarerait trop tard ; son « change » suivra, sans rien dire de neuf.
    const coche = cible.checked;
    for (const case_ of cases.slice(Math.min(depart, arrivee), Math.max(depart, arrivee) + 1)) {
      case_.checked = coche;
      case_.dispatchEvent(new Event('change', { bubbles: true }));
    }
  });
  return conteneur;
}

// Les tracés viennent des Octicons de GitHub, sur une grille de 16 : les mêmes
// pictogrammes que le site où mènent tous les liens de la page.
const TRACES = {
  crayon: 'M11.013 1.427a1.75 1.75 0 0 1 2.474 0l1.086 1.086a1.75 1.75 0 0 1 0 2.474l-8.61 ' +
    '8.61c-.21.21-.47.364-.756.445l-3.251.93a.75.75 0 0 1-.927-.928l.929-3.25c.081-.286.235' +
    '-.547.445-.758l8.61-8.61Zm.176 4.823L9.75 4.81l-6.286 6.287a.253.253 0 0 0-.064.108l-.' +
    '558 1.953 1.953-.558a.253.253 0 0 0 .108-.064Zm1.238-3.763a.25.25 0 0 0-.354 0L10.811 3' +
    '.75l1.439 1.44 1.263-1.263a.25.25 0 0 0 0-.354Z',
  corbeille: 'M11 1.75V3h2.25a.75.75 0 0 1 0 1.5H2.75a.75.75 0 0 1 0-1.5H5V1.75C5 .784 5.784' +
    ' 0 6.75 0h2.5C10.216 0 11 .784 11 1.75ZM4.496 6.675l.66 6.6a.25.25 0 0 0 .249.225h5.19a' +
    '.25.25 0 0 0 .249-.225l.66-6.6a.75.75 0 0 1 1.492.149l-.66 6.6A1.748 1.748 0 0 1 10.595' +
    ' 15h-5.19a1.75 1.75 0 0 1-1.741-1.575l-.66-6.6a.75.75 0 1 1 1.492-.15ZM6.5 1.75V3h3V1.7' +
    '5a.25.25 0 0 0-.25-.25h-2.5a.25.25 0 0 0-.25.25Z',
  gens: 'M2 5.5a3.5 3.5 0 1 1 5.898 2.549 5.508 5.508 0 0 1 3.034 4.084.75.75 0 1 1-1.482.2' +
    '35 4 4 0 0 0-7.9 0 .75.75 0 0 1-1.482-.236A5.507 5.507 0 0 1 3.102 8.05 3.493 3.493 0 0' +
    ' 1 2 5.5ZM11 4a3.001 3.001 0 0 1 2.22 5.018 5.01 5.01 0 0 1 2.56 3.012.749.749 0 0 1-.8' +
    '85.954.752.752 0 0 1-.549-.514 3.507 3.507 0 0 0-2.522-2.372.75.75 0 0 1-.574-.73v-.352' +
    'a.75.75 0 0 1 .416-.672A1.5 1.5 0 0 0 11 5.5.75.75 0 0 1 11 4Zm-5.5-.5a2 2 0 1 0-.001 3' +
    '.999A2 2 0 0 0 5.5 3.5Z',
  fleche: 'M8.22 2.97a.75.75 0 0 1 1.06 0l4.25 4.25a.75.75 0 0 1 0 1.06l-4.25 4.25a.751.751 ' +
    '0 0 1-1.042-.018.751.751 0 0 1-.018-1.042l2.97-2.97H3.75a.75.75 0 0 1 0-1.5h7.44L8.22 4' +
    '.03a.75.75 0 0 1 0-1.06Z',
  cle: 'M10.5 0a5.499 5.499 0 1 1-1.288 10.848l-.932.932a.749.749 0 0 1-.53.22H7v.75a.749.74' +
    '9 0 0 1-.22.53l-.5.5a.749.749 0 0 1-.53.22H5v.75a.749.749 0 0 1-.22.53l-.5.5a.749.749 0' +
    ' 0 1-.53.22h-2A1.75 1.75 0 0 1 0 14.25v-2c0-.199.079-.389.22-.53l4.932-4.932A5.5 5.5 0 ' +
    '0 1 10.5 0Zm-4 5.5c0 .458.06.902.173 1.324a.75.75 0 0 1-.193.72L1.5 12.562v1.688c0 .138' +
    '.112.25.25.25h1.689l.311-.311V13.25a.75.75 0 0 1 .75-.75h1.19l.31-.311V11a.75.75 0 0 1 ' +
    '.75-.75h1.19l.69-.691a.75.75 0 0 1 .718-.194c.422.112.866.172 1.324.172a4 4 0 1 0-4-4Zm' +
    '5-1.5a1 1 0 1 1 2 0 1 1 0 0 1-2 0Z',
};

// icone dessine un pictogramme. Un bouton qui n'a plus de texte n'a plus de nom
// non plus : title et aria-label le lui rendent, et le tracé reste hors de
// l'arbre d'accessibilité pour ne pas le dire deux fois.
function icone(nom) {
  const NS = 'http://www.w3.org/2000/svg';
  const dessin = document.createElementNS(NS, 'svg');
  dessin.setAttribute('viewBox', '0 0 16 16');
  dessin.setAttribute('width', '16');
  dessin.setAttribute('height', '16');
  dessin.setAttribute('fill', 'currentColor');
  dessin.setAttribute('aria-hidden', 'true');
  dessin.setAttribute('focusable', 'false');
  const trace = document.createElementNS(NS, 'path');
  trace.setAttribute('d', TRACES[nom]);
  dessin.append(trace);
  return dessin;
}

function message(texte, ton = 'succes', duree = 6000) {
  const avis = el('div', { classe: 'avis ' + ton, texte });
  $('messages').append(avis);
  setTimeout(() => avis.remove(), duree);
}

// api envoie une requête et renvoie le JSON, ou lève l'erreur du serveur.
async function api(methode, chemin, corps) {
  const options = { method: methode, headers: { 'X-Cohorte': '1' } };
  if (corps !== undefined) {
    options.headers['Content-Type'] = 'application/json';
    options.body = JSON.stringify(corps);
  }
  debutRequete();
  let reponse;
  let texte;
  try {
    reponse = await fetch(chemin, options);
    texte = await reponse.text();
  } finally {
    finRequete();
  }
  let donnees = null;
  if (texte) {
    try { donnees = JSON.parse(texte); } catch { donnees = { error: texte }; }
  }
  if (!reponse.ok) {
    const echec = new Error((donnees && donnees.error) || `Erreur ${reponse.status}`);
    // Le serveur nomme la portée qui manque quand GitHub l'a fait savoir :
    // c'est elle qui transforme un refus sec en proposition de reprise.
    if (donnees && donnees.scope) echec.portee = donnees.scope;
    throw echec;
  }
  return donnees;
}

// ------------------------------------------------------- attente du serveur

// Tout ce que la page demande passe par api() : compter les requêtes en route
// suffit à savoir si l'on attend. Le trait du haut ne paraît qu'au bout d'un
// moment — beaucoup de réponses arrivent en quelques millisecondes, et un
// clignotement se remarque plus qu'une attente courte.
const DELAI_ATTENTE = 250;

let requetesEnRoute = 0;
let minuterieAttente = null;

function debutRequete() {
  requetesEnRoute += 1;
  if (requetesEnRoute > 1 || minuterieAttente) return;
  minuterieAttente = setTimeout(() => {
    minuterieAttente = null;
    if (requetesEnRoute === 0) return;
    $('attente-reseau').hidden = false;
    document.body.setAttribute('aria-busy', 'true');
  }, DELAI_ATTENTE);
}

function finRequete() {
  requetesEnRoute = Math.max(0, requetesEnRoute - 1);
  if (requetesEnRoute > 0) return;
  if (minuterieAttente) {
    clearTimeout(minuterieAttente);
    minuterieAttente = null;
  }
  $('attente-reseau').hidden = true;
  document.body.removeAttribute('aria-busy');
}

// enAttente occupe une zone encore vide le temps qu'elle se remplisse : sans
// cela, rien ne distingue une liste vide d'une liste qui arrive.
function enAttente(conteneur, texte) {
  vider(conteneur);
  conteneur.append(el('div', { classe: 'boite-vide attente' },
    el('span', { classe: 'roue', 'aria-hidden': 'true' }),
    el('span', { texte })));
}

// enEchec remplace l'attente quand la réponse n'est jamais venue : rester sur
// « chargement… » ferait croire que ça arrive encore.
function enEchec(conteneur, texte) {
  vider(conteneur);
  conteneur.append(el('div', { classe: 'boite-vide', texte }));
}

// occuper estompe une zone déjà remplie pendant qu'elle se recharge : trier ou
// filtrer ne doit pas faire clignoter ce qu'on était en train de lire.
function occuper(noeud, occupe) {
  noeud.classList.toggle('occupe', occupe);
  if (occupe) noeud.setAttribute('aria-busy', 'true');
  else noeud.removeAttribute('aria-busy');
}

// attendreTable marque une des grandes tables — les dépôts d'un travail, les
// étudiants d'un groupe — pendant que le serveur répond : celle qui montre
// déjà quelque chose s'estompe, celle qui est vide annonce ce qu'elle attend.
// « fini » rend ce que la réponse vaut, et dit l'échec plutôt que de laisser
// « chargement… » à l'écran.
function attendreTable(table, vide, texte) {
  const corps = $(table);
  const mot = $(vide);
  const remplie = !corps.hidden;
  if (remplie) {
    occuper(corps, true);
  } else {
    mot.hidden = false;
    mot.textContent = texte;
  }
  return {
    fini(donnees, echec) {
      occuper(corps, false);
      if (!donnees && !remplie) mot.textContent = echec;
      return !!donnees;
    },
  };
}

// tenter exécute une action et affiche l'erreur éventuelle sans casser la page.
// Un refus faute de portée n'en est pas vraiment un : rien n'a été fait, et le
// jeton peut être regénéré sur place. L'action est alors rejouée — une fois,
// pour qu'un refus qui persiste finisse par se dire.
async function tenter(action, contexte, rejoue = false) {
  try {
    return await action();
  } catch (erreur) {
    if (erreur.portee && !rejoue && await proposerRegeneration(erreur.portee, contexte)) {
      return tenter(action, contexte, true);
    }
    message(contexte ? `${contexte} : ${erreur.message}` : erreur.message, 'erreur', 12000);
    return null;
  }
}

const encode = encodeURIComponent;

// ---------------------------------------------------------------------- état

const etat = {
  contexte: null,
  reglages: {},
  // vue retient l'écran affiché : une action lancée depuis un travail ne doit
  // pas ramener dans les équipes, et inversement.
  vue: '',
  // Ce que le jeton permet, et la fonction qui dit quelles portées sont cochées
  // dans les réglages généraux.
  jeton: null,
  porteesCochees: null,
  organisation: '',
  groupes: [],
  sessions: [],
  // parcours dit où l'on se trouve dans la hiérarchie : rien, une session, ou
  // une session et un cours.
  parcours: { session: '', cours: '' },
  groupe: null,
  travail: null,
  // Le travail dont les critères sont posés. Repasser par le groupe oublie le
  // travail ouvert ; y revenir ne doit pas pour autant effacer son filtre.
  travailRegle: '',
  selection: new Set(),
  acces: new Map(),
  etudiants: [],
  // Ce que la liste des étudiants montre, et de qui elle est cochée. Le tri et
  // le filtre partent au serveur : c'est lui qui sait ce qu'ils veulent dire.
  filtre: { texte: '', travail: '', activite: '', apres: '', avant: '', tri: 'nom', desc: false },
  // Ce que la liste d'un travail montre : les mêmes critères, appliqués aux
  // mêmes lignes — un dépôt par personne — par le même paquet du serveur.
  filtreTravail: { texte: '', activite: '', apres: '', avant: '', tri: 'nom', desc: false },
  // L'annuaire de l'organisation : ses lignes, ses critères, et qui est déplié.
  annuaire: {
    lignes: [],
    filtre: {
      texte: '', session: '', cours: '', activite: '', apres: '', avant: '',
      tri: 'nom', desc: false,
    },
    deplies: new Set(),
  },
  // La fiche d'un utilisateur : ce que le serveur en a rendu, et le compte
  // demandé. Le compte demandé n'est pas toujours celui qui désigne la
  // personne — on peut arriver par son second compte.
  fiche: { compte: '', donnees: null },
  // Le cloisonnement du groupe ouvert : son équipe enseignante, et qui peut
  // en être.
  cloisonnement: null,
  deplaces: new Set(),
  // Les travaux cochés dans la liste d'un groupe, pour les déplacer ensemble.
  travauxChoisis: new Set(),
  // Les équipes du groupe, telles que GitHub les donne, et les étudiants
  // qu'aucune n'accueille. Rien n'en est retenu localement : leur nom dit à
  // quel groupe elles appartiennent.
  equipes: [],
  orphelins: [],
  // Ce que l'assistant distribue : « individuel » ou « equipe ». La nature
  // d'un travail ne se déclare nulle part — elle se lit ensuite dans le nom de
  // ses dépôts —, mais il faut bien la choisir au moment de le créer.
  nature: 'individuel',
  destinataires: new Set(),
  reglagesTravail: {},
  nouveau: { org: '', etudiants: [], rejets: [] },
  etape: 1,
};

// ------------------------------------------------------- opérations et journal

let operationCourante = null;
// Un écran qui a son propre journal le dit ici, et l'opération s'y écrit aussi.
// C'est le cas de la reprise de dépôts : sa dernière étape est ce journal, et
// l'envoyer dans le panneau global seul obligerait à quitter l'écran des yeux.
let miroirOperation = null;

function ouvrirOperation(fiche) {
  operationCourante = fiche;
  if (miroirOperation) {
    vider(miroirOperation.journal);
    miroirOperation.barre.value = 0;
  }
  $('operation').hidden = false;
  $('operation-titre').textContent = fiche.label;
  $('operation-etat').textContent = fiche.status;
  $('operation-annuler').hidden = false;
  $('operation-barre').value = 0;
  vider($('operation-journal'));
}

function journaliser(texte, ton = '') {
  for (const journal of [$('operation-journal'),
    miroirOperation && miroirOperation.journal]) {
    if (!journal) continue;
    journal.append(el('div', { classe: ton, texte }));
    journal.scrollTop = journal.scrollHeight;
  }
}

function appliquerEvenement(evenement) {
  switch (evenement.kind) {
    case 'avancement':
      if (evenement.total > 0) {
        const part = Math.round((evenement.done / evenement.total) * 100);
        $('operation-barre').value = part;
        if (miroirOperation) miroirOperation.barre.value = part;
        $('operation-etat').textContent = `${evenement.done} / ${evenement.total}`;
      }
      break;
    case 'ligne':
      journaliser(evenement.text, tonDuResultat(evenement.data));
      break;
    case 'avertissement':
      journaliser(evenement.text, 'warn');
      break;
    case 'fin': {
      const fin = evenement.data || {};
      $('operation-etat').textContent = fin.status || 'terminé';
      $('operation-annuler').hidden = true;
      if (fin.failure) {
        journaliser(fin.failure, 'err');
      } else {
        $('operation-barre').value = 100;
        if (miroirOperation) miroirOperation.barre.value = 100;
      }
      // Une opération arrêtée faute de portée ne se rejoue pas toute seule :
      // une partie a pu aboutir, et c'est à la personne de dire ce qu'elle
      // relance. Le jeton, lui, peut être refait tout de suite.
      if (fin.scope) {
        proposerRegeneration(fin.scope, fin.label).then((refait) => {
          if (refait) journaliser('Jeton renouvelé : relancez l’opération.', 'ok');
        });
      }
      break;
    }
  }
}

// tonDuResultat colore une ligne selon l'issue rapportée.
function tonDuResultat(donnees) {
  if (!donnees || !donnees.status) return 'dim';
  const statut = donnees.status;
  if (statut === 'échec') return 'err';
  if (statut === 'ignoré' || statut === 'ignoré (dépôt non vide)') return 'warn';
  if (statut === 'créé' || statut === 'cloné' || statut === 'mis à jour') return 'ok';
  return 'dim';
}

// suivre branche le panneau de progression sur une opération et attend sa fin ;
// la promesse rend son bilan, ou rien si elle a échoué.
function suivre(fiche, miroir = null) {
  miroirOperation = miroir;
  ouvrirOperation(fiche);
  return new Promise((resolve) => {
    let seq = 0;
    let source = null;
    const brancher = () => {
      source = new EventSource(`/api/jobs/${encode(fiche.id)}/events?from=${seq}`);
      source.onmessage = (evenement) => {
        const item = JSON.parse(evenement.data);
        seq = item.seq;
        appliquerEvenement(item);
        if (item.kind === 'fin') {
          source.close();
          miroirOperation = null;
          resolve((item.data && item.data.result) || null);
        }
      };
      source.onerror = async () => {
        source.close();
        // La coupure peut venir d'une fermeture normale : l'état de l'opération
        // tranche, et la lecture reprend au dernier événement reçu.
        const fin = await api('GET', `/api/jobs/${encode(fiche.id)}`).catch(() => null);
        if (!fin || fin.status !== 'en cours') {
          miroirOperation = null;
          resolve((fin && fin.result) || null);
          return;
        }
        setTimeout(brancher, 1000);
      };
    };
    brancher();
  });
}

$('operation-fermer').addEventListener('click', () => { $('operation').hidden = true; });
$('operation-annuler').addEventListener('click', () => {
  if (operationCourante) {
    api('POST', `/api/jobs/${encode(operationCourante.id)}/cancel`).catch(() => {});
  }
});

// ------------------------------------------------------------------ dialogue

// demander ouvre le dialogue et renvoie vrai si la personne confirme.
// La réponse vient des boutons eux-mêmes : tous les moteurs n'émettent pas
// « close » quand un formulaire « method=dialog » referme la fenêtre.
// « preparer » reçoit le bouton de confirmation juste avant l'ouverture : un
// contenu qui exige un choix peut ainsi le tenir éteint tant que rien n'est
// désigné. Le bouton est remis d'aplomb à chaque question — celle d'avant a pu
// le laisser éteint.
function demander(titre, contenu, libelle = 'Confirmer', preparer = null) {
  const dialogue = $('dialogue');
  const valider = $('dialogue-ok');
  const annuler = $('dialogue-annuler');
  $('dialogue-titre').textContent = titre;
  const corps = $('dialogue-corps');
  vider(corps);
  corps.className = 'corps-dialogue';
  corps.append(contenu);
  valider.textContent = libelle;
  valider.disabled = false;

  return new Promise((resolve) => {
    let repondu = false;
    const repondre = (reponse) => {
      if (repondu) return;
      repondu = true;
      valider.removeEventListener('click', surOui);
      annuler.removeEventListener('click', surNon);
      dialogue.removeEventListener('close', surFermeture);
      dialogue.removeEventListener('cancel', surNon);
      if (dialogue.open) dialogue.close();
      resolve(reponse);
    };
    const surOui = () => repondre(true);
    const surNon = () => repondre(false);
    // Le dialogue est unique, et « close » part en différé : celui de la
    // question précédente peut arriver alors que la suivante est déjà ouverte,
    // et y répondrait tout seul. Il se reconnaît à ce que le dialogue est
    // encore ouvert — une fermeture vraie l'a forcément refermé d'abord.
    const surFermeture = () => {
      if (dialogue.open) return;
      repondre(dialogue.returnValue === 'ok');
    };
    valider.addEventListener('click', surOui);
    annuler.addEventListener('click', surNon);
    // Échap referme sans passer par les boutons.
    dialogue.addEventListener('cancel', surNon);
    dialogue.addEventListener('close', surFermeture);
    if (preparer) preparer(valider);
    dialogue.showModal();
  });
}

// --------------------------------------------------------------------- vues

// Les vues d'un groupe partagent ses onglets.
const ongletDeLaVue = {
  travaux: 'travaux', travail: 'travaux', assistant: 'travaux',
  etudiants: 'etudiants', equipes: 'equipes', 'groupe-reglages': 'groupe-reglages',
};

// aplati met un texte à plat pour la recherche : minuscules, accents retirés.
// « Été 2026 » se cherche alors aussi bien en tapant « ete ».
function aplati(texte) {
  return (texte || '').toLowerCase().normalize('NFD').replace(/\p{Diacritic}/gu, '');
}

// gens rend les personnes d'un groupe, une par personne et non une par compte :
// le matricule réunit les lignes d'un même étudiant, et les compter deux fois
// ferait deux inscrits d'un seul.
function gens(groupe) {
  if (!groupe) return [];
  return groupe.people || groupe.students || [];
}

// sigle rend un code de cours tel qu'on l'écrit : « 4w6 » se lit « 4W6 ». Les
// dépôts, eux, gardent la casse d'origine — GitHub ne la distingue pas.
function sigle(code) {
  return (code || '').toUpperCase();
}

// travaux accorde le mot avec le nombre.
function travaux(nombre) {
  return nombre === 1 ? '1 travail' : `${nombre} travaux`;
}

// groupes accorde le mot avec le nombre.
function groupesEnMots(nombre) {
  return nombre === 1 ? '1 groupe' : `${nombre} groupes`;
}

for (const bouton of $('onglets').querySelectorAll('button')) {
  bouton.addEventListener('click', () => afficherVue(bouton.dataset.vue));
}

// ------------------------------------------------------------------ adresses

// L'adresse dit où l'on se trouve : recharger, revenir en arrière ou coller un
// lien mènent au même endroit. Le serveur rend l'interface pour toute adresse
// qu'il ne connaît pas, ce qui permet de vraies routes plutôt qu'un fragment.

function cheminDeLaVue(nom) {
  const groupe = etat.groupe ? encode(etat.groupe.scope) : '';
  switch (nom) {
    case 'organisation': return '/organisation';
    case 'annuaire': return '/utilisateurs';
    case 'fiche': return `/u/${encode(etat.fiche.compte)}`;
    case 'nouveau-groupe': return '/nouveau-groupe';
    case 'import': return '/reprise';
    case 'reglages': return '/reglages';
    case 'travaux': return `/g/${groupe}`;
    case 'assistant': return `/g/${groupe}/nouveau-travail`;
    case 'etudiants': return `/g/${groupe}/etudiants`;
    case 'equipes': return `/g/${groupe}/equipes`;
    case 'groupe-reglages': return `/g/${groupe}/reglages`;
    case 'travail':
      return `/g/${groupe}/travaux/${encode(etat.travail ? etat.travail.name : '')}`;
    default: {
      const { session, cours } = etat.parcours;
      if (!session) return '/';
      if (!cours) return `/s/${encode(session)}`;
      return `/s/${encode(session)}/${encode(cours)}`;
    }
  }
}

// lireAdresse traduit l'adresse courante en destination.
function lireAdresse() {
  const morceaux = window.location.pathname.split('/')
    .filter(Boolean).map(decodeURIComponent);
  if (morceaux.length === 0) return { vue: 'parcours', session: '', cours: '' };
  switch (morceaux[0]) {
    case 's':
      return { vue: 'parcours', session: morceaux[1] || '', cours: morceaux[2] || '' };
    case 'g':
      return {
        vue: vueDuGroupe(morceaux[2]), groupe: morceaux[1] || '',
        travail: morceaux[3] || '',
      };
    case 'utilisateurs':
      return { vue: 'annuaire' };
    case 'u':
      return { vue: 'fiche', compte: morceaux[1] || '' };
    case 'reprise':
      return { vue: 'import' };
    case 'nouveau-groupe': case 'reglages': case 'organisation':
      return { vue: morceaux[0] };
    default:
      return { vue: 'parcours', session: '', cours: '' };
  }
}

function vueDuGroupe(segment) {
  switch (segment) {
    case 'travaux': return 'travail';
    case 'nouveau-travail': return 'assistant';
    case 'etudiants': return 'etudiants';
    case 'equipes': return 'equipes';
    case 'reglages': return 'groupe-reglages';
    default: return 'travaux';
  }
}

// allerA rejoint une destination lue dans l'adresse, sans rien empiler : c'est
// le chemin du retour arrière et du rechargement.
async function allerA(route) {
  if (!etat.organisation) { afficherVue('organisation', true); return; }
  if (route.vue === 'parcours') {
    etat.parcours = { session: route.session || '', cours: route.cours || '' };
    afficherVue('parcours', true);
    return;
  }
  if (route.vue === 'fiche') {
    // La fiche se désigne par un compte : c'est lui, et non un état retenu,
    // qui dit qui l'on regarde. Coller son adresse doit suffire.
    etat.fiche = { compte: route.compte || '', donnees: null };
    afficherVue('fiche', true);
    return;
  }
  if (!ongletDeLaVue[route.vue]) {
    // Une adresse peut ouvrir un écran directement : il faut alors le remplir
    // comme le ferait le bouton qui y mène.
    if (route.vue === 'nouveau-groupe') preparerNouveauGroupe();
    if (route.vue === 'import') preparerImport();
    afficherVue(route.vue, true);
    return;
  }

  // Les sessions et les groupes voisins viennent d'ici : arriver droit sur un
  // groupe par son adresse les demande quand même, pour que le fil d'Ariane
  // sache dire « Automne 2026 » plutôt que « a26 ».
  if (etat.groupes.length === 0) await chargerGroupes();
  if (!etat.groupe || etat.groupe.scope !== route.groupe) {
    if (!await ouvrirGroupe(route.groupe, false, true)) {
      // L'adresse désigne un groupe retiré depuis : on remonte à l'accueil.
      etat.parcours = { session: '', cours: '' };
      naviguer('/', true);
      afficherVue('parcours', true);
      return;
    }
  }
  if (route.vue !== 'travail') { afficherVue(route.vue, true); return; }

  const travail = (etat.groupe.assignments || []).find((item) =>
    item.name.toLowerCase() === (route.travail || '').toLowerCase());
  if (!travail) { afficherVue('travaux', true); return; }
  await ouvrirTravail(travail, false, true);
}

// naviguer pose une adresse sans changer de vue : les déplacements internes à
// la hiérarchie s'en servent.
function naviguer(chemin, remplacer) {
  if (window.location.pathname === chemin) return;
  window.history[remplacer ? 'replaceState' : 'pushState']({}, '', chemin);
}

window.addEventListener('popstate', () => { allerA(lireAdresse()); });

// ------------------------------------------------------------- affichage

function afficherVue(nom, sansHistorique) {
  // Tant qu'aucune organisation n'est choisie, rien d'autre n'est accessible :
  // tout ce que fait l'outil s'y passe.
  if (!etat.organisation && nom !== 'organisation') nom = 'organisation';
  const onglet = ongletDeLaVue[nom];
  $('ouvrir-reglages').hidden = !etat.organisation;
  $('accueil').disabled = !etat.organisation;
  for (const bouton of $('onglets').querySelectorAll('button')) {
    bouton.classList.toggle('actif', bouton.dataset.vue === onglet);
  }
  for (const vue of document.querySelectorAll('.vue')) {
    vue.hidden = vue.id !== 'vue-' + nom;
  }
  etat.vue = nom;
  dessinerEntete(nom, onglet);
  if (!sansHistorique) naviguer(cheminDeLaVue(nom));
  window.scrollTo(0, 0);
  // Les comptages d'une vue changent pendant qu'on est ailleurs : chaque
  // retour les redemande plutôt que de laisser voir un état périmé.
  if (nom === 'parcours') chargerGroupes();
  if (nom === 'reglages') rafraichirEmplacements();
  if (nom === 'annuaire') chargerAnnuaire();
  if (nom === 'fiche') chargerFiche();
  if (nom === 'etudiants') chargerEtudiants();
  if (nom === 'equipes') chargerEquipes();
  if (nom === 'groupe-reglages') { ecrireReglagesGroupe(); chargerCloisonnement(); }
}

// ------------------------------------------------------------ en-tête de page

// L'en-tête est le même partout : le fil d'Ariane, le titre, ce qu'il faut
// signaler. Passer d'une session à un cours puis à un groupe ne doit pas donner
// l'impression de changer d'application.
function dessinerEntete(nom, onglet) {
  const entete = $('entete-page');
  entete.hidden = nom === 'organisation';
  $('onglets').hidden = !onglet;
  entete.classList.toggle('avec-onglets', !!onglet);
  if (entete.hidden) return;

  const fiche = ficheDeLEntete(nom);
  const fil = $('fil');
  vider(fil);
  fiche.fil.forEach((etape, rang) => {
    if (rang > 0) fil.append(el('span', { classe: 'separateur', texte: '/' }));
    fil.append(etape.action
      ? el('button', { classe: 'lien', type: 'button', texte: etape.texte, onclick: etape.action })
      : el('span', { texte: etape.texte }));
  });

  $('page-titre').textContent = fiche.titre;
  $('page-sous-titre').textContent = fiche.sousTitre || '';
  $('page-sous-titre').hidden = !fiche.sousTitre;

  const nomenclature = $('page-nomenclature');
  vider(nomenclature);
  nomenclature.hidden = !fiche.nomenclature;
  if (fiche.nomenclature) {
    nomenclature.append(document.createTextNode('Ses dépôts s’appellent '));
    nomenclature.append(el('code', { texte: fiche.nomenclature }));
  }

  const avis = $('page-avis');
  vider(avis);
  avis.hidden = !fiche.avis;
  if (fiche.avis) avis.append(el('div', { classe: 'avis alerte', texte: fiche.avis }));

  const actions = $('page-actions');
  vider(actions);
  for (const bouton of fiche.actions || []) {
    actions.append(el('button', {
      classe: 'bouton ' + (bouton.classe || ''), type: 'button',
      texte: bouton.texte, onclick: bouton.action,
    }));
  }
}

// ficheDeLEntete dit ce que l'en-tête montre pour la vue courante.
function ficheDeLEntete(nom) {
  const racine = {
    texte: 'Sessions',
    action: () => { etat.parcours = { session: '', cours: '' }; afficherVue('parcours'); },
  };
  const { session, cours } = etat.parcours;

  if (nom === 'parcours') {
    const actions = [
      { texte: 'Recharger', action: () => chargerGroupes(true) },
      // La hiérarchie mène aux étudiants d'un groupe ; l'annuaire les prend
      // dans l'autre sens, et n'appartient donc à aucun niveau du parcours.
      // Il porte les deux rôles : un enseignant n'est dans aucun groupe.
      { texte: 'Utilisateurs', action: () => afficherVue('annuaire') },
      { texte: 'Reprendre des dépôts', action: () => ouvrirImport() },
      { texte: 'Nouveau groupe', classe: 'vert', action: () => ouvrirNouveauGroupe() },
    ];
    if (!session) {
      return {
        fil: [{ texte: 'Sessions' }], titre: 'Sessions', actions,
        sousTitre: `Organisation ${etat.organisation} · session, cours, groupe, travail`,
      };
    }
    if (!cours) {
      return {
        fil: [racine, { texte: nomDeSession(session) }],
        titre: nomDeSession(session), actions,
        sousTitre: `Cours de la session « ${session} »`,
      };
    }
    return {
      fil: [racine, etapeSession(session), { texte: sigle(cours) }],
      titre: sigle(cours), actions,
      sousTitre: `${nomDeSession(session)} · groupes du cours`,
    };
  }

  if (nom === 'annuaire') {
    return {
      fil: [racine, { texte: 'Utilisateurs' }], titre: 'Utilisateurs',
      sousTitre: `Organisation ${etat.organisation} · une personne, les cours qu'elle a suivis`,
      actions: [{ texte: 'Recharger', action: () => chargerAnnuaire(true) }],
    };
  }
  if (nom === 'fiche') {
    const personne = etat.fiche.donnees ? etat.fiche.donnees.user : null;
    const titre = (personne && personne.full_name) || '@' + etat.fiche.compte;
    return {
      fil: [racine,
        { texte: 'Utilisateurs', action: () => afficherVue('annuaire') },
        { texte: titre }],
      titre,
      sousTitre: personne
        ? `${personne.role} · ${etat.organisation}`
        : `Organisation ${etat.organisation}`,
      actions: [{ texte: 'Recharger', action: () => chargerFiche() }],
    };
  }
  if (nom === 'import') {
    return { fil: [racine, { texte: 'Reprendre des dépôts' }], titre: 'Reprendre des dépôts',
      sousTitre: `Dépôts de ${etat.organisation} nommés « travail-compte », `
        + 'comme GitHub Classroom les laisse.' };
  }
  if (nom === 'nouveau-groupe') {
    return { fil: [racine, { texte: 'Nouveau groupe' }], titre: 'Nouveau groupe',
      sousTitre: 'Des étudiants, une place dans la hiérarchie.' };
  }
  if (nom === 'reglages') {
    return { fil: [racine, { texte: 'Réglages' }], titre: 'Réglages',
      sousTitre: "Ce que l'outil retient d'une session à l'autre, et où il l'écrit." };
  }

  // Les vues d'un groupe : le fil remonte toute la hiérarchie.
  const groupe = etat.groupe || {};
  const fil = [racine];
  if (groupe.session) {
    fil.push(etapeSession(groupe.session), etapeCours(groupe.session, groupe.course));
  }
  fil.push(nom === 'travaux'
    ? { texte: groupe.label || '' }
    : { texte: groupe.label || '', action: () => afficherVue('travaux') });
  if (nom === 'travail' && etat.travail) fil.push({ texte: etat.travail.name });
  if (nom === 'assistant') fil.push({ texte: etat.assistantTitre || 'Nouveau travail' });
  if (nom === 'etudiants') fil.push({ texte: 'Étudiants' });
  if (nom === 'equipes') fil.push({ texte: 'Équipes' });
  if (nom === 'groupe-reglages') fil.push({ texte: 'Réglages du groupe' });

  return {
    fil, titre: groupe.label || '',
    sousTitre: sousTitreDuGroupe(groupe),
    nomenclature: nomenclatureDuGroupe(groupe),
    avis: avisDuGroupe(groupe),
  };
}

function etapeSession(court) {
  return {
    texte: nomDeSession(court),
    action: () => { etat.parcours = { session: court, cours: '' }; afficherVue('parcours'); },
  };
}

function etapeCours(court, cours) {
  return {
    texte: sigle(cours),
    action: () => { etat.parcours = { session: court, cours }; afficherVue('parcours'); },
  };
}

function sousTitreDuGroupe(groupe) {
  let compte = `${gens(groupe).length} étudiant(s)`;
  if (groupe.teams > 0) compte += ` · ${groupe.teams} équipe(s)`;
  if (!groupe.session) return `Organisation ${groupe.org} · ${compte}`;
  return `${groupe.session_name || groupe.session} · ${sigle(groupe.course)}` +
    ` · groupe ${groupe.group} · ${compte}`;
}

// nomenclatureDuGroupe montre en toutes lettres comment ses dépôts s'appellent.
function nomenclatureDuGroupe(groupe) {
  if (groupe.session) {
    // Le dernier niveau nomme le destinataire : l'étudiant, ou l'équipe quand
    // le groupe en a. C'est lui qui dit ensuite la nature d'un travail.
    const dernier = groupe.teams > 0 ? '<étudiant | équipe>' : '<étudiant>';
    return `${groupe.session}.${groupe.course}.${groupe.group}.<travail>.${dernier}`;
  }
  if (groupe.pattern) return groupe.pattern;
  if (groupe.prefix) {
    const separateur = groupe.prefix.includes('.') ? '.' : '-';
    return `${groupe.prefix}${separateur}<travail>${separateur}<étudiant>`;
  }
  return '';
}

function avisDuGroupe(groupe) {
  if (groupe.session || !(groupe.prefix || groupe.pattern)) return '';
  return "Ce groupe suit une nomenclature dépassée. Ses dépôts restent lisibles, mais on ne " +
    'peut plus lui distribuer de travail : renommez-les depuis « Réglages du groupe ».';
}

$('accueil').addEventListener('click', () => {
  etat.parcours = { session: '', cours: '' };
  afficherVue('parcours');
});
$('ouvrir-reglages').addEventListener('click', () => afficherVue('reglages'));

// ------------------------------------------------ parcours de la hiérarchie

async function chargerGroupes(force) {
  if (!etat.organisation) return;
  const liste = $('parcours-liste');
  const premiere = !liste.firstChild;
  if (premiere) enAttente(liste, 'Chargement des groupes…');
  else occuper(liste, true);

  const donnees = await tenter(() => api('GET', '/api/classrooms'), 'Groupes');
  occuper(liste, false);
  if (!donnees) {
    if (premiere) enEchec(liste, "Les groupes n'ont pas pu être chargés.");
    return;
  }
  // Seuls les groupes de l'organisation choisie sont montrés : c'est elle qui
  // cadre tout ce que l'interface propose.
  etat.groupes = (donnees.classrooms || []).filter((groupe) =>
    groupe.org.toLowerCase() === etat.organisation.toLowerCase());
  etat.sessions = donnees.sessions || [];
  dessinerParcours();
  // La détection relit tout l'inventaire : elle n'a lieu que là où elle sert.
}

// nomDeSession retrouve le nom long d'une session.
function nomDeSession(court) {
  const trouve = etat.sessions.find((session) =>
    session.short.toLowerCase() === court.toLowerCase());
  return trouve ? trouve.name : court;
}

// rangDeSession donne la place d'une session dans la suite des sessions. Le
// serveur les envoie déjà rangées — de la plus récente à la plus ancienne — et
// refaire ce calcul ici les ferait diverger. Une session qu'il ne connaît pas
// passe après.
function rangDeSession(court) {
  const rang = etat.sessions.findIndex((session) =>
    session.short.toLowerCase() === court.toLowerCase());
  return rang < 0 ? etat.sessions.length : rang;
}

// dessinerParcours montre le niveau courant : les sessions, les cours d'une
// session, ou les groupes d'un cours.
function dessinerParcours() {
  const { session, cours } = etat.parcours;
  // Le nom long d'une session arrive avec les groupes : l'en-tête, dessiné
  // avant eux, doit être repris une fois qu'ils sont là.
  if (!$('vue-parcours').hidden) dessinerEntete('parcours', null);
  const conteneur = $('parcours-liste');
  vider(conteneur);

  if (!session) {
    dessinerSessions(conteneur);
  } else if (!cours) {
    dessinerCours(conteneur, session);
  } else {
    dessinerGroupesDuCours(conteneur, session, cours);
  }
}

// herites rassemble les groupes qui n'ont pas encore de place dans la
// hiérarchie : ils suivent une nomenclature dépassée, et attendent d'être
// renommés.
function herites() {
  return etat.groupes.filter((groupe) => !groupe.session);
}

function dessinerSessions(conteneur) {
  const parSession = new Map();
  for (const groupe of etat.groupes) {
    if (!groupe.session) continue;
    const cle = groupe.session.toLowerCase();
    if (!parSession.has(cle)) parSession.set(cle, { court: groupe.session, groupes: [] });
    parSession.get(cle).groupes.push(groupe);
  }

  if (parSession.size === 0 && herites().length === 0) {
    conteneur.append(el('div', { classe: 'boite-vide' },
      el('p', { texte: 'Aucun groupe déclaré pour le moment.' }),
      el('p', { classe: 'note',
        texte: 'Déclarez-en un de toutes pièces, ou reprenez des dépôts qu\'une autre ' +
          'convention a nommés.' })));
  }

  const triees = [...parSession.values()].sort((a, b) =>
    rangDeSession(a.court) - rangDeSession(b.court) || a.court.localeCompare(b.court));
  for (const session of triees) {
    const cours = new Set(session.groupes.map((groupe) => groupe.course.toLowerCase()));
    conteneur.append(ligneParcours(
      nomDeSession(session.court),
      `${cours.size} cours · ${groupesEnMots(session.groupes.length)}`,
      [el('span', { classe: 'jeton', texte: session.court })],
      () => { etat.parcours = { session: session.court, cours: '' }; afficherVue('parcours'); }));
  }

  for (const groupe of herites()) {
    conteneur.append(ligneParcours(groupe.label, 'nomenclature dépassée — à renommer',
      [el('span', { classe: 'jeton non', texte: nomenclatureDuGroupe(groupe) })],
      () => ouvrirGroupe(groupe.scope)));
  }
}

function dessinerCours(conteneur, session) {
  const parCours = new Map();
  for (const groupe of etat.groupes) {
    if (!groupe.session || groupe.session.toLowerCase() !== session.toLowerCase()) continue;
    const cle = groupe.course.toLowerCase();
    if (!parCours.has(cle)) parCours.set(cle, { code: groupe.course, groupes: [] });
    parCours.get(cle).groupes.push(groupe);
  }
  const triees = [...parCours.values()].sort((a, b) => a.code.localeCompare(b.code));
  for (const cours of triees) {
    const etudiants = cours.groupes.reduce(
      (total, groupe) => total + gens(groupe).length, 0);
    conteneur.append(ligneParcours(sigle(cours.code),
      `${groupesEnMots(cours.groupes.length)} · ${etudiants} étudiant(s)`, [],
      () => {
        etat.parcours = { session, cours: cours.code };
        afficherVue('parcours');
      }));
  }
}

function dessinerGroupesDuCours(conteneur, session, cours) {
  const retenus = etat.groupes.filter((groupe) =>
    groupe.session && groupe.session.toLowerCase() === session.toLowerCase() &&
    groupe.course.toLowerCase() === cours.toLowerCase());
  for (const groupe of retenus) {
    const sesTravaux = groupe.assignments || [];
    const compte = groupe.known
      ? `${gens(groupe).length} étudiant(s)`
      : 'aucune liste retenue';
    conteneur.append(ligneParcours(groupe.label,
      `${compte} · ${travaux(sesTravaux.length)}`,
      [el('span', { classe: 'jeton', texte: groupe.group })],
      () => ouvrirGroupe(groupe.scope)));
  }
}

// ligneParcours est une ligne cliquable de la hiérarchie.
function ligneParcours(titre, detail, jetons, action) {
  return el('button', { classe: 'travail-ligne', type: 'button', onclick: action },
    el('span', { classe: 'travail-infos' },
      el('span', { classe: 'titre', texte: titre }),
      el('span', { classe: 'detail', texte: detail })),
    el('span', { classe: 'espace' }),
    jetons,
    el('span', { classe: 'chevron', texte: '›' }));
}

// ouvrirGroupe charge un groupe et montre ses travaux. Il rend faux quand le
// groupe n'existe pas — une adresse peut désigner un groupe retiré depuis.
async function ouvrirGroupe(id, force, sansHistorique) {
  const groupe = await tenter(() => api('GET',
    `/api/classrooms/${encode(id)}${force ? '?refresh=1' : ''}`), 'Groupe');
  if (!groupe) return false;
  // Le filtre et la sélection appartiennent au groupe qu'on regarde : passer à
  // un autre les remet à zéro, plutôt que d'y cacher des étudiants sans qu'on
  // s'y attende.
  const change = !etat.groupe || etat.groupe.scope !== groupe.scope;
  if (change) barreEtudiants.reinitialiser();
  etat.groupe = groupe;
  etat.travail = null;
  etat.etudiants = [];
  // Les équipes appartiennent au groupe : celles du précédent ne disent rien
  // de celui-ci.
  if (change) { etat.equipes = []; etat.orphelins = []; }
  if (groupe.session) etat.parcours = { session: groupe.session, cours: groupe.course };

  $('travaux-nouveau').disabled = !groupe.session;
  dessinerTravaux();
  afficherVue('travaux', sansHistorique);
  return true;
}

// ------------------------------------------ organisations et groupes repérés

// Les organisations du compte ne sont demandées qu'une fois : elles changent
// rarement, et deux écrans s'en servent.
let organisationsConnues = null;

async function organisations() {
  if (!organisationsConnues) {
    const donnees = await tenter(() => api('GET', '/api/orgs'), 'Organisations');
    organisationsConnues = donnees || { orgs: [] };
    if (organisationsConnues.notice) message(organisationsConnues.notice, 'alerte', 12000);
  }
  return organisationsConnues;
}

// remplirSelecteur pose les organisations du compte dans une liste déroulante.
function remplirSelecteur(selecteur, liste, choisie) {
  vider(selecteur);
  for (const acces of liste) {
    selecteur.append(el('option', { value: acces.login, texte: acces.label }));
  }
  selecteur.append(el('option', { value: '__saisir', texte: 'Saisir un autre nom…' }));

  if (choisie && liste.some((acces) => acces.login.toLowerCase() === choisie.toLowerCase())) {
    selecteur.value = choisie;
  } else if (choisie) {
    selecteur.value = '__saisir';
  } else if (liste.length > 0) {
    selecteur.value = liste[0].login;
  } else {
    selecteur.value = '__saisir';
  }
  return selecteur.value;
}

// --- l'écran d'entrée : choisir une organisation

async function demanderOrganisation() {
  const info = await organisations();
  const choisie = remplirSelecteur($('org-choix'), info.orgs || [], etat.reglages.org);
  $('org-libre-bloc').hidden = choisie !== '__saisir';
  $('org-libre').value = etat.reglages.org || '';
  afficherVue('organisation');
}

$('org-choix').addEventListener('change', () => {
  $('org-libre-bloc').hidden = $('org-choix').value !== '__saisir';
});

$('org-valider').addEventListener('click', async () => {
  const saisie = $('org-choix').value === '__saisir'
    ? $('org-libre').value.trim()
    : $('org-choix').value;
  if (!saisie) { message('Indiquez une organisation.', 'alerte'); return; }

  const avis = $('org-avis');
  vider(avis);
  const details = await tenter(() => api('GET', `/api/orgs/${encode(saisie)}`), 'Organisation');
  if (!details) return;
  if (details.warning) {
    avis.append(el('div', { classe: 'avis alerte', texte: details.warning }));
  }
  await retenirOrganisation(details.login);
  etat.parcours = { session: '', cours: '' };
  afficherVue('parcours');
});

// retenirOrganisation fixe l'organisation de la session et la mémorise.
async function retenirOrganisation(org) {
  etat.organisation = org;
  if (etat.reglages.org !== org) {
    etat.reglages.org = org;
    await api('PUT', '/api/settings', etat.reglages).catch(() => {});
  }
}

// --------------------------------------------------- déclaration d'un groupe

function ouvrirImport() {
  preparerImport();
  afficherVue('import');
}

function ouvrirNouveauGroupe() {
  preparerNouveauGroupe();
  afficherVue('nouveau-groupe');
  $('nouveau-session').focus();
}

function preparerNouveauGroupe() {
  etat.nouveau = { etudiants: [], rejets: [] };
  $('nouveau-session').value = etat.parcours.session || '';
  $('nouveau-cours').value = etat.parcours.cours || '';
  $('nouveau-section').value = '';
  $('nouveau-chemin').value = etat.reglages.roster_path || '';
  $('nouveau-texte').value = '';
  $('nouveau-resume').textContent = '';
  vider($('nouveau-rejets'));
  $('nouveau-table').hidden = true;

  // Les sessions déjà connues se proposent à la saisie.
  const suggestions = $('suggestions-sessions');
  vider(suggestions);
  for (const session of etat.sessions) {
    suggestions.append(el('option', { value: session.short, texte: session.name }));
  }
  majApercuPrefixe();
}

for (const id of ['nouveau-session', 'nouveau-cours', 'nouveau-section']) {
  $(id).addEventListener('input', majApercuPrefixe);
}

function majApercuPrefixe() {
  const session = $('nouveau-session').value.trim() || 'session';
  const cours = $('nouveau-cours').value.trim() || 'cours';
  const section = $('nouveau-section').value.trim() || 'groupe';
  $('nouveau-apercu').textContent = `${session}.${cours}.${section}.travail.prenom-nom`;
}

$('nouveau-lire').addEventListener('click', async () => {
  const texte = $('nouveau-texte').value;
  if (!texte.trim()) { message('La zone de texte est vide.', 'alerte'); return; }
  const liste = await tenter(() => api('POST', '/api/roster/parse', { text: texte }), 'Liste');
  if (liste) appliquerListeNouveau(liste);
});

$('nouveau-chemin').addEventListener('change', () => {
  if ($('nouveau-chemin').value.trim()) $('nouveau-charger').click();
});

$('nouveau-charger').addEventListener('click', async () => {
  const chemin = $('nouveau-chemin').value.trim();
  if (!chemin) { message('Indiquez un chemin de fichier.', 'alerte'); return; }
  const liste = await tenter(() => api('POST', '/api/roster/load', { path: chemin }), 'Fichier');
  if (!liste) return;
  $('nouveau-chemin').value = liste.path;
  appliquerListeNouveau(liste);
});

function appliquerListeNouveau(liste) {
  etat.nouveau.etudiants = liste.people || [];
  etat.nouveau.rejets = liste.issues || [];

  $('nouveau-resume').textContent = `${etat.nouveau.etudiants.length} étudiant(s)` +
    (etat.nouveau.rejets.length ? `, ${etat.nouveau.rejets.length} ligne(s) rejetée(s)` : '');
  dessinerRejets($('nouveau-rejets'), etat.nouveau.rejets);

  const table = $('nouveau-table');
  const corps = table.querySelector('tbody');
  vider(corps);
  table.hidden = etat.nouveau.etudiants.length === 0;
  for (const personne of etat.nouveau.etudiants) {
    corps.append(el('tr', {},
      el('td', personne.full_name
        ? { texte: personne.full_name }
        : { classe: 'vide', texte: 'nom à retrouver' }),
      el('td', {}, el('code', { texte: '@' + personne.username }))));
  }
}

function dessinerRejets(conteneur, rejets) {
  vider(conteneur);
  if (!rejets || rejets.length === 0) return;
  const details = el('details', {},
    el('summary', { texte: `${rejets.length} ligne(s) rejetée(s)` }));
  const corps = el('div', { classe: 'corps' });
  for (const rejet of rejets.slice(0, 30)) {
    corps.append(el('div', { classe: 'note',
      texte: (rejet.line > 0 ? `ligne ${rejet.line}` : 'fichier') + ` : ${rejet.message}` }));
  }
  details.append(corps);
  conteneur.append(details);
}

$('nouveau-creer').addEventListener('click', async () => {
  const cree = await tenter(() => api('POST', '/api/classrooms', {
    session: $('nouveau-session').value.trim(),
    course: $('nouveau-cours').value.trim(),
    group: $('nouveau-section').value.trim(),
    prefix: '', pattern: '',
    students: etat.nouveau.etudiants,
    roster_path: $('nouveau-chemin').value.trim(),
    defaults: {},
  }), 'Groupe');
  if (!cree) return;
  message(`Groupe « ${cree.label} » déclaré.`);
  await ouvrirGroupe(cree.scope);
});

// ---------------------------------------------------------------- travaux

function dessinerTravaux() {
  const groupe = etat.groupe;
  const sesTravaux = groupe.assignments || [];
  $('travaux-compte').textContent = travaux(sesTravaux.length);
  $('travaux-source').textContent = groupe.source
    ? `Dépôts de ${groupe.org} — source : ${groupe.source}`
    : '';

  // Une sélection ne survit pas à ce qui a disparu de la liste : on déplace ce
  // qu'on voit.
  const presents = new Set(sesTravaux.map((travail) => travail.id));
  etat.travauxChoisis = new Set([...etat.travauxChoisis].filter((id) => presents.has(id)));

  const conteneur = $('travaux-liste');
  vider(conteneur);
  $('travaux-bandeau').hidden = sesTravaux.length === 0;
  if (sesTravaux.length === 0) {
    conteneur.append(el('div', { classe: 'boite-vide' },
      el('p', { texte: 'Aucun travail dans ce groupe.' }),
      el('p', { classe: 'note',
        texte: '« Nouveau travail » crée un dépôt par étudiant du groupe.' })));
    return;
  }
  const total = gens(groupe).length;
  for (const travail of sesTravaux) {
    const equipe = travail.kind === 'équipe';
    const detail = equipe
      ? [`${travail.teams} équipe(s) ont un dépôt`]
      : [`${travail.students} étudiant(s) du groupe sur ${total}`];
    if (travail.others > 0) detail.push(`${travail.others} dépôt(s) hors liste`);
    const case_ = el('input', {
      type: 'checkbox', checked: etat.travauxChoisis.has(travail.id),
      'aria-label': `Choisir « ${travail.name} »`,
      onchange: (evenement) => {
        if (evenement.target.checked) etat.travauxChoisis.add(travail.id);
        else etat.travauxChoisis.delete(travail.id);
        majSelectionTravaux();
      },
    });
    conteneur.append(el('div', { classe: 'travail-rangee' },
      el('label', { classe: 'case travail-case' }, case_),
      el('button', {
        classe: 'travail-ligne', type: 'button', onclick: () => ouvrirTravail(travail),
      },
        el('span', { classe: 'travail-infos' },
          el('span', { classe: 'titre', texte: travail.name }),
          el('span', { classe: 'detail', texte: detail.join(' · ') })),
        el('span', { classe: 'espace' }),
        equipe ? el('span', { classe: 'jeton', texte: 'équipe' }) : null,
        el('span', {
          classe: 'jeton ' + (!equipe && total > 0 && travail.students >= total ? 'oui' : ''),
          texte: `${travail.repos} dépôt(s)`,
        }),
        el('span', { classe: 'chevron', texte: '›' }))));
  }
  majSelectionTravaux();
}

function majSelectionTravaux() {
  const total = (etat.groupe.assignments || []).length;
  const choisis = etat.travauxChoisis.size;
  $('travaux-selection').textContent = choisis === 0
    ? travaux(total)
    : `${choisis} sur ${total} sélectionné(s)`;
  $('travaux-tout').checked = total > 0 && choisis === total;
  $('travaux-deplacer').disabled = choisis === 0;
}

plageDeCases($('travaux-liste'));

$('travaux-tout').addEventListener('change', (evenement) => {
  etat.travauxChoisis = evenement.target.checked
    ? new Set((etat.groupe.assignments || []).map((travail) => travail.id))
    : new Set();
  for (const case_ of $('travaux-liste').querySelectorAll('input[type="checkbox"]')) {
    case_.checked = evenement.target.checked;
  }
  majSelectionTravaux();
});

$('travaux-deplacer').addEventListener('click', () => {
  const choisis = (etat.groupe.assignments || [])
    .filter((travail) => etat.travauxChoisis.has(travail.id));
  if (choisis.length) deplacerTravaux(choisis);
});

$('travaux-recharger').addEventListener('click', () => ouvrirGroupe(etat.groupe.scope, true));

// ------------------------------------------------------- détail d'un travail

// Le tri et le filtre ne sont pas appliqués ici : l'adresse les transmet, et le
// serveur répond la liste déjà réduite et ordonnée — celui-là même qui répond
// la liste des étudiants, avec les mêmes critères.
function adresseTravail(nom, force) {
  const critere = etat.filtreTravail;
  const parametres = new URLSearchParams();
  if (critere.texte) parametres.set('q', critere.texte);
  if (critere.activite) parametres.set('activity', critere.activite);
  if (critere.apres) parametres.set('after', critere.apres);
  if (critere.avant) parametres.set('before', critere.avant);
  if (critere.tri !== 'nom') parametres.set('sort', critere.tri);
  if (critere.desc) parametres.set('desc', '1');
  if (force) parametres.set('refresh', '1');
  const suite = parametres.toString();
  return `/api/classrooms/${encode(etat.groupe.scope)}/assignments/${encode(nom)}` +
    (suite ? '?' + suite : '');
}

async function ouvrirTravail(travail, force, sansHistorique) {
  // Les critères appartiennent au travail qu'on regarde : en ouvrir un autre
  // repart de zéro, mais revenir au même — après une suppression, après une
  // distribution — garde ce qui était posé.
  const meme = etat.travailRegle === travail.id;
  if (!meme) {
    barreTravail.reinitialiser();
    etat.acces.clear();
    etat.selection = new Set();
    etat.travailRegle = travail.id;
  }
  if (!await chargerTravail(travail, force, !meme)) return;
  afficherVue('travail', sansHistorique);
}

async function chargerTravail(travail, force, toutCocher) {
  const attente = attendreTable('detail-table', 'detail-vide', 'Chargement des dépôts…');
  const detail = await tenter(() => api('GET', adresseTravail(travail.name, force)), 'Travail');
  if (!attente.fini(detail, "Les dépôts n'ont pas pu être chargés.")) return false;

  // « repos » compte les dépôts dans la fiche du travail et les énumère dans le
  // détail : on ne garde que la liste, sous un nom qui ne prête pas à confusion.
  // « names » les nomme tous, filtrés compris : ce qu'on cache à l'écran ne sort
  // pas du travail pour autant.
  etat.travail = {
    id: travail.id, name: travail.name, depots: detail.repos,
    total: detail.total, noms: detail.names || [],
    // La nature vient du serveur, qui la lit dans le nom des dépôts : elle
    // n'est déclarée nulle part, et la fiche du groupe peut être plus vieille.
    kind: detail.kind || travail.kind,
  };
  // Une sélection ne survit pas à ce que le filtre écarte : on agit sur ce
  // qu'on voit, et rien d'autre.
  const visibles = new Set(detail.repos.map((repo) => repo.name));
  etat.selection = toutCocher
    ? visibles
    : new Set([...etat.selection].filter((nom) => visibles.has(nom)));
  $('detail-titre').textContent = travail.name;
  dessinerTravail();
  return true;
}

const rechargerTravail = () => {
  if (etat.groupe && etat.travail) chargerTravail(etat.travail, false, false);
};

// Un travail n'a qu'un dépôt par personne : « avec » et « sans dépôt » n'y
// diraient rien, et le menu ne garde que « jamais d'envoi » et les bornes du
// dernier envoi.
const barreTravail = barreDeFiltre({
  table: 'detail-table',
  texte: 'detail-texte',
  ouvrir: 'detail-filtre-ouvrir',
  menu: 'detail-filtre-menu',
  vider: 'detail-filtre-vider',
  champs: [
    ['detail-filtre-activite', 'activite'],
    ['detail-filtre-apres', 'apres'], ['detail-filtre-avant', 'avant'],
  ],
  criteres: () => etat.filtreTravail,
  effacer: () => {
    etat.filtreTravail = {
      texte: '', activite: '', apres: '', avant: '', tri: 'nom', desc: false,
    };
  },
  recharger: () => rechargerTravail(),
});

function dessinerTravail() {
  const depots = etat.travail.depots;
  const total = gens(etat.groupe).length;
  const equipe = etat.travail.kind === 'équipe';
  // Le serveur a déjà rattaché chaque dépôt à son destinataire : la page n'a
  // plus à deviner qui se cache derrière un nom.
  const servis = depots.filter((repo) => repo.username || repo.team).length;
  // Sous filtre, le résumé dit sur combien : sans cela, une liste courte ne
  // distinguerait pas un travail peu distribué d'un critère trop étroit.
  const parts = equipe
    ? [`${depots.length} dépôt(s) d'équipe` +
       (depots.length === etat.travail.total ? '' : ` sur ${etat.travail.total}`)]
    : (depots.length === etat.travail.total
      ? [`${depots.length} dépôt(s)`,
         `${servis} étudiant(s) du groupe sur ${total} en ont un`]
      : [`${depots.length} dépôt(s) sur ${etat.travail.total}`,
         `${servis} étudiant(s) du groupe`]);
  if (depots.length - servis > 0) parts.push(`${depots.length - servis} hors liste`);
  $('detail-resume').textContent = parts.join(' · ');
  $('detail-colonne').textContent = equipe ? 'Équipe' : 'Étudiant';
  // Repartager n'a de sens que pour un travail d'équipe : c'est ce qui achève
  // l'adoption d'un travail fait en équipe avant l'outil.
  $('detail-partager').hidden = !equipe;

  $('detail-table').hidden = depots.length === 0;
  $('detail-vide').hidden = depots.length > 0;
  $('detail-vide').textContent = 'Aucun dépôt ne répond à ces critères.';

  const corps = $('detail-table').querySelector('tbody');
  vider(corps);
  for (const repo of depots) {
    const acces = etat.acces.get(repo.name);
    corps.append(el('tr', {},
      el('td', {}, el('input', {
        type: 'checkbox',
        checked: etat.selection.has(repo.name),
        onchange: (evenement) => {
          if (evenement.target.checked) etat.selection.add(repo.name);
          else etat.selection.delete(repo.name);
          majSelection();
        },
      })),
      // « hors liste » dit que le dépôt n'est rattaché à personne, pas que son
      // nom complet manque : un dépôt nommé par le compte GitHub d'un inscrit
      // lui appartient, et le dire autrement ferait croire à un intrus. Un
      // dépôt d'équipe, lui, nomme l'équipe et ses membres : il n'appartient à
      // personne en particulier, et c'est normal.
      el('td', {}, repo.team
        ? el('div', {},
            el('strong', { texte: repo.full_name }),
            // Les membres se lisent par leur nom et par leur compte : un dépôt
            // d'équipe ne porte celui de personne, et c'est leur association
            // qu'on vient vérifier ici.
            (repo.members || []).length === 0
              ? el('span', { classe: 'note', texte: ' — équipe vide' })
              : el('div', { classe: 'equipe-membres' },
                  (repo.members || []).map((personne) => ligneDeMembre(personne, {
                    invite: (repo.waiting || []).some((compte) =>
                      compte.toLowerCase() === personne.username.toLowerCase()),
                  }))))
        : (repo.username
            ? lienVersLaFiche(repo)
            : el('span', { classe: 'vide', texte: repo.student + ' (hors liste)' }))),
      el('td', {}, el('a', {
        href: repo.url, target: '_blank', rel: 'noreferrer noopener', texte: repo.name,
      })),
      el('td', {}, el('span', { classe: 'jeton', texte: repo.visibility })),
      el('td', repo.pushed_at ? { texte: repo.pushed_at } : { classe: 'vide', texte: 'jamais' }),
      el('td', { texte: acces ? resumerAcces(acces) : '—' }),
      el('td', { classe: 'etroit' }, el('span', { classe: 'actions' },
        el('button', {
          type: 'button', classe: 'lien icone',
          title: 'Accès', 'aria-label': `Accès de ${repo.name}`,
          onclick: () => panneauAcces(repo),
        }, icone('cle')),
        el('button', {
          type: 'button', classe: 'lien icone rouge',
          title: 'Supprimer…', 'aria-label': `Supprimer ${repo.name}`,
          onclick: () => supprimerDepot(repo),
        }, icone('corbeille')))),
    ));
  }
  barreTravail.maj();
  majSelection();
}

function resumerAcces(acces) {
  const parts = [];
  if (acces.collaborators.length) parts.push(`${acces.collaborators.length} collab.`);
  if (acces.invitations.length) parts.push(`${acces.invitations.length} invit.`);
  return parts.length ? parts.join(' · ') : 'aucun';
}

function majSelection() {
  const total = etat.travail ? etat.travail.depots.length : 0;
  const choisis = etat.selection.size;
  // Même formulation que la liste des étudiants : les deux bandeaux disent la
  // même chose, et celui-ci porte assez de commandes pour ne pas s'allonger.
  $('detail-selection').textContent = choisis === 0
    ? `${total} dépôt(s) affiché(s)`
    : `${choisis} sur ${total} sélectionné(s)`;
  $('detail-tout').checked = total > 0 && choisis === total;
}

plageDeCases($('detail-table').querySelector('tbody'));

$('detail-tout').addEventListener('change', (evenement) => {
  etat.selection = evenement.target.checked
    ? new Set(etat.travail.depots.map((repo) => repo.name))
    : new Set();
  dessinerTravail();
});

function selectionnes() {
  return etat.travail.depots.filter((repo) => etat.selection.has(repo.name));
}

// --- ce qu'on fait au travail entier

// Les commandes du travail entier vivent dans un menu : elles sont rares, et la
// barre garde ainsi la seule qu'on vient y chercher — distribuer aux manquants.
const menuTravail = menuDeroulant('detail-menu-ouvrir', 'detail-menu');

// Le travail que la page montre, tel que le groupe le connaît : c'est la fiche
// que les commandes attendent, pas le détail chargé pour l'affichage.
function travailOuvert() {
  return (etat.groupe.assignments || []).find((item) => item.id === etat.travail.id);
}

// Chaque commande referme le menu avant d'agir : le dialogue qui suit se
// passerait mal d'un menu resté ouvert derrière lui.
function commandeDuTravail(bouton, action) {
  $(bouton).addEventListener('click', () => {
    menuTravail.deplier(false);
    const travail = travailOuvert();
    if (travail) action(travail);
  });
}

// Depuis la page d'un travail, c'est celui qu'on regarde qu'on déplace ou qu'on
// renomme : il n'y a pas à retourner à la liste pour le cocher.
commandeDuTravail('detail-deplacer', (travail) => deplacerTravaux([travail]));
commandeDuTravail('detail-renommer', (travail) => renommerTravail(travail));

$('detail-acces').addEventListener('click', async () => {
  menuTravail.deplier(false);
  const fiche = await tenter(() => api('POST',
    `/api/classrooms/${encode(etat.groupe.scope)}/assignments/${encode(etat.travail.name)}/access`),
    'Accès');
  if (!fiche) return;
  const resultats = await suivre(fiche);
  if (!Array.isArray(resultats)) return;
  for (const acces of resultats) etat.acces.set(acces.repo, acces);
  dessinerTravail();
});

// --- accès d'un dépôt

async function panneauAcces(repo) {
  const acces = await tenter(() =>
    api('GET', `/api/orgs/${encode(etat.groupe.org)}/repos/${encode(repo.name)}/access`), 'Accès');
  if (!acces) return;
  etat.acces.set(repo.name, acces);
  dessinerTravail();

  const liste = el('div', {});
  const redessiner = () => {
    vider(liste);
    const courant = etat.acces.get(repo.name);
    if (!courant.collaborators.length && !courant.invitations.length) {
      liste.append(el('p', { classe: 'note', texte: 'Aucun collaborateur direct, aucune invitation.' }));
    }
    for (const login of courant.collaborators) {
      liste.append(ligneAcces(repo, login, 'collaborateur', async () => {
        await api('DELETE',
          `/api/orgs/${encode(etat.groupe.org)}/repos/${encode(repo.name)}/collaborators/${encode(login)}`);
      }, redessiner));
    }
    for (const invitation of courant.invitations) {
      liste.append(ligneAcces(repo, invitation.login, 'invitation en attente', async () => {
        await api('DELETE',
          `/api/orgs/${encode(etat.groupe.org)}/repos/${encode(repo.name)}/invitations/${invitation.id}`);
      }, redessiner));
    }
  };
  redessiner();

  const compte = el('input', { type: 'text', classe: 'champ', placeholder: 'compte GitHub' });
  const droit = el('select', { classe: 'champ' }, etat.contexte.permissions.map(
    (option) => el('option', { value: option.value, texte: option.label })));
  droit.value = (etat.groupe.defaults && etat.groupe.defaults.permission) || 'push';
  const ajout = el('div', { classe: 'ligne-champ' }, compte, droit,
    el('button', {
      type: 'button', classe: 'bouton vert', texte: 'Inviter',
      onclick: async () => {
        const succes = await tenter(() => api('POST',
          `/api/orgs/${encode(etat.groupe.org)}/repos/${encode(repo.name)}/collaborators`,
          { username: compte.value.trim(), permission: droit.value }), 'Invitation');
        if (!succes) return;
        message(`@${succes.username} : ${succes.label} (${succes.permission}).`);
        compte.value = '';
        await rafraichirAcces(repo);
        redessiner();
      },
    }));

  await demander(`Accès de « ${repo.name} »`,
    el('div', {}, liste, el('h3', { texte: 'Ajouter un accès' }), ajout), 'Fermer');
}

function ligneAcces(repo, login, role, retirer, redessiner) {
  return el('div', { classe: 'ligne-champ' },
    el('span', { classe: 'espace', texte: `@${login} — ${role}` }),
    el('button', {
      type: 'button', classe: 'bouton petit', texte: 'Retirer',
      onclick: async () => {
        const fait = await tenter(retirer, 'Retrait');
        if (!fait) return;
        message(fait.message || 'Accès retiré.');
        await rafraichirAcces(repo);
        redessiner();
      },
    }));
}

async function rafraichirAcces(repo) {
  const acces = await tenter(() =>
    api('GET', `/api/orgs/${encode(etat.groupe.org)}/repos/${encode(repo.name)}/access`), 'Accès');
  if (acces) {
    etat.acces.set(repo.name, acces);
    dessinerTravail();
  }
}

// --- suppression

async function supprimerDepot(repo) {
  const saisie = el('input', { type: 'text', classe: 'champ', placeholder: repo.name });
  const confirme = await demander(`Supprimer « ${repo.name} » ?`, el('div', {},
    el('p', { classe: 'avis erreur', texte: 'Suppression définitive : le contenu, les tickets et ' +
      "l'historique seront perdus." }),
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: `Retapez « ${repo.name} » pour confirmer` }), saisie)),
    'Supprimer');
  if (!confirme) return;

  const fait = await tenter(() => api('DELETE',
    `/api/orgs/${encode(etat.groupe.org)}/repos/${encode(repo.name)}`,
    { confirm: saisie.value.trim() }), 'Suppression');
  if (!fait) return;
  message(fait.message);
  const travail = etat.travail;
  await ouvrirGroupe(etat.groupe.scope, true);
  const encore = (etat.groupe.assignments || []).find((item) => item.id === travail.id);
  if (encore) await ouvrirTravail(encore, true);
}

// --- URL, clonage, mise à jour

$('detail-copier').addEventListener('click', async () => {
  const urls = selectionnes().map((repo) => repo.url).join('\n');
  if (!urls) { message('Aucun dépôt sélectionné.', 'alerte'); return; }
  try {
    await navigator.clipboard.writeText(urls);
    message(`${etat.selection.size} URL copiée(s).`);
  } catch {
    message("Copie refusée par le navigateur : utilisez l'export CSV.", 'alerte');
  }
});

$('detail-csv').addEventListener('click', () => {
  const choisis = selectionnes();
  if (!choisis.length) { message('Aucun dépôt sélectionné.', 'alerte'); return; }
  const lignes = [['nom_complet', 'depot', 'url']];
  for (const repo of choisis) {
    lignes.push([repo.full_name || repo.student, repo.name, repo.url]);
  }
  const contenu = lignes.map((ligne) =>
    ligne.map((valeur) => `"${String(valeur).replace(/"/g, '""')}"`).join(',')).join('\n');
  telecharger(`${etat.travail.id}-urls.csv`, contenu, 'text/csv');
});

function telecharger(nom, contenu, type) {
  const adresse = URL.createObjectURL(new Blob([contenu], { type: `${type};charset=utf-8` }));
  const lien = el('a', { href: adresse, download: nom });
  document.body.append(lien);
  lien.click();
  lien.remove();
  URL.revokeObjectURL(adresse);
}

$('detail-cloner').addEventListener('click', async () => {
  const choisis = selectionnes();
  if (!choisis.length) { message('Aucun dépôt sélectionné.', 'alerte'); return; }

  const parent = etat.reglages.clone_dir || '.';
  const { zone, champ: destination } = zoneDepot({
    dossier: true, titre: 'Choisir où cloner',
    valeur: `${parent.replace(/[\\/]+$/, '')}/${etat.travail.id}`,
  });
  const confirme = await demander(`Cloner ${choisis.length} dépôt(s)`, el('div', {},
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: 'Dossier de destination' }), zone),
    el('p', { classe: 'note', texte: `${etat.contexte.jobs} clonage(s) en parallèle` +
      (etat.contexte.depth ? `, profondeur ${etat.contexte.depth}` : '') })), 'Cloner');
  if (!confirme) return;

  const fiche = await tenter(() => api('POST', '/api/clones/clone', {
    org: etat.groupe.org,
    names: choisis.map((repo) => repo.name),
    destination: destination.value.trim(),
  }), 'Clonage');
  if (!fiche) return;
  const bilan = await suivre(fiche);
  if (bilan && bilan.destination) {
    journaliser(`${bilan.cloned} cloné(s) · ${bilan.updated} mis à jour · ` +
      `${bilan.skipped} ignoré(s) · ${bilan.failed} en échec`, 'dim');
    etat.reglages.clone_dir = bilan.destination.replace(/[\\/][^\\/]+$/, '');
  }
});

$('detail-pull').addEventListener('click', async () => {
  const parent = etat.reglages.clone_dir || '.';
  const { zone, champ: dossier } = zoneDepot({
    dossier: true, titre: 'Choisir le dossier des clones',
    valeur: `${parent.replace(/[\\/]+$/, '')}/${etat.travail.id}`,
  });
  const trouve = await demander('Mettre à jour des clones', el('div', {},
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: 'Dossier contenant les clones' }), zone)),
    'Chercher');
  if (!trouve) return;

  const liste = await tenter(() =>
    api('POST', '/api/clones/find', { directory: dossier.value.trim() }), 'Clones');
  if (!liste) return;

  const cases = liste.clones.map((item) => {
    const coche = el('input', { type: 'checkbox', checked: true, value: item.name });
    const horsTravail = !etat.travail.noms.includes(item.name);
    return el('label', { classe: 'case' }, coche,
      el('span', { texte: item.name + (horsTravail ? '   (hors travail)' : '') }));
  });
  const confirme = await demander(`${liste.clones.length} clone(s) trouvé(s)`,
    plageDeCases(el('div', {}, cases)), 'Mettre à jour');
  if (!confirme) return;

  const noms = cases
    .filter((ligne) => ligne.querySelector('input').checked)
    .map((ligne) => ligne.querySelector('input').value);
  if (!noms.length) { message('Aucun clone sélectionné.', 'alerte'); return; }

  const fiche = await tenter(() => api('POST', '/api/clones/pull',
    { directory: liste.directory, names: noms }), 'Mise à jour');
  if (!fiche) return;
  const bilan = await suivre(fiche);
  if (bilan) journaliser(`${bilan.updated} mis à jour · ${bilan.failed} en échec`, 'dim');
});

// ------------------------------------------------------------------ assistant

$('travaux-nouveau').addEventListener('click', () => {
  ouvrirAssistant('', 'Nouveau travail');
});

// « Distribuer aux manquants » reprend le travail ouvert : mêmes réglages, et
// seuls les étudiants sans dépôt sont cochés.
$('detail-distribuer').addEventListener('click', () => {
  ouvrirAssistant(etat.travail.name, `Distribuer « ${etat.travail.name} »`, 3,
    etat.travail.kind === 'équipe' ? 'equipe' : 'individuel');
});

// Redonner à chaque équipe l'accès au dépôt qui porte son nom : c'est la
// dernière étape de l'adoption d'un travail fait en équipe avant l'outil, où
// les dépôts sont déjà là mais n'ont jamais été partagés.
$('detail-partager').addEventListener('click', async () => {
  const corps = el('div', {},
    el('p', { texte: `Partager les dépôts de « ${etat.travail.name} » avec leurs équipes ?` }),
    el('p', { classe: 'note', texte:
      "Chaque dépôt nommé d'après une équipe du groupe lui est accordé, au droit " +
      "réglé pour le groupe. Un dépôt déjà partagé ne change pas." }));
  if (!await demander('Partager avec les équipes', corps, 'Partager')) return;

  const fiche = await tenter(() => api('POST',
    `/api/classrooms/${encode(etat.groupe.scope)}/assignments/${encode(etat.travail.name)}/share`),
    'Partage');
  if (!fiche) return;
  const bilan = await suivre(fiche);
  if (!bilan) return;
  journaliser(`${bilan.shared} dépôt(s) partagé(s) · ${bilan.failed} en échec`,
    bilan.failed ? 'warn' : 'ok');
});

async function ouvrirAssistant(nom, titre, etape = 1, nature) {
  if (nature === 'equipe' || (etat.groupe.teams > 0 && etat.equipes.length === 0)) {
    await assurerEquipes();
  }
  etat.reglagesTravail = Object.assign({}, etat.groupe.defaults);
  etat.assistantTitre = titre;
  $('travail-nom').value = nom;
  // Un travail rouvert garde sa nature : on ne redistribue pas en équipe un
  // travail qui a été individuel, et l'inverse encore moins.
  etat.nature = nature || 'individuel';
  $('nature-equipe').checked = etat.nature === 'equipe';
  $('nature-individuel').checked = etat.nature !== 'equipe';
  ecrireReglagesTravail();
  dessinerDestinataires();
  afficherEtape(etape);
  afficherVue('assistant');
  if (etape === 1) $('travail-nom').focus();
}

// La nature du travail décide de qui reçoit un dépôt. Elle se choisit ici et
// nulle part ailleurs : une fois distribué, c'est le nom des dépôts qui la dit.
for (const id of ['nature-individuel', 'nature-equipe']) {
  $(id).addEventListener('change', async () => {
    etat.nature = $('nature-equipe').checked ? 'equipe' : 'individuel';
    if (enEquipe()) await assurerEquipes();
    majApercuDuNom();
    dessinerDestinataires();
    planifierApercu();
  });
}

// enEquipe dit si l'assistant prépare un travail d'équipe.
function enEquipe() { return etat.nature === 'equipe'; }

// assurerEquipes charge les équipes du groupe si l'on ne les a pas encore vues.
// L'assistant peut être ouvert sans être jamais passé par l'onglet Équipes.
async function assurerEquipes() {
  if (etat.equipes.length > 0 || !etat.groupe || !etat.groupe.session) return;
  const donnees = await tenter(() => api('GET',
    `/api/classrooms/${encode(etat.groupe.scope)}/teams`), 'Équipes');
  if (!donnees) return;
  etat.equipes = donnees.teams || [];
  etat.orphelins = donnees.unassigned || [];
}

for (const bouton of document.querySelectorAll('[data-continuer]')) {
  bouton.addEventListener('click', () => afficherEtape(Number(bouton.dataset.continuer)));
}

function afficherEtape(numero) {
  etat.etape = numero;
  for (const boite of document.querySelectorAll('.etape')) {
    boite.hidden = Number(boite.dataset.etape) !== numero;
  }
  for (const item of $('etapes').children) {
    const rang = Number(item.dataset.etape);
    item.classList.toggle('active', rang === numero);
    item.classList.toggle('faite', rang < numero);
  }
  if (numero === 3) planifierApercu();
  window.scrollTo(0, 0);
}

// --- réglages du travail

const champsTravail = {
  description_pattern: 'reglage-description',
  template: 'reglage-template',
  permission: 'reglage-permission',
  commit_message: 'reglage-commit',
  starter_dir: 'reglage-starter',
};

function ecrireReglagesTravail() {
  for (const [cle, id] of Object.entries(champsTravail)) {
    $(id).value = etat.reglagesTravail[cle] || '';
  }
  const publique = etat.reglagesTravail.visibility === 'public';
  $('visibilite-publique').checked = publique;
  $('visibilite-privee').checked = !publique;
  $('reglage-collaborateur').checked = etat.reglagesTravail.add_collaborator !== false;
  vider($('starter-resume'));
  majApercuDuNom();
}

function lireReglagesTravail() {
  for (const [cle, id] of Object.entries(champsTravail)) {
    etat.reglagesTravail[cle] = $(id).value.trim();
  }
  etat.reglagesTravail.visibility = $('visibilite-publique').checked ? 'public' : 'private';
  etat.reglagesTravail.add_collaborator = $('reglage-collaborateur').checked;
  return etat.reglagesTravail;
}

for (const id of Object.values(champsTravail)) {
  $(id).addEventListener('change', () => { lireReglagesTravail(); planifierApercu(); });
}
for (const id of ['visibilite-privee', 'visibilite-publique', 'reglage-collaborateur']) {
  $(id).addEventListener('change', lireReglagesTravail);
}

$('travail-nom').addEventListener('input', majApercuDuNom);
$('travail-nom').addEventListener('change', planifierApercu);

// Le nom des dépôts se lit avant qu'ils existent. Il n'est plus réglable : les
// cinq niveaux sont la nomenclature elle-même.
function majApercuDuNom() {
  const groupe = etat.groupe || {};
  const portee = groupe.session
    ? `${groupe.session}.${groupe.course}.${groupe.group}`
    : (groupe.prefix || 'session.cours.groupe');
  const travail = $('travail-nom').value.trim() || 'travail';
  // Le dernier niveau nomme le destinataire : l'étudiant, ou l'équipe. C'est
  // par lui qu'on relit ensuite la nature du travail.
  const dernier = enEquipe() ? (etat.equipes[0] ? etat.equipes[0].short : 'eq1') : 'prenom-nom';
  $('apercu-nom').textContent = `${portee}.${travail}.${dernier}`;
  $('apercu-suite').textContent = enEquipe()
    ? " — le nom de l'équipe, et le dépôt lui est partagé."
    : " — le nom de l'étudiant, pas son compte GitHub.";
}

// --- destinataires

// destinatairesPossibles rend ceux à qui le travail peut être distribué : les
// étudiants du groupe, ou ses équipes. Chacun n'y est que par son identifiant —
// un compte GitHub ou un nom court d'équipe —, si bien que la suite ne fait
// plus de différence entre les deux.
function destinatairesPossibles() {
  if (enEquipe()) {
    return etat.equipes.map((equipe) => ({
      cle: equipe.short,
      titre: equipe.short,
      detail: (equipe.members || []).length === 0
        ? 'équipe vide'
        : (equipe.members || []).map((compte) => '@' + compte).join(', '),
    }));
  }
  return gens(etat.groupe).map((personne) => ({
    cle: personne.username,
    titre: personne.full_name || '',
    detail: '@' + personne.username,
  }));
}

function dessinerDestinataires() {
  const possibles = destinatairesPossibles();
  etat.destinataires = new Set(possibles.map((item) => item.cle));

  // L'accès d'un travail d'équipe est accordé à l'équipe, pas à ses membres :
  // le libellé doit dire ce qui se passera vraiment.
  $('acces-titre').textContent = enEquipe()
    ? "Partager le dépôt avec son équipe"
    : "Inviter chaque étudiant sur son dépôt";
  $('acces-aide').textContent = enEquipe()
    ? "L'accès va à l'équipe entière : changer sa composition suffit ensuite à " +
      "changer qui voit le dépôt."
    : "Une invitation part vers le compte GitHub de la personne.";

  $('dest-intro').textContent = enEquipe()
    ? "Le travail est distribué aux équipes du groupe : un dépôt par équipe, " +
      "partagé avec elle. Rien n'oblige à toutes les servir maintenant."
    : 'Le travail est distribué aux étudiants du groupe : un dépôt chacun.';
  $('plan-colonne').textContent = enEquipe() ? 'Équipe' : 'Étudiant';

  const conteneur = $('dest-liste');
  vider(conteneur);
  if (possibles.length === 0) {
    conteneur.append(el('p', { classe: 'note', texte: enEquipe()
      ? "Ce groupe n'a aucune équipe : créez-en dans l'onglet Équipes."
      : "Ce groupe n'a aucun étudiant : importez sa liste dans l'onglet Étudiants." }));
  }
  for (const item of possibles) {
    const coche = el('input', {
      type: 'checkbox', checked: true, value: item.cle,
      onchange: (evenement) => {
        if (evenement.target.checked) etat.destinataires.add(item.cle);
        else etat.destinataires.delete(item.cle);
        majDestinataires();
        planifierApercu();
      },
    });
    conteneur.append(el('label', { classe: 'case' }, coche,
      el('span', {},
        item.titre ? item.titre + ' ' : '',
        el('span', { classe: 'compte', texte: item.detail }))));
  }
  majDestinataires();
}

function majDestinataires() {
  const total = destinatairesPossibles().length;
  const mot = enEquipe() ? 'équipe(s)' : 'étudiant(s)';
  $('dest-compte').textContent = `${etat.destinataires.size} ${mot} sur ${total}`;
  $('dest-tout').checked = total > 0 && etat.destinataires.size === total;
  for (const coche of $('dest-liste').querySelectorAll('input')) {
    coche.checked = etat.destinataires.has(coche.value);
  }
}

plageDeCases($('dest-liste'));

$('dest-tout').addEventListener('change', (evenement) => {
  etat.destinataires = evenement.target.checked
    ? new Set(destinatairesPossibles().map((item) => item.cle))
    : new Set();
  majDestinataires();
  planifierApercu();
});

// --- modèle et fichiers de départ

$('template-verifier').addEventListener('click', async () => {
  const reference = $('reglage-template').value.trim();
  if (!reference) { message('Aucun modèle : les dépôts seront créés neufs.'); return; }
  const bilan = await tenter(() =>
    api('POST', '/api/template/check', { template: reference }), 'Modèle');
  if (!bilan) return;
  $('reglage-template').value = bilan.template;
  lireReglagesTravail();
  if (bilan.warning) message(bilan.warning, 'alerte', 12000);
  else message(`Modèle vérifié : ${bilan.template}.`);
});

$('starter-inspecter').addEventListener('click', async () => {
  const chemin = $('reglage-starter').value.trim();
  const resume = $('starter-resume');
  vider(resume);
  if (!chemin) { lireReglagesTravail(); return; }

  const bundle = await tenter(() =>
    api('POST', '/api/starter/inspect', { path: chemin }), 'Fichiers de départ');
  if (!bundle) return;
  $('reglage-starter').value = bundle.root;
  lireReglagesTravail();

  resume.append(el('div', { classe: 'avis', texte: `${bundle.summary} depuis ${bundle.root}` }));
  if (bundle.warning) resume.append(el('div', { classe: 'avis alerte', texte: bundle.warning }));
  if (bundle.large) {
    resume.append(el('div', { classe: 'avis alerte',
      texte: "Envoi volumineux : un fichier par appel d'API. Un dépôt modèle serait plus rapide." }));
  }
  const details = el('details', {}, el('summary', { texte: `${bundle.files.length} fichier(s)` }));
  const corps = el('div', { classe: 'corps' });
  for (const fichier of bundle.files) {
    corps.append(el('div', { classe: 'note', texte: `${fichier.path} — ${fichier.label}` }));
  }
  for (const ecarte of bundle.skipped.slice(0, 10)) {
    corps.append(el('div', { classe: 'note', texte: `écarté : ${ecarte.path} (${ecarte.reason})` }));
  }
  details.append(corps);
  resume.append(details);
});

// --- aperçu

let minuterieApercu = null;

function planifierApercu() {
  clearTimeout(minuterieApercu);
  minuterieApercu = setTimeout(rafraichirApercu, 350);
}

function corpsDuTravail() {
  const corps = {
    name: $('travail-nom').value.trim(),
    settings: lireReglagesTravail(),
    teams: enEquipe(),
  };
  // Un travail d'équipe ne restreint pas par compte mais par équipe : envoyer
  // les deux listes laisserait le serveur choisir, et il ne doit pas avoir à le
  // faire.
  if (enEquipe()) corps.team_names = [...etat.destinataires];
  else corps.usernames = [...etat.destinataires];
  return corps;
}

async function rafraichirApercu() {
  const erreur = $('plan-erreur');
  vider(erreur);
  vider($('deja-servis'));
  const table = $('plan-table');
  const corps = table.querySelector('tbody');
  vider(corps);
  table.hidden = true;
  $('plan-resume').textContent = '';

  if (!etat.groupe || !$('travail-nom').value.trim()) return;
  try {
    const apercu = await api('POST',
      `/api/classrooms/${encode(etat.groupe.scope)}/assignments/preview`, corpsDuTravail());
    for (const item of apercu.items) {
      corps.append(el('tr', {},
        el('td', {}, el('code', { texte: item.name })),
        el('td', { texte: item.full_name || '—' }),
        el('td', {}, item.username
          ? el('code', { texte: '@' + item.username })
          : el('span', { classe: 'vide', texte: '—' })),
        el('td', { classe: 'note', texte: item.description })));
    }
    table.hidden = apercu.items.length === 0;
    $('plan-resume').textContent =
      `${apercu.items.length} dépôt(s) à créer dans « ${etat.groupe.org} » — travail « ${apercu.assignment} »`;

    if (apercu.served && apercu.served.length) {
      // « served » porte des comptes pour un travail individuel, des noms
      // d'équipes pour un travail d'équipe.
      const noms = apercu.served.map((item) =>
        typeof item === 'string' ? item : '@' + item.username);
      $('deja-servis').append(el('div', { classe: 'avis',
        texte: `${noms.length} ${apercu.teams ? 'équipe(s)' : 'étudiant(s)'} ont déjà ` +
          `un dépôt pour ce travail : ` + noms.join(', ') +
          ". Ils sont écartés de la distribution." }));
    }
  } catch (probleme) {
    erreur.append(el('div', { classe: 'avis erreur', texte: probleme.message }));
  }
}

// --- distribution

$('lancer-simulation').addEventListener('click', () => distribuer(true));
$('lancer-creation').addEventListener('click', () => distribuer(false));

async function distribuer(simulation) {
  const corps = corpsDuTravail();
  const retenus = corps.teams ? corps.team_names : corps.usernames;
  if (!corps.name) { message('Donnez un nom au travail.', 'alerte'); return; }
  if (retenus.length === 0) {
    message(corps.teams ? 'Aucune équipe retenue.' : 'Aucun étudiant retenu.', 'alerte');
    return;
  }

  if (!simulation) {
    // L'accès d'un travail d'équipe est accordé à l'équipe, pas à ses membres :
    // le dire ici évite de chercher ensuite pourquoi personne n'a été invité.
    const acces = corps.teams
      ? `Partage avec chaque équipe (${corps.settings.permission}).`
      : (corps.settings.add_collaborator
        ? `Invitations : oui (${corps.settings.permission}).` : 'Invitations : non.');
    const confirme = await demander('Confirmer la distribution', el('div', {},
      el('p', { texte: `${retenus.length} ${corps.teams ? 'équipe(s)' : 'étudiant(s)'} ` +
        `du groupe « ${etat.groupe.label} », travail « ${corps.name} ».` }),
      el('p', { classe: 'note', texte:
        `Visibilité : ${corps.settings.visibility === 'public' ? 'public' : 'privé'}. ` +
        acces +
        (corps.settings.template ? ` Modèle : ${corps.settings.template}.` : ' Dépôts neufs.') })),
      'Distribuer');
    if (!confirme) return;
  }

  const fiche = await tenter(() => api('POST',
    `/api/classrooms/${encode(etat.groupe.scope)}/assignments`,
    Object.assign({}, corps, {
      dry_run: simulation,
      force_starter: $('creer-force').checked,
    })), simulation ? 'Simulation' : 'Distribution');
  if (!fiche) return;

  const bilan = await suivre(fiche);
  if (!bilan || !bilan.report) return;

  journaliser(`${bilan.created} ${simulation ? 'à créer' : 'créé(s)'} · ` +
    `${bilan.existing} déjà présent(s) · ${bilan.failed} en échec`, bilan.failed ? 'warn' : 'ok');
  if (bilan.skipped && bilan.skipped.length) {
    journaliser(`${bilan.skipped.length} ${bilan.teams ? 'équipe(s)' : 'étudiant(s)'} ` +
      'avaient déjà un dépôt.', 'dim');
  }
  if (bilan.json_path) journaliser(`Bilan : ${bilan.json_path}`, 'dim');
  if (simulation) return;

  // Une fois distribué, le travail s'ouvre — comme Classroom mène à la page du
  // devoir une fois créé.
  await ouvrirGroupe(etat.groupe.scope, true);
  const travail = (etat.groupe.assignments || []).find((item) => item.id === bilan.assignment);
  if (travail) await ouvrirTravail(travail, true);
}

// ------------------------------------------------------------------ étudiants

// Le tri et le filtre ne sont pas appliqués ici : l'adresse les transmet, et le
// serveur répond la liste déjà réduite et ordonnée. C'est ce qui fait que
// « dernier envoi avant le 1er octobre » veut dire la même chose au navigateur
// et au terminal.
function adresseEtudiants(force) {
  const filtre = etat.filtre;
  const parametres = new URLSearchParams();
  if (filtre.texte) parametres.set('q', filtre.texte);
  if (filtre.travail) parametres.set('assignment', filtre.travail);
  if (filtre.activite) parametres.set('activity', filtre.activite);
  if (filtre.apres) parametres.set('after', filtre.apres);
  if (filtre.avant) parametres.set('before', filtre.avant);
  if (filtre.tri !== 'nom') parametres.set('sort', filtre.tri);
  if (filtre.desc) parametres.set('desc', '1');
  if (force) parametres.set('refresh', '1');
  const suite = parametres.toString();
  return `/api/classrooms/${encode(etat.groupe.scope)}/students${suite ? '?' + suite : ''}`;
}

async function chargerEtudiants(force) {
  const attente = attendreTable('etudiants-table', 'etudiants-vide',
    'Chargement des étudiants…');
  const donnees = await tenter(() => api('GET', adresseEtudiants(force)), 'Étudiants');
  if (!attente.fini(donnees, "La liste des étudiants n'a pas pu être chargée.")) return;
  etat.etudiants = donnees.students || [];

  // Une sélection ne survit pas à ce que le filtre écarte : on déplace ce
  // qu'on voit, et rien d'autre.
  const visibles = new Set(etat.etudiants.map((ligne) => ligne.username));
  etat.deplaces = new Set([...etat.deplaces].filter((compte) => visibles.has(compte)));

  remplirTravauxDuFiltre(donnees.assignments || []);

  const corps = $('etudiants-table').querySelector('tbody');
  vider(corps);
  $('etudiants-table').hidden = etat.etudiants.length === 0;
  $('etudiants-vide').hidden = etat.etudiants.length > 0;
  $('etudiants-vide').textContent = donnees.total === 0
    ? 'Aucun étudiant dans ce groupe. Importez une liste « nom complet, compte GitHub ».'
    : 'Aucun étudiant ne répond à ces critères.';

  for (const ligne of etat.etudiants) {
    corps.append(el('tr', {},
      el('td', {}, el('input', {
        type: 'checkbox',
        checked: etat.deplaces.has(ligne.username),
        onchange: (evenement) => {
          if (evenement.target.checked) etat.deplaces.add(ligne.username);
          else etat.deplaces.delete(ligne.username);
          majSelectionEtudiants();
        },
      })),
      el('td', {}, lienVersLaFiche(ligne)),
      // Une personne travaille parfois sous deux comptes : les montrer tous
      // les deux évite de la croire absente d'un dépôt qui est le sien.
      el('td', {}, (ligne.accounts || [ligne.username]).map((compte, rang) =>
        el('span', {}, rang > 0 ? ' ' : null, lienDeProfil(compte)))),
      el('td', ligne.team
        ? { texte: ligne.team }
        : { classe: 'vide', texte: 'aucune' }),
      el('td', {}, ligne.assignments.length === 0
        ? el('span', { classe: 'vide', texte: 'aucun dépôt' })
        : el('span', { classe: 'etiquettes' }, ligne.assignments.map((travail) =>
            el('a', {
              classe: 'jeton lien', href: travail.url,
              target: '_blank', rel: 'noreferrer noopener',
              // Un dépôt d'équipe figure chez chacun de ses membres : dire
              // lequel évite de croire qu'il porte leur nom.
              texte: travail.team ? `${travail.name} (${travail.team})` : travail.name,
            })))),
      el('td', ligne.pushed_at ? { texte: ligne.pushed_at } : { classe: 'vide', texte: 'jamais' }),
      el('td', {}, el('span', { classe: 'actions' },
        el('button', {
          classe: 'bouton petit icone', type: 'button',
          title: 'Renommer…', 'aria-label': `Renommer ${ligne.full_name || '@' + ligne.username}`,
          onclick: () => renommerEtudiant(ligne),
        }, icone('crayon')),
        boutonIcone('fleche', 'Déplacer…',
          `Déplacer ${ligne.full_name || '@' + ligne.username} vers un autre groupe`,
          () => deplacerEtudiants([ligne]))))));
  }

  const filtre = donnees.shown !== donnees.total;
  $('etudiants-resume').textContent =
    (filtre ? `${donnees.shown} étudiant(s) sur ${donnees.total}` : `${donnees.total} étudiant(s)`) +
    ` · ${travaux((donnees.assignments || []).length)}`;
  barreEtudiants.maj();
  $('etudiants-noms').disabled = donnees.missing_names === 0;
  $('etudiants-noms').textContent = donnees.missing_names === 0
    ? 'Noms complets connus'
    : `Retrouver ${donnees.missing_names} nom(s) complet(s)`;
  majSelectionEtudiants();
}

// remplirTravauxDuFiltre garde le travail retenu s'il existe encore : recharger
// la liste ne doit pas défaire le filtre en cours.
function remplirTravauxDuFiltre(liste) {
  const choix = $('filtre-travail');
  const retenu = etat.filtre.travail;
  vider(choix);
  choix.append(el('option', { value: '', texte: 'tous' }));
  for (const travail of liste) {
    choix.append(el('option', { value: travail.name, texte: travail.name }));
  }
  choix.value = liste.some((travail) => travail.name === retenu) ? retenu : '';
  etat.filtre.travail = choix.value;
}

function majSelectionEtudiants() {
  const total = etat.etudiants.length;
  const choisis = etat.deplaces.size;
  $('etudiants-selection').textContent = choisis === 0
    ? `${total} étudiant(s) affiché(s)`
    : `${choisis} sur ${total} sélectionné(s)`;
  $('etudiants-tout').checked = total > 0 && choisis === total;
  $('etudiants-deplacer').disabled = choisis === 0;
}

plageDeCases($('etudiants-table').querySelector('tbody'));

$('etudiants-tout').addEventListener('change', (evenement) => {
  etat.deplaces = evenement.target.checked
    ? new Set(etat.etudiants.map((ligne) => ligne.username))
    : new Set();
  for (const case_ of $('etudiants-table').querySelectorAll('tbody input[type="checkbox"]')) {
    case_.checked = evenement.target.checked;
  }
  majSelectionEtudiants();
});

$('etudiants-deplacer').addEventListener('click', () => {
  const choisis = etat.etudiants.filter((ligne) => etat.deplaces.has(ligne.username));
  if (choisis.length) deplacerEtudiants(choisis);
});

// --- filtres et tri

// Le tri vit dans l'en-tête de la colonne qu'il ordonne, et la recherche reste
// sous la main. Le reste des critères — travail, dépôts, dates — tient dans un
// menu qu'on n'ouvre qu'au besoin : ils servent rarement, et l'écran leur était
// entièrement donné.
//
// Deux listes s'en servent — les étudiants d'un groupe, les dépôts d'un travail
// — parce que ce sont les mêmes critères, appliqués aux mêmes lignes par le
// même paquet du serveur. Seuls changent la table, les identifiants de la
// barre, et les critères que le menu propose.

// Le sens par défaut suit ce qu'on cherche : un nom se lit de A à Z, une date
// du plus récent au plus ancien.
const sensParDefaut = { nom: false, compte: false, envoi: true };

// Une frappe ne part pas au serveur avant que la main se soit arrêtée.
function differer(action, delai = 250) {
  let minuteur = 0;
  return (...arguments_) => {
    clearTimeout(minuteur);
    minuteur = setTimeout(() => action(...arguments_), delai);
  };
}

// menuDeroulant relie un bouton au panneau qu'il déplie. Le panneau est posé
// au-dessus de la page plutôt que dans la boîte qui le contient — celle-ci
// rognerait ce qui dépasse d'elle, et un menu dépasse toujours —, si bien que
// sa place se calcule à l'ouverture, une fois sa largeur connue.
//
// Les critères d'une liste et les commandes d'un travail se déplient de la même
// façon : ce qui suit ne sait pas ce que le panneau contient.
function menuDeroulant(ouvrir, menu) {
  function deplier(ouvert) {
    const flottant = $(menu);
    flottant.hidden = !ouvert;
    $(ouvrir).setAttribute('aria-expanded', String(ouvert));
    if (!ouvert) return;
    // Sa largeur ne se connaît qu'une fois affiché : la place se calcule après.
    const bouton = $(ouvrir).getBoundingClientRect();
    flottant.style.top = `${bouton.bottom + 6}px`;
    flottant.style.left = `${Math.max(8, bouton.right - flottant.offsetWidth)}px`;
  }

  $(ouvrir).addEventListener('click', () => deplier($(menu).hidden));

  // Le menu se referme comme tout menu : ailleurs, à l'échappement, ou dès que
  // la page bouge sous lui — sa place a été calculée pour l'endroit qu'elle
  // occupait.
  document.addEventListener('click', (evenement) => {
    if (!$(menu).hidden && !evenement.target.closest('.menu-ancre')) deplier(false);
  });
  document.addEventListener('keydown', (evenement) => {
    if (evenement.key === 'Escape' && !$(menu).hidden) {
      deplier(false);
      $(ouvrir).focus();
    }
  });
  window.addEventListener('scroll', () => {
    if (!$(menu).hidden) deplier(false);
  }, true);

  return { deplier };
}

// barreDeFiltre relie une barre — recherche, menu de critères, en-têtes
// triables — aux critères que la liste retient. « criteres » les désigne
// plutôt que de les porter : « vider » les remplace par un objet neuf, et la
// barre doit suivre. Elle rend de quoi la remettre au diapason après un
// chargement, et de quoi tout effacer.
function barreDeFiltre({ table, texte, ouvrir, menu, vider, champs, criteres, effacer, recharger }) {
  const tableau = $(table);

  for (const entete of tableau.querySelectorAll('th[data-tri]')) {
    entete.querySelector('button').addEventListener('click', () => {
      const critere = criteres();
      const colonne = entete.dataset.tri;
      // Recliquer la colonne déjà triée retourne l'ordre ; en choisir une autre
      // repart de son sens naturel.
      critere.desc = critere.tri === colonne ? !critere.desc : sensParDefaut[colonne];
      critere.tri = colonne;
      recharger();
    });
  }

  function majEntetes() {
    const critere = criteres();
    for (const entete of tableau.querySelectorAll('th[data-tri]')) {
      const actif = entete.dataset.tri === critere.tri;
      if (actif) entete.setAttribute('aria-sort', critere.desc ? 'descending' : 'ascending');
      else entete.removeAttribute('aria-sort');
      entete.querySelector('.fleche').textContent = actif ? (critere.desc ? '▼' : '▲') : '';
    }
  }

  // Le bouton dit combien de critères sont posés : un filtre replié dans un
  // menu ne doit pas pouvoir se faire oublier.
  function majBouton() {
    const critere = criteres();
    const poses = champs.filter(([, nom]) => critere[nom]).length;
    $(ouvrir).textContent = poses ? `Filtrer · ${poses}` : 'Filtrer';
    $(ouvrir).classList.toggle('vert', poses > 0);
  }

  const { deplier } = menuDeroulant(ouvrir, menu);

  const plusTard = differer(recharger);
  $(texte).addEventListener('input', (evenement) => {
    criteres().texte = evenement.target.value.trim();
    plusTard();
  });
  for (const [identifiant, nom] of champs) {
    $(identifiant).addEventListener('change', (evenement) => {
      criteres()[nom] = evenement.target.value;
      recharger();
    });
  }

  // reinitialiser remet la barre et les critères dans le même état : c'est la
  // même remise à zéro qu'on change de liste ou qu'on efface tout.
  function reinitialiser() {
    effacer();
    $(texte).value = '';
    for (const [identifiant] of champs) $(identifiant).value = '';
    maj();
  }

  function maj() {
    majEntetes();
    majBouton();
  }

  $(vider).addEventListener('click', () => {
    reinitialiser();
    deplier(false);
    recharger();
  });

  return { maj, reinitialiser };
}

const rechargerEtudiants = () => { if (etat.groupe) chargerEtudiants(); };

const barreEtudiants = barreDeFiltre({
  table: 'etudiants-table',
  texte: 'filtre-texte',
  ouvrir: 'filtre-ouvrir',
  menu: 'filtre-menu',
  vider: 'filtre-vider',
  champs: [
    ['filtre-travail', 'travail'], ['filtre-activite', 'activite'],
    ['filtre-apres', 'apres'], ['filtre-avant', 'avant'],
  ],
  criteres: () => etat.filtre,
  effacer: () => {
    etat.filtre = {
      texte: '', travail: '', activite: '', apres: '', avant: '', tri: 'nom', desc: false,
    };
    etat.deplaces = new Set();
  },
  recharger: () => rechargerEtudiants(),
});

$('etudiants-recharger').addEventListener('click', () => chargerEtudiants(true));

$('etudiants-noms').addEventListener('click', async () => {
  const fiche = await tenter(() => api('POST',
    `/api/classrooms/${encode(etat.groupe.scope)}/students/names`), 'Noms');
  if (!fiche) return;
  const bilan = await suivre(fiche);
  if (!bilan) return;
  message(`${bilan.resolved} nom(s) complet(s) retrouvé(s).`);
  await ouvrirGroupe(etat.groupe.scope);
  afficherVue('etudiants');
});

// Une inscription tardive n'a pas à passer par le fichier : deux champs
// suffisent, et le reste de la liste ne bouge pas. Les travaux cochés lui sont
// remis dans la foulée, aux réglages que le groupe retient — sans quoi il
// faudrait revenir distribuer travail par travail.
$('etudiants-ajouter').addEventListener('click', async () => {
  const nom = el('input', { type: 'text', classe: 'champ', placeholder: 'Jean-Luc Picard' });
  const compte = el('input', { type: 'text', classe: 'champ', placeholder: 'jlpicard' });

  const existants = etat.groupe.assignments || [];
  const cases = new Map();
  for (const travail of existants) {
    cases.set(travail.name, el('input', { type: 'checkbox' }));
  }
  const reglages = etat.groupe.defaults || {};
  const listeTravaux = existants.length === 0
    ? el('p', { classe: 'note', texte: "Le groupe n'a encore aucun travail distribué." })
    : el('div', {},
        el('span', { classe: 'etiquette', texte: 'Lui créer les dépôts de' }),
        plageDeCases(el('div', { classe: 'cases-travaux' }, existants.map((travail) =>
          el('label', { classe: 'case' }, cases.get(travail.name),
            el('span', {},
              el('strong', { texte: travail.name }),
              el('span', { classe: 'aide',
                texte: `déjà remis à ${travail.students} étudiant(s) du groupe` })))))),
        el('p', { classe: 'aide',
          texte: `Aux réglages du groupe : ${reglages.visibility === 'public' ? 'public' : 'privé'}, ` +
            (reglages.add_collaborator
              ? `invitation en « ${reglages.permission} »` : 'sans invitation') +
            (reglages.template ? `, modèle ${reglages.template}` : ', dépôt neuf') + '.' }));

  const confirme = await demander('Ajouter un étudiant', el('div', {},
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: 'Nom complet' }), nom,
      el('span', { classe: 'aide',
        texte: 'C’est lui qui nomme ses dépôts. Laissé vide, il se retrouvera depuis ' +
          'son profil GitHub — mais aucun dépôt ne pourra lui être remis d’ici là.' })),
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: 'Compte GitHub' }), compte),
    listeTravaux), 'Ajouter');
  if (!confirme) return;

  const choisis = [...cases.entries()]
    .filter(([, coche]) => coche.checked).map(([travail]) => travail);
  const fiche = await tenter(() => api('POST',
    `/api/classrooms/${encode(etat.groupe.scope)}/students/add`, {
      full_name: nom.value.trim(),
      username: compte.value.trim(),
      assignments: choisis,
    }), 'Étudiant');
  if (!fiche) return;

  // Sans dépôt à créer, le serveur répond directement ; sinon c'est un travail
  // de fond, avec son journal.
  const bilan = fiche.id ? await suivre(fiche) : fiche;
  if (!bilan) return;
  message(`@${bilan.student.username} ajouté à « ${etat.groupe.label} »` +
    (bilan.created ? ` · ${bilan.created} dépôt(s) créé(s)` : '') +
    (bilan.failed ? ` · ${bilan.failed} en échec` : ''),
    bilan.failed ? 'alerte' : 'succes');
  await ouvrirGroupe(etat.groupe.scope, true, true);
  afficherVue('etudiants');
});

// --- renommer un étudiant

// Un prénom mal orthographié, un accent oublié, un compte changé : corriger une
// seule fiche passait par le remplacement de la liste entière — donc celle de
// tout le monde, et le fichier à retrouver.
//
// Le nom complet est le dernier niveau du nom des dépôts. Les renommer est une
// écriture sur GitHub : elle est proposée ici, jamais imposée.
async function renommerEtudiant(ligne) {
  const nom = el('input', { type: 'text', classe: 'champ',
    value: ligne.full_name || '', placeholder: 'Jean-Luc Picard' });
  const compte = el('input', { type: 'text', classe: 'champ', value: ligne.username });
  const depots = ligne.assignments.length;
  const avecDepots = el('input', { type: 'checkbox', checked: depots > 0 });

  const confirme = await demander(
    `Renommer ${ligne.full_name || '@' + ligne.username}`, el('div', {},
      el('label', { classe: 'champ-bloc' },
        el('span', { classe: 'etiquette', texte: 'Nom complet' }), nom,
        el('span', { classe: 'aide',
          texte: 'C’est lui qui nomme ses dépôts. Le corriger ne renomme pas ceux ' +
            'qui existent déjà, à moins de le demander ci-dessous.' })),
      el('label', { classe: 'champ-bloc' },
        el('span', { classe: 'etiquette', texte: 'Compte GitHub' }), compte,
        el('span', { classe: 'aide',
          texte: 'Il n’entre pas dans le nom des dépôts : le changer ne touche qu’à ' +
            'la liste. Le nouveau compte est vérifié sur GitHub.' })),
      depots === 0
        ? el('p', { classe: 'note',
            texte: 'Cette personne n’a encore aucun dépôt : il n’y a rien à renommer ' +
              'sur GitHub.' })
        : el('label', { classe: 'case' }, avecDepots,
            el('span', {}, el('strong', { texte: `Renommer aussi ses ${depots} dépôt(s)` }),
              el('span', { classe: 'aide',
                texte: 'Ils prennent son nouveau nom. GitHub garde une redirection depuis ' +
                  'chaque ancien nom.' })))),
    'Renommer');
  if (!confirme) return;

  const corps = {
    username: ligne.username,
    full_name: nom.value.trim(),
    new_username: compte.value.trim(),
    repos: depots > 0 && avecDepots.checked,
  };
  // Une fiche inchangée n'a rien à enregistrer : l'envoyer quand même ferait
  // croire à une correction qui n'a pas eu lieu.
  if (corps.full_name === (ligne.full_name || '') &&
      corps.new_username.toLowerCase() === ligne.username.toLowerCase()) {
    message('Ni le nom ni le compte n’ont changé.', 'alerte');
    return;
  }

  const fiche = await tenter(() => api('POST',
    `/api/classrooms/${encode(etat.groupe.scope)}/students/rename`, corps), 'Étudiant');
  if (!fiche) return;

  // Sans dépôt à renommer, le serveur répond directement ; sinon c'est un
  // travail de fond, avec son journal.
  const bilan = fiche.id ? await suivre(fiche) : fiche;
  if (!bilan) return;
  message(`${bilan.student.full_name || '@' + bilan.student.username} mis à jour` +
    (bilan.renamed ? ` · ${bilan.renamed} dépôt(s) renommé(s)` : '') +
    (bilan.failed ? ` · ${bilan.failed} en échec` : ''),
    bilan.failed ? 'alerte' : 'succes');
  await ouvrirGroupe(etat.groupe.scope, true, true);
  afficherVue('etudiants');
}

$('etudiants-importer').addEventListener('click', async () => {
  const { zone: bloc, champ: chemin } = zoneDepot({
    titre: 'Choisir la liste des étudiants', valeur: etat.groupe.roster_path || '',
  });
  const zone = el('textarea', { classe: 'champ', rows: '5',
    placeholder: 'Jean-Luc Picard, jlpicard' });
  const confirme = await demander('Remplacer la liste des étudiants', el('div', {},
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: 'Fichier CSV' }), bloc),
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: '…ou une liste collée' }), zone),
    el('p', { classe: 'note',
      texte: "La liste remplace l'ancienne. Aucun dépôt n'est touché." })), 'Remplacer');
  if (!confirme) return;

  let corps = null;
  if (zone.value.trim()) {
    const liste = await tenter(() =>
      api('POST', '/api/roster/parse', { text: zone.value }), 'Liste');
    if (!liste) return;
    corps = { people: liste.people };
  } else if (bloc.dataset.contenu) {
    const liste = await tenter(() =>
      api('POST', '/api/roster/parse', { content: bloc.dataset.contenu }), 'Liste');
    if (!liste) return;
    corps = { people: liste.people };
  } else if (chemin.value.trim()) {
    corps = { path: chemin.value.trim() };
  } else {
    message('Indiquez un fichier ou collez une liste.', 'alerte');
    return;
  }

  const bilan = await tenter(() => api('POST',
    `/api/classrooms/${encode(etat.groupe.scope)}/students`, corps), 'Étudiants');
  if (!bilan) return;
  if (bilan.issues && bilan.issues.length) {
    message(`${bilan.issues.length} ligne(s) rejetée(s).`, 'alerte', 10000);
  }
  await ouvrirGroupe(etat.groupe.scope);
  afficherVue('etudiants');
});

// ------------------------------------------- annuaire de l'organisation

// La hiérarchie va de la session au groupe puis à ses étudiants : elle répond
// à « qui est dans ce groupe ». L'annuaire prend le chemin inverse — une
// personne par ligne, ses cours en face — et répond à « qu'a-t-elle suivi ».
// C'est la seule vue où l'on voit quelqu'un revenir d'une session à l'autre :
// deux listes de groupe montrent deux inscriptions, jamais la même personne.
//
// Le tri et le filtre partent au serveur, comme pour la liste d'un groupe :
// c'est le même paquet qui décide de ce qu'ils veulent dire.

function adresseAnnuaire(force) {
  const filtre = etat.annuaire.filtre;
  const parametres = new URLSearchParams();
  if (filtre.texte) parametres.set('q', filtre.texte);
  if (filtre.session) parametres.set('session', filtre.session);
  if (filtre.cours) parametres.set('course', filtre.cours);
  if (filtre.activite) parametres.set('activity', filtre.activite);
  if (filtre.apres) parametres.set('after', filtre.apres);
  if (filtre.avant) parametres.set('before', filtre.avant);
  if (filtre.tri !== 'nom') parametres.set('sort', filtre.tri);
  if (filtre.desc) parametres.set('desc', '1');
  if (force) parametres.set('refresh', '1');
  const suite = parametres.toString();
  return `/api/users${suite ? '?' + suite : ''}`;
}

async function chargerAnnuaire(force) {
  if (!etat.organisation) return;
  const attente = attendreTable('annuaire-table', 'annuaire-vide',
    'Chargement des étudiants…');
  const donnees = await tenter(() => api('GET', adresseAnnuaire(force)), 'Étudiants');
  if (!attente.fini(donnees, "L'annuaire n'a pas pu être chargé.")) return;
  etat.annuaire.lignes = donnees.students || [];
  etat.annuaire.deplies = new Set();

  // Les sessions de l'annuaire servent aussi à nommer : « a26 » s'y lit
  // « Automne 2026 » même si aucun groupe n'a encore été ouvert.
  if (donnees.sessions && donnees.sessions.length) etat.sessions = donnees.sessions;
  remplirSessionsDuFiltre(donnees.sessions || []);
  if (remplirCoursDuFiltre(donnees.courses || [])) { chargerAnnuaire(force); return; }

  dessinerAnnuaire();
  resumerAnnuaire(donnees);
  barreAnnuaire.maj();
}

// dessinerAnnuaire rend les lignes déjà chargées : déplier un étudiant ne
// redemande rien au serveur, tout est là.
function dessinerAnnuaire() {
  const corps = $('annuaire-table').querySelector('tbody');
  vider(corps);
  $('annuaire-table').hidden = etat.annuaire.lignes.length === 0;
  $('annuaire-vide').hidden = etat.annuaire.lignes.length > 0;
  for (const ligne of etat.annuaire.lignes) {
    corps.append(ligneAnnuaire(ligne));
    if (etat.annuaire.deplies.has(ligne.username)) corps.append(depotsAnnuaire(ligne));
  }
}

function ligneAnnuaire(ligne) {
  return el('tr', {},
    el('td', {}, lienVersLaFiche(ligne)),
    el('td', {}, lienDeProfil(ligne.username)),
    el('td', {}, ligne.enrollments.length === 0
      ? el('span', { classe: 'vide', texte: 'aucun cours' })
      : el('span', { classe: 'etiquettes' },
          ligne.enrollments.map((inscription) => el('button', {
            classe: 'cours-suivi', type: 'button',
            texte: jetonDuCours(inscription), title: titreDuCours(inscription),
            onclick: () => ouvrirGroupe(inscription.scope),
          })))),
    el('td', {}, ligne.repos === 0
      ? el('span', { classe: 'vide', texte: 'aucun' })
      : el('button', {
          classe: 'lien', type: 'button', texte: String(ligne.repos),
          title: 'Voir ses dépôts',
          onclick: () => {
            if (etat.annuaire.deplies.has(ligne.username)) {
              etat.annuaire.deplies.delete(ligne.username);
            } else {
              etat.annuaire.deplies.add(ligne.username);
            }
            dessinerAnnuaire();
          },
        })),
    el('td', ligne.pushed_at ? { texte: ligne.pushed_at } : { classe: 'vide', texte: 'jamais' }));
}

function jetonDuCours(inscription) {
  if (!inscription.session) return inscription.label;
  return `${sigle(inscription.session)} · ${sigle(inscription.course)}`;
}

function titreDuCours(inscription) {
  if (!inscription.session) return inscription.scope;
  return `${inscription.session_name || inscription.session} · ${sigle(inscription.course)}` +
    ` · groupe ${inscription.group}`;
}

// depotsAnnuaire déplie les dépôts d'une personne, rangés sous le groupe d'où
// ils viennent : deux groupes peuvent avoir chacun leur « tp1 ».
function depotsAnnuaire(ligne) {
  // Les inscriptions arrivent déjà rangées, de la session la plus récente à
  // la plus ancienne : c'est le serveur qui en décide, pour tout le monde.
  const blocs = ligne.enrollments
    .filter((inscription) => inscription.assignments.length > 0)
    .map((inscription) => el('div', { classe: 'depots-du-cours' },
      el('span', { classe: 'etiquette', texte: titreDuCours(inscription) }),
      el('span', { classe: 'frise-travaux' },
        inscription.assignments.map(puceDeTravail))));
  return el('tr', { classe: 'ligne-depliee' },
    el('td', {}), el('td', { colspan: '4' }, el('div', { classe: 'depots' }, blocs)));
}

function resumerAnnuaire(donnees) {
  const parts = [donnees.shown !== donnees.total
    ? `${donnees.shown} étudiant(s) sur ${donnees.total}`
    : `${donnees.total} étudiant(s)`];
  parts.push(`${(donnees.sessions || []).length} session(s)`);
  parts.push(`${(donnees.courses || []).length} cours`);
  // Les dépôts que personne ne réclame ne sont pas des étudiants de plus :
  // leur nom slugifié ne désigne aucune liste. Les taire ferait lire une
  // liste trouée comme si elle était entière.
  if (donnees.unmatched) parts.push(`${donnees.unmatched} dépôt(s) sans étudiant connu`);
  $('annuaire-resume').textContent = parts.join(' · ');
  $('annuaire-vide').textContent = donnees.total === 0
    ? "Aucun étudiant connu dans cette organisation. Déclarez un groupe et importez sa liste."
    : 'Aucun étudiant ne répond à ces critères.';
}

function remplirSessionsDuFiltre(liste) {
  const choix = $('annuaire-session');
  const retenue = etat.annuaire.filtre.session;
  vider(choix);
  choix.append(el('option', { value: '', texte: 'toutes' }));
  for (const session of liste) {
    choix.append(el('option', { value: session.short, texte: session.name }));
  }
  choix.value = liste.some((session) => session.short === retenue) ? retenue : '';
  etat.annuaire.filtre.session = choix.value;
}

// remplirCoursDuFiltre suit la session choisie : les cours proposés sont les
// siens. Un cours devenu impossible est effacé plutôt que laissé à filtrer une
// liste vide — la fonction dit alors qu'il faut recharger.
function remplirCoursDuFiltre(liste) {
  const choix = $('annuaire-cours');
  const retenu = etat.annuaire.filtre.cours;
  vider(choix);
  choix.append(el('option', { value: '', texte: 'tous' }));
  for (const cours of liste) {
    choix.append(el('option', { value: cours, texte: sigle(cours) }));
  }
  choix.value = liste.includes(retenu) ? retenu : '';
  const efface = etat.annuaire.filtre.cours !== choix.value;
  etat.annuaire.filtre.cours = choix.value;
  return efface;
}

const barreAnnuaire = barreDeFiltre({
  table: 'annuaire-table',
  texte: 'annuaire-texte',
  ouvrir: 'annuaire-filtre-ouvrir',
  menu: 'annuaire-filtre-menu',
  vider: 'annuaire-vider',
  champs: [
    ['annuaire-session', 'session'], ['annuaire-cours', 'cours'],
    ['annuaire-activite', 'activite'],
    ['annuaire-apres', 'apres'], ['annuaire-avant', 'avant'],
  ],
  criteres: () => etat.annuaire.filtre,
  effacer: () => {
    etat.annuaire.filtre = {
      texte: '', session: '', cours: '', activite: '', apres: '', avant: '',
      tri: 'nom', desc: false,
    };
  },
  recharger: () => chargerAnnuaire(),
});

// ------------------------------------------- fiche d'un utilisateur

// La seule vue centrée sur une personne. L'annuaire répond à « qui a suivi
// quoi » pour tout le monde à la fois ; la fiche prend quelqu'un et déroule son
// passage, de la session la plus récente à la plus ancienne.
//
// Elle mêle les cours suivis et les cours donnés : chercher ce qu'un collègue a
// enseigné et retrouver ce qu'un étudiant a suivi sont la même question posée à
// deux personnes différentes.

// ouvrirFiche mène à la fiche de quelqu'un. C'est le geste que tous les noms
// de l'application déclenchent.
function ouvrirFiche(compte) {
  if (!compte) return;
  etat.fiche = { compte, donnees: null };
  afficherVue('fiche');
}

// lienVersLaFiche rend un nom cliquable. Sans nom connu, c'est le compte qui
// mène à la fiche : il désigne quand même quelqu'un.
function lienVersLaFiche(personne, options = {}) {
  const compte = personne.username || '';
  const nom = personne.full_name || '';
  if (!compte) return el('span', { classe: 'vide', texte: nom || 'nom inconnu' });
  return el('button', {
    classe: ('lien ' + (options.classe || '')).trim(), type: 'button',
    texte: nom || compte,
    title: `Voir la fiche de ${nom || '@' + compte}`,
    onclick: (evenement) => { evenement.stopPropagation(); ouvrirFiche(compte); },
  });
}

async function chargerFiche() {
  const compte = etat.fiche.compte;
  if (!etat.organisation || !compte) return;
  $('fiche-nom').textContent = '@' + compte;
  const donnees = await tenter(
    () => api('GET', `/api/users/${encode(compte)}`), 'Fiche');
  if (!donnees) {
    $('fiche-resume').textContent = "La fiche n'a pas pu être chargée.";
    return;
  }
  // Une autre fiche a pu être demandée pendant la requête.
  if (etat.fiche.compte !== compte) return;
  etat.fiche.donnees = donnees;
  dessinerFiche();
}

function dessinerFiche() {
  const donnees = etat.fiche.donnees;
  if (!donnees) return;
  const personne = donnees.user;

  // L'en-tête a été dessiné avant que le nom ne soit connu : il disait le
  // compte. Maintenant qu'on a la personne, il dit qui elle est.
  dessinerEntete('fiche', ongletDeLaVue['fiche']);

  $('fiche-nom').textContent = personne.full_name || '@' + personne.username;
  const role = $('fiche-role');
  role.hidden = false;
  role.textContent = personne.role;
  role.classList.toggle('enseignant', personne.is_teacher);

  dessinerComptesDeLaFiche(personne, donnees.host);
  const matricule = $('fiche-matricule');
  matricule.hidden = !personne.student_id;
  matricule.textContent = personne.student_id ? 'Matricule ' + personne.student_id : '';

  // Nommer quelqu'un n'est offert qu'à qui n'a pas de nom : corriger un nom
  // déjà donné touche aux dépôts qui le portent, et cela se fait là où on les
  // voit — dans la liste du groupe.
  const sansNom = $('fiche-sans-nom');
  sansNom.hidden = !!personne.full_name;
  if (!sansNom.hidden) $('fiche-nommer').onclick = () => nommerUnUtilisateur(personne);

  $('fiche-resume').textContent = resumerFiche(personne);
  dessinerAvisDeLaFiche(donnees, personne);
  dessinerCooptation(donnees, personne);
  dessinerFrise(personne);
}

// resumerFiche dit en une ligne ce que la chronologie détaille.
function resumerFiche(personne) {
  const parts = [];
  if (personne.courses) parts.push(`${personne.courses} cours suivi(s)`);
  if (personne.taught) parts.push(`${personne.taught} cours donné(s)`);
  if (personne.repos) parts.push(`${personne.repos} dépôt(s)`);
  parts.push(personne.pushed_at ? 'dernier envoi ' + personne.pushed_at : 'aucun envoi');
  return parts.join(' · ');
}

// dessinerComptesDeLaFiche montre tous les comptes de la personne. Chacun mène
// à son profil GitHub : c'est la seule chose qu'on veuille faire d'un compte.
function dessinerComptesDeLaFiche(personne, hote) {
  const zone = $('fiche-comptes');
  vider(zone);
  const domaine = hote || (etat.contexte && etat.contexte.host) || 'github.com';
  for (const compte of personne.accounts) {
    const principal = compte.toLowerCase() === (personne.username || '').toLowerCase();
    zone.append(el('a', {
      classe: 'fiche-compte' + (principal ? ' principal' : ''),
      href: `https://${domaine}/${encodeURIComponent(compte)}`,
      target: '_blank', rel: 'noreferrer noopener',
      texte: '@' + compte,
      title: `Ouvrir @${compte} sur ${domaine}`,
    }));
  }
}

// dessinerAvisDeLaFiche signale ce qu'il faut savoir avant de lire le reste :
// un compte que le registre ignore, un registre illisible.
function dessinerAvisDeLaFiche(donnees, personne) {
  const zone = $('fiche-avis');
  vider(zone);
  if (donnees.notice) {
    zone.append(el('div', { classe: 'avis alerte', texte: donnees.notice }));
  }
  if (!personne.known) {
    zone.append(el('div', { classe: 'avis', texte:
      `Le registre de « ${donnees.org} » ne connaît pas @${personne.username}. ` +
      "Le compte existe sur GitHub, il n'a simplement rien fait ici — ou son nom " +
      "complet n'a jamais été donné." }));
  }
}

// dessinerCooptation offre de reconnaître quelqu'un comme enseignant, ou de
// lui retirer ce rôle.
//
// Elle n'est offerte qu'à un enseignant. Ce n'est pas elle qui protège le
// registre — un étudiant n'a jamais eu le droit d'y écrire, et GitHub le lui
// refuserait bien avant nous : elle évite seulement de proposer un geste qui
// échouerait.
function dessinerCooptation(donnees, personne) {
  const pied = $('fiche-pied');
  const bouton = $('fiche-role-bouton');
  const aide = $('fiche-role-aide');
  // Tant que l'organisation n'a aucun enseignant, quelqu'un doit pouvoir
  // commencer : cacher le bouton ne laisserait aucun chemin pour le faire.
  const premier = donnees.teachers === 0;
  pied.hidden = !donnees.viewer_teaches && !premier;
  if (pied.hidden) return;

  bouton.textContent = personne.is_teacher
    ? "Retirer le rôle d'enseignant" : 'Reconnaître comme enseignant';
  bouton.classList.toggle('rouge', personne.is_teacher);
  if (premier && !personne.is_teacher) {
    aide.textContent = "Aucun enseignant n'est encore reconnu dans « " +
      donnees.org + " » : le premier se déclare, les suivants sont cooptés.";
  } else {
    aide.textContent = personne.is_teacher
      ? "Il pourra être inscrit à l'équipe enseignante d'un groupe."
      : "Le rôle ne donne aucun accès : il autorise à en donner, groupe par groupe.";
  }
  bouton.onclick = () => coopter(personne, !personne.is_teacher);
}

async function coopter(personne, enseignant) {
  const question = enseignant
    ? `Reconnaître ${personne.full_name || '@' + personne.username} comme enseignant ?`
    : `Retirer son rôle d'enseignant à ${personne.full_name || '@' + personne.username} ?`;
  const detail = enseignant
    ? "Cela ne lui ouvre aucun dépôt. Il pourra en revanche être inscrit à " +
      "l'équipe enseignante d'un groupe, et c'est cette inscription qui donne l'accès."
    : "Il restera dans les équipes enseignantes où il est déjà inscrit : " +
      "retirez-l'en groupe par groupe pour lui fermer les dépôts.";
  const confirme = await demander(question,
    el('p', { classe: 'note', texte: detail }),
    enseignant ? 'Reconnaître' : 'Retirer');
  if (!confirme) return;

  const reponse = await tenter(() => api(
    'POST', `/api/users/${encode(personne.username)}/role`,
    { is_teacher: enseignant }), 'Rôle');
  if (!reponse) return;
  message(`@${reponse.username} est désormais ${reponse.role}.`);
  chargerFiche();
}

// nommerUnUtilisateur donne son nom complet à quelqu'un qui n'en a pas.
//
// Le nom monte au registre de l'organisation, pas dans un groupe : c'est une
// propriété de la personne, et le lui donner depuis sa fiche vaut partout —
// y compris pour quelqu'un qu'aucun groupe déclaré ici ne connaît.
async function nommerUnUtilisateur(personne) {
  const champ = el('input', {
    classe: 'champ', type: 'text', placeholder: 'Prénom Nom' });
  const corps = el('div', {},
    el('p', {}, el('code', { texte: '@' + personne.username }),
      " n'a pas de nom complet."),
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: 'Nom complet' }), champ,
      el('span', { classe: 'aide', texte:
        "Il monte au registre de l'organisation et vaut pour tous ses groupes. "
        + "Aucun dépôt n'est renommé : ceux qui existent restent les siens." })));

  if (!await demander('Nommer cette personne', corps, 'Enregistrer',
    () => champ.focus())) return;
  const voulu = champ.value.trim();
  if (!voulu) return;

  const reponse = await tenter(() => api(
    'PUT', `/api/users/${encode(personne.username)}/name`,
    { full_name: voulu }), 'Nom complet');
  if (!reponse) return;
  message(`@${reponse.username} s'appelle « ${reponse.full_name} ».`);
  chargerFiche();
}

// dessinerFrise déroule la chronologie. Chaque étape porte sa session en toutes
// lettres, son cours, son groupe, et ce qu'il en reste : les dépôts rendus, ou
// le silence.
function dessinerFrise(personne) {
  const frise = $('fiche-frise');
  vider(frise);
  const rien = personne.timeline.length === 0;
  frise.hidden = rien;
  $('fiche-vide').hidden = !rien;
  $('fiche-vide').textContent = rien
    ? "Aucun cours suivi ni donné dans cette organisation."
    : '';
  for (const etape of personne.timeline) {
    frise.append(etapeDeLaFrise(etape));
  }
}

function etapeDeLaFrise(etape) {
  const enseigne = etape.role === 'enseignant';
  const ligne = el('li', { classe: 'frise-etape' + (enseigne ? ' enseigne' : '') },
    el('div', { classe: 'frise-tete' },
      el('span', { classe: 'frise-session', texte: etape.session_name || etape.session || etape.label }),
      el('button', {
        classe: 'frise-lien', type: 'button',
        texte: sigle(etape.course) + (etape.group ? ' · groupe ' + etape.group : ''),
        title: 'Ouvrir ' + etape.scope,
        onclick: () => ouvrirGroupe(etape.scope),
      }),
      el('span', { classe: 'jeton' + (enseigne ? ' enseignant' : ''), texte: etape.role })));

  ligne.append(el('p', { classe: 'frise-detail', texte: detailDeLEtape(etape) }));
  if (etape.assignments.length > 0) {
    ligne.append(el('div', { classe: 'frise-travaux' },
      etape.assignments.map(puceDeTravail)));
  }
  return ligne;
}

// puceDeTravail rend un dépôt rendu, cerné. Plusieurs travaux d'un même cours
// se suivent : sans bordure, « tp1 tp2 tp3 » se lit comme un seul mot, et rien
// ne dit où l'un finit et où l'autre commence. Sa date y figure pour la même
// raison — c'est aussi ce qu'on vient comparer entre deux travaux.
function puceDeTravail(travail) {
  return el('a', {
    classe: 'jeton lien travail-puce', href: travail.url,
    target: '_blank', rel: 'noreferrer noopener',
    title: travail.repo + (travail.pushed_at
      ? ' — dernier envoi ' + travail.pushed_at : ' — aucun envoi'),
  },
    el('span', { classe: 'travail-nom',
      texte: travail.name + (travail.team ? ' · ' + travail.team : '') }),
    el('span', { classe: 'travail-date',
      texte: travail.pushed_at || 'aucun envoi' }));
}

// detailDeLEtape dit ce que l'étape a laissé. « Muet » n'est pas « aucun
// dépôt » : le dépôt existe, il n'a simplement jamais rien reçu.
function detailDeLEtape(etape) {
  if (etape.role === 'enseignant') return 'A donné ce cours.';
  if (etape.assignments.length === 0) return 'Aucun dépôt dans ce groupe.';
  const combien = travaux(etape.assignments.length);
  if (etape.silent) return `${combien}, aucun envoi.`;
  return `${combien}, dernier envoi le ${etape.pushed_at}.`;
}

// --------------------------------------------- équipe enseignante du groupe

// Cloisonner un groupe, c'est donner ses dépôts à une équipe et n'y mettre que
// ceux qui l'enseignent. Le registre dit qui enseigne ; l'équipe dit où — et
// c'est elle, jamais le registre, qui ouvre l'accès.

async function chargerCloisonnement() {
  if (!etat.groupe) return;
  const donnees = await tenter(() => api(
    'GET', `/api/classrooms/${encode(etat.groupe.scope)}/teachers`), 'Équipe enseignante');
  if (!donnees || !etat.groupe || donnees.scope !== etat.groupe.scope) return;
  etat.cloisonnement = donnees;
  dessinerCloisonnement();
}

function dessinerCloisonnement() {
  const donnees = etat.cloisonnement;
  if (!donnees) return;
  const etat_ = donnees.state;

  $('cloison-nom').textContent = etat_.name;
  const jeton = $('cloison-etat');
  const cloisonne = etat_.exists && etat_.teachers.length > 0;
  jeton.textContent = cloisonne
    ? `${etat_.teachers.length} enseignant(s) · ${etat_.repos} dépôt(s)`
    : 'non cloisonné';
  jeton.classList.toggle('oui', cloisonne);

  const choix = $('cloison-choix');
  vider(choix);
  const aucun = donnees.candidates.length === 0;
  $('cloison-vide').hidden = !aucun;
  $('cloison-appliquer').disabled = aucun;
  for (const candidat of donnees.candidates) {
    choix.append(el('label', { classe: 'case' },
      el('input', {
        type: 'checkbox', value: candidat.username, checked: candidat.member,
      }),
      el('span', {},
        el('span', { texte: candidat.full_name || candidat.username }), ' ',
        lienDeProfil(candidat.username))));
  }

  const zone = $('cloison-avis');
  vider(zone);
  if (donnees.notice) zone.append(el('div', { classe: 'avis', texte: donnees.notice }));
  if (donnees.exposure) {
    zone.append(el('div', { classe: 'avis alerte', texte: donnees.exposure }));
  }
}

$('cloison-appliquer').addEventListener('click', async () => {
  if (!etat.groupe) return;
  const enseignants = comptesCoches($('cloison-choix'));
  const apercu = await tenter(() => api(
    'POST', `/api/classrooms/${encode(etat.groupe.scope)}/teachers/preview`,
    { teachers: enseignants }), 'Cloisonnement');
  if (!apercu) return;

  const plan = apercu.plan;
  if (plan.steps.length === 0) {
    message("L'équipe enseignante de ce groupe est déjà celle-là.", 'note');
    return;
  }
  const confirme = await demander('Appliquer cette composition ?',
    el('div', {},
      el('p', { classe: 'note', texte: resumerPlanDeCloisonnement(plan) }),
      el('p', { classe: 'aide', texte: plan.notice })),
    'Appliquer');
  if (!confirme) return;

  const fiche = await tenter(() => api(
    'POST', `/api/classrooms/${encode(etat.groupe.scope)}/teachers`,
    { teachers: enseignants }), 'Cloisonnement');
  if (!fiche) return;
  await suivre(fiche);
  chargerCloisonnement();
});

// resumerPlanDeCloisonnement dit ce que les écritures vont faire, par nature :
// une liste de quatre-vingts lignes ne se lit pas avant de confirmer.
function resumerPlanDeCloisonnement(plan) {
  const comptes = {};
  for (const etape of plan.steps) {
    comptes[etape.kind] = (comptes[etape.kind] || 0) + 1;
  }
  const parts = [];
  if (comptes['création']) parts.push("création de l'équipe");
  if (comptes['inscription']) parts.push(`${comptes['inscription']} inscription(s)`);
  if (comptes['retrait']) parts.push(`${comptes['retrait']} retrait(s)`);
  if (comptes['accès']) parts.push(`${comptes['accès']} dépôt(s) accordés en « ${plan.permission} »`);
  return parts.join(', ') + '.';
}

// ------------------------------------------------------------------ équipes

// Une équipe est une vraie équipe d'organisation GitHub, comme chez Classroom.
// Son nom porte la place du groupe — « a26.5n6.01.eq1 » — parce qu'une
// organisation n'accepte qu'un nom d'équipe donné : sans cela, deux groupes ne
// pourraient pas avoir chacun leur « eq1 ».
//
// L'accès au dépôt est accordé à l'équipe, jamais à ses membres un par un.
// Déplacer quelqu'un d'une équipe à l'autre suffit donc à changer ce qu'il
// voit, sans toucher à aucun dépôt.

async function chargerEquipes(force) {
  const conteneur = $('equipes-liste');
  enAttente(conteneur, 'Chargement des équipes…');
  $('equipes-vide').hidden = true;
  const adresse = `/api/classrooms/${encode(etat.groupe.scope)}/teams` + (force ? '?refresh=1' : '');
  const donnees = await tenter(() => api('GET', adresse), 'Équipes');
  if (!donnees) {
    enEchec(conteneur, "Les équipes n'ont pas pu être chargées.");
    return;
  }
  etat.equipes = donnees.teams || [];
  etat.orphelins = donnees.unassigned || [];
  etat.depotsDEquipe = donnees.repos || {};
  dessinerEquipes();
}

function dessinerEquipes() {
  const conteneur = $('equipes-liste');
  vider(conteneur);
  const total = gens(etat.groupe).length;
  const places = total - etat.orphelins.length;
  $('equipes-resume').textContent = etat.equipes.length === 0
    ? `Aucune équipe · ${total} étudiant(s) dans le groupe`
    : `${etat.equipes.length} équipe(s) · ${places} étudiant(s) sur ${total} en font partie`;

  $('equipes-vide').hidden = etat.equipes.length > 0;
  $('equipes-vide').textContent =
    "Aucune équipe. « Nouvelle équipe » en crée une sur GitHub ; « Adopter une équipe » " +
    "reprend une équipe déjà présente dans l'organisation.";

  for (const equipe of etat.equipes) {
    const attente = new Set(equipe.waiting || []);
    const membres = el('div', { classe: 'equipe-membres' },
      equipe.people.map((personne) => ligneDeMembre(personne, {
        invite: attente.has(personne.username),
      })),
      // Un compte hors liste porte quand même son nom quand le registre le
      // connaît : ce qu'on sait nommer doit être nommé.
      equipe.strangers.map((personne) => ligneDeMembre(personne, {
        invite: attente.has(personne.username),
        horsListe: true,
        action: { texte: 'Inscrire…', faire: () => inscrireUnMembre(personne) },
      })));
    if (equipe.people.length === 0 && equipe.strangers.length === 0) {
      vider(membres);
      membres.append(el('p', { classe: 'note vide',
        texte: "Aucun membre. « Composer » dit qui en fait partie." }));
    }
    conteneur.append(el('div', { classe: 'equipe-rangee' },
      el('div', { classe: 'equipe-entete' },
        el('span', { classe: 'equipe-titre', texte: equipe.short }),
        el('code', { classe: 'equipe-nom', texte: equipe.name }),
        el('span', { classe: 'espace' }),
        el('span', { classe: 'equipe-actions' },
          boutonIcone('gens', 'Composer', `Composer ${equipe.short}`,
            () => composerEquipe(equipe)),
          boutonIcone('crayon', 'Renommer', `Renommer ${equipe.short}`,
            () => renommerEquipe(equipe)),
          boutonIcone('fleche', 'Déplacer…',
            `Déplacer ${equipe.short} vers un autre groupe`,
            () => deplacerEquipe(equipe)),
          boutonIcone('corbeille', 'Supprimer', `Supprimer ${equipe.short}`,
            () => supprimerEquipe(equipe), 'rouge'))),
      membres));
  }

  const orphelins = $('equipes-orphelins');
  vider(orphelins);
  $('equipes-orphelins-boite').hidden = etat.orphelins.length === 0;
  for (const personne of etat.orphelins) {
    // Un nom qui manque se voit, et ce qu'il y a à faire est écrit : le placer
    // dans une équipe sans l'avoir remet le problème à la distribution.
    orphelins.append(ligneDeMembre(personne, {
      action: {
        texte: personne.full_name ? 'Placer…' : 'Nommer et placer…',
        faire: () => placerDansUneEquipe(personne),
      },
    }));
  }
}

$('equipes-recharger').addEventListener('click', () => chargerEquipes(true));
$('equipes-nouvelle').addEventListener('click', () => nouvelleEquipe());
$('equipes-adopter').addEventListener('click', () => adopterEquipe());

// ligneDeMembre écrit un membre d'équipe : son nom, ses comptes, et ce qui
// cloche s'il y a lieu.
//
// Une ligne par personne plutôt qu'une pastille dans un paragraphe : le nom et
// le compte s'y lisent en regard l'un de l'autre — c'est précisément ce qu'on
// vient vérifier —, et il reste la place de dire qu'une invitation n'a pas été
// acceptée ou qu'un nom manque, avec à côté le bouton qui le règle.
function ligneDeMembre(personne, etats = {}) {
  const comptes = [personne.username].concat(personne.also || []);
  const ligne = el('div', { classe: 'equipe-membre' },
    personne.username
      ? lienVersLaFiche(personne, { classe: 'membre-nom' })
      : el('span', { classe: 'membre-nom vide', texte: 'nom complet inconnu' }),
    el('span', { classe: 'membre-comptes' },
      comptes.map((compte, rang) =>
        el('span', {}, rang > 0 ? ' ' : null, lienDeProfil(compte)))));
  if (etats.invite) {
    ligne.append(el('span', { classe: 'jeton attente', texte: 'invité',
      title: "Invitation envoyée : la personne n'a pas encore accepté." }));
  }
  if (etats.horsListe) {
    ligne.append(el('span', { classe: 'jeton etranger', texte: 'hors liste',
      title: "Ce compte n'est dans la liste d'aucun groupe." }));
  }
  ligne.append(el('span', { classe: 'espace' }));
  // Ce qu'il y a à faire est écrit, pas deviné : rien n'indiquait qu'on pouvait
  // cliquer sur un membre pour le nommer.
  if (etats.action) {
    ligne.append(el('button', {
      classe: 'bouton petit', type: 'button', texte: etats.action.texte,
      onclick: etats.action.faire,
    }));
  } else if (!personne.full_name) {
    ligne.append(el('button', {
      classe: 'bouton petit', type: 'button', texte: 'Nommer…',
      title: "C'est le nom complet qui nomme ses dépôts.",
      onclick: () => nommerUnMembre(personne),
    }));
  }
  return ligne;
}

// lienDeProfil rend un compte GitHub cliquable. C'est la personne qu'on lit
// derrière le compte, et la vérifier veut dire ouvrir sa page : l'hôte vient du
// contexte, parce qu'il n'est pas toujours github.com.
function lienDeProfil(compte) {
  const hote = (etat.contexte && etat.contexte.host) || 'github.com';
  return el('a', {
    classe: 'compte lien-compte', texte: '@' + compte,
    href: `https://${hote}/${encodeURIComponent(compte)}`,
    target: '_blank', rel: 'noreferrer noopener',
    title: `Ouvrir @${compte} sur ${hote}`,
  });
}

// boutonIcone rend une commande à son pictogramme. Un bouton sans texte n'a
// plus de nom : title et aria-label le lui rendent.
function boutonIcone(trace, titre, description, faire, ton = '') {
  return el('button', {
    classe: ('bouton petit icone ' + ton).trim(), type: 'button',
    title: titre, 'aria-label': description, onclick: faire,
  }, icone(trace));
}

// nommerUnMembre donne son nom complet à quelqu'un qui n'en a pas. C'est lui
// qui nommera ses dépôts : sans lui, aucun travail ne peut lui être distribué.
async function nommerUnMembre(personne) {
  const nom = el('input', { classe: 'champ', type: 'text', placeholder: 'Prénom Nom' });
  const corps = el('div', {},
    el('p', {}, el('code', { texte: '@' + personne.username }),
      " n'a pas de nom complet."),
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: 'Nom complet' }), nom,
      el('span', { classe: 'aide', texte:
        "C'est lui qui nommera ses dépôts, et il monte au registre de "
        + "l'organisation pour valoir partout." })));

  if (!await demander('Nommer cette personne', corps, 'Enregistrer')) return;
  const voulu = nom.value.trim();
  if (!voulu) return;
  const fiche = await tenter(() => api('POST',
    `/api/classrooms/${encode(etat.groupe.scope)}/students/rename`,
    { username: personne.username, full_name: voulu }), 'Nom complet');
  if (!fiche) return;
  message(`@${personne.username} s'appelle « ${voulu} ».`);
  await revoirApresChangement();
}

// revoirApresChangement relit la fiche du groupe et remet sous les yeux ce
// qu'on regardait. Nommer quelqu'un se fait aussi bien depuis un travail que
// depuis les équipes : ramener chaque fois dans Équipes ferait perdre sa place.
async function revoirApresChangement() {
  const vue = etat.vue;
  const travail = etat.travail;
  if (!await ouvrirGroupe(etat.groupe.scope, true, true)) return;
  if (vue === 'travail' && travail) {
    const encore = (etat.groupe.assignments || []).find((item) => item.id === travail.id);
    if (encore) {
      await ouvrirTravail(encore, true);
      return;
    }
  }
  afficherVue(vue === 'travail' ? 'travaux' : vue, true);
}

// listeDeComptes rend la liste des membres d'une équipe, telle qu'on la saisit.
function listeDeComptes(equipe) {
  return (equipe.members || []).join(', ');
}

// choixDesEtudiants dresse des cases à cocher pour les étudiants du groupe,
// avec ceux qui sont déjà ailleurs signalés : les déplacer est permis, mais il
// faut le savoir.
function choixDesEtudiants(coches, sauf) {
  const liste = el('div', { classe: 'liste-cases' });
  for (const personne of gens(etat.groupe)) {
    const ailleurs = etat.equipes.find((equipe) =>
      equipe.short !== sauf &&
      (equipe.members || []).some((compte) => compte.toLowerCase() === personne.username.toLowerCase()));
    liste.append(el('label', { classe: 'case' },
      el('input', {
        type: 'checkbox', value: personne.username,
        checked: coches.has(personne.username.toLowerCase()),
      }),
      el('span', {},
        personne.full_name ? personne.full_name + ' ' : '',
        el('span', { classe: 'compte', texte: '@' + personne.username }),
        ailleurs ? el('span', { classe: 'note', texte: ` — actuellement dans ${ailleurs.short}` }) : null)));
  }
  if (gens(etat.groupe).length === 0) {
    liste.append(el('p', { classe: 'note',
      texte: "Ce groupe n'a aucun étudiant : importez sa liste avant de composer des équipes." }));
  }
  return liste;
}

// comptesCoches relève les cases cochées d'un formulaire d'équipe.
function comptesCoches(racine) {
  return [...racine.querySelectorAll('input[type="checkbox"]')]
    .filter((coche) => coche.checked).map((coche) => coche.value);
}

async function nouvelleEquipe() {
  const nom = el('input', { classe: 'champ', type: 'text', placeholder: 'eq1' });
  const coches = new Set();
  const liste = choixDesEtudiants(coches, null);
  const corps = el('div', {},
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: "Nom de l'équipe" }), nom,
      el('span', { classe: 'aide' },
        "Sur GitHub, l'équipe s'appellera ",
        el('code', { texte: `${etat.groupe.scope}.eq1` }),
        " : la place du groupe fait partie du nom, sans quoi deux groupes ne " +
        "pourraient pas avoir chacun leur « eq1 ».")),
    el('p', { classe: 'etiquette', texte: 'Membres' }),
    liste);

  if (!await demander('Nouvelle équipe', corps, 'Créer')) return;
  const fiche = await tenter(() => api('POST',
    `/api/classrooms/${encode(etat.groupe.scope)}/teams`,
    { name: nom.value.trim(), members: comptesCoches(liste) }), 'Équipe');
  if (!fiche) return;
  message(`Équipe « ${fiche.team.short} » créée.`);
  await chargerEquipes(true);
}

async function composerEquipe(equipe) {
  const coches = new Set((equipe.members || []).map((compte) => compte.toLowerCase()));
  const liste = choixDesEtudiants(coches, equipe.short);
  const corps = el('div', {},
    el('p', { classe: 'note', texte:
      `Composition de « ${equipe.name} ». Quelqu'un qui appartenait à une autre ` +
      "équipe la quitte au passage : on n'est que d'une équipe à la fois." }),
    liste);

  if (!await demander(`Composer ${equipe.short}`, corps, 'Enregistrer')) return;
  const bilan = await tenter(() => api('POST',
    `/api/classrooms/${encode(etat.groupe.scope)}/teams/${encode(equipe.short)}/members`,
    { usernames: comptesCoches(liste) }), 'Composition');
  if (!bilan) return;
  message(bilan.steps.length === 0
    ? `« ${equipe.short} » était déjà ainsi composée.`
    : `${bilan.steps.length} changement(s) appliqué(s) à « ${equipe.short} ».`);
  etat.equipes = bilan.teams || [];
  etat.orphelins = bilan.unassigned || [];
  dessinerEquipes();
}

async function placerDansUneEquipe(personne) {
  if (etat.equipes.length === 0) {
    message("Ce groupe n'a aucune équipe : créez-en une d'abord.", 'alerte');
    return;
  }
  const choix = el('select', { classe: 'champ' },
    etat.equipes.map((equipe) => el('option', { value: equipe.short, texte: equipe.short })));
  // Le nom complet nomme ses dépôts : le placer dans une équipe sans l'avoir
  // remettrait le problème au moment de distribuer, où il bloque tout.
  const nom = el('input', {
    classe: 'champ', type: 'text', value: personne.full_name || '',
    placeholder: 'Prénom Nom',
  });
  const corps = el('div', {},
    el('p', {}, el('code', { texte: '@' + personne.username }), ' rejoint :'),
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: 'Équipe' }), choix),
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: 'Nom complet' }), nom,
      el('span', { classe: 'aide', texte: personne.full_name
        ? "C'est lui qui nommera ses dépôts."
        : "Inconnu pour l'instant : c'est lui qui nommera ses dépôts." })),
    el('p', { classe: 'note', texte:
      "L'accès aux dépôts de l'équipe suit : rien d'autre n'est à faire." }));

  if (!await demander('Placer dans une équipe', corps, 'Inscrire')) return;
  const voulu = nom.value.trim();
  if (voulu && voulu !== personne.full_name) {
    const corrige = await tenter(() => api('POST',
      `/api/classrooms/${encode(etat.groupe.scope)}/students/rename`,
      { username: personne.username, full_name: voulu }), 'Nom complet');
    if (!corrige) return;
  }
  const bilan = await tenter(() => api('POST',
    `/api/classrooms/${encode(etat.groupe.scope)}/teams/members`,
    { team: choix.value, usernames: [personne.username] }), 'Équipe');
  if (!bilan) return;
  message(`${voulu || '@' + personne.username} rejoint « ${choix.value} ».`);
  etat.equipes = bilan.teams || [];
  etat.orphelins = bilan.unassigned || [];
  if (voulu && voulu !== personne.full_name) await ouvrirGroupe(etat.groupe.scope, true);
  dessinerEquipes();
}

// inscrireUnMembre fait entrer dans le groupe un compte qu'une équipe porte
// sans que personne ne l'ait inscrit — ce qu'une reprise laisse derrière elle.
// Son nom complet est demandé au passage : c'est lui qui nommera ses dépôts.
async function inscrireUnMembre(personne) {
  const nom = el('input', {
    classe: 'champ', type: 'text', value: personne.full_name || '',
    placeholder: 'Prénom Nom',
  });
  // Le même nom porté par deux comptes, ce n'est pas toujours deux personnes :
  // le dire ici évite de les traiter comme des homonymes, ce qui arrêterait
  // toute distribution.
  const rattache = el('select', { classe: 'champ' },
    el('option', { value: '', texte: '— une personne de plus —' }),
    gens(etat.groupe).map((autre) => el('option', {
      value: autre.username,
      texte: (autre.full_name || '@' + autre.username) + ' (@' + autre.username + ')',
    })));
  const corps = el('div', {},
    el('p', {}, el('code', { texte: '@' + personne.username }),
      " est dans une équipe sans être dans la liste du groupe."),
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: 'Nom complet' }), nom),
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: 'Ou : autre compte de' }), rattache,
      el('span', { classe: 'aide', texte:
        "Une même personne travaille parfois sous deux comptes. Le dire ici lui "
        + "laisse un seul dépôt par travail, où elle est invitée sous les deux." })));

  if (!await demander("Inscrire ce compte", corps, 'Inscrire')) return;
  const fiche = rattache.value
    ? await tenter(() => api('POST',
        `/api/classrooms/${encode(etat.groupe.scope)}/students/accounts`,
        { username: rattache.value, account: personne.username }), 'Compte')
    : await tenter(() => api('POST',
        `/api/classrooms/${encode(etat.groupe.scope)}/students/add`,
        { username: personne.username, full_name: nom.value.trim() }), 'Étudiant');
  if (!fiche) return;
  message(rattache.value
    ? `@${personne.username} rattaché à la même personne.`
    : `@${personne.username} inscrit dans « ${etat.groupe.label} ».`);
  await ouvrirGroupe(etat.groupe.scope, true);
  await chargerEquipes(true);
}

async function renommerEquipe(equipe) {
  const nom = el('input', { classe: 'champ', type: 'text', value: equipe.short });
  const corps = el('div', {},
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: 'Nouveau nom' }), nom,
      el('span', { classe: 'aide', texte:
        "L'équipe garde ses membres et ses accès. Les dépôts déjà créés, eux, " +
        "gardent l'ancien nom : ils portent celui qu'elle avait au moment de la distribution." })),
    el('p', { classe: 'note' },
      el('code', { texte: equipe.name }), ' devient ',
      el('code', { texte: `${etat.groupe.scope}.` }),
      el('em', { texte: 'nouveau nom' })));

  if (!await demander(`Renommer ${equipe.short}`, corps, 'Renommer')) return;
  const fiche = await tenter(() => api('PUT',
    `/api/classrooms/${encode(etat.groupe.scope)}/teams/${encode(equipe.short)}`,
    { name: nom.value.trim() }), 'Renommage');
  if (!fiche) return;
  message(`« ${fiche.previous} » devient « ${fiche.team.short} ».`);
  await chargerEquipes(true);
}

// deplacerEquipe fait passer une équipe dans un autre groupe. Elle appartient à
// celui qu'elle a — son nom le dit, ses dépôts le portent —, et n'y va donc pas
// seule : ses membres et tout ce qu'ils ont rendu changent de groupe avec elle.
async function deplacerEquipe(equipe) {
  const { bloc, destination, preparer } = await choixDeGroupe();
  const membres = (equipe.people || []).length + (equipe.strangers || []).length;
  const depots = (etat.depotsDEquipe || {})[equipe.short] || 0;

  const confirme = await demander(`Déplacer ${equipe.short}`, el('div', {},
    bloc,
    el('p', { classe: 'note', texte:
      `${membres} membre(s) partent avec elle : une équipe appartient à son groupe, ` +
      'et ses membres en font partie.' }),
    el('p', { classe: 'note', texte: depots === 0
      ? "Ses dépôts et ceux de ses membres seront renommés pour porter la place du " +
        "groupe d'arrivée. GitHub garde une redirection depuis chaque ancien nom."
      : `Ses ${depots} dépôt(s) et ceux de ses membres seront renommés pour porter la ` +
        "place du groupe d'arrivée. GitHub garde une redirection depuis chaque " +
        'ancien nom.' })), 'Déplacer', preparer);
  if (!confirme) return;
  const cible = destination();
  if (!cible) return;

  const fiche = await tenter(() => api('POST',
    `/api/classrooms/${encode(etat.groupe.scope)}/teams/${encode(equipe.short)}/move`,
    cible), 'Déplacement');
  if (!fiche) return;
  // Sans dépôt à renommer, le serveur répond directement ; sinon c'est un
  // travail de fond, avec son journal.
  const bilan = fiche.id ? await suivre(fiche) : fiche;
  if (!bilan) return;
  // L'équipe ne suit que si tous ses dépôts sont arrivés : un déplacement
  // interrompu n'a rien déplacé, et le journal dit lesquels ont résisté.
  if (bilan.failed) {
    message(`« ${equipe.short} » n'a pas bougé · ${bilan.failed} dépôt(s) en échec`,
      'alerte');
  } else {
    message(`« ${equipe.short} » rejoint « ${bilan.target} »` +
      (bilan.created ? ' · groupe créé' : '') +
      (bilan.count ? ` · ${bilan.count} étudiant(s)` : '') +
      (bilan.renamed ? ` · ${bilan.renamed} dépôt(s) renommé(s)` : ''));
  }
  await chargerGroupes(true);
  await rafraichirGroupeEtEquipes();
}

async function supprimerEquipe(equipe) {
  const depots = (etat.depotsDEquipe || {})[equipe.short] || 0;
  const avecDepots = el('input', { type: 'checkbox' });
  const saisie = el('input', { type: 'text', classe: 'champ', placeholder: equipe.short });
  // La confirmation ne vaut que pour les dépôts : une équipe supprimée se
  // recrée, un dépôt effacé ne revient pas.
  const confirmation = el('div', { classe: 'champ-bloc' },
    el('span', { classe: 'etiquette', texte: `Retapez « ${equipe.short} » pour confirmer` }),
    saisie);
  confirmation.hidden = true;
  avecDepots.addEventListener('change', () => {
    confirmation.hidden = !avecDepots.checked;
    if (avecDepots.checked) saisie.focus();
  });

  const corps = el('div', {},
    el('p', { texte: `Supprimer « ${equipe.name} » ?` }),
    el('p', { classe: 'note', texte:
      "Ses membres restent dans le groupe : c'est l'accès que l'équipe donnait " +
      'qui disparaît.' }),
    depots === 0
      ? el('p', { classe: 'aide', texte: "Elle n'a encore rendu aucun dépôt." })
      : el('div', {},
          el('label', { classe: 'case' }, avecDepots,
            el('span', {},
              el('strong', { texte: depots === 1
                ? 'Supprimer aussi son dépôt sur GitHub'
                : `Supprimer aussi ses ${depots} dépôts sur GitHub` }),
              el('span', { classe: 'aide', texte:
                "Suppression définitive : le contenu, les tickets et l'historique " +
                'seront perdus.' }))),
          confirmation));

  if (!await demander('Supprimer une équipe', corps, 'Supprimer')) return;
  const bilan = await tenter(() => api('DELETE',
    `/api/classrooms/${encode(etat.groupe.scope)}/teams/${encode(equipe.short)}`,
    { repos: avecDepots.checked, confirm: saisie.value.trim() }),
    'Suppression');
  if (!bilan) return;
  message(bilan.message);
  await rafraichirGroupeEtEquipes();
}

// rafraichirGroupeEtEquipes relit la fiche du groupe sans quitter l'onglet :
// une équipe supprimée avec ses dépôts change à la fois le nombre d'équipes et
// la liste des travaux, que l'entête et l'onglet Travaux montrent encore.
async function rafraichirGroupeEtEquipes() {
  const groupe = etat.groupe && await tenter(() => api('GET',
    `/api/classrooms/${encode(etat.groupe.scope)}?refresh=1`), 'Groupe');
  if (!groupe) {
    await chargerEquipes(true);
    return;
  }
  etat.groupe = groupe;
  dessinerTravaux();
  // Réafficher la vue courante redessine l'entête et recharge les équipes.
  afficherVue('equipes', true);
}

// adopterEquipe fait entrer dans le groupe une équipe déjà présente dans
// l'organisation, en la renommant. C'est ce qui permet de reprendre un travail
// d'équipe commencé sans l'outil : l'équipe garde ses membres et ses accès.
async function adopterEquipe() {
  const libres = await tenter(() => api('GET',
    `/api/orgs/${encode(etat.groupe.org)}/teams`), 'Équipes');
  if (!libres) return;
  if ((libres.teams || []).length === 0) {
    message("Aucune équipe de l'organisation n'est libre d'un groupe.", 'alerte');
    return;
  }

  const source = el('select', { classe: 'champ' },
    libres.teams.map((equipe) => el('option', {
      value: equipe.slug, texte: `${equipe.name} (${equipe.members.length} membre(s))`,
    })));
  const nom = el('input', { classe: 'champ', type: 'text', placeholder: 'eq1' });
  const corps = el('div', {},
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: "Équipe de l'organisation" }), source),
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: `Nom dans « ${etat.groupe.label} »` }), nom),
    el('p', { classe: 'note', texte:
      "Adopter, c'est renommer : l'équipe garde ses membres, ses accès et son " +
      "histoire, et devient lisible pour l'outil." }));

  if (!await demander('Adopter une équipe', corps, 'Adopter')) return;
  const fiche = await tenter(() => api('POST',
    `/api/classrooms/${encode(etat.groupe.scope)}/teams/adopt`,
    { slug: source.value, name: nom.value.trim() }), 'Adoption');
  if (!fiche) return;
  message(`« ${fiche.previous} » rejoint le groupe sous le nom « ${fiche.team.short} ».`);
  await chargerEquipes(true);
}

// -------------------------------------------------------- réglages du groupe

const champsGroupe = {
  description_pattern: 'gr-description',
  template: 'gr-template',
  visibility: 'gr-visibilite',
  permission: 'gr-permission',
};

function ecrireReglagesGroupe() {
  const groupe = etat.groupe;
  preparerMigration(groupe);
  const defauts = groupe.defaults || {};
  for (const [cle, id] of Object.entries(champsGroupe)) {
    $(id).value = defauts[cle] || '';
  }
  $('gr-collaborateur').checked = defauts.add_collaborator !== false;
}

// enregistrerGroupe renvoie tout ce qu'on retient du groupe : le serveur
// remplace la fiche, et taire un champ l'effacerait. Sa place n'y figure pas —
// elle vient de l'adresse, et la changer renommerait des dépôts.
async function enregistrerGroupe(modifications) {
  const groupe = etat.groupe;
  return tenter(() => api('PUT', `/api/classrooms/${encode(groupe.scope)}`, Object.assign({
    session: groupe.session || '',
    course: groupe.course || '',
    group: groupe.group || '',
    prefix: groupe.prefix || '',
    pattern: groupe.pattern || '',
    students: groupe.students,
    roster_path: groupe.roster_path || '',
    defaults: groupe.defaults || {},
  }, modifications)), 'Groupe');
}

$('gr-enregistrer').addEventListener('click', async () => {
  const defauts = Object.assign({}, etat.groupe.defaults);
  for (const [cle, id] of Object.entries(champsGroupe)) {
    defauts[cle] = $(id).value.trim();
  }
  defauts.add_collaborator = $('gr-collaborateur').checked;

  const modifie = await enregistrerGroupe({ defaults: defauts });
  if (!modifie) return;
  message('Réglages du groupe enregistrés.');
  await ouvrirGroupe(modifie.scope, true, true);
  afficherVue('groupe-reglages');
});

// ------------------------------------------------------------------ migration

// preparerMigration remplit l'écran qui renomme les dépôts d'un groupe. Le
// mécanisme est le même qu'on vienne d'une nomenclature dépassée ou qu'on
// change simplement de session : ce sont les mots qui diffèrent.
function preparerMigration(groupe) {
  const herite = !groupe.session;
  $('migration-titre').textContent = herite
    ? 'Migrer vers la nomenclature courante'
    : 'Renommer ou déplacer le groupe';
  $('migration-note').textContent = herite
    ? "Les dépôts de ce groupe ne suivent pas la nomenclature courante. Les renommer leur " +
      'donne une place dans la hiérarchie, et rend la distribution possible.'
    : 'Changer la session, le cours ou le numéro du groupe renomme tous ses dépôts. GitHub ' +
      'garde une redirection depuis chaque ancien nom : les clones déjà faits continuent de ' +
      'fonctionner.';

  oublierApercuMigration();

  if (!herite) {
    $('mig-session').value = groupe.session;
    $('mig-cours').value = groupe.course;
    $('mig-section').value = groupe.group;
  } else {
    // « a26-5n6 » suit l'habitude « session-cours » : le premier segment fait
    // la session, le dernier le cours. C'est une proposition, pas une règle.
    const prefixe = groupe.prefix || '';
    const segments = prefixe.split(/[-.]/).filter(Boolean);
    $('mig-session').value = segments.length > 1 ? segments[0] : '';
    $('mig-cours').value = segments.length > 1 ? segments[segments.length - 1] : prefixe;
    $('mig-section').value = '01';
  }
  majApercuMigration();
}

function majApercuMigration() {
  const session = $('mig-session').value.trim() || 'session';
  const cours = $('mig-cours').value.trim() || 'cours';
  const section = $('mig-section').value.trim() || 'groupe';
  $('mig-apercu').textContent = `${session}.${cours}.${section}.<travail>.<étudiant>`;
}

for (const id of ['mig-session', 'mig-cours', 'mig-section']) {
  $(id).addEventListener('input', majApercuMigration);
}

function corpsMigration() {
  return {
    session: $('mig-session').value.trim(),
    course: $('mig-cours').value.trim(),
    group: $('mig-section').value.trim(),
    skip_blocked: $('mig-ignorer').checked,
  };
}

// Le dernier aperçu obtenu. La case « laisser en place » ne change pas le plan
// — les mêmes dépôts sont bloqués qu'on l'accepte ou non —, elle change ce qu'on
// en fait : garder l'aperçu permet de redire les conséquences du choix sans
// redemander le plan au serveur.
let apercuMigration = null;

// oublierApercuMigration ramène l'écran à avant l'aperçu : changer la place
// d'arrivée périme le plan affiché.
function oublierApercuMigration() {
  apercuMigration = null;
  $('mig-table').hidden = true;
  $('mig-ignorer').checked = false;
  $('mig-resume').textContent = '';
  majAvisMigration();
}

// majAvisMigration dit ce que l'aperçu implique, la case comprise, et n'ouvre le
// bouton que si la migration peut aboutir. La case ne se montre que lorsqu'elle
// a quelque chose à décider : la proposer quand aucun dépôt n'est bloqué, c'est
// offrir un choix sans effet.
function majAvisMigration() {
  const apercu = apercuMigration;
  const bloques = apercu ? apercu.blocked : 0;
  const ignorer = $('mig-ignorer').checked;
  $('mig-ignorer-case').hidden = bloques === 0;
  vider($('mig-avis'));
  if (!apercu) {
    $('mig-lancer').disabled = true;
    return;
  }
  if (bloques && !ignorer) {
    $('mig-avis').append(el('div', { classe: 'avis alerte',
      texte: `${bloques} dépôt(s) ne peuvent pas être renommés. Complétez la liste des ` +
        "étudiants — comptes manquants, noms complets à retrouver — ou acceptez de les laisser " +
        'en place.' }));
  } else if (apercu.ready === 0) {
    $('mig-avis').append(el('div', { classe: 'avis alerte', texte: 'Aucun dépôt à renommer.' }));
  } else {
    $('mig-avis').append(el('div', { classe: 'avis',
      texte: (bloques ? `${bloques} dépôt(s) garderont leur nom actuel. ` : '') +
        phraseBascule(apercu) }));
  }
  $('mig-lancer').disabled = apercu.ready === 0 || (bloques > 0 && !ignorer);
}

// phraseBascule dit ce que devient le groupe lui-même. C'est le serveur qui en
// décide — un groupe ne suit ses dépôts que si aucun ne reste en arrière — et
// l'aperçu le rapporte, plutôt que de le redéduire ici.
function phraseBascule(apercu) {
  if (apercu.switch) return `Le groupe devient « ${apercu.scope} ».`;
  return `Le groupe reste « ${etat.groupe.label} » : c'est ainsi qu'il continue de les voir. ` +
    `Les dépôts renommés apparaîtront à part, sous « ${apercu.scope} ».`;
}

$('mig-apercu-bouton').addEventListener('click', async () => {
  const apercu = await tenter(() => api('POST',
    `/api/classrooms/${encode(etat.groupe.scope)}/migration/preview`, corpsMigration()), 'Migration');
  if (!apercu) return;

  const corps = $('mig-table').querySelector('tbody');
  vider(corps);
  for (const ligne of apercu.rows) {
    corps.append(el('tr', {},
      el('td', {}, el('code', { texte: ligne.repo })),
      ligne.target
        ? el('td', {}, el('code', { texte: ligne.target }))
        : el('td', { classe: 'vide', texte: ligne.problem })));
  }
  $('mig-table').hidden = apercu.rows.length === 0;
  $('mig-resume').textContent = `${apercu.ready} dépôt(s) à renommer` +
    (apercu.blocked ? `, ${apercu.blocked} bloqué(s)` : '');
  apercuMigration = apercu;
  majAvisMigration();
});

for (const id of ['mig-session', 'mig-cours', 'mig-section']) {
  $(id).addEventListener('change', oublierApercuMigration);
}

// La case ne périme pas l'aperçu : elle ne touche pas au plan, seulement à ce
// qu'on décide d'en faire. La relire suffit.
$('mig-ignorer').addEventListener('change', majAvisMigration);

$('mig-lancer').addEventListener('click', async () => {
  const corps = corpsMigration();
  const confirme = await demander('Renommer les dépôts', el('div', {},
    el('p', { texte: `Les dépôts de « ${etat.groupe.label} » seront renommés en ` +
      `« ${corps.session}.${corps.course}.${corps.group}.travail.étudiant ».` }),
    el('p', { classe: 'note',
      texte: 'GitHub garde une redirection depuis chaque ancien nom : les clones et les liens ' +
        'déjà distribués continuent de fonctionner.' })), 'Renommer');
  if (!confirme) return;

  const fiche = await tenter(() => api('POST',
    `/api/classrooms/${encode(etat.groupe.scope)}/migration/apply`, corps), 'Migration');
  if (!fiche) return;
  const bilan = await suivre(fiche);
  if (!bilan) return;

  journaliser(`${bilan.renamed} renommé(s) · ${bilan.skipped} laissé(s) en place · ` +
    `${bilan.failed} en échec`, bilan.failed ? 'warn' : 'ok');
  if (!bilan.switched) {
    journaliser(`Le groupe reste « ${etat.groupe.label} » : des dépôts sont restés en arrière, ` +
      'et il continue de les voir.', 'warn');
  }
  await ouvrirGroupe(etat.groupe.scope, true, true);
  afficherVue('groupe-reglages');
});

$('gr-supprimer').addEventListener('click', async () => {
  const confirme = await demander(`Oublier « ${etat.groupe.label} » ?`, el('div', {},
    el('p', { texte: 'La liste des étudiants et les réglages retenus pour ce groupe sont ' +
      'oubliés.' }),
    el('p', { classe: 'note',
      texte: "Aucun dépôt n'est supprimé sur GitHub. S'il en reste, le groupe continue de " +
        "s'afficher — sans sa liste." })),
    'Oublier');
  if (!confirme) return;

  const fait = await tenter(() =>
    api('DELETE', `/api/classrooms/${encode(etat.groupe.scope)}`), 'Suppression');
  if (!fait) return;
  message(fait.message, 'succes', 10000);
  etat.groupe = null;
  afficherVue('parcours');
});

// ------------------------------------------------------- réglages généraux

async function rafraichirEmplacements() {
  const info = await organisations();
  remplirSelecteur($('reglages-org'), info.orgs || [], etat.organisation);
  $('reglages-org-libre-bloc').hidden = $('reglages-org').value !== '__saisir';
  $('reglages-org-libre').value = etat.organisation || '';

  const contexte = await api('GET', '/api/context').catch(() => null);
  if (!contexte) return;
  etat.contexte = contexte;
  dessinerPortees(contexte.token);
  dessinerChemins(contexte.paths);
  ecrireReglagesGeneraux();
}

$('reglages-org').addEventListener('change', () => {
  const choisie = $('reglages-org').value;
  $('reglages-org-libre-bloc').hidden = choisie !== '__saisir';
  if (choisie !== '__saisir') changerOrganisation(choisie);
});

$('reglages-org-libre').addEventListener('change', () => {
  const saisie = $('reglages-org-libre').value.trim();
  if (saisie) changerOrganisation(saisie);
});

// changerOrganisation remet la navigation à zéro : les groupes déclarés
// ailleurs restent en place, mais ne concernent plus cet écran.
async function changerOrganisation(org) {
  const avis = $('reglages-org-avis');
  vider(avis);
  const details = await tenter(() => api('GET', `/api/orgs/${encode(org)}`), 'Organisation');
  if (!details) return;
  if (details.warning) {
    avis.append(el('div', { classe: 'avis alerte', texte: details.warning }));
  }
  await retenirOrganisation(details.login);
  etat.parcours = { session: '', cours: '' };
  etat.groupe = null;
  message(`Organisation : ${details.name}.`);
}

$('reglages-enregistrer').addEventListener('click', async () => {
  etat.reglages.delay_seconds = Number($('reglage-delay').value) || 0;
  etat.reglages.clone_dir = $('reglage-clone-dir').value.trim();
  const bilan = await tenter(() => api('PUT', '/api/settings', etat.reglages), 'Réglages');
  if (!bilan) return;
  $('reglages-etat').textContent = bilan.saved
    ? `Mémorisés dans ${bilan.path}`
    : 'Mémorisation désactivée (--no-save-config)';
});

$('cache-vider').addEventListener('click', async () => {
  const bilan = await tenter(() => api('POST', '/api/cache/clear'), 'Cache');
  if (!bilan) return;
  message(`Cache vidé (${bilan.removed} entrée(s)).`);
  dessinerChemins(bilan.paths);
});

// ------------------------------------------------- reprise de dépôts

// Des dépôts qu'une autre convention a nommés — « travail-compte », ce que
// GitHub Classroom produit. L'écran suit l'ordre des questions : quel travail,
// quelle liste, quelle place. Le rapprochement des comptes est montré avant
// d'écrire, avec la raison qui l'a produit : une suggestion et une preuve ne se
// lisent pas de la même façon.

let importPlan = null;
let importTravail = '';
// Les noms de la liste, tous, et le compte auquel chacun est associé. C'est
// cette association-là qu'on corrige à l'écran : le reste en découle.
let importNoms = [];
let importChoix = new Map();
let importAttente = null;
// Les dépôts du travail, et ceux qu'on garde. Tout est coché d'entrée : écarter
// un dépôt est le geste rare, et le reste du parcours n'en connaît que la
// sortie — une liste de noms, vide quand rien n'a été touché.
let importDepots = [];
let importRetenus = new Set();
// Passer la liste est un choix, et il faut le distinguer d'une liste qu'on n'a
// pas encore fournie : l'une laisse continuer, l'autre est un oubli.
let importSansListe = false;

// Un travail d'équipe se reprend autrement : ce qui suit le préfixe nomme une
// équipe, ses membres viennent des accès au dépôt, et il n'y a pas de liste à
// rapprocher.
let importEquipe = false;

// Ce qu'on a tranché à l'écran, équipe par équipe. Une entrée ici a le dernier
// mot : le serveur ne redevine plus rien pour cette équipe, y compris quand on
// a choisi de n'y mettre personne.
let importCrews = new Map();

// --- l'accordéon

// Une seule étape ouverte à la fois : la page garde la même hauteur, que le
// groupe compte cinq personnes ou cinquante.
function ouvrirEtape(nom) {
  for (const etape of document.querySelectorAll('#import-stepper .etape')) {
    etape.classList.toggle('ouverte', etape.dataset.etape === nom);
  }
}

function etape(nom) {
  return document.querySelector(`#import-stepper .etape[data-etape="${nom}"]`);
}

// marquerEtape écrit ce qu'une étape a retenu, et la dit faite si elle l'est.
function marquerEtape(nom, resume) {
  $('resume-' + nom).textContent = resume;
  etape(nom).classList.toggle('faite', !!resume);
}

for (const tete of document.querySelectorAll('#import-stepper .etape-tete')) {
  tete.addEventListener('click', () => {
    const bloc = tete.closest('.etape');
    ouvrirEtape(bloc.classList.contains('ouverte') ? '' : bloc.dataset.etape);
  });
}

// --- 1. le travail

// viderImport ramène l'assistant à son premier écran. Effacer les seuls résumés
// des étapes ne suffit pas : leurs corps sont repliés, pas vides, et une reprise
// rouverte montrerait la liste, la place et les rapprochements de la
// précédente — que « Vérifier » renverrait au serveur.
function viderImport() {
  // Une correction faite juste avant de partir a pu laisser une vérification en
  // vol : elle redessinerait l'écran qu'on vient de vider.
  clearTimeout(importAttente);
  importTravail = '';
  importPlan = null;
  importNoms = [];
  importChoix = new Map();
  importDevinee = {};
  importDepots = [];
  importRetenus = new Set();
  importSansListe = false;
  importEquipe = false;
  importCrews = new Map();
  $('import-individuel').checked = true;
  $('import-equipe').checked = false;
  majNatureImport();

  vider($('import-travaux'));
  vider($('import-depots').querySelector('tbody'));
  $('import-depots-compte').textContent = '';
  $('import-depots-tout').checked = true;
  dire('import-deja-nommes', '');
  viderDepot('import-liste');
  for (const id of ['import-session', 'import-cours', 'import-groupe', 'import-nom']) {
    $(id).value = '';
  }
  $('import-nommes').checked = false;
  $('import-filtre').value = 'tout';
  $('import-suite-bloc').hidden = false;
  vider($('import-rapprochements').querySelector('tbody'));
  vider($('import-renommages').querySelector('tbody'));
  vider($('import-avis'));
  vider($('import-journal'));
  $('import-barre').value = 0;
  $('import-resume').textContent = '';
  $('import-compte').textContent = '';
  $('import-etat').textContent = '';
  dire('import-place-note', '');
  for (const nom of ['travail', 'depots', 'liste', 'place', 'verifier', 'noms', 'journal']) {
    marquerEtape(nom, '');
  }
  ouvrirEtape('travail');
}

async function preparerImport() {
  const org = etat.organisation;
  if (!org) return;
  // Vidé avant d'aller chercher : une reprise qui échoue laisserait sinon
  // l'écran de la précédente, intact et trompeur.
  viderImport();
  const vue = await tenter(() => api('GET', `/api/orgs/${encode(org)}/foreign`), 'Reprise');
  if (!vue) return;

  $('import-aide-texte').textContent = vue.help || '';
  const travaux = vue.assignments || [];
  const conteneur = $('import-travaux');

  $('import-resume').textContent = travaux.length
    ? `${vue.repos.length} dépôt(s) ne suivent pas la nomenclature. Choisissez le travail à reprendre.`
    : `${vue.repos.length} dépôt(s) hors nomenclature, mais aucun préfixe commun : `
      + "il n'y a rien à reprendre d'un bloc.";

  for (const travail of travaux) {
    conteneur.append(el('label', { classe: 'case' },
      el('input', {
        type: 'radio', name: 'import-travail', value: travail.prefix,
        onchange: () => {
          importTravail = travail.prefix;
          if (!$('import-nom').value.trim()) $('import-nom').value = travail.prefix;
          marquerEtape('travail', `${travail.prefix} · ${travail.count} dépôt(s)`);
          chargerDepots(travail.prefix);
        },
      }),
      el('span', {}, el('code', { texte: travail.prefix }),
        el('span', { classe: 'jeton', texte: `${travail.count} dépôt(s)` }))));
  }
}

// --- 2. les dépôts à reprendre

// chargerDepots demande les dépôts du travail choisi et les coche tous. Ils
// viennent du serveur plutôt que d'un filtrage à l'écran : savoir quels dépôts
// portent un préfixe est une règle de lecture des noms, et elle n'a pas à être
// réécrite ici.
async function chargerDepots(prefixe, ouvrir = true) {
  const vue = await tenter(() => api('POST',
    `/api/orgs/${encode(etat.organisation)}/import/repos`, { prefix: prefixe }), 'Dépôts');
  if (!vue) return;
  importDepots = vue.repos || [];
  importRetenus = new Set(importDepots.map((depot) => depot.name));
  dessinerDepots();
  if (ouvrir) ouvrirEtape('depots');
}

function dessinerDepots() {
  const corps = $('import-depots').querySelector('tbody');
  vider(corps);
  for (const depot of importDepots) {
    const coche = el('input', {
      type: 'checkbox', checked: importRetenus.has(depot.name),
      onchange: (evenement) => {
        if (evenement.target.checked) importRetenus.add(depot.name);
        else importRetenus.delete(depot.name);
        majDepots();
      },
    });
    // Le nom mène au dépôt : c'est le seul moyen de trancher pour de bon,
    // sans quitter la reprise pour aller le chercher sur GitHub.
    const lien = el('a', {
      href: depot.url, target: '_blank', rel: 'noopener noreferrer',
    }, el('code', { texte: depot.name }));
    corps.append(el('tr', {},
      el('td', { classe: 'etroit' }, el('label', { classe: 'case' }, coche)),
      el('td', {}, lien),
      importEquipe
        ? el('td', {}, el('strong', { texte: equipeDe(depot.name) }))
        : (depot.student
          ? el('td', { texte: depot.student })
          : el('td', { classe: 'vide', texte: depot.login ? '@' + depot.login : '—' })),
      el('td', { classe: 'note', texte: depot.pushed_at || 'jamais' })));
  }
  majDepots();
}

// majDepots redit ce que la sélection garde, et remet la case du bandeau
// d'accord avec elle.
function majDepots() {
  const total = importDepots.length;
  const gardes = importRetenus.size;
  direDejaNommes();
  $('import-depots-compte').textContent = gardes === total
    ? `${total} dépôt(s) — tous repris`
    : `${gardes} dépôt(s) sur ${total}`;
  $('import-depots-tout').checked = gardes === total && total > 0;
  $('import-depots-tout').indeterminate = gardes > 0 && gardes < total;
  marquerEtape('depots', $('import-depots-compte').textContent);
  // Une sélection changée périme ce qui en découlait : les rapprochements
  // regardaient d'autres dépôts.
  importPlan = null;
  marquerEtape('verifier', '');
  marquerEtape('noms', '');
}

// direDejaNommes dit ce que l'organisation sait déjà des dépôts retenus. C'est
// ce qui répond à la seule question de l'étape suivante : cette liste
// a-t-elle encore quelque chose à apprendre ?
function direDejaNommes() {
  const retenus = importDepots.filter((depot) => importRetenus.has(depot.name));
  const nommes = retenus.filter((depot) => depot.student).length;
  if (retenus.length === 0) {
    dire('import-deja-nommes', '');
    return;
  }
  if (nommes === retenus.length) {
    dire('import-deja-nommes', `Les ${retenus.length} dépôt(s) retenus sont déjà associés à `
      + "un étudiant connu de l'organisation : la liste n'a rien à apprendre de plus.");
    return;
  }
  dire('import-deja-nommes', `${nommes} des ${retenus.length} dépôt(s) retenus sont déjà `
    + "associés à un étudiant connu de l'organisation ; la liste nommera les autres.");
}

// majNatureImport accorde l'écran à la nature du travail : l'étape de la liste
// n'a plus lieu d'être, et ce qu'on vérifie n'est plus le même.
function majNatureImport() {
  $('import-colonne').textContent = importEquipe ? 'Équipe' : 'Étudiant';
  // La liste sert dans les deux cas : les équipes disent qui a fait le travail,
  // elle dit comment ces gens s'appellent.
  $('import-equipes-bloc').hidden = !importEquipe;
  $('import-nommes-bloc').hidden = importEquipe;
  document.querySelector('#import-stepper .etape[data-etape="verifier"] .etape-titre')
    .textContent = importEquipe
      ? 'Vérifier les équipes et les noms'
      : 'Vérifier les rapprochements';
  dire('import-noms-note', importEquipe
    ? "Les dépôts prendront le nom de leur équipe. Chaque équipe sera créée sur "
      + "GitHub sous la nomenclature du groupe, et recevra le sien."
    : "Les dépôts prendront le nom complet de l'étudiant. Un compte laissé sans "
      + 'personne gardera le sien.');
  dire('import-suite-note', importEquipe
    ? "La reprise s'y lance."
    : "Le sort des dépôts sans étudiant s'y décide, et la reprise s'y lance.");
  renumeroterImport();
}

// renumeroterImport renumérote les étapes visibles. Sauter celle de la liste
// laisserait sinon lire « 1, 2, 4 » : un trou dans un compte se lit comme une
// étape manquée, pas comme une étape sans objet.
function renumeroterImport() {
  let rang = 0;
  for (const bloc of document.querySelectorAll('#import-stepper .etape')) {
    if (bloc.hidden) continue;
    rang += 1;
    bloc.querySelector('.etape-num').textContent = String(rang);
  }
}

for (const id of ['import-individuel', 'import-equipe']) {
  $(id).addEventListener('change', () => {
    importEquipe = $('import-equipe').checked;
    // Ce qui découlait de l'autre nature ne vaut plus rien.
    importPlan = null;
    importNoms = [];
    importChoix = new Map();
    importCrews = new Map();
    marquerEtape('verifier', '');
    marquerEtape('noms', '');
    majNatureImport();
    dessinerDepots();
  });
}

$('import-liste-passer').addEventListener('click', () => {
  viderDepot('import-liste');
  importSansListe = true;
  marquerEtape('liste', 'sans liste');
  importNoms = [];
  importChoix = new Map();
  marquerEtape('verifier', '');
  ouvrirEtape('place');
  devinerPlace();
});

$('import-depots-tout').addEventListener('change', () => {
  const tout = $('import-depots-tout').checked;
  importRetenus = new Set(tout ? importDepots.map((depot) => depot.name) : []);
  dessinerDepots();
});

$('import-depots-suite').addEventListener('click', () => {
  if (importRetenus.size === 0) {
    message('Aucun dépôt retenu : cochez-en au moins un.', 'alerte');
    return;
  }
  if (!$('import-liste').value.trim()) {
    ouvrirEtape('liste');
    return;
  }
  ouvrirEtape('place');
  devinerPlace();
});

// equipeDe lit le nom de l'équipe dans celui du dépôt : ce qui suit le préfixe,
// et rien de plus. Le serveur le slugifiera ; l'écran le montre tel quel.
function equipeDe(depot) {
  const prefixe = (importTravail || '').toLowerCase();
  const nom = depot.toLowerCase();
  if (!prefixe || !nom.startsWith(prefixe) || nom.length <= prefixe.length + 1) {
    return depot;
  }
  return depot.slice(prefixe.length + 1);
}

// selection rend les dépôts retenus, ou rien quand ils le sont tous : le plan
// n'a pas à distinguer « tout coché » de « pas encore touché ».
function selection() {
  if (importRetenus.size === 0 || importRetenus.size === importDepots.length) return [];
  return importDepots.map((depot) => depot.name).filter((nom) => importRetenus.has(nom));
}

// --- 3. la liste

$('import-liste').addEventListener('change', async () => {
  const valeur = $('import-liste').value.trim();
  importSansListe = false;
  marquerEtape('liste', valeur);
  // Une liste qui change annule les rapprochements déjà retenus : ils
  // désignaient les noms de l'ancienne.
  importNoms = [];
  importChoix = new Map();
  marquerEtape('verifier', '');
  if (!valeur || !importTravail) return;
  ouvrirEtape('place');
  await devinerPlace();
});

// laListe rassemble ce qui désigne la liste : son chemin quand elle en a un,
// son contenu quand elle a été déposée, et son nom dans les deux cas — c'est
// lui qui porte le cours et le groupe.
function laListe() {
  return {
    prefix: importTravail,
    only: selection(),
    path: cheminDepot('import-liste'),
    filename: $('import-liste').value.trim(),
    content: contenuDepot('import-liste') || null,
  };
}

// --- 4. la place, devinée puis corrigée

// Les trois champs arrivent préremplis : le cours est dans le nom du fichier
// d'Omnivox, le groupe dans la colonne qui le porte, et la session dans la
// date du plus vieux commit du travail. Rien n'est imposé — une devinette
// propose, elle ne reprend jamais la main sur ce qu'on a tapé.
let importDevinee = {};

async function devinerPlace() {
  const place = await api('POST',
    `/api/orgs/${encode(etat.organisation)}/import/place`, laListe()).catch(() => null);
  // Ne pas savoir deviner n'est pas une panne : les champs restent à remplir.
  if (!place) return;

  poser('import-session', place.session, importDevinee.session);
  poser('import-cours', place.course, importDevinee.course);
  poser('import-groupe', place.group, importDevinee.group);
  importDevinee = place;

  if ((place.groups || []).length) {
    dire('import-place-note', 'La liste mêle les groupes ' + place.groups.join(', ')
      + ' : indiquez celui qui reçoit ces dépôts.');
    return;
  }
  const devines = [place.session, place.course, place.group].filter(Boolean).length;
  dire('import-place-note', devines
    ? 'Prérempli depuis la liste et le premier commit du travail. Corrigez au besoin.'
    : '');
}

// dire écrit une note, et l'efface de la mise en page quand elle n'a rien à
// dire : une ligne vide compte comme un bloc dans une colonne espacée.
function dire(id, texte) {
  const note = $(id);
  note.textContent = texte;
  note.hidden = !texte;
}

// poser remplit un champ deviné sans effacer ce qu'on a tapé soi-même.
function poser(id, valeur, ancienne) {
  const champ = $(id);
  const actuel = champ.value.trim();
  if (actuel && actuel !== ancienne) return;
  champ.value = valeur || '';
}

// --- 5. la vérification

// corpsImport rassemble ce que les premières étapes disent.
function corpsImport() {
  const manque = [];
  if (!importTravail) manque.push('un travail');
  if (!importSansListe && !$('import-liste').value.trim()) {
    manque.push('la liste des étudiants');
  }
  const session = $('import-session').value.trim();
  const cours = $('import-cours').value.trim();
  const groupe = $('import-groupe').value.trim();
  if (!session || !cours || !groupe) manque.push("la place d'arrivée");
  if (manque.length) {
    $('import-etat').textContent = 'Il manque ' + manque.join(', ') + '.';
    return null;
  }
  $('import-etat').textContent = '';

  const corps = {
    prefix: importTravail,
    only: selection(),
    name: $('import-nom').value.trim() || importTravail,
    scope: [session, cours, groupe].join('.'),
    path: cheminDepot('import-liste'),
    filename: $('import-liste').value.trim(),
    content: contenuDepot('import-liste') || null,
    named_only: $('import-nommes').checked,
    teams: importEquipe,
  };
  if (importEquipe && importCrews.size) {
    corps.crews = Object.fromEntries(importCrews);
  }
  // Dès qu'un rapprochement a été touché, c'est l'écran qui fait foi : le
  // serveur ne redevine plus rien, y compris là où on a choisi « personne ».
  if (importNoms.length) {
    corps.people = importNoms.map((nom) => ({ full_name: nom, username: compteDe(nom) }));
  }
  return corps;
}

// compteDe rend le compte associé à un nom, ou rien.
function compteDe(nom) {
  for (const [login, retenu] of importChoix) if (retenu === nom) return login;
  return '';
}

$('import-apercu').addEventListener('click', () => verifier(true));

// verifier redemande le plan et redessine ce qui doit l'être. Le tableau des
// rapprochements, lui, n'est refait qu'à la première lecture : le refaire
// effacerait les choix en cours.
async function verifier(complet) {
  const corps = corpsImport();
  if (!corps) return;
  const plan = await tenter(
    () => api('POST', `/api/orgs/${encode(etat.organisation)}/import/preview`, corps),
    'Reprise');
  if (!plan) return;
  importPlan = plan;
  marquerEtape('place', plan.scope);
  // Un travail d'équipe n'a rien à rapprocher : ce qu'il y a à vérifier, c'est
  // la composition de chaque équipe.
  if (plan.team_work) {
    $('import-suite-bloc').hidden = false;
    dessinerEquipesReprises(plan);
    // Les comptes se nomment comme ailleurs : c'est le même rapprochement, le
    // même tableau, et les mêmes corrections.
    if (complet) {
      lireNoms(plan);
      dessinerRapprochements(plan);
    }
    compter();
    dessinerRenommages(plan);
    if (complet) ouvrirEtape('verifier');
    return;
  }
  // Le préfixe deviné cachait plusieurs travaux : les accès viennent de le
  // dire, et il n'y a rien à rapprocher tant qu'on n'a pas choisi lequel.
  if ((plan.splits || []).length > 1) {
    montrerTravauxCaches(plan);
    ouvrirEtape('verifier');
    return;
  }
  $('import-suite-bloc').hidden = false;
  if (complet) {
    lireNoms(plan);
    dessinerRapprochements(plan);
  }
  dessinerAvis(plan);
  dessinerRenommages(plan);
  compter();
  if (complet) ouvrirEtape('verifier');
}

// dessinerEquipesReprises montre ce que la reprise reconstituera : une équipe
// par dépôt, ses membres tels que les accès les donnent, et si elle est déjà là.
function dessinerEquipesReprises(plan) {
  const equipes = plan.teams || [];
  const corps = $('import-equipes').querySelector('tbody');
  vider(corps);
  for (const equipe of equipes) {
    const membres = equipe.members || [];
    const sources = equipe.sources || {};
    corps.append(el('tr', {},
      el('td', {}, el('strong', { texte: equipe.short })),
      el('td', {}, el('code', { texte: equipe.repo })),
      membres.length
        // D'où vient chaque membre se lit au survol : une composition devinée
        // doit pouvoir être démentie, et pour cela il faut voir sur quoi elle
        // repose.
        ? el('td', {}, el('span', { classe: 'etiquettes' }, membres.map((compte) =>
            el('span', {
              classe: 'jeton' + (sources[compte] === 'choisi' ? ' choisi' : ''),
              texte: '@' + compte,
              title: sources[compte] ? 'reconnu par : ' + sources[compte] : '',
            }))))
        : el('td', { classe: 'vide',
            texte: 'personne : ni équipe, ni accès, ni commit' }),
      el('td', { classe: 'note', texte: equipe.exists ? 'déjà là' : 'à créer' }),
      el('td', { classe: 'etroit' }, el('button', {
        classe: 'bouton petit', type: 'button', texte: 'Composer…',
        onclick: () => composerEquipeReprise(plan, equipe),
      }))));
  }

  const avis = $('import-avis');
  vider(avis);
  const personnes = (plan.students || []).length;
  avis.append(el('div', { classe: 'avis' },
    el('p', { texte: `${equipes.length} équipe(s) seront composées, et `
      + `${personnes} personne(s) rejoindront « ${plan.scope} ». L'accès au dépôt `
      + "est accordé à l'équipe, pas à ses membres un par un." })));
  // Un dépôt que personne n'a touché ne dit pas qui en est : le taire ferait
  // croire à une équipe vide par choix.
  if ((plan.silent || []).length) {
    avis.append(el('div', { classe: 'avis alerte',
      texte: `Rien ne dit qui a fait ${plan.silent.join(', ')} — ni équipe GitHub, `
        + "ni accès, ni commit. « Composer… » permet de le dire ; sans quoi leur "
        + "équipe naîtra vide." }));
  }
  // Les équipes disent qui a fait le travail ; elles ne disent pas son nom.
  if ((plan.unmatched || []).length) {
    avis.append(el('div', { classe: 'avis alerte',
      texte: `${plan.unmatched.length} compte(s) qu'aucun nom ne désigne : `
        + plan.unmatched.map((compte) => '@' + compte).join(', ')
        + ". Ils rejoindront le groupe sous leur compte ; le tableau ci-dessous "
        + "permet de les nommer." }));
  }
  if ((plan.absent || []).length) {
    avis.append(el('div', { classe: 'avis',
      texte: `${plan.absent.length} étudiant(s) de la liste ne sont dans aucune `
        + 'équipe : ' + plan.absent.join(', ') + '.' }));
  }
  marquerEtape('verifier', `${equipes.length} équipe(s)`);
}

// composerEquipeReprise laisse dire qui est dans une équipe, quand ce que
// GitHub en montre ne suffit pas — un dépôt que personne n'a touché, une équipe
// que l'outil n'a pas su lire. Ce qu'on y tranche tient : la vérification
// suivante ne le redevine plus.
async function composerEquipeReprise(plan, equipe) {
  // Le vivier est ce que la reprise a trouvé partout : c'est là qu'on va
  // chercher quelqu'un rangé dans la mauvaise équipe.
  const vivier = new Set();
  for (const autre of plan.teams || []) {
    for (const compte of autre.members || []) vivier.add(compte);
  }
  const coches = new Set((equipe.members || []).map((compte) => compte.toLowerCase()));
  for (const compte of equipe.members || []) vivier.add(compte);

  const liste = el('div', { classe: 'liste-cases' },
    [...vivier].sort().map((compte) => el('label', { classe: 'case' },
      el('input', {
        type: 'checkbox', value: compte, checked: coches.has(compte.toLowerCase()),
      }),
      el('span', {}, el('span', { classe: 'compte', texte: '@' + compte })))));
  const autres = el('input', {
    classe: 'champ', type: 'text', placeholder: 'compte1, compte2',
  });
  const corps = el('div', {},
    el('p', { classe: 'note', texte:
      `Qui compose « ${equipe.short} » ? Le dépôt ${equipe.repo} lui reviendra, `
      + "et l'accès est accordé à l'équipe entière." }),
    vivier.size ? liste : el('p', { classe: 'note vide',
      texte: "La reprise n'a trouvé personne sur ce travail." }),
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: "Ajouter d'autres comptes" }), autres,
      el('span', { classe: 'aide', texte: 'Séparés par des virgules.' })));

  if (!await demander(`Composer ${equipe.short}`, corps, 'Retenir')) return;
  const voulus = [...liste.querySelectorAll('input:checked')].map((coche) => coche.value);
  for (const compte of autres.value.split(/[,\s]+/)) {
    const propre = compte.trim().replace(/^@/, '');
    if (propre && !voulus.some((autre) => autre.toLowerCase() === propre.toLowerCase())) {
      voulus.push(propre);
    }
  }
  importCrews.set(equipe.short, voulus);
  await verifier(true);
}

// montrerTravauxCaches propose les travaux qu'un préfixe fourre-tout
// rassemblait. « kickmyb » n'est pas un travail : « kickmyb-firebase » et
// « kickmyb-android » en sont deux, et se reprennent l'un après l'autre.
function montrerTravauxCaches(plan) {
  vider($('import-rapprochements').querySelector('tbody'));
  vider($('import-renommages').querySelector('tbody'));
  // Rien à continuer tant que le travail n'est pas tranché : l'étape des noms
  // n'aurait aucun dépôt à montrer.
  $('import-suite-bloc').hidden = true;
  $('import-compte').textContent = '';
  marquerEtape('verifier', '');
  marquerEtape('noms', '');

  const avis = $('import-avis');
  vider(avis);
  const bloc = el('div', { classe: 'avis alerte' },
    el('p', { texte: `« ${plan.prefix} » n'est pas un travail : les accès aux dépôts en `
      + `révèlent ${plan.splits.length}. Reprenez-les un à la fois.` }));
  const choix = el('div', { classe: 'actions' });
  for (const travail of plan.splits) {
    choix.append(el('button', {
      classe: 'bouton', type: 'button',
      texte: `${travail.prefix} · ${travail.count} dépôt(s)`,
      onclick: () => reprendreTravail(travail.prefix),
    }));
  }
  bloc.append(choix);
  avis.append(bloc);
}

// reprendreTravail refait la lecture pour un seul des travaux révélés. Le nom
// d'arrivée le suit quand rien d'autre n'a été tapé : c'est celui-là qu'on vient
// de choisir.
async function reprendreTravail(prefixe) {
  const ancien = importTravail;
  importTravail = prefixe;
  marquerEtape('travail', prefixe);
  if (!$('import-nom').value.trim() || $('import-nom').value.trim() === ancien) {
    $('import-nom').value = prefixe;
  }
  // Les dépôts de l'étape des dépôts étaient ceux du fourre-tout : les garder
  // ferait dire « 3 dépôts, tous repris » à une reprise qui n'en emporte que
  // deux, et laisserait cochés des dépôts d'un autre travail.
  await chargerDepots(prefixe, false);
  await verifier(true);
}

// lireNoms retient tout ce que la liste portait : ceux qu'un dépôt a trouvés,
// et ceux qu'aucun ne concerne. Les deux ensemble font le groupe.
function lireNoms(plan) {
  const noms = new Set(plan.absent || []);
  importChoix = new Map();
  for (const trouve of plan.pairings || []) {
    const nom = (trouve.entry && trouve.entry.full_name) || '';
    if (!nom) continue;
    noms.add(nom);
    importChoix.set(trouve.login, nom);
  }
  importNoms = [...noms].sort((a, b) => a.localeCompare(b, 'fr'));
}

// aVerifier dit qu'un rapprochement mérite un coup d'œil : rien de trouvé, deux
// personnes qui se valaient, ou une ressemblance trop mince pour faire une
// preuve. Un choix rendu à la main, lui, n'est plus à vérifier — c'est la ligne
// qui s'en souvient.
function aVerifier(trouve) {
  return trouve.ambiguous || !trouve.entry || !trouve.entry.full_name || trouve.score < 60;
}

function dessinerRapprochements(plan) {
  const corps = $('import-rapprochements').querySelector('tbody');
  vider(corps);
  for (const trouve of plan.pairings || []) {
    const raison = el('td', { classe: 'note',
      texte: trouve.ambiguous
        ? 'à trancher : ' + (trouve.rivals || []).join(', ')
        : trouve.reason || '' });
    const choix = el('select', { classe: 'champ choix-etudiant' });
    choix.dataset.login = trouve.login;

    const ligne = el('tr', {},
      el('td', {}, el('code', { texte: '@' + trouve.login })),
      el('td', {}, choix), raison);
    ligne.dataset.verifier = aVerifier(trouve) ? '1' : '';
    ligne.dataset.connu = trouve.entry && trouve.entry.full_name ? '1' : '';

    choix.addEventListener('change', () => {
      importChoix.set(trouve.login, choix.value);
      raison.textContent = choix.value ? 'choisi à la main' : 'laissé sans personne';
      // Un choix rendu à la main est vérifié par définition.
      ligne.dataset.verifier = '';
      ligne.dataset.connu = choix.value ? '1' : '';
      majOptions();
      compter();
      filtrer();
      planifierVerification();
    });
    corps.append(ligne);
  }
  majOptions();
  filtrer();
}

// majOptions refait les choix offerts : un nom déjà associé à un compte ne peut
// plus l'être à un autre, et celui qu'on vient de libérer revient partout.
function majOptions() {
  const pris = new Set([...importChoix.values()].filter(Boolean));
  for (const choix of $('import-rapprochements').querySelectorAll('select')) {
    const retenu = importChoix.get(choix.dataset.login) || '';
    vider(choix);
    choix.append(el('option', { value: '', texte: '— personne —' }));
    for (const nom of importNoms) {
      if (nom !== retenu && pris.has(nom)) continue;
      choix.append(el('option', { value: nom, texte: nom }));
    }
    choix.value = retenu;
  }
}

// planifierVerification attend que la rafale de corrections retombe avant de
// redemander le plan : dix menus corrigés d'affilée ne font qu'une requête.
function planifierVerification() {
  clearTimeout(importAttente);
  importAttente = setTimeout(() => verifier(false), 400);
}

$('import-filtre').addEventListener('change', filtrer);
$('import-nommes').addEventListener('change', () => verifier(false));
$('import-suite').addEventListener('click', () => ouvrirEtape('noms'));

// filtrer ne change que ce qu'on regarde : les dépôts cachés sont repris comme
// les autres. Ce qui entre ou non dans la reprise se décide à l'étape suivante.
function filtrer() {
  const vue = $('import-filtre').value;
  for (const ligne of $('import-rapprochements').querySelectorAll('tbody tr')) {
    ligne.hidden = (vue === 'verifier' && ligne.dataset.verifier !== '1')
      || (vue === 'connu' && ligne.dataset.connu !== '1');
  }
}

function compter() {
  const total = $('import-rapprochements').querySelectorAll('tbody tr').length;
  const reste = [...$('import-rapprochements').querySelectorAll('tbody tr')]
    .filter((ligne) => ligne.dataset.verifier === '1').length;
  $('import-compte').textContent = reste
    ? `${total} dépôt(s) · ${reste} à vérifier`
    : `${total} dépôt(s) · tout est rapproché`;
  marquerEtape('verifier', $('import-compte').textContent);
}

// quelquesNoms énumère sans laisser la liste enfler : un avis qui déroulerait
// trente noms ferait grandir la page avec la cohorte, et c'est justement ce
// qu'on veut éviter.
function quelquesNoms(noms, prefixe = '') {
  const montres = noms.slice(0, 4).map((nom) => prefixe + nom).join(', ');
  const reste = noms.length - 4;
  return reste > 0 ? `${montres}… et ${reste} autre(s)` : montres;
}

function dessinerAvis(plan) {
  const avis = $('import-avis');
  vider(avis);
  avis.append(el('div', { classe: 'avis',
    texte: `${plan.moves.length} dépôt(s) seront renommés vers « ${plan.scope} ».` }));
  if ((plan.unmatched || []).length) {
    const sort = plan.named_only
      ? 'leurs dépôts resteront où ils sont'
      : "leurs dépôts garderont le nom qu'ils portent";
    avis.append(el('div', { classe: 'avis alerte',
      texte: `${plan.unmatched.length} compte(s) ne mènent à personne : ${sort} — `
        + quelquesNoms(plan.unmatched, '@') + '.' }));
  }
  if ((plan.absent || []).length) {
    avis.append(el('div', { classe: 'avis alerte',
      texte: `${plan.absent.length} étudiant(s) de la liste n'ont pas de dépôt pour ce `
        + 'travail — ' + quelquesNoms(plan.absent) + '.' }));
  }
  // Ailleurs, le compte vient des accès ou du registre. Ici, il ne vient que du
  // nom : c'est le seul qui puisse encore être faux, et le seul à signaler.
  if ((plan.unconfirmed || []).length) {
    avis.append(el('div', { classe: 'avis alerte',
      texte: `${plan.unconfirmed.length} dépôt(s) dont le compte n'a pas pu être confirmé : `
        + "leurs accès ne désignent personne, et l'organisation ne connaît pas ce compte. "
        + 'Il est lu dans leur nom — ' + quelquesNoms(plan.unconfirmed) + '.' }));
  }
}

function dessinerRenommages(plan) {
  const corps = $('import-renommages').querySelector('tbody');
  vider(corps);
  for (const ligne of plan.moves || []) {
    corps.append(el('tr', {},
      el('td', {}, el('code', { texte: ligne.repo })),
      el('td', {}, el('code', { texte: ligne.target }))));
  }
  marquerEtape('noms', `${(plan.moves || []).length} dépôt(s) renommés`);
}

// --- 6. écrire

$('import-appliquer').addEventListener('click', async () => {
  const corps = corpsImport();
  if (!corps || !importPlan) return;
  const accord = await demander('Reprendre ces dépôts', el('div', {},
    el('p', { texte: `${importPlan.moves.length} dépôt(s) seront renommés vers `
      + `« ${importPlan.scope} ». GitHub garde une redirection depuis chaque ancien nom.` }),
    el('p', { classe: 'note', texte: importEquipe
      ? `${(importPlan.teams || []).length} équipe(s) seront créées sur GitHub et `
        + 'recevront leur dépôt. Le groupe sera déclaré, et les noms montés au '
        + "registre de l'organisation."
      : "Le groupe sera déclaré, et les noms montés au registre de l'organisation." })),
    'Reprendre');
  if (!accord) return;

  const fiche = await tenter(
    () => api('POST', `/api/orgs/${encode(etat.organisation)}/import`, corps), 'Reprise');
  if (!fiche) return;
  ouvrirEtape('journal');
  const bilan = await suivre(fiche,
    { journal: $('import-journal'), barre: $('import-barre') });
  if (!bilan) return;
  marquerEtape('journal', `${bilan.renamed} dépôt(s) repris`);
  message(bilan.team_work
    ? `${bilan.renamed} dépôt(s) repris dans « ${bilan.scope} », `
      + `${bilan.teams} équipe(s) composée(s).`
    : `${bilan.renamed} dépôt(s) repris dans « ${bilan.scope} ».`);
  await ouvrirGroupe(bilan.scope, true);
});

// ------------------------------------------------------ registre des étudiants

// Les noms complets vivent dans un dépôt privé de l'organisation : c'est ce qui
// permet à un collègue de les voir sans avoir rien déclaré chez lui. Ce qu'un
// poste a accumulé avant le registre ne monte pas tout seul, et rien n'est
// versé sans avoir été montré d'abord.

let registreApercu = null;

// lignesDeFiches met en table des personnes.
function lignesDeFiches(entetes, lignes) {
  return el('table', { classe: 'tableau' },
    el('thead', {}, el('tr', {}, entetes.map((titre) => el('th', { texte: titre })))),
    el('tbody', {}, lignes.map((cellules) =>
      el('tr', {}, cellules.map((valeur) => el('td', { texte: valeur }))))));
}

// dessinerRegistre écrit ce que la publication ferait.
function dessinerRegistre(vue) {
  const resume = $('registre-resume');
  const detail = $('registre-detail');
  vider(resume);
  vider(detail);
  registreApercu = vue;

  const plan = vue.plan || {};
  const neuves = plan.new || [];
  const desaccords = plan.renamed || [];
  const ambigus = plan.ambiguous || [];
  const sansNom = plan.nameless || [];

  const publie = vue.published || 0;
  if (publie > 0) {
    resume.append(el('div', { classe: 'avis succes',
      texte: `${publie} fiche(s) publiée(s). Le registre en compte ${vue.registry_size}.` }));
  } else {
    resume.append(el('div', { classe: 'avis',
      texte: `Le registre connaît ${vue.registry_size} fiche(s) ; ce poste en apporte `
        + `${vue.total} à écrire.` }));
  }
  if (vue.exposure) {
    resume.append(el('div', { classe: 'avis alerte', texte: vue.exposure }));
  }
  if (vue.notice) {
    resume.append(el('div', { classe: 'avis alerte', texte: vue.notice }));
  }

  if (neuves.length) {
    detail.append(el('p', { classe: 'note', texte: `${neuves.length} nouvelle(s) fiche(s)` }));
    detail.append(lignesDeFiches(['Nom complet', 'Compte'],
      neuves.map((fiche) => [fiche.full_name, '@' + fiche.username])));
  }
  if (desaccords.length) {
    detail.append(el('p', { classe: 'note',
      texte: `${desaccords.length} désaccord(s) de nom — cochez ci-dessous pour que ce `
        + 'poste l\'emporte.' }));
    detail.append(lignesDeFiches(['Compte', 'Au registre', 'Sur ce poste'],
      desaccords.map((item) => ['@' + item.username, item.registry, item.local])));
  }
  if (ambigus.length) {
    detail.append(el('p', { classe: 'note',
      texte: `${ambigus.length} compte(s) que ce poste nomme de plusieurs façons. Le premier `
        + 'est retenu ; les autres restent rattachés par leur slug.' }));
    detail.append(lignesDeFiches(['Compte', 'Retenu', 'Trouvés'],
      ambigus.map((item) => ['@' + item.username, item.chosen, (item.names || []).join(' · ')])));
  }
  if (sansNom.length) {
    detail.append(el('p', { classe: 'note',
      texte: `${sansNom.length} compte(s) sans nom complet connu : rien ne peut être publié `
        + 'pour eux (@' + sansNom.join(', @') + ').' }));
  }

  const aPublier = vue.total > 0;
  $('registre-publier').hidden = !aPublier;
  $('registre-prefer-local-bloc').hidden = desaccords.length === 0;
  $('registre-etat').textContent = aPublier ? '' : 'Rien à publier.';

  // Les équipes ne s'offrent que si le compte en voit : celui qui n'est pas
  // membre de l'organisation n'en voit aucune, et il n'y a rien à proposer.
  const equipes = vue.teams || [];
  const choix = $('registre-equipe');
  vider(choix);
  for (const nom of equipes) choix.append(el('option', { value: nom, texte: nom }));
  $('registre-equipe-bloc').hidden = equipes.length === 0;
}

$('registre-apercu').addEventListener('click', async () => {
  const org = etat.organisation;
  if (!org) { message('Choisissez d\'abord une organisation.', 'erreur'); return; }
  const vue = await tenter(() => api('GET', `/api/orgs/${encode(org)}/registry`), 'Registre');
  if (vue) dessinerRegistre(vue);
});

$('registre-publier').addEventListener('click', async () => {
  const org = etat.organisation;
  if (!org || !registreApercu) return;
  const total = registreApercu.total;
  const local = $('registre-prefer-local').checked;
  const accord = await demander('Publier le registre',
    el('p', { texte: `${total} fiche(s) seront écrites dans « ${org}/${registreApercu.repo} ». `
      + 'Le fichier de ce poste reste inchangé.' }), 'Publier');
  if (!accord) return;

  const vue = await tenter(
    () => api('POST', `/api/orgs/${encode(org)}/registry`, { prefer_local: local }), 'Registre');
  if (!vue) return;
  message(`${vue.published} fiche(s) publiée(s).`);
  dessinerRegistre(vue);
});

// Un étudiant retiré du registre reste dans l'historique : c'est ce que git
// est. Réécrire la branche en un commit sans passé est ce qu'on peut promettre
// de mieux — et pas davantage, ce que le dialogue dit sans détour.
$('registre-donner').addEventListener('click', async () => {
  const org = etat.organisation;
  const equipe = $('registre-equipe').value;
  if (!org || !equipe) return;
  const fait = await tenter(() => api('POST', `/api/orgs/${encode(org)}/registry/team`,
    { team: equipe }), 'Registre');
  if (fait) message(fait.message, 'succes', 12000);
});

$('registre-oublier').addEventListener('click', async () => {
  const org = etat.organisation;
  if (!org) { message('Choisissez d\'abord une organisation.', 'erreur'); return; }
  const cible = `${org}/.cohorte`;
  const saisie = el('input', { type: 'text', classe: 'champ', placeholder: cible });
  const accord = await demander('Effacer l\'historique du registre ?', el('div', {},
    el('p', { classe: 'avis erreur',
      texte: 'Le registre garde son contenu ; c\'est son passé qui disparaît, sans retour.' }),
    el('p', { classe: 'note',
      texte: 'GitHub garde un temps les objets devenus inaccessibles, et un clone déjà fait '
        + 'garde ce qu\'il avait : rien de plus n\'est promis ici.' }),
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: `Retapez « ${cible} » pour confirmer` }), saisie)),
    'Effacer');
  if (!accord) return;

  const fait = await tenter(() => api('POST', `/api/orgs/${encode(org)}/registry/history`,
    { confirm: saisie.value.trim() }), 'Registre');
  if (!fait) return;
  message(fait.message, 'succes', 12000);
});

// ------------------------------------------------------- portées du jeton

// L'outil ne fabrique aucun jeton : il redemande à gh d'en obtenir un portant
// les portées voulues. L'échange avec GitHub — un code à recopier — se joue
// dans le terminal d'où l'outil a été lancé, y compris quand la demande part
// d'ici : c'est la seule chose que le navigateur ne peut pas mener seul.

// tonDePortee traduit l'état d'une portée en couleur d'étiquette.
function tonDePortee(etat) {
  if (etat === 'présente') return 'oui';
  if (etat === 'absente') return 'non';
  return '';
}

// casesDePortees dresse la liste à cocher des portées, et rend une fonction qui
// dit lesquelles le sont. Les portées du socle de gh restent cochées : elles
// accompagnent tout jeton qu'il crée et ne peuvent pas en être retirées.
function casesDePortees(conteneur, jeton, ajoutee) {
  vider(conteneur);
  const cases = [];
  for (const portee of jeton.scopes || []) {
    const coche = portee.minimal || portee.state === 'présente' || portee.name === ajoutee;
    const entree = el('input', {
      type: 'checkbox', checked: coche, disabled: portee.minimal, value: portee.name,
    });
    cases.push(entree);
    conteneur.append(el('label', { classe: 'case' }, entree, el('span', {},
      el('code', { texte: portee.name }), ' ',
      el('span', { classe: 'jeton ' + tonDePortee(portee.state), texte: portee.state }),
      el('span', { classe: 'aide', texte: `${portee.label} — ${portee.purpose}` }))));
  }
  return () => cases.filter((entree) => entree.checked).map((entree) => entree.value);
}

// dessinerPortees remplit la boîte des réglages généraux.
function dessinerPortees(jeton) {
  etat.jeton = jeton;
  const provenance = $('jeton-provenance');
  provenance.textContent = `Jeton de @${jeton.viewer} sur ${jeton.host}` +
    (jeton.origin ? ` (${jeton.origin}).` : '.');

  etat.porteesCochees = casesDePortees($('portees'), jeton);

  const avis = $('jeton-avis');
  vider(avis);
  if (!jeton.refreshable) {
    avis.append(el('div', { classe: 'avis alerte',
      texte: `Ce jeton vient de l'environnement (${jeton.origin}) : gh ne peut pas le ` +
        'renouveler. Effacez la variable et relancez « gh auth login », ou donnez-lui ' +
        'un jeton portant les portées voulues.' }));
  }
  if ((jeton.missing || []).length) {
    avis.append(el('div', { classe: 'avis alerte',
      texte: `Toujours absente(s) après le renouvellement : ${jeton.missing.join(', ')}. ` +
        "GitHub n'accorde que ce qui lui a été accordé dans le navigateur." }));
  }
  $('jeton-renouveler').disabled = !jeton.refreshable;
}

// regenererJeton demande le nouveau jeton et redessine ce qu'il permet.
// L'appel reste ouvert le temps de l'échange dans le terminal : il n'y a rien à
// montrer ici, sinon dire où regarder.
async function regenererJeton(portees) {
  // L'attente peut durer : GitHub veut une confirmation, et elle se donne
  // ailleurs. Un avis reste affiché tant que gh n'a pas rendu la main, sans
  // quoi la page semblerait ne rien faire.
  const attente = el('div', { classe: 'avis', texte:
    "Renouvellement en cours : suivez les instructions dans le terminal d'où " +
    'gh cohorte a été lancé.' });
  $('messages').append(attente);
  $('jeton-etat').textContent = 'Suivez les instructions dans le terminal…';
  try {
    const jeton = await api('POST', '/api/token/refresh', { scopes: portees });
    dessinerPortees(jeton);
    if (etat.contexte) etat.contexte.token = jeton;
    message((jeton.missing || []).length
      ? `Jeton renouvelé, mais ${jeton.missing.join(', ')} manque toujours.`
      : 'Jeton renouvelé.', (jeton.missing || []).length ? 'alerte' : 'succes');
    return jeton;
  } catch (erreur) {
    message(`Renouvellement du jeton : ${erreur.message}`, 'erreur', 12000);
    return null;
  } finally {
    attente.remove();
    $('jeton-etat').textContent = '';
  }
}

// proposerRegeneration ouvre le dialogue quand une action bute sur une portée
// absente. Ce qui était déjà accordé reste coché, la portée manquante s'ajoute :
// obtenir un droit de plus ne doit jamais en faire perdre un autre.
async function proposerRegeneration(portee, contexte) {
  const jeton = await api('GET', '/api/token').catch(() => null);
  if (!jeton) return false;
  if (!jeton.refreshable) {
    message(`La portée « ${portee} » manque, et ce jeton vient de l'environnement ` +
      `(${jeton.origin}) : gh ne peut pas le renouveler.`, 'erreur', 12000);
    return false;
  }

  const conteneur = el('div', { classe: 'cases-travaux' });
  const corps = el('div', { classe: 'corps-dialogue' },
    el('p', { texte: (contexte ? `${contexte} : ` : '') +
      `le jeton n'a pas la portée « ${portee} ». Un nouveau jeton peut être ` +
      "obtenu tout de suite, avec ce qu'il permettait déjà et cette portée en plus." }),
    conteneur,
    el('p', { classe: 'note',
      texte: "GitHub demande une confirmation dans le navigateur : le code à recopier " +
        "paraît dans le terminal d'où gh cohorte a été lancé." }));
  const cochees = casesDePortees(conteneur, jeton, portee);

  if (!await demander('Générer un nouveau jeton', corps, 'Générer le jeton')) return false;
  const renouvele = await regenererJeton(cochees());
  if (!renouvele) return false;
  // Rejouer l'action sans la portée voulue ne ferait que répéter le refus.
  return !(renouvele.missing || []).includes(portee);
}

$('jeton-renouveler').addEventListener('click', () => {
  regenererJeton(etat.porteesCochees ? etat.porteesCochees() : []);
});

function dessinerChemins(chemins) {
  const corps = $('chemins').querySelector('tbody');
  vider(corps);
  for (const item of chemins) {
    corps.append(el('tr', {},
      el('td', { texte: item.label }),
      el('td', {}, el('code', { texte: item.path })),
      el('td', { classe: 'note', texte: item.state })));
  }
}

// ------------------------------------------- déplacer des étudiants de groupe

// Une personne change de groupe en cours de session : c'est fréquent, et cela
// arrive rarement à une seule. Ses dépôts suivent toujours : leur nom porte la
// place du groupe, et c'est lui qui dit à qui ils appartiennent. Les laisser en
// arrière montrerait la personne des deux côtés — dans la liste du groupe
// d'arrivée, et dans le groupe de départ que ses dépôts lui rattachent encore.
//
// Le groupe d'arrivée n'a pas à exister d'avance : le déplacement peut le
// déclarer au passage, plutôt que d'obliger à sortir d'ici pour le créer.

const GROUPE_NEUF = '\u0000neuf';

// choixDeGroupe compose la destination d'un déplacement : un groupe de
// l'organisation, ou « ＋ Nouveau groupe… », qui le déclare au passage plutôt que
// d'obliger à sortir d'ici pour le créer. Déplacer des personnes et déplacer des
// travaux visent la même chose : la destination ne se compose qu'une fois.
//
// Une liste déroulante n'y suffisait plus. Une session apporte une dizaine de
// groupes, l'organisation en garde plusieurs sessions, et « Groupe 02 · a26 ·
// 5N6 · 02 » répété quarante fois ne se lit pas. Les groupes sont donc rangés
// comme dans le parcours — par session, puis par cours —, et une recherche les
// réduit à mesure qu'on tape.
//
// Rien n'est retenu d'avance : le déplacement renomme des dépôts, il ne doit
// pas partir vers une destination que personne n'a désignée. Tant qu'aucune ne
// l'est, le bouton du dialogue reste éteint.
async function choixDeGroupe() {
  // Arriver droit sur un groupe par son adresse ne charge pas les autres.
  if (etat.groupes.length === 0) await chargerGroupes();
  const ailleurs = etat.groupes.filter((groupe) =>
    groupe.scope !== etat.groupe.scope &&
    groupe.org.toLowerCase() === etat.groupe.org.toLowerCase())
    .sort((a, b) => rangDeSession(a.session) - rangDeSession(b.session) ||
      a.session.localeCompare(b.session) || a.course.localeCompare(b.course) ||
      a.group.localeCompare(b.group, undefined, { numeric: true }));

  // Le scope retenu, ou GROUPE_NEUF. Le bouton du dialogue n'est connu qu'à
  // l'ouverture : « preparer » le confie, et il suit le choix ensuite.
  let choisi = null;
  let valider = null;

  const recherche = el('input', { classe: 'champ', type: 'search',
    placeholder: 'Filtrer : session, cours, groupe…' });
  const liste = el('div', { classe: 'choix-places' });
  const resume = el('p', { classe: 'note' });

  const session = el('input', { classe: 'champ', type: 'text',
    value: etat.groupe.session || '', placeholder: 'a26' });
  const cours = el('input', { classe: 'champ', type: 'text',
    value: etat.groupe.course || '', placeholder: '5n6' });
  const numero = el('input', { classe: 'champ', type: 'text', placeholder: '02' });

  const neuf = el('div', { classe: 'rangee serree' },
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: 'Session' }), session),
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: 'Cours' }), cours),
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: 'Groupe' }), numero));

  // La ligne qui déclare un groupe reste hors de la liste filtrée : une
  // recherche qui ne rend rien est précisément le moment où elle sert.
  const ligneNeuve = el('button', { classe: 'choix-place neuf', type: 'button',
    onclick: () => retenir(GROUPE_NEUF) },
    el('span', { classe: 'choix-infos' },
      el('span', { classe: 'titre', texte: '＋ Nouveau groupe…' }),
      el('span', { classe: 'detail', texte: 'déclaré au passage, sans sortir d’ici' })));

  // niveaux rend la place saisie pour un groupe à déclarer, ou rien tant que
  // les trois niveaux ne sont pas là : un nom de dépôt les veut tous.
  function niveaux() {
    const saisis = [session.value, cours.value, numero.value]
      .map((valeur) => valeur.trim());
    return saisis.every(Boolean) ? saisis : null;
  }

  function pret() {
    if (choisi === GROUPE_NEUF) return niveaux() !== null;
    return choisi !== null;
  }

  // Le résumé dit l'arrivée en toutes lettres : la ligne retenue peut avoir
  // défilé hors de vue, et c'est cette place-là qui sera écrite dans le nom de
  // chaque dépôt.
  function majResume() {
    if (choisi === GROUPE_NEUF) {
      const saisis = niveaux();
      resume.textContent = saisis
        ? `Nouveau groupe à la place ${saisis.join('.')}`
        : 'Session, cours et groupe sont tous les trois nécessaires.';
      return;
    }
    const groupe = ailleurs.find((autre) => autre.scope === choisi);
    resume.textContent = groupe
      ? `Arrivée : ${groupe.label} — ${groupe.scope}`
      : 'Choisissez le groupe d’arrivée.';
  }

  function maj() {
    ligneNeuve.classList.toggle('choisi', choisi === GROUPE_NEUF);
    for (const ligne of liste.querySelectorAll('.choix-place')) {
      ligne.classList.toggle('choisi', ligne.dataset.scope === choisi);
    }
    neuf.hidden = choisi !== GROUPE_NEUF;
    majResume();
    if (valider) valider.disabled = !pret();
  }

  function retenir(valeur) {
    choisi = valeur;
    maj();
    if (valeur === GROUPE_NEUF) session.focus();
  }

  // Les mots sous lesquels un groupe se cherche : sa place, son numéro, le
  // sigle de son cours, le nom long de sa session — « automne » se tape plus
  // volontiers que « a26 ». Les accents tombent au passage : « Été » se cherche
  // aussi bien en tapant « ete ».
  const mots = new Map(ailleurs.map((groupe) => [groupe.scope,
    aplati([groupe.label, groupe.scope, groupe.session, nomDeSession(groupe.session),
      groupe.course, groupe.group].join(' ')).split(/[^\p{L}\p{N}]+/u).filter(Boolean)]));

  function ligneDeGroupe(groupe) {
    const compte = groupe.known
      ? `${gens(groupe).length} étudiant(s)`
      : 'aucune liste retenue';
    return el('button', { classe: 'choix-place', type: 'button',
      'data-scope': groupe.scope, onclick: () => retenir(groupe.scope) },
      el('span', { classe: 'choix-infos' },
        el('span', { classe: 'titre', texte: groupe.label }),
        el('span', { classe: 'detail',
          texte: `${compte} · ${travaux((groupe.assignments || []).length)}` })),
      el('span', { classe: 'espace' }),
      el('code', { classe: 'jeton', texte: groupe.scope }));
  }

  function dessiner() {
    // Chaque mot cherché doit en ouvrir un du groupe, plutôt que se retrouver
    // n'importe où dedans : « 02 » désigne ainsi le groupe 02, et non l'année
    // 2026 où il se cache aussi.
    const termes = aplati(recherche.value.trim()).split(/\s+/).filter(Boolean);
    const retenus = ailleurs.filter((groupe) => termes.every((terme) =>
      mots.get(groupe.scope).some((mot) => mot.startsWith(terme))));

    vider(liste);
    let section = '';
    for (const groupe of retenus) {
      const titre = `${nomDeSession(groupe.session)} · ${sigle(groupe.course)}`;
      if (titre !== section) {
        section = titre;
        liste.append(el('div', { classe: 'choix-section', texte: titre }));
      }
      liste.append(ligneDeGroupe(groupe));
    }
    if (retenus.length === 0) {
      liste.append(el('div', { classe: 'boite-vide', texte: ailleurs.length === 0
        ? 'Aucun autre groupe dans cette organisation.'
        : 'Aucun groupe ne correspond.' }));
    }
    maj();
  }

  recherche.addEventListener('input', dessiner);
  for (const champ of [session, cours, numero]) champ.addEventListener('input', maj);

  const bloc = el('div', { classe: 'choix-arrivee' },
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: "Groupe d'arrivée" }), recherche),
    ligneNeuve, liste, neuf, resume);

  // Le dialogue est un formulaire : « Entrée » y vaut « valider », et fermerait
  // la question au lieu de servir la saisie en cours. Dans la recherche, elle
  // retient le premier groupe trouvé — c'est tout l'intérêt de filtrer ; les
  // boutons de la liste, eux, gardent leur « Entrée » native.
  bloc.addEventListener('keydown', (evenement) => {
    if (evenement.key !== 'Enter' || evenement.target.closest('button')) return;
    evenement.preventDefault();
    if (evenement.target !== recherche) return;
    const premiere = liste.querySelector('.choix-place');
    if (premiere) retenir(premiere.dataset.scope);
  });

  dessiner();
  // Sans autre groupe où aller, seule la déclaration reste : autant l'ouvrir.
  if (ailleurs.length === 0) retenir(GROUPE_NEUF);

  // destination rend ce que l'API attend : la place d'un groupe existant, ou
  // celle d'un groupe à déclarer. Elle ne rend rien tant que rien n'est choisi.
  const destination = () => {
    if (choisi === GROUPE_NEUF) {
      const saisis = niveaux() || ['', '', ''];
      return { new_group: { session: saisis[0], course: saisis[1], group: saisis[2] } };
    }
    return choisi ? { target: choisi } : null;
  };

  // preparer reçoit le bouton du dialogue à l'ouverture : c'est lui qui reste
  // éteint tant qu'aucune arrivée n'est désignée.
  const preparer = (bouton) => { valider = bouton; maj(); };

  return { bloc, destination, preparer };
}

async function deplacerEtudiants(personnes) {
  const { bloc, destination, preparer } = await choixDeGroupe();
  const seule = personnes.length === 1;
  const titre = seule
    ? `Déplacer ${personnes[0].full_name || '@' + personnes[0].username}`
    : `Déplacer ${personnes.length} étudiants`;
  const leurs = seule ? 'ses' : 'leurs';
  const depots = personnes.reduce(
    (total, personne) => total + (personne.assignments || []).length, 0);
  const sansNom = personnes.some((personne) => !personne.full_name);

  // Le titre dit déjà combien de personnes partent, et la liste d'où l'on vient
  // dit lesquelles : les renommer toutes ici ferait un mur avant la question.
  const confirme = await demander(titre, el('div', {},
    bloc,
    el('p', { classe: 'note', texte: depots === 0
      ? `Aucun dépôt à renommer : ${leurs} fiches sont tout ce qui change de groupe.`
      : `${depots} dépôt(s) seront renommés : leur nom porte la place du groupe, et ` +
        'c’est lui qui dit à qui ils appartiennent. GitHub garde une redirection ' +
        'depuis chaque ancien nom.' }),
    depots === 0 || !sansNom ? null : el('p', { classe: 'note',
      texte: 'Un dépôt dont le nom complet manque encore garde le dernier niveau de son ' +
        'nom — souvent le compte GitHub. Il arrive quand même à la bonne place, et se ' +
        'renomme une fois le nom retrouvé.' })), 'Déplacer', preparer);
  if (!confirme) return;
  const cible = destination();
  if (!cible) return;

  const corps = Object.assign({
    usernames: personnes.map((personne) => personne.username),
  }, cible);

  const fiche = await tenter(() => api('POST',
    `/api/classrooms/${encode(etat.groupe.scope)}/students/move`, corps), 'Déplacement');
  if (!fiche) return;

  // Sans dépôt à renommer, le serveur répond directement ; sinon c'est un
  // travail de fond, avec son journal.
  const bilan = fiche.id ? await suivre(fiche) : fiche;
  if (!bilan) return;
  // Les listes ne suivent que si tous les dépôts sont arrivés : un déplacement
  // interrompu n'a déplacé personne, et le journal dit lesquels ont résisté.
  if (!bilan.count) {
    message(`Aucun étudiant déplacé · ${bilan.failed} dépôt(s) en échec`, 'alerte');
    await ouvrirGroupe(etat.groupe.scope, true, true);
    afficherVue('etudiants');
    return;
  }
  const qui = bilan.count === 1
    ? `@${bilan.moved[0]} déplacé`
    : `${bilan.count} étudiants déplacés`;
  message(`${qui} vers « ${bilan.target} »` +
    (bilan.created ? ' · groupe créé' : '') +
    (bilan.renamed ? ` · ${bilan.renamed} dépôt(s) renommé(s)` : ''));
  etat.deplaces = new Set();
  await chargerGroupes(true);
  await ouvrirGroupe(etat.groupe.scope, true, true);
  afficherVue('etudiants');
}

// ------------------------------------------------ déplacer un travail entier

// Un préfixe fourre-tout — « travail-de » — rassemble parfois les travaux de
// plusieurs groupes et de plusieurs sessions. Les séparer ne se fait pas
// étudiant par étudiant : c'est le travail qui appartient à un groupe, et c'est
// lui qu'on en sort.
//
// Rien n'oblige à connaître les personnes pour cela. Un dépôt dont l'étudiant
// reste inconnu garde le dernier niveau de son nom — souvent son compte
// GitHub —, arrive quand même à la bonne place, et son nom complet se corrige
// ensuite, depuis la liste des étudiants. Sans cette règle, déplacer réclamerait
// un nom complet, et le retrouver réclamerait un groupe déplacé.
async function deplacerTravaux(travauxChoisis) {
  const { bloc, destination, preparer } = await choixDeGroupe();
  const seul = travauxChoisis.length === 1;
  const nom = el('input', { type: 'text', classe: 'champ',
    value: seul ? travauxChoisis[0].name : '' });

  const confirme = await demander(
    seul ? `Déplacer « ${travauxChoisis[0].name} »` : `Déplacer ${travauxChoisis.length} travaux`,
    el('div', {},
      seul ? null : el('p', { classe: 'note',
        texte: travauxChoisis.map((travail) => travail.name).join(', ') }),
      bloc,
      seul
        ? el('label', { classe: 'champ-bloc' },
            el('span', { classe: 'etiquette', texte: "Nom du travail à l'arrivée" }), nom,
            el('span', { classe: 'aide',
              texte: 'Il entre dans le nom de chaque dépôt : c’est le moment de le corriger.' }))
        : el('p', { classe: 'note', texte: 'Chaque travail garde son nom.' }),
      el('p', { classe: 'note',
        texte: 'Les dépôts dont l’étudiant reste inconnu gardent le dernier niveau de leur ' +
          'nom — souvent son compte GitHub. Les noms complets se corrigent ensuite, depuis ' +
          'la liste des étudiants du groupe d’arrivée.' })),
    'Voir le renommage', preparer);
  if (!confirme) return;
  const cible = destination();
  if (!cible) return;

  const corps = Object.assign({
    assignments: travauxChoisis.map((travail) => ({
      id: travail.id, name: seul ? nom.value.trim() : '',
    })),
  }, cible);

  // Rien n'est écrit tant que le renommage n'a pas été montré : c'est la seule
  // façon de vérifier que ce sont bien ces dépôts-là qu'on sort du fourre-tout.
  const apercu = await tenter(() => api('POST',
    `/api/classrooms/${encode(etat.groupe.scope)}/assignments/move/preview`, corps),
  'Déplacement');
  if (!apercu) return;

  const lignes = apercu.rows.map((ligne) => el('tr', {},
    el('td', {}, el('code', { texte: ligne.repo })),
    el('td', {}, el('code', { texte: ligne.target }))));
  const suivies = (apercu.students || []).length;
  const partantes = (apercu.leaving || []).length;

  const parti = await demander(`Renommer ${apercu.ready} dépôt(s)`, el('div', {},
    el('p', { texte: `Vers « ${apercu.target} » — ${apercu.target_scope}` +
      (apercu.created ? ', déclaré au passage.' : '.') }),
    el('div', { classe: 'apercu-renommage' },
      el('table', { classe: 'tableau' },
        el('thead', {}, el('tr', {},
          el('th', { texte: 'Dépôt actuel' }), el('th', { texte: 'Nouveau nom' }))),
        el('tbody', {}, lignes))),
    el('p', { classe: 'note',
      texte: suivies === 0
        ? 'Aucune fiche à faire suivre : ces dépôts ne sont rattachés à personne.'
        : `${suivies} fiche(s) suivront, dont ${partantes} qui quittent ` +
          `« ${etat.groupe.label} » faute d’y garder un dépôt.` }),
    el('p', { classe: 'note',
      texte: 'GitHub garde une redirection depuis chaque ancien nom : les clones et les ' +
        'liens déjà distribués continuent de fonctionner.' })), 'Déplacer');
  if (!parti) return;

  const fiche = await tenter(() => api('POST',
    `/api/classrooms/${encode(etat.groupe.scope)}/assignments/move`, corps), 'Déplacement');
  if (!fiche) return;
  const bilan = await suivre(fiche);
  if (!bilan) return;
  message(`${bilan.renamed} dépôt(s) déplacé(s) vers « ${bilan.target} »` +
    (bilan.created ? ' · groupe déclaré' : '') +
    (bilan.moved ? ` · ${bilan.moved} fiche(s) suivies` : '') +
    (bilan.failed ? ` · ${bilan.failed} en échec` : ''),
  bilan.failed ? 'alerte' : 'succes');
  etat.travauxChoisis = new Set();
  await chargerGroupes(true);
  await ouvrirGroupe(etat.groupe.scope, true, true);
}

// ------------------------------------------------------ renommer un travail

// Un travail mal nommé — « tp1 » pour ce qui est devenu le projet final, une
// faute de frappe distribuée à trente personnes — n'avait qu'une issue : le
// déplacer vers un autre groupe pour profiter du nom qu'on y choisit au
// passage. C'est beaucoup demander pour corriger un mot.
//
// Le nom du travail est un niveau du nom de chaque dépôt : il n'y a pas de
// fiche où le corriger, les dépôts sont tout ce qu'un travail est. Le renommer,
// c'est donc les renommer tous — et c'est pourquoi le renommage se montre en
// entier avant la première écriture.
async function renommerTravail(travail) {
  const nom = el('input', { type: 'text', classe: 'champ', value: travail.name });

  const confirme = await demander(`Renommer « ${travail.name} »`, el('div', {},
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: 'Nouveau nom du travail' }), nom,
      el('span', { classe: 'aide',
        texte: 'Il entre dans le nom de chaque dépôt : le corriger renomme les ' +
          `${travail.repos} dépôt(s) du travail.` })),
    el('p', { classe: 'note',
      texte: 'Le groupe et le nom des étudiants ne bougent pas : seul le niveau du ' +
        'travail change.' })), 'Voir le renommage');
  if (!confirme) return;

  const corps = { id: travail.id, name: nom.value.trim() };
  const apercu = await tenter(() => api('POST',
    `/api/classrooms/${encode(etat.groupe.scope)}/assignments/rename/preview`, corps),
  'Renommage');
  if (!apercu) return;

  const lignes = apercu.rows.map((ligne) => el('tr', {},
    el('td', {}, el('code', { texte: ligne.repo })),
    el('td', {}, el('code', { texte: ligne.target }))));

  const parti = await demander(`Renommer ${apercu.ready} dépôt(s)`, el('div', {},
    el('div', { classe: 'apercu-renommage' },
      el('table', { classe: 'tableau' },
        el('thead', {}, el('tr', {},
          el('th', { texte: 'Dépôt actuel' }), el('th', { texte: 'Nouveau nom' }))),
        el('tbody', {}, lignes))),
    el('p', { classe: 'note',
      texte: 'GitHub garde une redirection depuis chaque ancien nom : les clones et les ' +
        'liens déjà distribués continuent de fonctionner.' })), 'Renommer');
  if (!parti) return;

  const fiche = await tenter(() => api('POST',
    `/api/classrooms/${encode(etat.groupe.scope)}/assignments/rename`, corps), 'Renommage');
  if (!fiche) return;
  const bilan = await suivre(fiche);
  if (!bilan) return;
  message(`${bilan.renamed} dépôt(s) renommé(s) · « ${bilan.previous} » devient ` +
    `« ${bilan.name} »` + (bilan.failed ? ` · ${bilan.failed} en échec` : ''),
  bilan.failed ? 'alerte' : 'succes');

  // La page revient sur le travail sous son nouveau nom : l'ancien ne désigne
  // plus rien, et rester dessus montrerait une liste vide. L'adresse suit, elle
  // aussi — recharger la page sur l'ancien nom ne trouverait plus de travail.
  await chargerGroupes(true);
  if (!await ouvrirGroupe(etat.groupe.scope, true, true)) return;
  const renomme = (etat.groupe.assignments || []).find((item) => item.id === bilan.id);
  if (renomme) await ouvrirTravail(renomme, false, false);
}

// ----------------------------------------------------- choix d'un chemin

// Le navigateur ne donne jamais le chemin d'un fichier déposé : c'est le
// serveur, qui tourne sur la même machine, qui demande au système d'ouvrir sa
// fenêtre. Quand la plateforme n'en a pas — un serveur sans session graphique —,
// l'explorateur de l'interface prend le relais.
async function choisirChemin(champ, options = {}) {
  const requete = {
    path: champ.value.trim(),
    dirs: !!options.dossier,
    title: options.titre || 'Choisir',
  };
  if (etat.contexte && etat.contexte.native_picker) {
    const reponse = await tenter(() => api('POST', '/api/paths/pick', requete), 'Sélection');
    if (!reponse || reponse.canceled) return;
    champ.value = reponse.path;
    champ.dispatchEvent(new Event('change', { bubbles: true }));
    return;
  }
  const choisi = await explorer(requete);
  if (choisi === null) return;
  champ.value = choisi;
  champ.dispatchEvent(new Event('change', { bubbles: true }));
}

// zoneDepot construit une zone de dépôt pour un dialogue, où il n'y a pas de
// balisage à réutiliser. Elle rend le bloc à insérer et le champ qui porte la
// valeur.
function zoneDepot(options = {}) {
  const champ = el('input', { type: 'hidden', value: options.valeur || '' });
  const zone = el('div', { classe: 'depot', tabindex: '0', role: 'button' },
    champ, el('span', { classe: 'depot-texte' }));
  if (options.dossier) zone.dataset.dossier = '1';
  zone.dataset.titre = options.titre || 'Choisir';
  brancherDepot(zone);
  return { zone, champ };
}

// ------------------------------------------------------------ zones de dépôt

// Un chemin ne se tape pas : il se dépose, ou il se choisit. Le champ caché de
// la zone garde la valeur — tout ce qui la lisait la lit encore —, et l'y
// écrire rafraîchit l'affichage, d'où que l'écriture vienne.
//
// Un fichier déposé, lui, n'a pas de chemin : le navigateur ne le donne jamais.
// Il a un contenu, et c'est le contenu qui part au serveur.
const valeurBrute = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value');

function brancherDepot(zone) {
  const champ = zone.querySelector('input');
  const texte = zone.querySelector('.depot-texte');
  const dossier = zone.dataset.dossier === '1';
  const vide = dossier
    ? 'Cliquez pour choisir un dossier.'
    : 'Déposez le fichier ici, ou cliquez pour le choisir.';

  const montrer = () => {
    const valeur = valeurBrute.get.call(champ).trim();
    zone.classList.toggle('vide', !valeur);
    vider(texte);
    if (valeur) texte.append(el('code', { texte: valeur }));
    else texte.textContent = vide;
  };
  Object.defineProperty(champ, 'value', {
    get() { return valeurBrute.get.call(this); },
    set(valeur) { valeurBrute.set.call(this, valeur); montrer(); },
  });
  montrer();

  const ouvrir = async () => {
    const avant = champ.value;
    await choisirChemin(champ, { dossier, titre: zone.dataset.titre });
    // Un chemin choisi remplace ce qui avait été déposé : les deux ne peuvent
    // pas désigner le même fichier.
    if (champ.value !== avant) delete zone.dataset.contenu;
  };
  zone.addEventListener('click', ouvrir);
  zone.addEventListener('keydown', (evenement) => {
    if (evenement.key !== 'Enter' && evenement.key !== ' ') return;
    evenement.preventDefault();
    ouvrir();
  });

  // Un dossier déposé ne donne ni chemin ni contenu lisible : il ne reste que
  // le clic.
  if (dossier) return;
  zone.addEventListener('dragover', (evenement) => {
    evenement.preventDefault();
    zone.classList.add('survol');
  });
  zone.addEventListener('dragleave', () => zone.classList.remove('survol'));
  zone.addEventListener('drop', async (evenement) => {
    evenement.preventDefault();
    zone.classList.remove('survol');
    const fichier = evenement.dataTransfer.files[0];
    if (!fichier) return;
    zone.dataset.contenu = await octets(fichier);
    champ.value = fichier.name;
    champ.dispatchEvent(new Event('change', { bubbles: true }));
  });
}

// octets rend le contenu d'un fichier en base64 : la forme qu'un []byte prend
// en JSON du côté de Go.
function octets(fichier) {
  return new Promise((resolve, reject) => {
    const lecteur = new FileReader();
    lecteur.onload = () => resolve(lecteur.result.split(',')[1] || '');
    lecteur.onerror = () => reject(lecteur.error);
    lecteur.readAsDataURL(fichier);
  });
}

// contenuDepot rend les octets déposés dans la zone d'un champ, ou rien.
function contenuDepot(id) {
  const zone = $(id).closest('.depot');
  return (zone && zone.dataset.contenu) || '';
}

// cheminDepot rend le chemin d'un champ, ou rien s'il tient un fichier déposé :
// un tel fichier n'a pas de chemin, et son nom n'en fait pas un.
function cheminDepot(id) {
  return contenuDepot(id) ? '' : $(id).value.trim();
}

// viderDepot oublie le fichier d'une zone : le chemin comme les octets déposés.
// Effacer le champ seul laisserait le contenu derrière, et l'écran dirait qu'il
// n'y a plus de liste alors qu'une requête en enverrait encore une.
function viderDepot(id) {
  const champ = $(id);
  const zone = champ.closest('.depot');
  if (zone) delete zone.dataset.contenu;
  champ.value = '';
}

for (const zone of document.querySelectorAll('[data-depot]')) brancherDepot(zone);

// explorer ouvre l'explorateur interne et renvoie le chemin retenu, ou null.
function explorer(requete) {
  const dialogue = $('explorateur');
  const saisie = $('explorateur-saisie');
  $('explorateur-titre').textContent = requete.title;
  saisie.value = requete.path || '';

  let dossierCourant = '';
  let maison = '';
  // Déclarée ici pour que la liste puisse répondre d'un double-clic.
  let repondre = () => {};

  async function lister(chemin) {
    const listing = await tenter(() => api('POST', '/api/paths/browse',
      { path: chemin, dirs: requete.dirs, title: '' }), 'Dossier');
    if (!listing) return;
    dossierCourant = listing.path;
    maison = listing.home;
    $('explorateur-chemin').textContent = listing.path;
    $('explorateur-parent').disabled = !listing.parent;
    $('explorateur-parent').dataset.cible = listing.parent || '';
    if (requete.dirs) saisie.value = listing.path;

    const liste = $('explorateur-liste');
    vider(liste);
    for (const entree of listing.entries) {
      liste.append(el('button', {
        classe: 'ligne-entree', type: 'button',
        onclick: (evenement) => {
          if (entree.dir) { lister(entree.path); return; }
          saisie.value = entree.path;
          for (const autre of liste.querySelectorAll('button')) autre.classList.remove('choisi');
          evenement.currentTarget.classList.add('choisi');
        },
        ondblclick: () => { if (!entree.dir) repondre(entree.path); },
      },
        el('span', { classe: 'marque', texte: entree.dir ? '📁' : '·' }),
        el('span', { classe: entree.dir ? '' : 'fichier', texte: entree.name })));
    }
    if (listing.entries.length === 0) {
      liste.append(el('div', { classe: 'boite-vide',
        texte: requete.dirs ? 'Aucun sous-dossier.' : 'Dossier vide.' }));
    }
    if (listing.truncated) {
      liste.append(el('div', { classe: 'boite-vide',
        texte: 'Dossier trop fourni : la liste est écourtée. Tapez le chemin ci-dessous.' }));
    }
  }

  return new Promise((resolve) => {
    let repondu = false;
    repondre = (valeur) => {
      if (repondu) return;
      repondu = true;
      $('explorateur-ok').removeEventListener('click', surOui);
      $('explorateur-annuler').removeEventListener('click', surNon);
      $('explorateur-parent').removeEventListener('click', surParent);
      $('explorateur-maison').removeEventListener('click', surMaison);
      dialogue.removeEventListener('cancel', surNon);
      if (dialogue.open) dialogue.close();
      resolve(valeur);
    };
    const surOui = () => repondre(saisie.value.trim() || dossierCourant);
    const surNon = () => repondre(null);
    const surParent = () => lister($('explorateur-parent').dataset.cible || dossierCourant);
    const surMaison = () => lister(maison);
    $('explorateur-ok').addEventListener('click', surOui);
    $('explorateur-annuler').addEventListener('click', surNon);
    $('explorateur-parent').addEventListener('click', surParent);
    $('explorateur-maison').addEventListener('click', surMaison);
    dialogue.addEventListener('cancel', surNon);
    dialogue.showModal();
    lister(requete.path);
  });
}

// -------------------------------------------------------------------- quitter

$('quitter').addEventListener('click', async () => {
  const confirme = await demander("Fermer l'interface",
    el('p', { texte: "Le serveur local s'arrête et la commande rend la main au terminal." }),
    'Fermer');
  if (!confirme) return;
  await api('POST', '/api/quit').catch(() => {});
  document.body.textContent = '';
  document.body.append(el('main', {},
    el('h2', { texte: 'Interface fermée.' }),
    el('p', { classe: 'note', texte: 'Vous pouvez fermer cet onglet.' })));
});

// ------------------------------------------------------------------ démarrage

// L'écran du lancement dit ce qu'on attend et, si rien ne vient, laisse de
// quoi réessayer : une fenêtre blanche ne disait ni l'un ni l'autre.
function demarrageDit(texte, detail = '') {
  $('demarrage-roue').hidden = false;
  $('demarrage-texte').textContent = texte;
  $('demarrage-detail').textContent = detail;
  $('demarrage-reessayer').hidden = true;
}

function demarrageEchoue(raison) {
  for (const vue of document.querySelectorAll('.vue')) {
    vue.hidden = vue.id !== 'vue-demarrage';
  }
  $('demarrage-roue').hidden = true;
  $('demarrage-texte').textContent = "Le serveur local n'a pas répondu.";
  $('demarrage-detail').textContent = raison;
  $('demarrage-reessayer').hidden = false;
}

$('demarrage-reessayer').addEventListener('click', () => { demarrer(); });

async function demarrer() {
  demarrageDit('Connexion au serveur local…');
  let contexte;
  try {
    contexte = await api('GET', '/api/context');
  } catch (erreur) {
    demarrageEchoue(erreur.message);
    return;
  }
  etat.contexte = contexte;
  etat.reglages = contexte.settings;

  $('version').textContent = contexte.version;
  $('compte').textContent = `@${contexte.viewer} sur ${contexte.host}`;
  $('aide-champs').textContent = 'Champs disponibles dans la description : ' +
    contexte.placeholders.map((nom) => `{${nom}}`).join(', ');

  for (const id of ['reglage-permission', 'gr-permission']) {
    const droits = $(id);
    vider(droits);
    for (const option of contexte.permissions) {
      droits.append(el('option', { value: option.value, texte: option.label }));
    }
  }

  dessinerPortees(contexte.token);
  dessinerChemins(contexte.paths);
  ecrireReglagesGeneraux();

  // Rien n'est accessible tant qu'une organisation n'a pas été choisie : si
  // aucune n'a été mémorisée, c'est la première chose demandée.
  if (!etat.reglages.org) {
    demarrageDit('Lecture de vos organisations GitHub…',
      'GitHub est interrogé pour la première fois : cela peut prendre un moment.');
    await demanderOrganisation();
    return;
  }
  etat.organisation = etat.reglages.org;
  demarrageDit(`Lecture des groupes de ${etat.organisation}…`);
  // L'adresse dit où reprendre : recharger la page ou coller un lien ramène au
  // même endroit qu'avant.
  await allerA(lireAdresse());
}

// ecrireReglagesGeneraux remplit l'écran des réglages de la session.
function ecrireReglagesGeneraux() {
  const contexte = etat.contexte || {};
  $('reglage-delay').value = etat.reglages.delay_seconds ?? 1;
  $('reglage-clone-dir').value = etat.reglages.clone_dir || '';
  $('reglages-selecteur').textContent = contexte.native_picker
    ? `« Parcourir… » ouvre la fenêtre du système (${contexte.native_picker}).`
    : "Cette machine n'a pas de fenêtre de sélection : « Parcourir… » ouvre " +
      "l'explorateur de l'interface.";
  if (!contexte.save_config) {
    $('reglages-etat').textContent = 'Mémorisation désactivée (--no-save-config)';
  }
}

demarrer();
