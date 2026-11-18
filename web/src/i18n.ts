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

export function dayLabel(date: Date) {
  const days = calendarDaysAgo(date)
  if (days === 0) return calendar.format(0, 'day')
  return days === 1 ? calendar.format(-1, 'day') : fullDay.format(date)
}
