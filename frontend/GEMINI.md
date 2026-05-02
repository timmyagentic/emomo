# EMOMO Frontend

> Frontend subproject of the emomo monorepo. Repo-wide context: [../GEMINI.md](../GEMINI.md). Backend context: [../backend/GEMINI.md](../backend/GEMINI.md).
> All commands below assume `cd frontend`.

## Project Overview
`emomo-frontend` is a React-based web application designed for searching, viewing, and discovering memes. It serves as the frontend for the Emomo platform, interacting with a backend API whose default search path uses multimodal image embeddings. VLM/OCR descriptions are auxiliary metadata returned for display, not the primary retrieval mechanism.

**Key Features:**
- **Semantic Search:** Users can search for memes using natural language queries.
- **Backend Filters:** Search can pass category/profile filters; "has visible text" is represented backend-side as a derived `text_presence` filter from `meme_annotations.labels.text.present`.
- **Meme Discovery:** A homepage displaying recommended memes.
- **Detailed View:** Modal view for individual memes with high-resolution images and metadata.
- **Resilience:** Includes a "Demo Mode" with hardcoded data that activates automatically if the backend API is unreachable.

## Tech Stack
- **Framework:** React 19 (TypeScript)
- **Build Tool:** Vite 7
- **Styling:** CSS Modules (`*.module.css`) + Global CSS
- **Animation:** Framer Motion
- **Testing:** Playwright (End-to-End)
- **Linting:** ESLint + TypeScript-ESLint

## Project Structure
```text
/
├── src/
│   ├── api/            # API client functions (search, fetch memes)
│   ├── components/     # UI Components (Header, MemeGrid, etc.)
│   ├── types/          # Shared TypeScript definitions (Meme, API responses)
│   ├── App.tsx         # Main application logic (Routing/State)
│   └── main.tsx        # Entry point
├── e2e/                # Playwright end-to-end tests
├── public/             # Static assets
└── .env.example        # Template for environment variables
```

## Setup & Development

### 1. Installation
```bash
npm install
```

### 2. Environment Configuration
Copy `.env.example` to `.env` and configure:
```bash
cp .env.example .env
```
- `VITE_API_BASE`: URL of the backend API (defaults to `http://localhost:8080/api/v1`).
- `VITE_API_TOKEN`: Optional authentication token.

### 3. Running the Dev Server
```bash
npm run dev
# Starts Vite server at http://localhost:5173
```

### 4. Building for Production
```bash
npm run build
# Outputs to dist/ directory
```

## Testing & Quality
- **Run E2E Tests:** `npm run test` (Headless) or `npm run test:ui` (Interactive UI)
- **Lint Code:** `npm run lint`

## Development Conventions
- **Components:** Create new components in `src/components/` with a corresponding `PascalCase.module.css` file.
- **State Management:** Uses React hooks (`useState`, `useEffect`) for local state. Complex global state is currently minimal.
- **API:** All network requests are encapsulated in `src/api/index.ts`.
- **Types:** Define shared interfaces in `src/types/index.ts`.
- **Styling:** Prefer CSS Modules for component-specific styles. Global styles go in `src/App.css` or `src/index.css`.
