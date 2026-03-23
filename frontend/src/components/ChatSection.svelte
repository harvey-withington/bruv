<script lang="ts">
  import { tick } from 'svelte'
  import { Send, MapPin, Check, X, Wrench } from 'lucide-svelte'
  import { LoadChatHistory, SendChatMessage, IsLLMConfigured, AcceptPinSuggestion, RejectPinSuggestion } from '../lib/api'
  import { renderMarkdown } from '../lib/markdown'

  let { cardId, visible = $bindable(false), onCardChanged }: { cardId: string; visible: boolean; onCardChanged?: () => void } = $props()

  let messages = $state<Array<{id: string, role: string, content: string, timestamp: string, tool_actions?: any[], pin_suggestion?: any}>>([])
  let inputText = $state('')
  let loading = $state(true)
  let sending = $state(false)
  let configured = $state(true)
  let messagesEndEl = $state<HTMLDivElement | null>(null)

  async function loadChat() {
    loading = true
    try {
      const [result, isConfigured] = await Promise.all([
        LoadChatHistory(cardId),
        IsLLMConfigured(),
      ])
      messages = result?.messages || []
      configured = isConfigured
    } catch (e) {
      console.error('Failed to load chat:', e)
    }
    loading = false
  }

  async function send() {
    const text = inputText.trim()
    if (!text || sending) return
    sending = true
    inputText = ''
    try {
      const result = await SendChatMessage(cardId, text)
      messages = result?.messages || []
      // Notify parent that card data may have changed (AI may have set type, blocks, tags)
      onCardChanged?.()
      // Refresh sidebar in case LLM created new hierarchy
      window.dispatchEvent(new CustomEvent('bruv:sidebar-changed'))
      window.dispatchEvent(new CustomEvent('bruv:inbox-changed'))
      await tick()
      messagesEndEl?.scrollIntoView({ behavior: 'smooth' })
    } catch (e) {
      console.error('Failed to send message:', e)
    }
    sending = false
  }

  async function acceptPin(msgId: string) {
    try {
      await AcceptPinSuggestion(cardId, msgId)
      // Update the local message state
      messages = messages.map(m =>
        m.id === msgId && m.pin_suggestion
          ? { ...m, pin_suggestion: { ...m.pin_suggestion, status: 'accepted' } }
          : m
      )
      onCardChanged?.()
      // Fire events so inbox and sidebar re-fetch
      window.dispatchEvent(new CustomEvent('bruv:inbox-changed'))
      window.dispatchEvent(new CustomEvent('bruv:sidebar-changed'))
    } catch (e) {
      console.error('Failed to accept pin:', e)
    }
  }

  async function rejectPin(msgId: string) {
    try {
      await RejectPinSuggestion(cardId, msgId)
      messages = messages.map(m =>
        m.id === msgId && m.pin_suggestion
          ? { ...m, pin_suggestion: { ...m.pin_suggestion, status: 'rejected' } }
          : m
      )
    } catch (e) {
      console.error('Failed to reject pin:', e)
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send()
    }
  }

  function formatTime(ts: string): string {
    try {
      const d = new Date(ts)
      return d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
    } catch {
      return ''
    }
  }

  function toolActionLabel(action: any): string {
    switch (action.tool) {
      case 'set_title': return `Title: ${action.input?.title || '?'}`
      case 'set_due_date': return action.input?.due_date ? `Due: ${action.input.due_date}` : 'Cleared due date'
      case 'set_card_type': return `Set type: ${action.input?.card_type || '?'}`
      case 'set_fields':
      case 'update_blocks': {
        const fields = action.input?.fields || action.input?.blocks
        const keys = fields ? Object.keys(fields) : []
        return `Updated: ${keys.join(', ') || '?'}`
      }
      case 'add_tags': return `Added tags: ${(action.input?.tags || []).join(', ')}`
      case 'suggest_pin': return `Suggested pin: ${action.result || '?'}`
      default: return action.tool
    }
  }

  $effect(() => {
    if (visible) loadChat()
  })
</script>

{#if visible}
  <div class="chat-panel">
    <div class="chat-header">
      <span class="chat-title">Chat{messages.length > 0 ? ` (${messages.length})` : ''}</span>
    </div>

    {#if !configured}
      <div class="chat-banner">
        AI is not configured. Messages will be saved but won't receive AI responses. Open <strong>AI Settings</strong> to connect a provider.
      </div>
    {/if}

    <div class="chat-messages">
      {#if loading}
        <div class="chat-empty">Loading...</div>
      {:else if messages.length === 0}
        <div class="chat-empty">No messages yet. Start a conversation.</div>
      {:else}
        {#each messages as msg (msg.id)}
          <div class="chat-msg chat-msg-{msg.role}">
            <div class="chat-msg-content">{@html renderMarkdown(msg.content)}</div>

            {#if msg.tool_actions?.length}
              <div class="tool-actions">
                {#each msg.tool_actions as action}
                  <div class="tool-action">
                    <Wrench size={10} />
                    <span>{toolActionLabel(action)}</span>
                  </div>
                {/each}
              </div>
            {/if}

            {#if msg.pin_suggestion}
              <div class="pin-suggestion" class:accepted={msg.pin_suggestion.status === 'accepted'} class:rejected={msg.pin_suggestion.status === 'rejected'}>
                <div class="pin-suggestion-header">
                  <MapPin size={12} />
                  <span class="pin-suggestion-label">Pin suggestion</span>
                </div>
                <div class="pin-suggestion-breadcrumb">{msg.pin_suggestion.breadcrumb}</div>
                {#if msg.pin_suggestion.reason}
                  <div class="pin-suggestion-reason">{msg.pin_suggestion.reason}</div>
                {/if}
                {#if msg.pin_suggestion.status === 'pending'}
                  <div class="pin-suggestion-actions">
                    <button class="pin-btn pin-accept" onclick={() => acceptPin(msg.id)} title="Accept pin">
                      <Check size={12} /> Accept
                    </button>
                    <button class="pin-btn pin-reject" onclick={() => rejectPin(msg.id)} title="Dismiss">
                      <X size={12} /> Dismiss
                    </button>
                  </div>
                {:else if msg.pin_suggestion.status === 'accepted'}
                  <div class="pin-suggestion-resolved">Pinned</div>
                {:else}
                  <div class="pin-suggestion-resolved">Dismissed</div>
                {/if}
              </div>
            {/if}

            <span class="chat-msg-time">{formatTime(msg.timestamp)}</span>
          </div>
        {/each}
        {#if sending}
          <div class="chat-msg chat-msg-assistant thinking">
            <span class="dot"></span><span class="dot"></span><span class="dot"></span>
          </div>
        {/if}
        <div bind:this={messagesEndEl}></div>
      {/if}
    </div>

    <div class="chat-input-row">
      <textarea
        bind:value={inputText}
        onkeydown={handleKeydown}
        placeholder="Type a message..."
        disabled={sending}
        rows="1"
      ></textarea>
      <button class="chat-send-btn" onclick={send} disabled={!inputText.trim() || sending}>
        <Send size={14} />
      </button>
    </div>
  </div>
{/if}

<style>
  .chat-panel {
    width: 340px;
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
    border-left: 1px solid var(--border-muted);
    background: var(--bg-surface);
  }

  .chat-header {
    padding: 0.75rem 1rem;
    border-bottom: 1px solid var(--border-muted);
    flex-shrink: 0;
  }

  .chat-title {
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .chat-banner {
    padding: 0.5rem 0.75rem;
    font-size: 0.75rem;
    color: var(--text-muted);
    background: var(--bg-elevated);
    border-bottom: 1px solid var(--border-muted);
    line-height: 1.4;
    flex-shrink: 0;
  }

  .chat-messages {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 0.75rem;
  }

  .chat-empty {
    text-align: center;
    color: var(--text-muted);
    font-size: 0.8rem;
    padding: 2rem 0.5rem;
  }

  .chat-msg {
    max-width: 90%;
    padding: 6px 10px;
    border-radius: 8px;
    font-size: 0.85rem;
    line-height: 1.4;
  }
  .chat-msg-user {
    align-self: flex-end;
    background: var(--accent);
    color: #fff;
  }
  .chat-msg-assistant {
    align-self: flex-start;
    background: var(--bg-elevated);
    color: var(--text-primary);
  }
  .chat-msg-system {
    align-self: center;
    background: none;
    color: var(--text-muted);
    font-size: 0.75rem;
    font-style: italic;
    max-width: 100%;
  }

  .chat-msg-content :global(p) {
    margin: 0;
  }
  .chat-msg-content :global(p + p) {
    margin-top: 4px;
  }

  .chat-msg-time {
    display: block;
    font-size: 0.65rem;
    color: var(--text-muted);
    margin-top: 2px;
  }
  .chat-msg-user .chat-msg-time {
    color: rgba(255,255,255,0.6);
    text-align: right;
  }

  /* Tool actions */
  .tool-actions {
    margin-top: 4px;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .tool-action {
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 0.7rem;
    color: var(--text-muted);
    padding: 2px 0;
  }

  /* Pin suggestion */
  .pin-suggestion {
    margin-top: 6px;
    padding: 6px 8px;
    border-radius: 6px;
    border: 1px solid var(--border);
    background: var(--bg-surface);
  }
  .pin-suggestion.accepted {
    border-color: var(--success, #22c55e);
    background: color-mix(in srgb, var(--success, #22c55e) 8%, var(--bg-surface));
  }
  .pin-suggestion.rejected {
    opacity: 0.5;
  }

  .pin-suggestion-header {
    display: flex;
    align-items: center;
    gap: 4px;
    color: var(--accent);
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
  .pin-suggestion-breadcrumb {
    font-size: 0.8rem;
    color: var(--text-primary);
    margin-top: 2px;
    font-weight: 500;
  }
  .pin-suggestion-reason {
    font-size: 0.72rem;
    color: var(--text-muted);
    margin-top: 2px;
    line-height: 1.3;
  }
  .pin-suggestion-actions {
    display: flex;
    gap: 6px;
    margin-top: 6px;
  }
  .pin-btn {
    display: flex;
    align-items: center;
    gap: 3px;
    padding: 3px 8px;
    border-radius: 4px;
    font-size: 0.72rem;
    font-weight: 500;
    cursor: pointer;
    border: 1px solid var(--border);
    background: var(--bg-elevated);
    color: var(--text-secondary);
  }
  .pin-accept:hover {
    border-color: var(--success, #22c55e);
    color: var(--success, #22c55e);
    background: color-mix(in srgb, var(--success, #22c55e) 10%, var(--bg-elevated));
  }
  .pin-reject:hover {
    border-color: var(--danger, #ef4444);
    color: var(--danger, #ef4444);
  }
  .pin-suggestion-resolved {
    font-size: 0.7rem;
    color: var(--text-muted);
    margin-top: 4px;
    font-style: italic;
  }

  /* Thinking indicator */
  .thinking {
    display: flex;
    gap: 4px;
    padding: 10px 14px;
  }
  .thinking .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--text-muted);
    animation: bounce 1.2s infinite;
  }
  .thinking .dot:nth-child(2) { animation-delay: 0.2s; }
  .thinking .dot:nth-child(3) { animation-delay: 0.4s; }

  @keyframes bounce {
    0%, 60%, 100% { transform: translateY(0); opacity: 0.4; }
    30% { transform: translateY(-4px); opacity: 1; }
  }

  .chat-input-row {
    display: flex;
    gap: 6px;
    align-items: flex-end;
    padding: 0.5rem 0.75rem;
    border-top: 1px solid var(--border-muted);
    flex-shrink: 0;
  }

  .chat-input-row textarea {
    flex: 1;
    resize: none;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text-primary);
    padding: 6px 10px;
    font-size: 0.85rem;
    font-family: inherit;
    line-height: 1.4;
    min-height: 32px;
    max-height: 120px;
  }
  .chat-input-row textarea:focus {
    outline: none;
    border-color: var(--accent);
  }

  .chat-send-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    background: var(--accent);
    border: none;
    border-radius: 6px;
    color: #fff;
    cursor: pointer;
    flex-shrink: 0;
  }
  .chat-send-btn:hover:not(:disabled) {
    background: var(--accent-hover);
  }
  .chat-send-btn:disabled {
    opacity: 0.4;
    cursor: default;
  }
</style>
