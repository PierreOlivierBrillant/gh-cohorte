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
  const reponse = await fetch(chemin, options);
  const texte = await reponse.text();
  let donnees = null;
  if (texte) {
    try { donnees = JSON.parse(texte); } catch { donnees = { error: texte }; }
  }
  if (!reponse.ok) {
    throw new Error((donnees && donnees.error) || `Erreur ${reponse.status}`);
  }
  return donnees;
}

// tenter exécute une action et affiche l'erreur éventuelle sans casser la page.
async function tenter(action, contexte) {
  try {
    return await action();
  } catch (erreur) {
    message(contexte ? `${contexte} : ${erreur.message}` : erreur.message, 'erreur', 12000);
    return null;
  }
}

const encode = encodeURIComponent;

// ---------------------------------------------------------------------- état

const etat = {
  contexte: null,
  reglages: {},
  groupes: [],
  groupe: null,
  travail: null,
  selection: new Set(),
  acces: new Map(),
  etudiants: [],
  destinataires: new Set(),
  reglagesTravail: {},
  nouveau: { org: '', etudiants: [], rejets: [] },
  etape: 1,
};

// ------------------------------------------------------- opérations et journal

let operationCourante = null;

function ouvrirOperation(fiche) {
  operationCourante = fiche;
  $('operation').hidden = false;
  $('operation-titre').textContent = fiche.label;
  $('operation-etat').textContent = fiche.status;
  $('operation-annuler').hidden = false;
  $('operation-barre').value = 0;
  vider($('operation-journal'));
}

function journaliser(texte, ton = '') {
  const journal = $('operation-journal');
  journal.append(el('div', { classe: ton, texte }));
  journal.scrollTop = journal.scrollHeight;
}

function appliquerEvenement(evenement) {
  switch (evenement.kind) {
    case 'avancement':
      if (evenement.total > 0) {
        $('operation-barre').value = Math.round((evenement.done / evenement.total) * 100);
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
      if (fin.failure) journaliser(fin.failure, 'err');
      else $('operation-barre').value = 100;
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
function suivre(fiche) {
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
          resolve((item.data && item.data.result) || null);
        }
      };
      source.onerror = async () => {
        source.close();
        // La coupure peut venir d'une fermeture normale : l'état de l'opération
        // tranche, et la lecture reprend au dernier événement reçu.
        const fin = await api('GET', `/api/jobs/${encode(fiche.id)}`).catch(() => null);
        if (!fin || fin.status !== 'en cours') {
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
function demander(titre, contenu, libelle = 'Confirmer') {
  const dialogue = $('dialogue');
  const valider = $('dialogue-ok');
  const annuler = $('dialogue-annuler');
  $('dialogue-titre').textContent = titre;
  const corps = $('dialogue-corps');
  vider(corps);
  corps.className = 'corps-dialogue';
  corps.append(contenu);
  valider.textContent = libelle;

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
    const surFermeture = () => repondre(dialogue.returnValue === 'ok');
    valider.addEventListener('click', surOui);
    annuler.addEventListener('click', surNon);
    // Échap referme sans passer par les boutons.
    dialogue.addEventListener('cancel', surNon);
    dialogue.addEventListener('close', surFermeture);
    dialogue.showModal();
  });
}

// --------------------------------------------------------------------- vues

// Les vues d'un groupe partagent son en-tête et ses onglets.
const ongletDeLaVue = {
  travaux: 'travaux', travail: 'travaux', assistant: 'travaux',
  etudiants: 'etudiants', 'groupe-reglages': 'groupe-reglages',
};

for (const bouton of $('onglets').querySelectorAll('button')) {
  bouton.addEventListener('click', () => afficherVue(bouton.dataset.vue));
}

function afficherVue(nom) {
  const onglet = ongletDeLaVue[nom];
  $('entete-groupe').hidden = !onglet;
  for (const bouton of $('onglets').querySelectorAll('button')) {
    bouton.classList.toggle('actif', bouton.dataset.vue === onglet);
  }
  for (const vue of document.querySelectorAll('.vue')) {
    vue.hidden = vue.id !== 'vue-' + nom;
  }
  window.scrollTo(0, 0);
  // Les comptages d'une vue changent pendant qu'on est ailleurs : chaque
  // retour les redemande plutôt que de laisser voir un état périmé.
  if (nom === 'groupes') chargerGroupes();
  if (nom === 'reglages') rafraichirEmplacements();
  if (nom === 'etudiants') chargerEtudiants();
  if (nom === 'groupe-reglages') ecrireReglagesGroupe();
}

// travaux accorde le mot avec le nombre.
function travaux(nombre) {
  return nombre === 1 ? '1 travail' : `${nombre} travaux`;
}

$('accueil').addEventListener('click', () => afficherVue('groupes'));
$('retour-groupes').addEventListener('click', () => afficherVue('groupes'));
$('nouveau-retour').addEventListener('click', () => afficherVue('groupes'));
$('reglages-retour').addEventListener('click', () => afficherVue('groupes'));
$('ouvrir-reglages').addEventListener('click', () => afficherVue('reglages'));
$('travail-retour').addEventListener('click', () => afficherVue('travaux'));
$('assistant-retour').addEventListener('click', () => afficherVue('travaux'));

// --------------------------------------------------------- liste des groupes

async function chargerGroupes() {
  const donnees = await tenter(() => api('GET', '/api/classrooms'), 'Groupes');
  if (!donnees) return;
  etat.groupes = donnees.classrooms || [];
  dessinerGroupes();
  await chargerAccueilOrganisations();
}

function dessinerGroupes() {
  const conteneur = $('groupes-liste');
  vider(conteneur);
  if (etat.groupes.length === 0) {
    conteneur.append(el('div', { classe: 'boite-vide' },
      el('p', { texte: 'Aucun groupe déclaré pour le moment.' }),
      el('p', { classe: 'note',
        texte: 'Adoptez ci-dessous un groupe repéré dans une organisation, ' +
          'ou déclarez-en un de toutes pièces.' })));
    return;
  }
  for (const groupe of etat.groupes) {
    const sesTravaux = groupe.assignments || [];
    conteneur.append(el('button', {
      classe: 'travail-ligne groupe-ligne', type: 'button',
      onclick: () => ouvrirGroupe(groupe.id),
    },
      el('span', { classe: 'travail-infos' },
        el('span', { classe: 'titre', texte: groupe.name }),
        el('span', { classe: 'detail',
          texte: `${groupe.org}${groupe.prefix ? ' · ' + groupe.prefix + '-…' : ''}` })),
      el('span', { classe: 'espace' }),
      el('span', { classe: 'jeton', texte: `${(groupe.students || []).length} étudiant(s)` }),
      el('span', { classe: 'jeton', texte: travaux(sesTravaux.length) }),
      el('span', { classe: 'chevron', texte: '›' })));
  }
}

async function ouvrirGroupe(id, force) {
  const groupe = await tenter(() => api('GET',
    `/api/classrooms/${encode(id)}${force ? '?refresh=1' : ''}`), 'Groupe');
  if (!groupe) return;
  etat.groupe = groupe;
  etat.travail = null;
  etat.etudiants = [];

  $('groupe-titre').textContent = groupe.name;
  $('groupe-prefixe').textContent = groupe.prefix ? groupe.prefix + '-…' : 'sans préfixe';
  $('groupe-sous-titre').textContent =
    `Organisation ${groupe.org} · ${(groupe.students || []).length} étudiant(s)`;
  dessinerTravaux();
  afficherVue('travaux');
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

// remplirSelecteur pose les organisations du compte dans une liste déroulante,
// et retient celle de la dernière fois.
function remplirSelecteur(selecteur, liste) {
  if (selecteur.options.length > 0) return selecteur.value;
  for (const acces of liste) {
    selecteur.append(el('option', { value: acces.login, texte: acces.label }));
  }
  selecteur.append(el('option', { value: '__saisir', texte: 'Saisir un autre nom…' }));

  const memorisee = etat.reglages.org;
  if (memorisee && liste.some((acces) => acces.login.toLowerCase() === memorisee.toLowerCase())) {
    selecteur.value = memorisee;
  } else if (liste.length > 0) {
    selecteur.value = liste[0].login;
  } else {
    selecteur.value = '__saisir';
  }
  return selecteur.value;
}

async function chargerAccueilOrganisations() {
  const info = await organisations();
  const choisie = remplirSelecteur($('accueil-org'), info.orgs || []);
  await choisirOrgAccueil(choisie);
}

$('accueil-org').addEventListener('change', async () => {
  await choisirOrgAccueil($('accueil-org').value);
  memoriserOrganisation($('accueil-org').value);
});
$('accueil-org-libre').addEventListener('change', async () => {
  const saisie = $('accueil-org-libre').value.trim();
  if (!saisie) return;
  await montrerCandidats(saisie);
  memoriserOrganisation(saisie);
});

// memoriserOrganisation retient l'organisation regardée, pour rouvrir dessus la
// prochaine fois. L'écriture n'a lieu que si le choix a vraiment changé.
function memoriserOrganisation(org) {
  if (!org || org === '__saisir' || org === etat.reglages.org) return;
  etat.reglages.org = org;
  api('PUT', '/api/settings', etat.reglages).catch(() => {});
}
$('accueil-recharger').addEventListener('click', () => {
  const org = $('accueil-org-libre').hidden
    ? $('accueil-org').value : $('accueil-org-libre').value.trim();
  if (org && org !== '__saisir') montrerCandidats(org, true);
});

async function choisirOrgAccueil(valeur) {
  const libre = valeur === '__saisir';
  $('accueil-org-libre').hidden = !libre;
  if (libre) {
    $('accueil-org-libre').value = etat.reglages.org || '';
    vider($('accueil-candidats'));
    return;
  }
  await montrerCandidats(valeur);
}

async function montrerCandidats(org, force) {
  const avis = $('accueil-org-avis');
  const conteneur = $('accueil-candidats');
  vider(avis);
  vider(conteneur);
  conteneur.append(el('div', { classe: 'boite-vide', texte: 'Lecture des dépôts…' }));

  const donnees = await tenter(() => api('GET',
    `/api/orgs/${encode(org)}/candidates${force ? '?refresh=1' : ''}`), 'Inventaire');
  vider(conteneur);
  if (!donnees) return;

  const candidats = donnees.candidates || [];
  if (candidats.length === 0) {
    conteneur.append(el('div', { classe: 'boite-vide' },
      el('p', { texte: `Aucun groupe repéré dans « ${org} ».` }),
      el('p', { classe: 'note',
        texte: `${donnees.total} dépôt(s) lus. Soit ils appartiennent déjà à un groupe ` +
          'déclaré, soit leurs noms ne laissent pas deviner de préfixe commun : ' +
          '« Nouveau groupe » permet alors de le déclarer à la main.' })));
    return;
  }
  for (const candidat of candidats) {
    conteneur.append(el('button', {
      classe: 'travail-ligne', type: 'button',
      onclick: () => adopter(candidat, org),
    },
      el('span', { classe: 'travail-infos' },
        el('span', { classe: 'titre', texte: candidat.prefix || "racine de l'organisation" }),
        el('span', { classe: 'detail',
          texte: candidat.assignments.join(', ') || 'aucun travail' })),
      el('span', { classe: 'espace' }),
      el('span', { classe: 'jeton', texte: `${candidat.students.length} compte(s)` }),
      el('span', { classe: 'jeton', texte: `${candidat.repos} dépôt(s)` }),
      el('span', { classe: 'jeton lien', texte: 'Adopter' })));
  }
}

// adopter déclare un groupe à partir d'un préfixe repéré : le nom est la seule
// chose à décider, le reste vient des dépôts.
async function adopter(candidat, org) {
  const nom = el('input', {
    type: 'text', classe: 'champ',
    value: candidat.prefix || org,
  });
  const confirme = await demander(`Adopter « ${candidat.prefix || org} »`, el('div', {},
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: 'Nom du groupe' }), nom),
    el('p', { classe: 'note',
      texte: `${travaux(candidat.assignments.length)} et ${candidat.students.length} ` +
        'compte(s) trouvés dans les dépôts existants. Les comptes deviennent la liste ' +
        "des étudiants ; aucun dépôt n'est touché." })), 'Adopter');
  if (!confirme) return;

  const cree = await tenter(() => api('POST', '/api/classrooms', {
    org,
    prefix: candidat.prefix,
    name: nom.value.trim() || candidat.prefix || org,
    students: candidat.students.map((compte) => ({ username: compte, full_name: '' })),
    roster_path: '',
    defaults: {},
  }), 'Groupe');
  if (!cree) return;
  message(`Groupe « ${cree.name} » adopté.`);
  await ouvrirGroupe(cree.id);
}

// --------------------------------------------------- déclaration d'un groupe

$('groupes-nouveau').addEventListener('click', async () => {
  etat.nouveau = { org: '', etudiants: [], rejets: [] };
  $('nouveau-prefixe').value = '';
  $('nouveau-nom').value = '';
  $('nouveau-chemin').value = etat.reglages.roster_path || '';
  $('nouveau-texte').value = '';
  $('nouveau-resume').textContent = '';
  vider($('nouveau-rejets'));
  $('nouveau-table').hidden = true;
  majApercuPrefixe();
  afficherVue('nouveau-groupe');
  await chargerOrganisations();
});

async function chargerOrganisations() {
  const info = await organisations();
  const choisie = remplirSelecteur($('nouveau-org'), info.orgs || []);
  await choisirOrganisation(choisie);
}

$('nouveau-org').addEventListener('change', () => choisirOrganisation($('nouveau-org').value));
$('nouveau-org-libre').addEventListener('change', () => {
  const saisie = $('nouveau-org-libre').value.trim();
  if (saisie) retenirOrganisation(saisie);
});

async function choisirOrganisation(valeur) {
  const libre = valeur === '__saisir';
  $('nouveau-org-libre').hidden = !libre;
  if (libre) {
    $('nouveau-org-libre').value = etat.nouveau.org || '';
    $('nouveau-org-libre').focus();
    return;
  }
  await retenirOrganisation(valeur);
}

async function retenirOrganisation(org) {
  const avis = $('nouveau-org-avis');
  vider(avis);
  const details = await tenter(() => api('GET', `/api/orgs/${encode(org)}`), 'Organisation');
  if (!details) return;
  etat.nouveau.org = details.login;
  if (details.warning) {
    avis.append(el('div', { classe: 'avis alerte', texte: details.warning }));
  }
  await chargerCandidats(details.login);
}

async function chargerCandidats(org) {
  const bloc = $('candidats-bloc');
  const conteneur = $('candidats-liste');
  vider(conteneur);
  bloc.hidden = true;

  const donnees = await tenter(() =>
    api('GET', `/api/orgs/${encode(org)}/candidates`), 'Inventaire');
  if (!donnees || !donnees.candidates || donnees.candidates.length === 0) return;

  bloc.hidden = false;
  for (const candidat of donnees.candidates) {
    const libelle = candidat.prefix || 'racine de l\'organisation';
    conteneur.append(el('button', {
      classe: 'travail-ligne', type: 'button',
      onclick: () => adopterCandidat(candidat),
    },
      el('span', { classe: 'travail-infos' },
        el('span', { classe: 'titre', texte: libelle }),
        el('span', { classe: 'detail',
          texte: candidat.assignments.join(', ') || 'aucun travail' })),
      el('span', { classe: 'espace' }),
      el('span', { classe: 'jeton', texte: `${candidat.students.length} compte(s)` }),
      el('span', { classe: 'jeton', texte: `${candidat.repos} dépôt(s)` })));
  }
}

// adopterCandidat reprend un préfixe déjà en place et les comptes qu'on y trouve.
function adopterCandidat(candidat) {
  $('nouveau-prefixe').value = candidat.prefix;
  if (!$('nouveau-nom').value.trim()) {
    $('nouveau-nom').value = candidat.prefix || etat.nouveau.org;
  }
  majApercuPrefixe();
  appliquerListeNouveau({
    people: candidat.students.map((compte) => ({ username: compte, full_name: '' })),
    issues: [],
  });
  message(`Préfixe « ${candidat.prefix || 'racine' } » repris, ` +
    `${candidat.students.length} compte(s) trouvé(s) dans les dépôts existants.`);
}

$('nouveau-prefixe').addEventListener('input', majApercuPrefixe);

function majApercuPrefixe() {
  const prefixe = $('nouveau-prefixe').value.trim();
  $('nouveau-apercu').textContent = prefixe ? `${prefixe}-travail-compte` : 'travail-compte';
}

$('nouveau-lire').addEventListener('click', async () => {
  const texte = $('nouveau-texte').value;
  if (!texte.trim()) { message('La zone de texte est vide.', 'alerte'); return; }
  const liste = await tenter(() => api('POST', '/api/roster/parse', { text: texte }), 'Liste');
  if (liste) appliquerListeNouveau(liste);
});

$('nouveau-charger').addEventListener('click', async () => {
  const chemin = $('nouveau-chemin').value.trim();
  if (!chemin) { message('Indiquez un chemin de fichier.', 'alerte'); return; }
  const liste = await tenter(() => api('POST', '/api/roster/load', { path: chemin }), 'Fichier');
  if (!liste) return;
  $('nouveau-chemin').value = liste.path;
  appliquerListeNouveau(liste);
});

$('nouveau-fichier').addEventListener('change', async (evenement) => {
  const fichier = evenement.target.files[0];
  if (!fichier) return;
  const texte = await fichier.text();
  $('nouveau-texte').value = texte;
  const liste = await tenter(() => api('POST', '/api/roster/parse', { text: texte }), 'Liste');
  if (liste) appliquerListeNouveau(liste);
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
  const org = $('nouveau-org-libre').hidden
    ? $('nouveau-org').value
    : $('nouveau-org-libre').value.trim();
  if (!org || org === '__saisir') { message('Choisissez une organisation.', 'alerte'); return; }

  const cree = await tenter(() => api('POST', '/api/classrooms', {
    org,
    prefix: $('nouveau-prefixe').value.trim(),
    name: $('nouveau-nom').value.trim(),
    students: etat.nouveau.etudiants,
    roster_path: $('nouveau-chemin').value.trim(),
    defaults: {},
  }), 'Groupe');
  if (!cree) return;
  message(`Groupe « ${cree.name} » créé.`);
  await ouvrirGroupe(cree.id);
});

// ---------------------------------------------------------------- travaux

function dessinerTravaux() {
  const groupe = etat.groupe;
  const sesTravaux = groupe.assignments || [];
  $('travaux-compte').textContent = travaux(sesTravaux.length);
  $('travaux-source').textContent = groupe.source
    ? `Dépôts de ${groupe.org} — source : ${groupe.source}`
    : '';

  const conteneur = $('travaux-liste');
  vider(conteneur);
  if (sesTravaux.length === 0) {
    conteneur.append(el('div', { classe: 'boite-vide' },
      el('p', { texte: 'Aucun travail dans ce groupe.' }),
      el('p', { classe: 'note',
        texte: '« Nouveau travail » crée un dépôt par étudiant du groupe.' })));
    return;
  }
  const total = (groupe.students || []).length;
  for (const travail of sesTravaux) {
    const detail = [`${travail.students} étudiant(s) du groupe sur ${total}`];
    if (travail.others > 0) detail.push(`${travail.others} dépôt(s) hors liste`);
    conteneur.append(el('button', {
      classe: 'travail-ligne', type: 'button', onclick: () => ouvrirTravail(travail),
    },
      el('span', { classe: 'travail-infos' },
        el('span', { classe: 'titre', texte: travail.name }),
        el('span', { classe: 'detail', texte: detail.join(' · ') })),
      el('span', { classe: 'espace' }),
      el('span', {
        classe: 'jeton ' + (total > 0 && travail.students >= total ? 'oui' : ''),
        texte: `${travail.repos} dépôt(s)`,
      }),
      el('span', { classe: 'chevron', texte: '›' })));
  }
}

$('travaux-recharger').addEventListener('click', () => ouvrirGroupe(etat.groupe.id, true));

// ------------------------------------------------------- détail d'un travail

async function ouvrirTravail(travail, force) {
  const chemin = `/api/orgs/${encode(etat.groupe.org)}/groups/${encode(travail.id)}` +
    (force ? '?refresh=1' : '');
  const detail = await tenter(() => api('GET', chemin), 'Travail');
  if (!detail) return;

  // « repos » compte les dépôts dans la fiche du travail et les énumère dans le
  // détail : on ne garde que la liste, sous un nom qui ne prête pas à confusion.
  etat.travail = { id: travail.id, name: travail.name, depots: detail.repos };
  etat.selection = new Set(detail.repos.map((repo) => repo.name));
  etat.acces.clear();
  $('fil-travail').textContent = travail.name;
  $('detail-titre').textContent = travail.name;
  dessinerTravail();
  afficherVue('travail');
}

function dessinerTravail() {
  const depots = etat.travail.depots;
  const total = (etat.groupe.students || []).length;
  const servis = depots.filter((repo) => estDuGroupe(repo.suffix)).length;
  $('detail-resume').textContent =
    `${depots.length} dépôt(s) · ${servis} étudiant(s) du groupe sur ${total} en ont un` +
    (depots.length - servis > 0 ? ` · ${depots.length - servis} hors liste` : '');

  const corps = $('detail-table').querySelector('tbody');
  vider(corps);
  for (const repo of depots) {
    const acces = etat.acces.get(repo.name);
    const nom = nomDe(repo.suffix) || repo.full_name;
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
      el('td', nom ? { texte: nom } : { classe: 'vide', texte: '@' + repo.suffix }),
      el('td', {}, el('a', {
        href: repo.url, target: '_blank', rel: 'noreferrer noopener', texte: repo.name,
      })),
      el('td', {}, el('span', { classe: 'jeton', texte: repo.visibility })),
      el('td', repo.pushed_at ? { texte: repo.pushed_at } : { classe: 'vide', texte: 'jamais' }),
      el('td', { texte: acces ? resumerAcces(acces) : '—' }),
      el('td', { classe: 'etroit' },
        el('button', { type: 'button', classe: 'lien', texte: 'Accès', onclick: () => panneauAcces(repo) }),
        el('button', { type: 'button', classe: 'lien', texte: 'Supprimer', onclick: () => supprimerDepot(repo) })),
    ));
  }
  majSelection();
}

// estDuGroupe dit si un suffixe de dépôt correspond à un étudiant inscrit.
function estDuGroupe(compte) {
  return (etat.groupe.students || []).some((personne) =>
    personne.username.toLowerCase() === compte.toLowerCase());
}

// nomDe retrouve le nom complet d'un étudiant du groupe.
function nomDe(compte) {
  const trouve = (etat.groupe.students || []).find((personne) =>
    personne.username.toLowerCase() === compte.toLowerCase());
  return trouve ? trouve.full_name : '';
}

function resumerAcces(acces) {
  const parts = [];
  if (acces.collaborators.length) parts.push(`${acces.collaborators.length} collab.`);
  if (acces.invitations.length) parts.push(`${acces.invitations.length} invit.`);
  return parts.length ? parts.join(' · ') : 'aucun';
}

function majSelection() {
  const total = etat.travail ? etat.travail.depots.length : 0;
  $('detail-selection').textContent = `${etat.selection.size} dépôt(s) sur ${total} sélectionné(s)`;
  $('detail-tout').checked = total > 0 && etat.selection.size === total;
}

$('detail-tout').addEventListener('change', (evenement) => {
  etat.selection = evenement.target.checked
    ? new Set(etat.travail.depots.map((repo) => repo.name))
    : new Set();
  dessinerTravail();
});

function selectionnes() {
  return etat.travail.depots.filter((repo) => etat.selection.has(repo.name));
}

// --- accès de tout le travail

$('detail-acces').addEventListener('click', async () => {
  const fiche = await tenter(() => api('POST',
    `/api/orgs/${encode(etat.groupe.org)}/groups/${encode(etat.travail.id)}/access`), 'Accès');
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
  await ouvrirGroupe(etat.groupe.id, true);
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
    lignes.push([nomDe(repo.suffix) || repo.full_name || '', repo.name, repo.url]);
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
  const destination = el('input', {
    type: 'text', classe: 'champ',
    value: `${parent.replace(/[\\/]+$/, '')}/${etat.travail.id}`,
  });
  const confirme = await demander(`Cloner ${choisis.length} dépôt(s)`, el('div', {},
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: 'Dossier de destination' }), destination),
    el('p', { classe: 'note', texte: `${etat.contexte.jobs} clonage(s) en parallèle` +
      (etat.contexte.depth ? `, profondeur ${etat.contexte.depth}` : '') })), 'Cloner');
  if (!confirme) return;

  const fiche = await tenter(() => api('POST', '/api/clones/clone', {
    org: etat.groupe.org,
    prefix: etat.travail.id,
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
  const dossier = el('input', {
    type: 'text', classe: 'champ',
    value: `${parent.replace(/[\\/]+$/, '')}/${etat.travail.id}`,
  });
  const trouve = await demander('Mettre à jour des clones', el('div', {},
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: 'Dossier contenant les clones' }), dossier)),
    'Chercher');
  if (!trouve) return;

  const liste = await tenter(() =>
    api('POST', '/api/clones/find', { directory: dossier.value.trim() }), 'Clones');
  if (!liste) return;

  const cases = liste.clones.map((item) => {
    const coche = el('input', { type: 'checkbox', checked: true, value: item.name });
    const horsTravail = !etat.travail.depots.some((repo) => repo.name === item.name);
    return el('label', { classe: 'case' }, coche,
      el('span', { texte: item.name + (horsTravail ? '   (hors travail)' : '') }));
  });
  const confirme = await demander(`${liste.clones.length} clone(s) trouvé(s)`,
    el('div', {}, cases), 'Mettre à jour');
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
  ouvrirAssistant(etat.travail.name, `Distribuer « ${etat.travail.name} »`, 3);
});

function ouvrirAssistant(nom, titre, etape = 1) {
  etat.reglagesTravail = Object.assign({}, etat.groupe.defaults);
  $('travail-nom').value = nom;
  $('fil-assistant').textContent = titre;
  ecrireReglagesTravail();
  dessinerDestinataires();
  afficherEtape(etape);
  afficherVue('assistant');
  if (etape === 1) $('travail-nom').focus();
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
  name_pattern: 'reglage-pattern',
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
$('reglage-pattern').addEventListener('input', majApercuDuNom);
$('travail-nom').addEventListener('change', planifierApercu);

// Le nom des dépôts se lit avant qu'ils existent : le gabarit est rendu au vol.
function majApercuDuNom() {
  const gabarit = $('reglage-pattern').value.trim() || '{assignment}-{username}';
  const prefixe = etat.groupe && etat.groupe.prefix ? etat.groupe.prefix + '-' : '';
  const valeurs = {
    assignment: prefixe + ($('travail-nom').value.trim() || 'travail'),
    username: 'compte', name: 'prenom-nom', fullname: 'Prénom Nom',
    first: 'prenom', last: 'nom', index: '01',
  };
  $('apercu-nom').textContent = gabarit.replace(/\{([a-z_]+)\}/g,
    (tout, champ) => (champ in valeurs ? valeurs[champ] : tout));
}

// --- destinataires

function dessinerDestinataires() {
  const etudiants = etat.groupe.students || [];
  etat.destinataires = new Set(etudiants.map((personne) => personne.username));

  const conteneur = $('dest-liste');
  vider(conteneur);
  for (const personne of etudiants) {
    const coche = el('input', {
      type: 'checkbox', checked: true, value: personne.username,
      onchange: (evenement) => {
        if (evenement.target.checked) etat.destinataires.add(personne.username);
        else etat.destinataires.delete(personne.username);
        majDestinataires();
        planifierApercu();
      },
    });
    conteneur.append(el('label', { classe: 'case' }, coche,
      el('span', {},
        personne.full_name ? personne.full_name + ' ' : '',
        el('span', { classe: 'compte', texte: '@' + personne.username }))));
  }
  majDestinataires();
}

function majDestinataires() {
  const total = (etat.groupe.students || []).length;
  $('dest-compte').textContent = `${etat.destinataires.size} étudiant(s) sur ${total}`;
  $('dest-tout').checked = total > 0 && etat.destinataires.size === total;
  for (const coche of $('dest-liste').querySelectorAll('input')) {
    coche.checked = etat.destinataires.has(coche.value);
  }
}

$('dest-tout').addEventListener('change', (evenement) => {
  etat.destinataires = evenement.target.checked
    ? new Set((etat.groupe.students || []).map((personne) => personne.username))
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
  return {
    name: $('travail-nom').value.trim(),
    settings: lireReglagesTravail(),
    usernames: [...etat.destinataires],
  };
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
      `/api/classrooms/${encode(etat.groupe.id)}/assignments/preview`, corpsDuTravail());
    for (const item of apercu.items) {
      corps.append(el('tr', {},
        el('td', {}, el('code', { texte: item.name })),
        el('td', { texte: item.full_name || '—' }),
        el('td', {}, el('code', { texte: '@' + item.username })),
        el('td', { classe: 'note', texte: item.description })));
    }
    table.hidden = apercu.items.length === 0;
    $('plan-resume').textContent =
      `${apercu.items.length} dépôt(s) à créer dans « ${etat.groupe.org} » — travail « ${apercu.assignment} »`;

    if (apercu.served && apercu.served.length) {
      $('deja-servis').append(el('div', { classe: 'avis',
        texte: `${apercu.served.length} étudiant(s) ont déjà un dépôt pour ce travail : ` +
          apercu.served.map((personne) => '@' + personne.username).join(', ') +
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
  if (!corps.name) { message('Donnez un nom au travail.', 'alerte'); return; }
  if (corps.usernames.length === 0) { message('Aucun étudiant retenu.', 'alerte'); return; }

  if (!simulation) {
    const confirme = await demander('Confirmer la distribution', el('div', {},
      el('p', { texte: `${corps.usernames.length} étudiant(s) du groupe « ${etat.groupe.name} », ` +
        `travail « ${corps.name} ».` }),
      el('p', { classe: 'note', texte:
        `Visibilité : ${corps.settings.visibility === 'public' ? 'public' : 'privé'}. ` +
        (corps.settings.add_collaborator
          ? `Invitations : oui (${corps.settings.permission}).` : 'Invitations : non.') +
        (corps.settings.template ? ` Modèle : ${corps.settings.template}.` : ' Dépôts neufs.') })),
      'Distribuer');
    if (!confirme) return;
  }

  const fiche = await tenter(() => api('POST',
    `/api/classrooms/${encode(etat.groupe.id)}/assignments`,
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
    journaliser(`${bilan.skipped.length} étudiant(s) avaient déjà un dépôt.`, 'dim');
  }
  if (bilan.json_path) journaliser(`Bilan : ${bilan.json_path}`, 'dim');
  if (simulation) return;

  // Une fois distribué, le travail s'ouvre — comme Classroom mène à la page du
  // devoir une fois créé.
  await ouvrirGroupe(etat.groupe.id, true);
  const travail = (etat.groupe.assignments || []).find((item) => item.id === bilan.assignment);
  if (travail) await ouvrirTravail(travail, true);
}

// ------------------------------------------------------------------ étudiants

async function chargerEtudiants(force) {
  const donnees = await tenter(() => api('GET',
    `/api/classrooms/${encode(etat.groupe.id)}/students${force ? '?refresh=1' : ''}`), 'Étudiants');
  if (!donnees) return;
  etat.etudiants = donnees.students || [];

  const corps = $('etudiants-table').querySelector('tbody');
  vider(corps);
  $('etudiants-table').hidden = etat.etudiants.length === 0;
  $('etudiants-vide').hidden = etat.etudiants.length > 0;

  let sansNom = 0;
  for (const ligne of etat.etudiants) {
    if (!ligne.full_name) sansNom++;
    corps.append(el('tr', {},
      el('td', ligne.full_name
        ? { texte: ligne.full_name }
        : { classe: 'vide', texte: 'nom inconnu' }),
      el('td', {}, el('code', { texte: '@' + ligne.username })),
      el('td', {}, ligne.assignments.length === 0
        ? el('span', { classe: 'vide', texte: 'aucun dépôt' })
        : el('span', { classe: 'etiquettes' }, ligne.assignments.map((travail) =>
            el('a', {
              classe: 'jeton lien', href: travail.url,
              target: '_blank', rel: 'noreferrer noopener', texte: travail.name,
            }))))));
  }

  $('etudiants-resume').textContent =
    `${etat.etudiants.length} étudiant(s) · ${travaux((donnees.assignments || []).length)}`;
  $('etudiants-noms').disabled = sansNom === 0;
  $('etudiants-noms').textContent = sansNom === 0
    ? 'Noms complets connus'
    : `Retrouver ${sansNom} nom(s) complet(s)`;
}

$('etudiants-recharger').addEventListener('click', () => chargerEtudiants(true));

$('etudiants-noms').addEventListener('click', async () => {
  const fiche = await tenter(() => api('POST',
    `/api/classrooms/${encode(etat.groupe.id)}/students/names`), 'Noms');
  if (!fiche) return;
  const bilan = await suivre(fiche);
  if (!bilan) return;
  message(`${bilan.resolved} nom(s) complet(s) retrouvé(s).`);
  await ouvrirGroupe(etat.groupe.id);
  afficherVue('etudiants');
});

$('etudiants-importer').addEventListener('click', async () => {
  const chemin = el('input', {
    type: 'text', classe: 'champ',
    value: etat.groupe.roster_path || '',
    placeholder: 'cohorte.csv',
  });
  const zone = el('textarea', { classe: 'champ', rows: '5',
    placeholder: 'Jean-Luc Picard, jlpicard' });
  const confirme = await demander('Remplacer la liste des étudiants', el('div', {},
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: 'Fichier CSV de la machine' }), chemin),
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
  } else if (chemin.value.trim()) {
    corps = { path: chemin.value.trim() };
  } else {
    message('Indiquez un fichier ou collez une liste.', 'alerte');
    return;
  }

  const bilan = await tenter(() => api('POST',
    `/api/classrooms/${encode(etat.groupe.id)}/students`, corps), 'Étudiants');
  if (!bilan) return;
  if (bilan.issues && bilan.issues.length) {
    message(`${bilan.issues.length} ligne(s) rejetée(s).`, 'alerte', 10000);
  }
  await ouvrirGroupe(etat.groupe.id);
  afficherVue('etudiants');
});

// -------------------------------------------------------- réglages du groupe

const champsGroupe = {
  name_pattern: 'gr-pattern',
  description_pattern: 'gr-description',
  template: 'gr-template',
  visibility: 'gr-visibilite',
  permission: 'gr-permission',
};

function ecrireReglagesGroupe() {
  const groupe = etat.groupe;
  $('gr-nom').value = groupe.name || '';
  $('gr-org').value = groupe.org || '';
  $('gr-prefixe').value = groupe.prefix || '';
  const defauts = groupe.defaults || {};
  for (const [cle, id] of Object.entries(champsGroupe)) {
    $(id).value = defauts[cle] || '';
  }
  $('gr-collaborateur').checked = defauts.add_collaborator !== false;
}

$('gr-enregistrer').addEventListener('click', async () => {
  const defauts = Object.assign({}, etat.groupe.defaults);
  for (const [cle, id] of Object.entries(champsGroupe)) {
    defauts[cle] = $(id).value.trim();
  }
  defauts.add_collaborator = $('gr-collaborateur').checked;

  const modifie = await tenter(() => api('PUT', `/api/classrooms/${encode(etat.groupe.id)}`, {
    name: $('gr-nom').value.trim(),
    org: $('gr-org').value.trim(),
    prefix: $('gr-prefixe').value.trim(),
    students: etat.groupe.students,
    roster_path: etat.groupe.roster_path || '',
    defaults: defauts,
  }), 'Groupe');
  if (!modifie) return;
  message('Réglages du groupe enregistrés.');
  await ouvrirGroupe(modifie.id, true);
  afficherVue('groupe-reglages');
});

$('gr-supprimer').addEventListener('click', async () => {
  const confirme = await demander(`Retirer « ${etat.groupe.name} » ?`, el('div', {},
    el('p', { texte: 'Le groupe disparaît de cette liste.' }),
    el('p', { classe: 'note',
      texte: "Aucun dépôt n'est supprimé sur GitHub : le groupe n'est qu'une vue locale." })),
    'Retirer le groupe');
  if (!confirme) return;

  const fait = await tenter(() =>
    api('DELETE', `/api/classrooms/${encode(etat.groupe.id)}`), 'Suppression');
  if (!fait) return;
  message(fait.message, 'succes', 10000);
  etat.groupe = null;
  afficherVue('groupes');
});

// ------------------------------------------------------- réglages généraux

async function rafraichirEmplacements() {
  const contexte = await api('GET', '/api/context').catch(() => null);
  if (!contexte) return;
  etat.contexte = contexte;
  dessinerPortees(contexte.scopes);
  dessinerChemins(contexte.paths);
  $('reglage-delay').value = etat.reglages.delay_seconds ?? 1;
}

$('reglages-enregistrer').addEventListener('click', async () => {
  etat.reglages.delay_seconds = Number($('reglage-delay').value) || 0;
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

function dessinerPortees(portees) {
  const conteneur = $('portees');
  vider(conteneur);
  for (const [nom, valeur] of Object.entries(portees)) {
    const ton = valeur === 'présente' ? 'oui' : (valeur === 'absente' ? 'non' : '');
    conteneur.append(el('span', { classe: 'jeton ' + ton, texte: `${nom} : ${valeur}` }));
  }
}

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

// ------------------------------------------------------- complétion de chemins

// brancherCompletion propose les chemins de la machine au fil de la frappe,
// comme le fait la tabulation au terminal. Le navigateur ne voit jamais le
// disque : c'est le serveur local qui répond.
function brancherCompletion(champ, liste, dossiersSeulement) {
  let minuterie = null;
  champ.addEventListener('input', () => {
    clearTimeout(minuterie);
    minuterie = setTimeout(async () => {
      const reponse = await api('POST', '/api/paths/suggest',
        { path: champ.value, dirs: dossiersSeulement }).catch(() => null);
      if (!reponse) return;
      vider(liste);
      for (const chemin of reponse.suggestions) liste.append(el('option', { value: chemin }));
    }, 250);
  });
}

brancherCompletion($('nouveau-chemin'), $('suggestions-roster'), false);
brancherCompletion($('reglage-starter'), $('suggestions-starter'), true);

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

async function demarrer() {
  const contexte = await tenter(() => api('GET', '/api/context'), 'Session');
  if (!contexte) return;
  etat.contexte = contexte;
  etat.reglages = contexte.settings;

  $('version').textContent = contexte.version;
  $('compte').textContent = `@${contexte.viewer} sur ${contexte.host}`;
  $('aide-champs').textContent = 'Champs disponibles : ' +
    contexte.placeholders.map((nom) => `{${nom}}`).join(', ');

  for (const id of ['reglage-permission', 'gr-permission']) {
    const droits = $(id);
    vider(droits);
    for (const option of contexte.permissions) {
      droits.append(el('option', { value: option.value, texte: option.label }));
    }
  }

  dessinerPortees(contexte.scopes);
  dessinerChemins(contexte.paths);
  $('reglage-delay').value = etat.reglages.delay_seconds ?? 1;
  if (!contexte.save_config) {
    $('reglages-etat').textContent = 'Mémorisation désactivée (--no-save-config)';
  }

  // Afficher la vue suffit à la remplir : elle recharge ce qu'elle montre.
  afficherVue('groupes');
}

demarrer();
