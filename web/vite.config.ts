import react from '@vitejs/plugin-react';
import { loadEnv } from 'vite';
import { defineConfig } from 'vitest/config';

// https://vite.dev/config/  +  https://vitest.dev/config/
// Using `vitest/config`'s defineConfig so the `test` block is
// type-checked. It re-exports Vite's defineConfig under the hood.
export default defineConfig(({ mode }) => {
  // A production bundle without VITE_API_BASE throws at module load
  // (src/lib/api.ts), leaving a blank page; failing the build stops
  // that bundle from shipping.
  if (mode === 'production') {
    const { VITE_API_BASE = '' } = loadEnv(mode, import.meta.dirname, 'VITE_');
    if (VITE_API_BASE.trim() === '') {
      throw new Error(
        'VITE_API_BASE is not set. Set it to the API origin for production builds: ' +
          'web/.env.local locally (see .env.example), mise.ci.toml in CI, ' +
          'the Workers build settings for deploys.',
      );
    }
  }
  return {
    plugins: [react()],
    test: {
      environment: 'jsdom',
      globals: true,
      css: false,
      setupFiles: ['./src/test/setup.ts'],
    },
  };
});
