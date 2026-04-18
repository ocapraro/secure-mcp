/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** e.g. http://127.0.0.1:8787 — separate host/port so concurrent streams do not share the dev page’s HTTP/1.1 connection pool */
  readonly VITE_API_BASE?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
