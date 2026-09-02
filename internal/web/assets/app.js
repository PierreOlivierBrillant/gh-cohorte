'use strict';

// Interface locale de gh cohorte, organisée comme GitHub Classroom :
// l'organisation tient lieu de cours, un travail rassemble les dépôts d'un
// même préfixe, et la liste des étudiants dit qui en a déjà un.
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
  organisation: '',
  travail: null,        // le travail ouvert : préfixe, dépôts, noms manquants
  selection: new Set(),
  acces: new Map(),
  personnes: [],        // liste chargée dans l'assistant
  rejets: [],
  etudiants: [],        // liste chargée dans l'onglet Étudiants
  ajoutAuTravail: '',   // préfixe quand l'assistant complète un travail existant
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

// Le détail d'un travail et l'assistant vivent sous l'onglet « Travaux ».
const ongletDeLaVue = {
  travaux: 'travaux', travail: 'travaux', assistant: 'travaux',
  etudiants: 'etudiants', reglages: 'reglages',
};

for (const bouton of $('onglets').querySelectorAll('button')) {
  bouton.addEventListener('click', () => afficherVue(bouton.dataset.vue));
}

function afficherVue(nom) {
  for (const bouton of $('onglets').querySelectorAll('button')) {
    bouton.classList.toggle('actif', bouton.dataset.vue === ongletDeLaVue[nom]);
  }
  for (const vue of document.querySelectorAll('.vue')) {
    vue.hidden = vue.id !== 'vue-' + nom;
  }
  window.scrollTo(0, 0);
  // Les emplacements changent au fil de la session : un bilan écrit, un cache
  // rempli. La page les redemande à chaque visite plutôt que de les figer.
  if (nom === 'reglages') rafraichirEmplacements();
  if (nom === 'etudiants') ouvrirEtudiants();
}

async function rafraichirEmplacements() {
  const contexte = await api('GET', '/api/context').catch(() => null);
  if (!contexte) return;
  etat.contexte = contexte;
  dessinerPortees(contexte.scopes);
  dessinerChemins(contexte.paths);
}

// -------------------------------------------------------------- organisations

async function chargerOrganisations() {
  const donnees = await tenter(() => api('GET', '/api/orgs'), 'Organisations');
  const choix = $('organisation');
  vider(choix);
  const liste = (donnees && donnees.orgs) || [];
  for (const acces of liste) {
    choix.append(el('option', { value: acces.login, texte: acces.label }));
  }
  choix.append(el('option', { value: '__saisir', texte: 'Saisir un autre nom…' }));
  if (donnees && donnees.notice) message(donnees.notice, 'alerte', 12000);

  const memorisee = etat.reglages.org;
  if (memorisee && liste.some((acces) => acces.login.toLowerCase() === memorisee.toLowerCase())) {
    choix.value = memorisee;
  } else if (liste.length > 0) {
    choix.value = liste[0].login;
  } else {
    choix.value = '__saisir';
  }
  await choisirOrganisation(choix.value);
}

$('organisation').addEventListener('change', () => choisirOrganisation($('organisation').value));

$('organisation-libre').addEventListener('change', async () => {
  const saisie = $('organisation-libre').value.trim();
  if (saisie) await retenirOrganisation(saisie);
});

async function choisirOrganisation(valeur) {
  const libre = valeur === '__saisir';
  $('organisation-libre').hidden = !libre;
  if (libre) {
    $('organisation-libre').value = etat.organisation || '';
    $('organisation-libre').focus();
    return;
  }
  await retenirOrganisation(valeur);
}

async function retenirOrganisation(org) {
  const details = await tenter(() => api('GET', `/api/orgs/${encode(org)}`), 'Organisation');
  if (!details) return;
  etat.organisation = details.login;
  etat.reglages.org = details.login;
  etat.travail = null;
  etat.ajoutAuTravail = '';
  etat.etudiants = [];
  $('cours-titre').textContent = details.name;
  $('cours-sous-titre').textContent =
    `Organisation ${details.login} sur ${etat.contexte.host} · un dépôt par étudiant et par travail`;
  if (details.warning) message(details.warning, 'alerte', 12000);
  afficherVue('travaux');
  await chargerTravaux(false);
}

// ---------------------------------------------------------- liste des travaux

async function chargerTravaux(force) {
  if (!etat.organisation) return;
  const chemin = `/api/orgs/${encode(etat.organisation)}/groups${force ? '?refresh=1' : ''}`;
  const donnees = await tenter(() => api('GET', chemin), 'Inventaire');
  if (!donnees) return;

  const travaux = donnees.groups || [];
  $('travaux-compte').textContent = travaux.length === 1 ? '1 travail' : `${travaux.length} travaux`;
  $('travaux-source').textContent =
    `${donnees.total} dépôt(s) dans l'organisation — source : ${donnees.source}`;

  const conteneur = $('travaux-liste');
  vider(conteneur);
  if (travaux.length === 0) {
    conteneur.append(el('div', { classe: 'boite-vide' },
      el('p', { texte: 'Aucun travail dans cette organisation.' }),
      el('p', { classe: 'note',
        texte: 'Un travail est un groupe de dépôts partageant un préfixe, ' +
          'par exemple « tp1-jlpicard » et « tp1-emilie-cote ».' })));
    return;
  }
  for (const item of travaux) {
    conteneur.append(el('button', {
      classe: 'travail-ligne', type: 'button', onclick: () => ouvrirTravail(item.prefix),
    },
      el('span', { classe: 'travail-infos' },
        el('span', { classe: 'titre', texte: item.prefix }),
        el('span', { classe: 'detail', texte: 'Travail individuel · un dépôt par étudiant' })),
      el('span', { classe: 'espace' }),
      el('span', { classe: 'jeton', texte: `${item.count} dépôt(s)` }),
      el('span', { classe: 'chevron', texte: '›' })));
  }
}

$('travaux-recharger').addEventListener('click', () => chargerTravaux(true));
$('travail-retour').addEventListener('click', () => afficherVue('travaux'));
$('assistant-retour').addEventListener('click', () => afficherVue('travaux'));

// --------------------------------------------------------- détail d'un travail

async function ouvrirTravail(prefixe, force) {
  const chemin = `/api/orgs/${encode(etat.organisation)}/groups/${encode(prefixe)}` +
    (force ? '?refresh=1' : '');
  const travail = await tenter(() => api('GET', chemin), 'Travail');
  if (!travail) return;

  etat.travail = travail;
  etat.selection = new Set(travail.repos.map((repo) => repo.name));
  etat.acces.clear();
  $('fil-travail').textContent = travail.prefix;
  $('detail-titre').textContent = travail.prefix;
  majBoutonNoms();
  dessinerTravail();
  afficherVue('travail');
}

// majBoutonNoms reflète ce qu'il reste de noms complets à retrouver.
function majBoutonNoms() {
  const restants = etat.travail ? etat.travail.missing_names : 0;
  $('detail-noms').disabled = restants === 0;
  $('detail-noms').textContent = restants === 0
    ? 'Noms complets connus'
    : `Retrouver ${restants} nom(s)`;
}

function dessinerTravail() {
  const travail = etat.travail;
  $('detail-resume').textContent = resumerTravail(travail);

  const corps = $('detail-table').querySelector('tbody');
  vider(corps);
  for (const repo of travail.repos) {
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
      el('td', repo.full_name
        ? { texte: repo.full_name }
        : { classe: 'vide', texte: '@' + repo.suffix }),
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

// resumerTravail dit combien d'étudiants ont un dépôt. Quand la liste de la
// cohorte est chargée, la proportion se lit comme le « X of Y students
// accepted » de Classroom ; sinon, seuls les dépôts sont comptés.
function resumerTravail(travail) {
  const total = travail.repos.length;
  if (etat.etudiants.length > 0) {
    const servis = new Set(travail.repos.map((repo) => repo.suffix.toLowerCase()));
    const comptes = etat.etudiants.filter((personne) =>
      servis.has(personne.username.toLowerCase())).length;
    return `${total} dépôt(s) · ${comptes} étudiant(s) sur ${etat.etudiants.length} ` +
      'de la liste ont un dépôt';
  }
  const nommes = travail.repos.filter((repo) => repo.full_name).length;
  return `${total} dépôt(s) · ${nommes} étudiant(s) identifié(s) par leur nom complet`;
}

function resumerAcces(acces) {
  const parts = [];
  if (acces.collaborators.length) parts.push(`${acces.collaborators.length} collab.`);
  if (acces.invitations.length) parts.push(`${acces.invitations.length} invit.`);
  return parts.length ? parts.join(' · ') : 'aucun';
}

function majSelection() {
  const total = etat.travail ? etat.travail.repos.length : 0;
  $('detail-selection').textContent = `${etat.selection.size} dépôt(s) sur ${total} sélectionné(s)`;
  $('detail-tout').checked = total > 0 && etat.selection.size === total;
}

$('detail-tout').addEventListener('change', (evenement) => {
  etat.selection = evenement.target.checked
    ? new Set(etat.travail.repos.map((repo) => repo.name))
    : new Set();
  dessinerTravail();
});

function selectionnes() {
  return etat.travail.repos.filter((repo) => etat.selection.has(repo.name));
}

// --- noms complets

$('detail-noms').addEventListener('click', async () => {
  const fiche = await tenter(() => api('POST',
    `/api/orgs/${encode(etat.organisation)}/groups/${encode(etat.travail.prefix)}/names`), 'Noms');
  if (!fiche) return;
  const noms = await suivre(fiche);
  if (!noms) return;
  for (const repo of etat.travail.repos) {
    if (noms[repo.name]) repo.full_name = noms[repo.name];
  }
  etat.travail.missing_names = etat.travail.repos.filter((repo) => !repo.full_name).length;
  majBoutonNoms();
  dessinerTravail();
});

// --- accès de tout le travail

$('detail-acces').addEventListener('click', async () => {
  const fiche = await tenter(() => api('POST',
    `/api/orgs/${encode(etat.organisation)}/groups/${encode(etat.travail.prefix)}/access`), 'Accès');
  if (!fiche) return;
  const resultats = await suivre(fiche);
  if (!Array.isArray(resultats)) return;
  for (const acces of resultats) etat.acces.set(acces.repo, acces);
  dessinerTravail();
});

// --- accès d'un dépôt

async function panneauAcces(repo) {
  const acces = await tenter(() =>
    api('GET', `/api/orgs/${encode(etat.organisation)}/repos/${encode(repo.name)}/access`), 'Accès');
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
          `/api/orgs/${encode(etat.organisation)}/repos/${encode(repo.name)}/collaborators/${encode(login)}`);
      }, redessiner));
    }
    for (const invitation of courant.invitations) {
      liste.append(ligneAcces(repo, invitation.login, 'invitation en attente', async () => {
        await api('DELETE',
          `/api/orgs/${encode(etat.organisation)}/repos/${encode(repo.name)}/invitations/${invitation.id}`);
      }, redessiner));
    }
  };
  redessiner();

  const compte = el('input', { type: 'text', classe: 'champ', placeholder: 'compte GitHub' });
  const droit = el('select', { classe: 'champ' }, etat.contexte.permissions.map(
    (option) => el('option', { value: option.value, texte: option.label })));
  droit.value = etat.reglages.permission || 'push';
  const ajout = el('div', { classe: 'ligne-champ' }, compte, droit,
    el('button', {
      type: 'button',
      classe: 'bouton vert',
      texte: 'Inviter',
      onclick: async () => {
        const succes = await tenter(() => api('POST',
          `/api/orgs/${encode(etat.organisation)}/repos/${encode(repo.name)}/collaborators`,
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
    api('GET', `/api/orgs/${encode(etat.organisation)}/repos/${encode(repo.name)}/access`), 'Accès');
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
    `/api/orgs/${encode(etat.organisation)}/repos/${encode(repo.name)}`,
    { confirm: saisie.value.trim() }), 'Suppression');
  if (!fait) return;
  message(fait.message);
  const prefixe = etat.travail.prefix;
  await chargerTravaux(true);
  await ouvrirTravail(prefixe, true);
}

// --- URL

$('detail-copier').addEventListener('click', async () => {
  const urls = selectionnes().map((repo) => repo.url).join('\n');
  if (!urls) { message('Aucun dépôt sélectionné.', 'alerte'); return; }
  try {
    await navigator.clipboard.writeText(urls);
    message(`${etat.selection.size} URL copiée(s).`);
  } catch {
    message('Copie refusée par le navigateur : utilisez l\'export CSV.', 'alerte');
  }
});

$('detail-csv').addEventListener('click', () => {
  const choisis = selectionnes();
  if (!choisis.length) { message('Aucun dépôt sélectionné.', 'alerte'); return; }
  const lignes = [['nom_complet', 'depot', 'url']];
  for (const repo of choisis) lignes.push([repo.full_name || '', repo.name, repo.url]);
  const contenu = lignes.map((ligne) =>
    ligne.map((valeur) => `"${String(valeur).replace(/"/g, '""')}"`).join(',')).join('\n');
  telecharger(`${etat.travail.prefix}-urls.csv`, contenu, 'text/csv');
});

function telecharger(nom, contenu, type) {
  const adresse = URL.createObjectURL(new Blob([contenu], { type: `${type};charset=utf-8` }));
  const lien = el('a', { href: adresse, download: nom });
  document.body.append(lien);
  lien.click();
  lien.remove();
  URL.revokeObjectURL(adresse);
}

// --- clonage

$('detail-cloner').addEventListener('click', async () => {
  const choisis = selectionnes();
  if (!choisis.length) { message('Aucun dépôt sélectionné.', 'alerte'); return; }

  const parent = etat.reglages.clone_dir || '.';
  const destination = el('input', {
    type: 'text', classe: 'champ',
    value: `${parent.replace(/[\\/]+$/, '')}/${etat.travail.prefix}`,
  });
  const confirme = await demander(`Cloner ${choisis.length} dépôt(s)`, el('div', {},
    el('label', { classe: 'champ-bloc' },
      el('span', { classe: 'etiquette', texte: 'Dossier de destination' }), destination),
    el('p', { classe: 'note', texte: `${etat.contexte.jobs} clonage(s) en parallèle` +
      (etat.contexte.depth ? `, profondeur ${etat.contexte.depth}` : '') })), 'Cloner');
  if (!confirme) return;

  const fiche = await tenter(() => api('POST', '/api/clones/clone', {
    org: etat.organisation,
    prefix: etat.travail.prefix,
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

// --- mise à jour des clones

$('detail-pull').addEventListener('click', async () => {
  const parent = etat.reglages.clone_dir || '.';
  const dossier = el('input', {
    type: 'text', classe: 'champ',
    value: `${parent.replace(/[\\/]+$/, '')}/${etat.travail.prefix}`,
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
    const horsTravail = etat.travail && !etat.travail.repos.some((repo) => repo.name === item.name);
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
  etat.ajoutAuTravail = '';
  etat.reglages.assignment = '';
  ecrireReglages();
  $('fil-assistant').textContent = 'Nouveau travail';
  $('assistant-ajout').hidden = true;
  $('detail-ajouter').blur();
  afficherEtape(1);
  afficherVue('assistant');
  $('reglage-assignment').focus();
});

// « Ajouter des étudiants » reprend un travail existant : les bases et le code
// de départ en viennent, seule la liste manque.
$('detail-ajouter').addEventListener('click', async () => {
  const prefixe = etat.travail.prefix;
  etat.ajoutAuTravail = prefixe;
  etat.reglages.assignment = prefixe;

  const modele = await tenter(() => api('GET',
    `/api/orgs/${encode(etat.organisation)}/groups/${encode(prefixe)}/template`));
  if (modele) etat.reglages.template = modele.template;
  ecrireReglages();

  $('fil-assistant').textContent = `Ajouter des étudiants à « ${prefixe} »`;
  const avis = $('assistant-ajout');
  avis.hidden = false;
  avis.textContent = modele && modele.template
    ? `Ajout au travail « ${prefixe} » — modèle réutilisé : ${modele.template}. ` +
      'Les étudiants ayant déjà un dépôt seront écartés.'
    : `Ajout au travail « ${prefixe} ». Les étudiants ayant déjà un dépôt seront écartés.`;

  afficherEtape(3);
  afficherVue('assistant');
  planifierApercu();
});

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

// ------------------------------------------------------------------ réglages

const champsReglages = {
  assignment: 'reglage-assignment',
  name_pattern: 'reglage-pattern',
  description_pattern: 'reglage-description',
  template: 'reglage-template',
  permission: 'reglage-permission',
  commit_message: 'reglage-commit',
  starter_dir: 'reglage-starter',
};

function ecrireReglages() {
  for (const [cle, id] of Object.entries(champsReglages)) {
    $(id).value = etat.reglages[cle] || '';
  }
  const publique = etat.reglages.visibility === 'public';
  $('visibilite-publique').checked = publique;
  $('visibilite-privee').checked = !publique;
  $('reglage-delay').value = etat.reglages.delay_seconds ?? 1;
  $('reglage-collaborateur').checked = etat.reglages.add_collaborator !== false;
  $('roster-chemin').value = etat.reglages.roster_path || '';
  majApercuDuNom();
}

function lireReglages() {
  for (const [cle, id] of Object.entries(champsReglages)) {
    etat.reglages[cle] = $(id).value.trim();
  }
  // Changer l'identifiant du travail sort du mode « ajout » : la liste n'a plus
  // de raison d'être filtrée sur les dépôts de l'ancien.
  if (etat.ajoutAuTravail && etat.reglages.assignment !== etat.ajoutAuTravail) {
    etat.ajoutAuTravail = '';
    $('assistant-ajout').hidden = true;
  }
  etat.reglages.visibility = $('visibilite-publique').checked ? 'public' : 'private';
  etat.reglages.delay_seconds = Number($('reglage-delay').value) || 0;
  etat.reglages.add_collaborator = $('reglage-collaborateur').checked;
  etat.reglages.org = etat.organisation;
  return etat.reglages;
}

for (const id of Object.values(champsReglages)) {
  $(id).addEventListener('change', () => { lireReglages(); planifierApercu(); });
}
for (const id of ['visibilite-privee', 'visibilite-publique', 'reglage-collaborateur', 'reglage-delay']) {
  $(id).addEventListener('change', lireReglages);
}

// Le nom des dépôts se lit avant d'exister : le gabarit est rendu au vol.
$('reglage-assignment').addEventListener('input', majApercuDuNom);
$('reglage-pattern').addEventListener('input', majApercuDuNom);

function majApercuDuNom() {
  const gabarit = $('reglage-pattern').value.trim() || '{assignment}-{username}';
  const valeurs = {
    assignment: $('reglage-assignment').value.trim() || 'travail',
    username: 'compte', name: 'prenom-nom', fullname: 'Prénom Nom',
    first: 'prenom', last: 'nom', index: '01',
  };
  $('apercu-nom').textContent = gabarit.replace(/\{([a-z_]+)\}/g,
    (tout, champ) => (champ in valeurs ? valeurs[champ] : tout));
}

$('reglages-enregistrer').addEventListener('click', async () => {
  const bilan = await tenter(() => api('PUT', '/api/settings', lireReglages()), 'Réglages');
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
  await chargerTravaux(false);
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

brancherCompletion($('roster-chemin'), $('suggestions-roster'), false);
brancherCompletion($('etudiants-chemin'), $('suggestions-etudiants'), false);
brancherCompletion($('reglage-starter'), $('suggestions-starter'), true);

// ------------------------------------------------------ liste dans l'assistant

$('roster-lire').addEventListener('click', async () => {
  const texte = $('roster-texte').value;
  if (!texte.trim()) { message('La zone de texte est vide.', 'alerte'); return; }
  const liste = await tenter(() => api('POST', '/api/roster/parse', { text: texte }), 'Liste');
  if (liste) appliquerListe(liste);
});

$('roster-charger').addEventListener('click', async () => {
  const chemin = $('roster-chemin').value.trim();
  if (!chemin) { message('Indiquez un chemin de fichier.', 'alerte'); return; }
  const liste = await tenter(() => api('POST', '/api/roster/load', { path: chemin }), 'Fichier');
  if (!liste) return;
  etat.reglages.roster_path = liste.path;
  appliquerListe(liste);
});

$('roster-fichier').addEventListener('change', async (evenement) => {
  const fichier = evenement.target.files[0];
  if (!fichier) return;
  const texte = await fichier.text();
  $('roster-texte').value = texte;
  const liste = await tenter(() => api('POST', '/api/roster/parse', { text: texte }), 'Liste');
  if (liste) appliquerListe(liste);
});

$('roster-enregistrer').addEventListener('click', async () => {
  if (!etat.personnes.length) { message('Aucun étudiant à enregistrer.', 'alerte'); return; }
  const chemin = el('input', {
    type: 'text', classe: 'champ',
    value: $('roster-chemin').value.trim() || 'cohorte.csv',
  });
  const confirme = await demander('Enregistrer la liste', el('label', { classe: 'champ-bloc' },
    el('span', { classe: 'etiquette', texte: 'Chemin du fichier CSV' }), chemin), 'Enregistrer');
  if (!confirme) return;
  const bilan = await tenter(() => api('POST', '/api/roster/save',
    { path: chemin.value.trim(), people: etat.personnes }), 'Enregistrement');
  if (!bilan) return;
  etat.reglages.roster_path = bilan.path;
  $('roster-chemin').value = bilan.path;
  message(`Liste enregistrée : ${bilan.path}`);
});

function appliquerListe(liste) {
  etat.personnes = liste.people || [];
  etat.rejets = liste.issues || [];
  if (liste.path) $('roster-chemin').value = liste.path;

  $('roster-resume').textContent =
    `${etat.personnes.length} étudiant(s) retenu(s)` +
    (etat.rejets.length ? `, ${etat.rejets.length} ligne(s) rejetée(s)` : '');

  const rejets = $('roster-rejets');
  vider(rejets);
  if (etat.rejets.length) {
    const details = el('details', {},
      el('summary', { texte: `${etat.rejets.length} ligne(s) rejetée(s)` }));
    const corps = el('div', { classe: 'corps' });
    for (const rejet of etat.rejets.slice(0, 30)) {
      corps.append(el('div', { classe: 'note',
        texte: (rejet.line > 0 ? `ligne ${rejet.line}` : 'fichier') + ` : ${rejet.message}` }));
    }
    details.append(corps);
    rejets.append(details);
  }
  planifierApercu();
}

// ------------------------------------------------------------ modèle et départ

$('template-verifier').addEventListener('click', async () => {
  const reference = $('reglage-template').value.trim();
  if (!reference) { message('Aucun modèle : les dépôts seront créés neufs.'); return; }
  const bilan = await tenter(() => api('POST', '/api/template/check', { template: reference }), 'Modèle');
  if (!bilan) return;
  $('reglage-template').value = bilan.template;
  etat.reglages.template = bilan.template;
  if (bilan.warning) message(bilan.warning, 'alerte', 12000);
  else message(`Modèle vérifié : ${bilan.template}.`);
});

$('starter-inspecter').addEventListener('click', async () => {
  const chemin = $('reglage-starter').value.trim();
  const resume = $('starter-resume');
  vider(resume);
  if (!chemin) { etat.reglages.starter_dir = ''; return; }

  const bundle = await tenter(() =>
    api('POST', '/api/starter/inspect', { path: chemin }), 'Fichiers de départ');
  if (!bundle) return;
  $('reglage-starter').value = bundle.root;
  etat.reglages.starter_dir = bundle.root;

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

// -------------------------------------------------------------------- aperçu

let minuterieApercu = null;

function planifierApercu() {
  clearTimeout(minuterieApercu);
  minuterieApercu = setTimeout(rafraichirApercu, 350);
}

async function rafraichirApercu() {
  const erreur = $('plan-erreur');
  vider(erreur);
  const table = $('plan-table');
  const corps = table.querySelector('tbody');
  vider(corps);
  table.hidden = true;
  $('plan-resume').textContent = '';

  if (!etat.personnes.length || !etat.organisation) return;
  try {
    const plan = await api('POST', '/api/plan',
      { settings: lireReglages(), people: etat.personnes });
    for (const item of plan.items) {
      corps.append(el('tr', {},
        el('td', {}, el('code', { texte: item.name })),
        el('td', { texte: item.full_name }),
        el('td', {}, el('code', { texte: '@' + item.username })),
        el('td', { classe: 'note', texte: item.description })));
    }
    table.hidden = plan.items.length === 0;
    $('plan-resume').textContent = `${plan.items.length} dépôt(s) dans « ${etat.organisation} »` +
      (etat.ajoutAuTravail ? ` — ajout au travail « ${etat.ajoutAuTravail} »` : '');
  } catch (probleme) {
    erreur.append(el('div', { classe: 'avis erreur', texte: probleme.message }));
  }
}

// ---------------------------------------------------- vérification et création

$('comptes-verifier').addEventListener('click', async () => {
  if (!etat.personnes.length) { message('Aucun étudiant à vérifier.', 'alerte'); return; }
  const fiche = await tenter(() =>
    api('POST', '/api/accounts/verify', { people: etat.personnes }), 'Vérification');
  if (!fiche) return;
  const bilan = await suivre(fiche);
  if (!bilan || !bilan.missing) return;

  if (bilan.missing.length === 0) {
    journaliser(`${bilan.checked} compte(s) vérifié(s), aucun manquant.`, 'ok');
    message(`${bilan.checked} compte(s) vérifié(s).`);
    return;
  }
  const liste = el('div', {}, bilan.missing.map((personne) =>
    el('div', { classe: 'note', texte: `${personne.full_name} → @${personne.username}` })));
  const retirer = await demander(`${bilan.missing.length} compte(s) introuvable(s)`,
    el('div', {}, liste, el('p', { classe: 'note',
      texte: 'Retirer ces étudiants de la liste, ou poursuivre malgré tout ' +
        '(les invitations échoueront) ?' })), 'Retirer ces étudiants');
  if (!retirer) return;
  const exclus = new Set(bilan.missing.map((personne) => personne.username.toLowerCase()));
  etat.personnes = etat.personnes.filter((personne) => !exclus.has(personne.username.toLowerCase()));
  appliquerListe({ people: etat.personnes, issues: etat.rejets });
});

// visibiliteEnMots met la visibilité dans la langue de l'interface.
function visibiliteEnMots(valeur) {
  return valeur === 'public' ? 'public' : 'privé';
}

$('lancer-simulation').addEventListener('click', () => lancer(true));
$('lancer-creation').addEventListener('click', () => lancer(false));

async function lancer(simulation) {
  if (!etat.personnes.length) { message('Aucun étudiant dans la liste.', 'alerte'); return; }
  const reglages = lireReglages();

  if (!simulation) {
    const confirme = await demander('Confirmer la création', el('div', {},
      el('p', { texte: `${etat.personnes.length} étudiant(s) — organisation « ${reglages.org} », ` +
        `travail « ${reglages.assignment} ».` }),
      el('p', { classe: 'note', texte: `Visibilité : ${visibiliteEnMots(reglages.visibility)}. ` +
        (reglages.add_collaborator ? `Invitations : oui (${reglages.permission}).` : 'Invitations : non.') +
        (reglages.template ? ` Modèle : ${reglages.template}.` : ' Dépôts neufs.') })),
      'Créer les dépôts');
    if (!confirme) return;
  }

  const fiche = await tenter(() => api('POST', '/api/create', {
    settings: reglages,
    people: etat.personnes,
    dry_run: simulation,
    force_starter: $('creer-force').checked,
    group: etat.ajoutAuTravail,
  }), simulation ? 'Simulation' : 'Création');
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

  // Après une création réelle, le travail s'ouvre sur ses dépôts, comme
  // Classroom mène à la page du devoir une fois créé.
  const prefixe = reglages.assignment;
  etat.ajoutAuTravail = '';
  $('assistant-ajout').hidden = true;
  await chargerTravaux(true);
  await ouvrirTravail(prefixe, true);
}

// ------------------------------------------------------------------ étudiants

async function ouvrirEtudiants() {
  if (etat.etudiants.length === 0 && etat.reglages.roster_path) {
    $('etudiants-chemin').value = etat.reglages.roster_path;
    await chargerEtudiants(etat.reglages.roster_path);
    return;
  }
  await croiserEtudiants();
}

$('etudiants-charger').addEventListener('click', () => chargerEtudiants($('etudiants-chemin').value.trim()));
$('etudiants-recharger').addEventListener('click', () => croiserEtudiants());

async function chargerEtudiants(chemin) {
  if (!chemin) { message('Indiquez le chemin de la liste.', 'alerte'); return; }
  const liste = await tenter(() => api('POST', '/api/roster/load', { path: chemin }), 'Liste');
  if (!liste) return;
  etat.etudiants = liste.people || [];
  etat.reglages.roster_path = liste.path;
  $('etudiants-chemin').value = liste.path;
  $('roster-chemin').value = liste.path;
  await croiserEtudiants(liste.issues);
}

async function croiserEtudiants(rejets) {
  const corps = $('etudiants-table').querySelector('tbody');
  vider(corps);
  const vide = $('etudiants-vide');
  if (etat.etudiants.length === 0) {
    $('etudiants-table').hidden = true;
    vide.hidden = false;
    $('etudiants-resume').textContent = '';
    return;
  }
  $('etudiants-table').hidden = false;
  vide.hidden = true;

  const croisement = await tenter(() => api('POST',
    `/api/orgs/${encode(etat.organisation)}/students`, { people: etat.etudiants }), 'Étudiants');
  const lignes = (croisement && croisement.students) || etat.etudiants.map((personne) => ({
    full_name: personne.full_name, username: personne.username, assignments: [],
  }));

  let servis = 0;
  for (const ligne of lignes) {
    if (ligne.assignments.length > 0) servis++;
    corps.append(el('tr', {},
      el('td', { texte: ligne.full_name }),
      el('td', {}, el('code', { texte: '@' + ligne.username })),
      el('td', {}, ligne.assignments.length === 0
        ? el('span', { classe: 'vide', texte: 'aucun dépôt' })
        : el('span', { classe: 'etiquettes' }, ligne.assignments.map((travail) =>
            el('a', {
              classe: 'jeton lien', href: travail.url,
              target: '_blank', rel: 'noreferrer noopener', texte: travail.prefix,
            }))))));
  }

  $('etudiants-resume').textContent =
    `${lignes.length} étudiant(s) · ${servis} ayant au moins un dépôt` +
    (rejets && rejets.length ? ` · ${rejets.length} ligne(s) rejetée(s)` : '');
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

async function demarrer() {
  const contexte = await tenter(() => api('GET', '/api/context'), 'Session');
  if (!contexte) return;
  etat.contexte = contexte;
  etat.reglages = contexte.settings;

  $('version').textContent = contexte.version;
  $('compte').textContent = `@${contexte.viewer}`;
  $('aide-champs').textContent = 'Champs disponibles : ' +
    contexte.placeholders.map((nom) => `{${nom}}`).join(', ');

  const droits = $('reglage-permission');
  vider(droits);
  for (const option of contexte.permissions) {
    droits.append(el('option', { value: option.value, texte: option.label }));
  }

  dessinerPortees(contexte.scopes);
  dessinerChemins(contexte.paths);
  if (!contexte.save_config) {
    $('reglages-etat').textContent = 'Mémorisation désactivée (--no-save-config)';
  }

  ecrireReglages();
  afficherEtape(1);
  await chargerOrganisations();
}

demarrer();
