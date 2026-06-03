# emomo iOS App Store Release Checklist

Last updated: 2026-06-03

This checklist tracks the remaining work needed to move `mobile/` from simulator-ready MVP to an App Store submission.

## Current Release Identity

- App name: `emomo`
- Expo owner: `timmy3956`
- Bundle ID: `com.timmy.emomo`
- Version: `1.0.0`
- Initial iOS build number: `1`
- EAS project ID: `9550b08d-da2c-4075-8bbc-e4579773fc30`
- Backend API base for EAS builds: `https://tingjunn-emomo.hf.space/api/v1`

## Human-Owned Apple Setup

1. Confirm Apple Developer Program membership is active.
2. Create the app record in App Store Connect using bundle ID `com.timmy.emomo`.
3. Record the App Store Connect Apple ID (`ascAppId`) from App Information.
4. Run `cd mobile && npx eas-cli credentials --platform ios` and configure distribution credentials plus an App Store Connect API key.
5. Run `cd mobile && npx eas-cli build:version:set` if you want to initialize EAS remote build numbers before the first production build.
6. Add the `ascAppId` to `mobile/eas.json` under `submit.production.ios` after the app record exists.

## Repository Readiness

- `mobile/app.json` sets `ios.bundleIdentifier`, `ios.buildNumber`, and `ios.config.usesNonExemptEncryption`.
- `mobile/eas.json` has production EAS build and submit profiles.
- `mobile/store.config.json` contains initial App Store metadata.
- `docs/PRIVACY.md` is the initial privacy policy URL target.
- The app exposes About/Privacy/Support information in-app.

## Build And Submit

```bash
cd mobile
npm run test -- --runInBand
npm run typecheck
npm run lint
npx expo-doctor
npx eas-cli build --platform ios --profile production
npx eas-cli submit --platform ios --profile production --latest
```

The production build creates an App Store/TestFlight archive, not the existing simulator `.app` artifact.

## App Store Connect Metadata

Before review, fill or verify:

- App privacy questionnaire based on actual backend logging and data retention.
- Privacy Policy URL: `https://github.com/systemoutprintlnnnn/emomo/blob/main/docs/PRIVACY.md`
- Support URL: `https://github.com/systemoutprintlnnnn/emomo/issues`
- Category, age rating, description, keywords, and review notes.
- At least one iPhone screenshot; up to ten screenshots are allowed.
- TestFlight internal testing results.

## Still Needed Before Public Release

- Replace Expo placeholder icon and splash assets with final emomo brand assets.
- Capture real iPhone screenshots after the final visual pass.
- Add the real `ascAppId` once App Store Connect creates the app record.
- Verify production backend stability, rate limits, and cost caps.
- Complete a real-device TestFlight pass for search, detail, share, save, copy link, error states, and photo permission prompts.
