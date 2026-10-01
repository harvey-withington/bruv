import type { MCPServerSpec } from '@shared/types'

/**
 * The exact command line an MCP server spec runs, for the confirmations
 * shown before a server is enabled or approved on this machine.
 */
export function formatMCPCommand(spec: MCPServerSpec): string {
  return [spec.command, ...(spec.args ?? [])].join(' ').trim()
}
