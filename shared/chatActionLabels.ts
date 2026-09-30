// One-line labels for the tool calls an AI made in chat, shared by the
// desktop ChatSection and the mobile ChatMessage so both surfaces name
// every native board tool the same way (they drifted: mobile didn't know
// the native set_card_* names and showed raw tool ids).
//
// Chat records the NATIVE registry names (core/boardtools); the older
// chat-only names still appear in existing chat history, so both are
// mapped. The translate function is injected — shared/ can't import a
// surface's i18n module.
import type { ToolAction } from './types'
import type { TranslateFn } from './relativeTime'

export function toolActionLabel(action: Pick<ToolAction, 'tool' | 'input' | 'result'>, t: TranslateFn): string {
  const inp = (action.input ?? {}) as Record<string, unknown>
  const str = (key: string) => (typeof inp[key] === 'string' ? (inp[key] as string) : '')
  const list = (key: string) => (Array.isArray(inp[key]) ? (inp[key] as unknown[]).map(String) : [])
  switch (action.tool) {
    case 'set_card_title':
    case 'set_title': return t('chat.action_title', { title: str('title') || '?' })
    case 'set_card_description':
    case 'set_description': return str('description') ? t('chat.action_set_description') : t('chat.action_description_cleared')
    case 'set_card_due_date':
    case 'set_due_date': return str('due_date') ? t('chat.action_due_date', { date: str('due_date') }) : t('chat.action_due_date_cleared')
    case 'set_card_type': return t('chat.action_set_type', { type: str('card_type') || '?' })
    case 'create_card_type': return t('chat.action_created_type', { type: str('label') || '?' })
    case 'set_card_fields':
    case 'set_fields':
    case 'update_blocks': {
      const fields = (inp.fields || inp.blocks) as Record<string, unknown> | undefined
      const keys = fields && typeof fields === 'object' ? Object.keys(fields) : []
      return t('chat.action_updated_fields', { fields: keys.join(', ') || '?' })
    }
    case 'add_card_blocks': {
      const blocks = Array.isArray(inp.blocks) ? (inp.blocks as Record<string, unknown>[]) : []
      const names = blocks.map((b) => String(b.label || b.key || '?'))
      return t('chat.action_added_blocks', { fields: names.join(', ') || '?' })
    }
    case 'add_card_tags':
    case 'add_tags': return t('chat.action_added_tags', { tags: list('tags').join(', ') })
    case 'remove_card_tags':
      return inp.all === true ? t('chat.action_removed_all_tags') : t('chat.action_removed_tags', { tags: list('tags').join(', ') })
    case 'add_field': return t('chat.action_added_field', { field: str('label') || str('key') || '?', type: str('field_type') || '?' })
    case 'add_card_comment': return t('chat.action_commented')
    case 'pin_card': return t('chat.action_pinned_card')
    case 'unpin_card': return t('chat.action_unpinned_card')
    case 'suggest_pin': return t('chat.action_suggested_pin', { result: action.result || '?' })
    case 'create_card': return t('chat.action_created_card', { title: str('title') || '?' })
    case 'create_brand':
    case 'create_stream':
    case 'create_project':
    case 'create_category': return t('chat.action_created_named', { name: str('name') || '?' })
    case 'add_tags_to_cards':
      return t('chat.action_tagged_cards', { count: list('card_ids').length, tags: list('tags').join(', ') })
    case 'update_cards': return t('chat.action_updated_cards', { count: Array.isArray(inp.updates) ? inp.updates.length : list('card_ids').length })
    case 'move_card': return t('chat.action_moved_card')
    case 'update_card': return action.result || t('chat.action_updated_card')
    case 'configure_card_agent': return t('chat.action_configured_agent')
    default: return action.tool
  }
}
