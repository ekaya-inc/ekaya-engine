/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_AUTH_SERVER_URL?: string;
  readonly VITE_HOSTNAME?: string;
  readonly VITE_PORT?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
