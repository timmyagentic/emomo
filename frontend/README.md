# Emomo Frontend

> This directory is the frontend subproject of the emomo monorepo. See the root [../README.md](../README.md) for repo-wide context. The Go backend lives in [../backend](../backend).

Emomo is a meme search engine application that allows users to find memes using semantic search queries. The backend default search path compares text query embeddings directly against multimodal image vectors; VLM/OCR descriptions are returned as auxiliary display metadata. This frontend provides a responsive interface for searching, browsing, and viewing memes.

The backend API contract is generated from `../backend/proto/emomo/v1/` into `gen/emomo/v1/`. Generated protobuf types are used only at the API boundary in `src/api/`; after decoding, data is projected into UI-owned types in `src/types/` so React state, props, and local fallback data are not coupled to protobuf message shapes.

## Features

- **Semantic Search**: Find memes by describing them in natural language (e.g., "happy cat", "surprised dog").
- **Meme Grid**: Browse search results or recommended memes in a responsive grid layout.
- **Detailed View**: View high-resolution memes in a modal with metadata such as category, tags, and auxiliary description.
- **Interactive UI**: Animated transitions and hover effects using Framer Motion.
- **Recommendations**: Displays recommended memes on the home page.
- **Copy & Download**: Easily copy image links or download memes directly.

## Prerequisites

Before you begin, ensure you have the following installed:

- [Node.js](https://nodejs.org/) (v16 or higher recommended)
- [npm](https://www.npmjs.com/) (usually comes with Node.js)

## Installation

1. Clone the monorepo and move into this subproject:
   ```bash
   git clone https://github.com/timmyagentic/emomo.git
   cd emomo/frontend
   ```

2. Install dependencies:
   ```bash
   npm install
   ```

3. Configure environment variables:
   Copy `.env.example` to `.env` and update the values if necessary.
   ```bash
   cp .env.example .env
   ```

   - `VITE_API_BASE`: Local development API override (default: `http://localhost:8080/api/v1`). Production builds always use `https://api.emomo.net/api/v1`.
   - Do not expose Hugging Face tokens in frontend environment variables. Production clients should go through the Cloudflare API gateway, which injects the upstream token server-side.

## Usage

### Development

To start the development server with Hot Module Replacement (HMR):

```bash
npm run dev
```

Open your browser and navigate to `http://localhost:5173` (or the port shown in the terminal).

### Build

To build the application for production:

```bash
npm run build
```

The output will be in the `dist/` directory.

### Preview

To preview the production build locally:

```bash
npm run preview
```

### Linting

To run ESLint and check for code quality issues:

```bash
npm run lint
```

### Testing

To run End-to-End (E2E) tests using Playwright:

```bash
# Run tests in headless mode
npm run test

# Run tests with UI runner
npm run test:ui

# Run tests in headed browser mode
npm run test:headed
```

## Project Structure

```
src/
├── api/            # API client and service functions
├── assets/         # Static assets (images, icons)
├── components/     # React components
│   ├── Header.tsx      # App header
│   ├── SearchHero.tsx  # Search bar section
│   ├── MemeCard.tsx    # Individual meme card
│   ├── MemeGrid.tsx    # Grid of meme cards
│   ├── MemeModal.tsx   # Detail view modal
│   └── ...
├── types/          # TypeScript type definitions
├── App.tsx         # Main application component
├── main.tsx        # Entry point
└── ...
```

## Contributing

1. Fork the repository.
2. Create a new branch (`git checkout -b feature/amazing-feature`).
3. Make your changes.
4. Commit your changes (`git commit -m 'feat: Add some amazing feature'`).
5. Push to the branch (`git push origin feature/amazing-feature`).
6. Open a Pull Request.

## License

[MIT](LICENSE)
