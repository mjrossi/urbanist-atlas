import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

// https://vite.dev/config/  +  https://vitest.dev/config/
// Using `vitest/config`'s defineConfig so the `test` block is
// type-checked. It re-exports Vite's defineConfig under the hood.
export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals: true,
    css: false,
    setupFiles: ['./src/test/setup.ts'],
  },
});
