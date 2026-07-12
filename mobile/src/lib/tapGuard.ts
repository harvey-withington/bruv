// Tap-through guard for collapsing editors.
//
// Tapping ✓ Done commits on pointerdown; the editor unmounts while the
// finger is still mid-gesture, content reflows upward, and the tail of
// the SAME tap (pointerup / synthesized click) retargets to whatever
// slid underneath — usually the tap-to-edit read view, bouncing the
// user straight back into edit mode. Canceling pointerdown does not
// reliably suppress the compat click across mobile browsers, so the
// fix is temporal: the ✓ button arms this guard, and enter-edit
// handlers ignore activation while it is hot. 400ms comfortably covers
// pointerup + click synthesis without ever eating a deliberate second
// tap.

let hotUntil = 0

export function armTapGuard(ms = 400): void {
  hotUntil = performance.now() + ms
}

export function tapGuardActive(): boolean {
  return performance.now() < hotUntil
}
