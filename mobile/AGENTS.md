# Repository Guidelines (mobile/)

> Mobile subproject of the emomo monorepo. Repo-wide conventions: [../AGENTS.md](../AGENTS.md).
> All commands below assume `cd mobile`.

## Project Structure

- `App.tsx` is the Expo entry component.
- `src/api/` contains the protobuf JSON API client for the Go backend.
- `src/components/` contains React Native UI components.
- `src/storage/` contains local-only AsyncStorage helpers.
- `src/types/` contains mobile-owned view models projected from generated protobuf DTOs.
- `src/utils/` contains platform integrations such as share/save/copy actions.
- `gen/emomo/v1/` contains generated TypeScript protobuf code from `../backend/proto/emomo/v1/`. Do not hand-edit.

## Commands

- `npm install` installs dependencies.
- `npm run start` starts Expo.
- `npm run ios` / `npm run android` opens the app in a simulator or connected device.
- `npm run gen` regenerates protobuf TypeScript under `gen/`.
- `npm run typecheck` runs TypeScript checking.
- `npm run lint` runs Expo lint.
- `npm run test` runs Jest tests.

## Style

- TypeScript / React Native: 2 spaces, semicolons, single quotes.
- Keep generated protobuf DTOs at the API boundary. Components and hooks should use mobile-owned types from `src/types/`.
- Do not embed private API tokens, model keys, object storage secrets, or Qdrant credentials in the app.
- Expo SDK is currently v54. Use the versioned Expo docs at https://docs.expo.dev/versions/v54.0.0/ for native module behavior.
