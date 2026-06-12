// Toast state lives in @shared/toast.svelte so desktop and mobile share
// one implementation; this re-export preserves the existing import path
// used across desktop components.
export * from '@shared/toast.svelte'
