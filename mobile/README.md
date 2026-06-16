# emomo Mobile

Expo + React Native app for iOS and Android. The app is public, no-login, search-first, and talks only to the Go backend REST API.

## Commands

```bash
npm install
npm run gen
npm run start
```

Useful checks:

```bash
npm run test
npm run typecheck
npm run lint
npx expo-doctor
```

Regenerate the committed App Store/TestFlight icon, Android adaptive icon, splash image, and favicon after changing the mobile brand asset script:

```bash
npm run assets:generate
```

Local native runs require the platform SDKs:

```bash
npm run ios      # requires full Xcode with Simulator tools
npm run android  # requires Java, Android SDK, and an emulator or USB device
```

## CI/CD builds

The GitHub Actions `Required CI` workflow runs backend, frontend, and mobile required checks for every PR and every push to `main`. The mobile job covers tests, type checking, linting, protobuf generation checks, Expo doctor, and CI wiring checks.

Android preview APK packaging is started manually from the GitHub Actions `Mobile Artifacts` workflow with `workflow_dispatch`. It uses the EAS `preview` profile in `eas.json`, which is configured for internal distribution and Android `apk` output.

iOS simulator packaging is also manual and uses the EAS `ios-simulator` profile, but it runs as a local EAS build on a macOS GitHub Actions runner with `eas build --local --output`. It produces a simulator artifact that can be installed into an iOS Simulator, does not require App Store signing credentials, and does not consume EAS iOS cloud build quota. Device/TestFlight iOS builds should also be run locally from a Mac with the EAS `preview` or `production` profiles after Apple Developer credentials are configured.

Required repository setup:

- Add an Expo access token as the GitHub Actions secret `EXPO_TOKEN`.
- Add the Expo project UUID as the GitHub Actions secret `EXPO_PROJECT_ID`; CI links `mobile/` with `eas init --id` before the non-interactive build.
- Update `mobile/eas.json` if the preview build should point at a different public backend API than `https://api.emomo.net/api/v1`.

The finished APK is uploaded as a GitHub Actions artifact named `emomo-android-preview-apk-*`. The local iOS simulator archive is uploaded as `emomo-ios-simulator-*`.

## iOS App Store readiness

The iOS release identity is configured in `app.json` with bundle ID `com.timmy.emomo`, version `1.0.0`, build number `1`, and `usesNonExemptEncryption=false`. The `production` EAS profile builds the App Store archive, and `store.config.json` contains initial App Store metadata.

Before submitting, initialize remote build numbers if needed and make sure iOS distribution credentials plus App Store Connect API access are configured. The App Store Connect app record is `emomo - Meme Search`, and its Apple ID (`ascAppId`) is already set in `submit.production.ios`.

```bash
npx eas-cli credentials --platform ios
npx eas-cli build:version:set
npx eas-cli build --platform ios --profile production --local
npx eas-cli submit --platform ios --profile production --latest
```

See `../docs/mobile-app-store-release.md` for the full release checklist.

Set the backend API base with:

```bash
EXPO_PUBLIC_API_BASE=http://localhost:8080/api/v1 npm run start
```

For a physical phone, replace `localhost` with the LAN IP of the machine running the backend.

## Privacy / Secrets

- No account system in v1.
- Search history is stored only on the device via AsyncStorage.
- The app must not embed private bearer tokens, model keys, storage credentials, database credentials, or Qdrant credentials.
