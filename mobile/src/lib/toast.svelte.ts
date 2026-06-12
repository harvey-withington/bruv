// Toast state lives in @shared/toast.svelte so desktop and mobile share
// one implementation; this re-export gives mobile the same ergonomic
// import path as desktop ('../lib/toast.svelte').
export * from '@shared/toast.svelte'
