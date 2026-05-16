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

Set the backend API base with:

```bash
EXPO_PUBLIC_API_BASE=http://localhost:8080/api/v1 npm run start
```

For a physical phone, replace `localhost` with the LAN IP of the machine running the backend.

## Privacy / Secrets

- No account system in v1.
- Search history is stored only on the device via AsyncStorage.
- The app must not embed private bearer tokens, model keys, storage credentials, database credentials, or Qdrant credentials.

