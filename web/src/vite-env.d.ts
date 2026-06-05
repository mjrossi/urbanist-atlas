/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Base URL of the Urbanist Atlas API. Defaults to localhost in dev. */
  readonly VITE_API_BASE?: string;
  /** Phase-1 shared client secret, sent as the `X-Atlas-Client` header. */
  readonly VITE_API_CLIENT_SECRET?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
