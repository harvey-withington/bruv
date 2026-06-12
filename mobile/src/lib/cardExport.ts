import { repoRPC } from './auth'
import {
  buildCardExportPayload as buildPayload,
  importCardFromJson as importFromJson,
  type CardTransferApi,
  type ImportOutcome,
} from '@shared/cardTransfer'
import type { BruvCardExport } from '@shared/cardJson'
import type { CardMarkdownLabels } from '@shared/cardMarkdown'
import type { Card, CardComment } from '@shared/types'
import { t } from './i18n.svelte'

// Mobile binding of @shared/cardTransfer: injects the repoRPC
// transport plus localized strings. All envelope/replay logic lives
// in the shared module.

const api: CardTransferApi = {
  createCard: (cardType, title) => repoRPC<Card>('CreateCard', [cardType, title]),
  deleteCard: async (cardId) => { await repoRPC('DeleteCard', [cardId]) },
  pinCard: async (cardId, categoryId) => { await repoRPC('PinCard', [cardId, categoryId]) },
  updateCardType: (cardId, cardType) => repoRPC('UpdateCardType', [cardId, cardType]),
  updateCardDescription: (cardId, description) => repoRPC('UpdateCardDescription', [cardId, description]),
  updateCardBlocks: (cardId, blocks) => repoRPC('UpdateCardBlocks', [cardId, blocks]),
  updateCardTags: (cardId, tags) => repoRPC('UpdateCardTags', [cardId, tags]),
  updateCardDueDate: (cardId, dueDate) => repoRPC('UpdateCardDueDate', [cardId, dueDate]),
  addCardAttachment: (cardId, name, base64Data) => repoRPC('AddCardAttachment', [cardId, name, base64Data]),
  addCardComment: (cardId, author, text) => repoRPC('AddCardComment', [cardId, author, text]),
  listCardComments: async (cardId) => (await repoRPC<CardComment[]>('ListCardComments', [cardId])) ?? [],
  signAttachmentURL: (cardId, attachmentId) => repoRPC<string>('SignAttachmentURL', [cardId, attachmentId]),
}

export function buildCardExportPayload(card: Card): Promise<BruvCardExport> {
  return buildPayload(api, card)
}

export function importCardFromJson(text: string, categoryId: string): Promise<ImportOutcome> {
  return importFromJson(api, text, categoryId, { fallbackTitle: t('card.import_fallback_title') })
}

/** Localized section labels for cardToMarkdown. */
export function cardMarkdownLabels(): CardMarkdownLabels {
  return {
    attachments: t('card.md_attachments'),
    comments: t('card.md_comments'),
    unknownAuthor: t('card.md_unknown_author'),
    noAnswer: t('card.md_no_answer'),
    alarm: t('card.md_alarm'),
    alarmFired: t('card.md_alarm_fired'),
  }
}

export { ImportError } from '@shared/cardTransfer'
export type { ImportOutcome } from '@shared/cardTransfer'
