# Emomo Semantic Spotlight — Design QA

## Visual truth and captures

- Selected source: `/Users/timmy/.codex/visualizations/2026/07/19/019f79d0-42b0-7cc3-be35-4b071ec9aeec/five-site-redesign/selected/emomo.png` (1487 × 1058).
- Baseline desktop: `/Users/timmy/.codex/visualizations/2026/07/19/019f79d0-42b0-7cc3-be35-4b071ec9aeec/five-site-redesign/before/emomo-desktop-full.png`.
- Baseline mobile: `/Users/timmy/.codex/visualizations/2026/07/19/019f79d0-42b0-7cc3-be35-4b071ec9aeec/five-site-redesign/before/emomo-mobile-full.png`.
- Desktop implementation, browse state, 1440 × 1024 viewport: `/Users/timmy/.codex/visualizations/2026/07/19/019f79d0-42b0-7cc3-be35-4b071ec9aeec/five-site-redesign/local/emomo/desktop-1440x1024.png`.
- Mobile implementation, browse state, 390 × 844 viewport: `/Users/timmy/.codex/visualizations/2026/07/19/019f79d0-42b0-7cc3-be35-4b071ec9aeec/five-site-redesign/local/emomo/mobile-390x844.png`.
- Detail dialog implementation: `/Users/timmy/.codex/visualizations/2026/07/19/019f79d0-42b0-7cc3-be35-4b071ec9aeec/five-site-redesign/local/emomo/desktop-modal-1440x1024.png`.

## Same-input comparisons

- Full desktop comparison: `/Users/timmy/.codex/visualizations/2026/07/19/019f79d0-42b0-7cc3-be35-4b071ec9aeec/five-site-redesign/comparisons/local/emomo-desktop.png`.
- Responsive comparison: `/Users/timmy/.codex/visualizations/2026/07/19/019f79d0-42b0-7cc3-be35-4b071ec9aeec/five-site-redesign/comparisons/local/emomo-mobile.png`.
- Focused hero and gallery comparison: `/Users/timmy/.codex/visualizations/2026/07/19/019f79d0-42b0-7cc3-be35-4b071ec9aeec/five-site-redesign/comparisons/local/emomo-focus.png`.

The approved source defines a desktop composition only. Mobile therefore preserves the source hierarchy, palette, typography, search priority, recommendation row, and gallery behavior rather than inventing a separate mobile art direction.

## Comparison history

1. Full desktop comparison confirmed the warm paper surface, sparse header, date marker, oversized Song-style title, pill search control, colored keyword underlines, five-column media rail, and fixed shortcut footer.
2. Focused comparison confirmed the hero-to-gallery spacing, image crop density, rounded image corners, restrained borders, and hierarchy. The source shows a selected carousel item; the production browse state remains neutral until a user selects a card.
3. The 390 × 844 capture retained the complete search path and two-column media grid with no horizontal overflow.
4. The semantic search was exercised with `一只超级开心的柴犬`; result filters appeared, the first card opened a labelled modal, and close restored the gallery. Local API failures correctly activated the curated real-image fallback because the backend was intentionally not started for this frontend-only capture.

## Findings

- P0: none.
- P1: none.
- P2: none.
- Desktop horizontal overflow: none.
- Mobile horizontal overflow: none at 390 × 844.
- Core interactions: search, fallback results, filters, card preview, modal close, copy, and download contracts remain wired.
- Motion: one-time spatial continuity is retained for selected media; decorative looping motion was removed; reduced-motion and reduced-transparency modes are supported.

final result: passed
