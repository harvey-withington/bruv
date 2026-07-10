// The set of event topics the Go backend publishes. Listed explicitly
// because the SSE transport registers a per-topic listener (named SSE
// events) and the Wails transport mirrors that shape for parity.
//
// Extend this list when adding a new event topic server-side. A missed
// entry here means the event fires but no subscribers receive it.
export const KNOWN_TOPICS = [
  'card:created',
  'card:updated',
  'card:deleted',
  'brand:updated',
  'brand:deleted',
  'stream:updated',
  'stream:deleted',
  'project:updated',
  'project:deleted',
  'category:updated',
  'category:deleted',
  'labels:updated',
  'cardtype:updated',
  'cardtype:deleted',
  'agent:started',
  'agent:completed',
  'agent:failed',
  'scheduler:paused',
  'index:stale',
  'notification:new',
  'workspace:updated',
  'workspace:deleted',
  'workspace:templates',
] as const

export type KnownTopic = (typeof KNOWN_TOPICS)[number]
