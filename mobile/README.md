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
```

## CI/CD builds

The GitHub Actions `Mobile CI` workflow runs mobile tests, type checking, linting, protobuf generation checks, and CI wiring checks for mobile changes.

Android preview APK packaging runs on pushes to `main` and can also be started manually from GitHub Actions with `workflow_dispatch`. It uses the EAS `preview` profile in `eas.json`, which is configured for internal distribution and Android `apk` output.

Required repository setup:

- Add an Expo access token as the GitHub Actions secret `EXPO_TOKEN`.
- Add the Expo project UUID as the GitHub Actions secret `EXPO_PROJECT_ID`; CI links `mobile/` with `eas init --id` before the non-interactive build.
- Update `mobile/eas.json` if the preview build should point at a different public backend API than `https://tingjunn-emomo.hf.space/api/v1`.

The finished APK is uploaded as a GitHub Actions artifact named `emomo-android-preview-apk-*`. EAS also keeps the build details and install URL.

Set the backend API base with:

```bash
EXPO_PUBLIC_API_BASE=http://localhost:8080/api/v1 npm run start
```

For a physical phone, replace `localhost` with the LAN IP of the machine running the backend.

## Privacy / Secrets

- No account system in v1.
- Search history is stored only on the device via AsyncStorage.
- The app must not embed private bearer tokens, model keys, storage credentials, database credentials, or Qdrant credentials.
