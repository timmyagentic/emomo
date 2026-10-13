# Emomo website design QA

Date: 2026-10-13

**Findings:** No remaining P0/P1/P2 findings.

The selected QuotaMonitor concept establishes the shared visual direction, not a pixel-level Emomo mock. The Emomo adaptation keeps the spacious centered typography and clear primary action while retaining its original warm paper/green palette, orange brand icon and real meme examples. The split example surface deliberately replaces the native-app window composition because Emomo is a retrieval tool.

Desktop viewport: 1280 × 1000 CSS pixels, full-page capture 1265 × 4788. Mobile viewport: 390 × 844, full-page capture 375 × 5822.

## Evidence and comparison method

- Selected visual truth: `website/design/qa/visual-target.webp` (1019 × 1543 pixels). The user selected the first displayed concept, a light, spacious QuotaMonitor direction. It is a generated design reference, not application evidence.
- Browser-rendered implementation: `website/design/qa/home-desktop.webp` and `website/design/qa/home-mobile.webp`.
- Full-view comparison: `website/design/qa/comparison.webp`. Source and implementation appear in the same image; the implementation is scaled to the reference width for the top-region comparison. Full-page implementation files retain their original browser pixels.
- Focused comparison: `website/design/qa/type-and-spacing.webp`, covering navigation, headline, supporting copy, CTA and the start of the product surface. This was inspected in addition to the full view.
- Device scale factor is 1. Browser screenshots omit the 15-pixel scrollbar: a 1024/1280/390 CSS-pixel viewport therefore produces a 1009/1265/375-pixel content capture. No device frame is included.
- State: light theme, Chinese, default first tab, menus and FAQ closed. Additional English, alternate-tab, menu-open, keyboard-focus, document and narrow-screen states were checked separately.

## Required fidelity surfaces

1. **Fonts and typography:** native macOS system sans with PingFang fallback; dark, tightly tracked display type and quieter body text. Heading hierarchy, wrapping and button labels are readable on desktop and mobile. Browser rendering replaces the generated lettering in the reference.
2. **Spacing and layout rhythm:** centered hero, single dominant action, a substantial product surface and clearly separated content sections. Mobile layouts reflow into one column; no horizontal overflow was observed at 320, 375, 390, 430, 768 or 1440 CSS pixels. Product-specific sections below the reference crop are deliberate additions.
3. **Colors and visual tokens:** light surfaces, dark primary actions, restrained accents, visible keyboard outlines and non-color selected-state semantics. Reduced-motion styles disable transitions and smooth scrolling. No looping or scroll-triggered animation is required to read the page.
4. **Image quality and asset fidelity:** existing product/brand assets and licensed Tabler Icons are used. The shared landscape is an ImageGen raster asset. Screenshots and examples retain their aspect ratio and complete image content; no chart, logo or illustration is reconstructed with CSS/SVG drawing tricks.
5. **Copy and content:** the implementation uses product-specific, verifiable claims. Generated interface text in the concept is not shipped as product truth. Demos are labeled and do not impersonate live services.

## Verification boundary

This is browser-simulated responsive QA in the Codex in-app browser, not a physical-device test. Production publication is not included in this result. The three final homepages loaded without browser console errors. Every referenced local route, image, script and in-page anchor was checked. No personal account data, live credentials or real conversation content is present in the captures.

## Comparison history

- The initial desktop composition had no actionable P0/P1/P2 differences from the shared direction after accounting for the product adaptation.
- Initial P2: at 320 pixels the account CTA wrapped its last character to a second line; the display title also needed a smaller lower bound. Initial evidence: `website/design/qa/account-before-fix.webp`.
- Fix: a concise mobile CTA label retains the full accessible name, and the hero font scales to 38.4 pixels at 320 pixels. The final title occupies two lines (88.31 pixels), without overflow. Post-fix evidence: `website/design/qa/account-mobile.webp` and `home-mobile.webp`.

## Primary interactions and checks

- Two tabs switch complete original images, captions and context; arrow-key navigation selects the matching panel.
- Install Prompt copy produced the success state. The fallback selects text for manual copy when the Clipboard API rejects access.
- Mobile menu opens and closes with Escape; `/docs`, `/account`, installation instructions and existing API/Skill entry points remain reachable.
- Account page inspected on desktop and mobile. Required email validation blocked an empty submission. No email was sent and no real key was created.
- Local browser preview uses a signed-out account response; it is not evidence of production email delivery. The account logic itself is unchanged.
- `npm run check`: generated DTO check, Worker types, TypeScript, both client scripts, all 5 account tests and deployment dry run passed. Account tests use local workerd Durable Objects and recorded email delivery.
- Live email, production login, key issuance and CLI access using a real account: **UNVERIFIED in this website-only change**.
- Copy now correctly states that keys belonging to one account share account quotas.

## Open questions

None for the frontend acceptance. Merge and production publication await explicit authorization.

## Follow-up polish

P3: additional approved public meme examples can expand the two-example showcase later. It is clearly labeled as a curated demonstration.

## Implementation checklist

- [x] Shared-direction and mobile visual comparison
- [x] Real image switching, keyboard behavior and copy feedback
- [x] Documentation and account styling
- [x] Account contract tests and deployment dry run

final result: passed
