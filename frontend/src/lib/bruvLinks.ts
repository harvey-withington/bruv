// Following a bruv: link on desktop: App.svelte's `bruv:navigate`
// listener opens the card or project. The document's click delegate
// (main.ts) and the UNCLI chat's openLink, whose links live in its
// shadow root out of the delegate's sight, both come through here.

/** target is the link's data-bruv form: "card:<id>" or "project:<id>". */
export function followBruv(target: string): void {
  if (target.startsWith('card:')) {
    document.dispatchEvent(new CustomEvent('bruv:navigate', { detail: { type: 'card', id: target.slice('card:'.length) } }))
  } else if (target.startsWith('project:')) {
    document.dispatchEvent(new CustomEvent('bruv:navigate', { detail: { type: 'project', id: target.slice('project:'.length) } }))
  }
}
