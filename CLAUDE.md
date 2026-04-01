# BRUV Project — Coding Preferences

## Code Quality Philosophy
We are building a high-quality, maintainable application — not just working code. Always prioritize clean architecture, readability, and long-term maintainability. Proactively offer to refactor and improve code quality whenever an opportunity arises.

## Localization
- All user-facing strings must be localized — never hardcode display text directly in components.
- Use the project's localization system for every label, message, placeholder, tooltip, error, and confirmation.
- When adding new features, always create/update the relevant localization keys.

## Drag and Drop
- Implement drag-and-drop interactions wherever they make sense (reordering lists, moving items between containers, organizing cards, etc.).
- Prefer drag-and-drop over less intuitive alternatives (e.g. up/down buttons).

## Reusable Components
- Extract reusable UI components whenever a pattern appears more than once (or is likely to).
- Components should be self-contained with clear props/interfaces.
- Prefer composition over duplication.

## Reusable Behaviours
- Extract shared logic into reusable Svelte actions, stores, or utility functions.
- Avoid duplicating event handling, validation, formatting, or data-fetching logic across components.
- Use Svelte actions for reusable DOM behaviours (e.g. click-outside, auto-focus, drag handle).

## Refactoring
- Always flag opportunities to improve code quality: reducing duplication, simplifying logic, improving naming, strengthening types, or restructuring modules.
- Offer refactoring suggestions alongside feature work — don't wait to be asked.

## Simplify and Clean Up
- Always simplify where possible, and always clean up unnecessary code.
- Remove dead code, unused imports, and redundant logic proactively.

## Strict TypeScript — No `any`
- All types must be explicit. Never use `any` — use proper interfaces, union types, or generics.
- `Promise<any>` and `$state<any>` are code smells; define the shape.

## No Native confirm() / alert()
- Never use browser-native `confirm()` or `alert()` for destructive actions or errors.
- All confirmations must use a custom in-app ConfirmDialog component.
- All errors that affect the user must surface in the UI (toast, inline error, etc.).

## Component Size Limit
- No component should exceed ~300 lines. If it does, extract sub-concerns into child components.
- A component should own one clear responsibility.

## Centralized Design Tokens
- Colors, sizing, and theming values belong in CSS custom properties or a central tokens file.
- Never hardcode color values inline in component logic or templates.

## Reusable Svelte Actions for DOM Behaviours
- Repeated DOM patterns (focus-on-flag, click-outside, auto-select) must be extracted as Svelte actions.
- Check `lib/` for existing actions before writing new logic.

## User-Visible Error Handling
- `catch (e) { console.error(e) }` is never acceptable when the error affects the user.
- Errors from API calls must be surfaced via toast or inline error state.

## ID-based State, Not Index-based
- Never key mutable state (drafts, editing flags) by array index — indices shift on reorder/delete.
- Always key by stable entity IDs.

## Living Documentation
- `UI-CONVENTIONS.md` is a contract. Update it whenever adding new shared components or patterns.
- Keep prop tables, examples, and keyboard behaviour in sync with actual implementation.
