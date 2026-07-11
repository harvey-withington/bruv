import {
  AddCardAttachment,
  AddCardComment,
  CreateCard,
  DeleteCard,
  GetCategoryAcceptedTypes,
  ListCardComments,
  PinCard,
  SignAttachmentURL,
  UpdateCardBlocks,
  UpdateCardDescription,
  UpdateCardDueDate,
  UpdateCardTags,
  UpdateCardType,
} from '@shared/api'
import {
  buildCardExportPayload as buildPayload,
  importCardFromJson as importFromJson,
  type CardTransferApi,
  type ImportOptions,
  type ImportOutcome,
} from '@shared/cardTransfer'
import type { BruvCardExport } from '@shared/cardJson'
import type { CardMarkdownLabels } from '@shared/cardMarkdown'
import type { Card } from '@shared/types'
import { t } from './i18n.svelte'

// Desktop binding of @shared/cardTransfer: injects the Wails/API
// wrapper layer plus localized strings. All envelope/replay logic
// lives in the shared module.

const api: CardTransferApi = {
  createCard: (cardType, title) => CreateCard(cardType, title),
  deleteCard: async (cardId) => { await DeleteCard(cardId) },
  pinCard: async (cardId, categoryId) => { await PinCard(cardId, categoryId) },
  getCategoryAcceptedTypes: (categoryId) => GetCategoryAcceptedTypes(categoryId),
  updateCardType: (cardId, cardType) => UpdateCardType(cardId, cardType),
  updateCardDescription: (cardId, description) => UpdateCardDescription(cardId, description),
  updateCardBlocks: (cardId, blocks) => UpdateCardBlocks(cardId, blocks),
  updateCardTags: (cardId, tags) => UpdateCardTags(cardId, tags),
  updateCardDueDate: (cardId, dueDate) => UpdateCardDueDate(cardId, dueDate),
  addCardAttachment: (cardId, name, base64Data) => AddCardAttachment(cardId, name, base64Data),
  addCardComment: (cardId, author, text) => AddCardComment(cardId, author, text),
  listCardComments: (cardId) => ListCardComments(cardId),
  signAttachmentURL: (cardId, attachmentId) => SignAttachmentURL(cardId, attachmentId),
}

export function buildCardExportPayload(card: Card): Promise<BruvCardExport> {
  return buildPayload(api, card)
}

export function importCardFromJson(
  text: string,
  categoryId: string,
  opts: Omit<ImportOptions, 'fallbackTitle'> = {},
): Promise<ImportOutcome | null> {
  return importFromJson(api, text, categoryId, { fallbackTitle: t('card.import_fallback_title'), ...opts })
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
export type { ImportOutcome, TypeConflictResolution } from '@shared/cardTransfer'
