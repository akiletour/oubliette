const en = {
  signInTitle: 'Sign in to Oubliette',
  email: 'Email',
  password: 'Password',
  signIn: 'Sign in',
  signingIn: 'Signing in…',
  signInRefused: 'Wrong email or password.',
  signInSlowedDown: 'Too many attempts. Try again in a few minutes.',
  unreachable: 'The server did not answer. Try again.',
  failed: 'Something went wrong. Try again.',
  inboxes: 'Inboxes',
  messages: 'Messages',
  messageCount: (count: number) => (count === 1 ? '1 message' : `${count} messages`),
  signOut: 'Sign out',
  toDarkTheme: 'Switch to the dark theme',
  toLightTheme: 'Switch to the light theme',
  noSubject: 'No subject',
  emptyInbox: 'No message yet',
  emptyInboxHint: 'Messages sent with the credentials of this inbox will appear here.',
  loadFailed: 'The messages could not be loaded.',
  olderMessages: 'Show older messages',
  inboxMissing: 'This inbox does not exist.',
  goToInbox: (name: string) => `Go to ${name}`,
  closeMessage: 'Close the message',
  messageViews: 'Message views',
  rendered: 'Rendered',
  text: 'Text',
  renderingOf: (subject: string) => `Rendering of “${subject}”`,
  noTextVersion: 'This message has no text version.',
  messageMissing: 'This message does not exist.',
  messageFailed: 'The message could not be loaded.',
  newInbox: 'New inbox',
  inboxName: 'Name',
  create: 'Create',
  cancel: 'Cancel',
  renameInbox: 'Rename the inbox',
  rename: 'Rename',
  deleteInbox: 'Delete the inbox',
  deleteInboxTitle: (name: string) => `Delete “${name}”?`,
  deleteInboxWarning: (count: number) =>
    `${count > 1 ? `Its ${count} messages will be deleted` : count === 1 ? 'Its message will be deleted' : 'It holds no message'}, and its SMTP credentials will stop working at once. This cannot be undone.`,
  delete: 'Delete',
  filterInboxes: 'Filter inboxes',
  clearFilter: 'Clear the filter',
  noInboxMatches: 'No inbox matches.',
  noInbox: 'No inbox yet',
  noInboxHint: 'Create an inbox for each application whose emails you want to capture.',
  switchInbox: 'Switch inbox',
  inboxNameRefused: 'Use 1 to 100 visible characters, without line breaks.',
  shareInbox: 'Share the inbox',
  shareTitle: (name: string) => `Share “${name}”`,
  shareWarning: 'Anyone with one of these links can read every message of this inbox, without signing in.',
  noShareLink: 'No link yet.',
  newShareLink: 'New link',
  expiry: 'Expires on (optional)',
  createdOn: (date: string) => `Created ${date}`,
  expiresOn: (date: string) => `Expires ${date}`,
  expiredOn: (date: string) => `Expired ${date}`,
  noExpiry: 'No expiry',
  openedAgo: (when: string) => `Opened ${when}`,
  neverOpened: 'Never opened',
  copyLink: (label: string) => `Copy the link for ${label}`,
  linkOf: (label: string) => `Link for ${label}`,
  revoke: 'Revoke',
  revokeLink: (label: string) => `Revoke the link for ${label}`,
  revokeConfirm: 'It stops working at once, for good.',
  shareLabel: 'Who is it for?',
  shareLabelExample: 'Brand review team',
  createLink: 'Create the link',
  copy: 'Copy',
  copied: 'Copied',
  done: 'Done',
  readOnly: 'read only',
  emptyInboxVisitorHint: 'Emails sent by the site will appear here.',
  linkGone: 'This link is no longer active',
  linkGoneHint: 'Ask the person who sent it to you for a new one.',
}

const fr: typeof en = {
  signInTitle: 'Se connecter à Oubliette',
  email: 'Adresse e-mail',
  password: 'Mot de passe',
  signIn: 'Se connecter',
  signingIn: 'Connexion…',
  signInRefused: 'Adresse e-mail ou mot de passe incorrect.',
  signInSlowedDown: 'Trop de tentatives. Réessayez dans quelques minutes.',
  unreachable: 'Le serveur ne répond pas. Réessayez.',
  failed: 'Une erreur est survenue. Réessayez.',
  inboxes: 'Boîtes',
  messages: 'Messages',
  messageCount: (count: number) => (count > 1 ? `${count} messages` : `${count} message`),
  signOut: 'Se déconnecter',
  toDarkTheme: 'Passer au thème sombre',
  toLightTheme: 'Passer au thème clair',
  noSubject: 'Sans objet',
  emptyInbox: 'Aucun message pour l’instant',
  emptyInboxHint: 'Les messages envoyés avec les identifiants de cette boîte apparaîtront ici.',
  loadFailed: 'Les messages n’ont pas pu être chargés.',
  olderMessages: 'Afficher les messages plus anciens',
  inboxMissing: 'Cette boîte n’existe pas.',
  goToInbox: (name: string) => `Aller à ${name}`,
  closeMessage: 'Fermer le message',
  messageViews: 'Vues du message',
  rendered: 'Rendu',
  text: 'Texte',
  renderingOf: (subject: string) => `Rendu de « ${subject} »`,
  noTextVersion: 'Ce message n’a pas de version texte.',
  messageMissing: 'Ce message n’existe pas.',
  messageFailed: 'Le message n’a pas pu être chargé.',
  newInbox: 'Nouvelle boîte',
  inboxName: 'Nom',
  create: 'Créer',
  cancel: 'Annuler',
  renameInbox: 'Renommer la boîte',
  rename: 'Renommer',
  deleteInbox: 'Supprimer la boîte',
  deleteInboxTitle: (name: string) => `Supprimer « ${name} » ?`,
  deleteInboxWarning: (count: number) =>
    `${count > 1 ? `Ses ${count} messages seront supprimés` : count === 1 ? 'Son message sera supprimé' : 'Elle ne contient aucun message'}, et ses identifiants SMTP cesseront de fonctionner immédiatement. Cette action est définitive.`,
  delete: 'Supprimer',
  filterInboxes: 'Filtrer les boîtes',
  clearFilter: 'Effacer le filtre',
  noInboxMatches: 'Aucune boîte ne correspond.',
  noInbox: 'Aucune boîte pour l’instant',
  noInboxHint: 'Créez une boîte pour chaque application dont vous voulez capturer les e-mails.',
  switchInbox: 'Changer de boîte',
  inboxNameRefused: 'Utilisez de 1 à 100 caractères visibles, sans retour à la ligne.',
  shareInbox: 'Partager la boîte',
  shareTitle: (name: string) => `Partager « ${name} »`,
  shareWarning: 'Toute personne qui a l’un de ces liens peut lire tous les messages de cette boîte, sans se connecter.',
  noShareLink: 'Aucun lien pour l’instant.',
  newShareLink: 'Nouveau lien',
  expiry: 'Expire le (facultatif)',
  createdOn: (date: string) => `Créé le ${date}`,
  expiresOn: (date: string) => `Expire le ${date}`,
  expiredOn: (date: string) => `Expiré le ${date}`,
  noExpiry: 'Sans expiration',
  openedAgo: (when: string) => `Ouvert ${when}`,
  neverOpened: 'Jamais ouvert',
  copyLink: (label: string) => `Copier le lien pour ${label}`,
  linkOf: (label: string) => `Lien pour ${label}`,
  revoke: 'Révoquer',
  revokeLink: (label: string) => `Révoquer le lien pour ${label}`,
  revokeConfirm: 'Il cessera de fonctionner immédiatement et définitivement.',
  shareLabel: 'Pour qui ?',
  shareLabelExample: 'Équipe recette de la marque',
  createLink: 'Créer le lien',
  copy: 'Copier',
  copied: 'Copié',
  done: 'Terminé',
  readOnly: 'lecture seule',
  emptyInboxVisitorHint: 'Les e-mails envoyés par le site apparaîtront ici.',
  linkGone: 'Ce lien n’est plus actif',
  linkGoneHint: 'Demandez-en un nouveau à la personne qui vous l’a envoyé.',
}

const catalogs = { en, fr }

type Locale = keyof typeof catalogs

export const locale: Locale =
  navigator.languages
    .map((language) => language.slice(0, 2).toLowerCase())
    .find((language): language is Locale => Object.hasOwn(catalogs, language)) ?? 'en'

export const t = catalogs[locale]

document.documentElement.lang = locale

const elapsed = new Intl.RelativeTimeFormat(locale, { numeric: 'auto', style: 'short' })
const calendar = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' })
const dayAndMonth = new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'short' })
const fullDay = new Intl.DateTimeFormat(locale, { weekday: 'long', day: 'numeric', month: 'long' })

function startOfDay(date: Date) {
  const day = new Date(date)
  day.setHours(0, 0, 0, 0)
  return day.getTime()
}

function calendarDaysAgo(date: Date) {
  return Math.max(0, Math.round((startOfDay(new Date()) - startOfDay(date)) / 86_400_000))
}

export function relativeTime(date: Date) {
  const minutes = Math.round((Date.now() - date.getTime()) / 60_000)
  if (minutes < 1) return elapsed.format(0, 'second')
  if (minutes < 60) return elapsed.format(-minutes, 'minute')
  const days = calendarDaysAgo(date)
  if (days === 0 || minutes < 12 * 60) return elapsed.format(-Math.round(minutes / 60), 'hour')
  if (days < 7) return elapsed.format(-days, 'day')
  return dayAndMonth.format(date)
}

const dateFormat = new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'short', year: 'numeric' })

export const shortDate = (date: Date) => dateFormat.format(date)

export function dayLabel(date: Date) {
  const days = calendarDaysAgo(date)
  if (days === 0) return calendar.format(0, 'day')
  return days === 1 ? calendar.format(-1, 'day') : fullDay.format(date)
}
