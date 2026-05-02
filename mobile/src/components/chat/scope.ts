// ChatScope — discriminated union describing where the chat sheet
// was invoked from. Determines which RPCs the sheet calls and whether
// project-context and pending-edits flows apply.
//
// Card scope mirrors desktop's per-card chat (LoadChatHistory /
// SendChatMessage / ClearCardChatHistory / ApplyPendingEdits). Project
// scope mirrors desktop's project chat (LoadProjectChatHistory /
// SendProjectChatMessage / ClearProjectChatHistory / ApplyProjectPendingEdits).
//
// Vault-level / unscoped chat does NOT exist on desktop and so is not
// represented here — mobile mirrors what desktop already does.

export type ChatScope =
  | { kind: 'card'; cardID: string }
  | { kind: 'project'; brand: string; stream: string; project: string }
