/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Origin of the deployed backend (e.g. "https://cafe-backend.onrender.com").
   *  Empty in local dev so the Vite proxy handles `/api` and `/uploads`. */
  readonly VITE_API_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
