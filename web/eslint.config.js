import js from '@eslint/js';
import eslintReact from '@eslint-react/eslint-plugin';
import vitest from '@vitest/eslint-plugin';
import { defineConfig, globalIgnores } from 'eslint/config';
import eslintConfigPrettier from 'eslint-config-prettier/flat';
import jsxA11y from 'eslint-plugin-jsx-a11y';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';
import simpleImportSort from 'eslint-plugin-simple-import-sort';
import testingLibrary from 'eslint-plugin-testing-library';
import globals from 'globals';
import tseslint from 'typescript-eslint';

export default defineConfig([
  // Build artefacts and machine-generated TS (the openapi-typescript output)
  // are not hand-edited and routinely fail style/whitespace rules, so we
  // skip them here. Regenerate api.gen.ts via `npm run generate:api`.
  globalIgnores(['dist', 'src/lib/api.gen.ts']),

  // Application source — full type-aware TypeScript, React 19, a11y, and
  // import sorting. Type information comes from the TS project graph via
  // `projectService` (tsconfig.app.json for src, tsconfig.node.json for
  // vite.config.ts), which is faster than the legacy `project` option.
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      tseslint.configs.strictTypeChecked,
      tseslint.configs.stylisticTypeChecked,
      eslintReact.configs['recommended-typescript'],
      // @eslint-react ships a few rules that overlap the official
      // react-hooks plugin; defer to react-hooks for those.
      eslintReact.configs['disable-conflict-eslint-plugin-react-hooks'],
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
      jsxA11y.flatConfigs.recommended,
    ],
    languageOptions: {
      globals: globals.browser,
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
    plugins: { 'simple-import-sort': simpleImportSort },
    rules: {
      'simple-import-sort/imports': 'error',
      'simple-import-sort/exports': 'error',
      // react-hook-form binds a Promise-returning submit handler to a
      // void-returning JSX attribute: <form onSubmit={handleSubmit(onValid)}>.
      // That pattern is safe; only relax the attribute check, not the rest.
      '@typescript-eslint/no-misused-promises': [
        'error',
        { checksVoidReturn: { attributes: false } },
      ],
      // Interpolating numbers into template strings (countdowns, counts,
      // numerals) is safe and ubiquitous here; only catch genuinely unsafe
      // stringifications (objects, nullish, any).
      '@typescript-eslint/restrict-template-expressions': [
        'error',
        { allowNumber: true },
      ],
      // Allow intentionally-discarded bindings: `_`-prefixed names and the
      // object-rest "omit a key" idiom (`const { [k]: _omit, ...rest } = o`).
      '@typescript-eslint/no-unused-vars': [
        'error',
        { argsIgnorePattern: '^_', varsIgnorePattern: '^_', ignoreRestSiblings: true },
      ],
    },
  },

  // Tests — vitest + Testing Library rules, and a couple of test-only
  // ergonomic relaxations (non-null assertions on known-present fixtures).
  {
    files: ['**/*.test.{ts,tsx}', 'src/test/**/*.{ts,tsx}'],
    extends: [vitest.configs.recommended, testingLibrary.configs['flat/react']],
    languageOptions: { globals: vitest.environments.env.globals },
    rules: {
      // Test fixtures legitimately assert on values they know are present.
      '@typescript-eslint/no-non-null-assertion': 'off',
      // Mocks (vi.fn(), fetch stubs, JSON fixtures) are inherently `any`, so
      // the type-aware "unsafe" family is pure noise in test code. The
      // higher-value type-aware rules (e.g. no-floating-promises) stay on.
      '@typescript-eslint/no-unsafe-assignment': 'off',
      '@typescript-eslint/no-unsafe-member-access': 'off',
      '@typescript-eslint/no-unsafe-call': 'off',
      '@typescript-eslint/no-unsafe-argument': 'off',
      '@typescript-eslint/no-unsafe-return': 'off',
      // Empty no-op callbacks are a normal mocking shorthand.
      '@typescript-eslint/no-empty-function': 'off',
      // This broadsheet UI has many presentational elements (rule
      // separators, section kickers, numerals) with no accessible role;
      // querying them by class through `container` is a legitimate way to
      // assert layout structure, so the accessible-query-only rules don't fit.
      'testing-library/no-node-access': 'off',
      'testing-library/no-container': 'off',
    },
  },

  // Plain JS (this config file) lives outside the TS project graph, so it
  // gets basic correctness rules but no type-aware linting.
  {
    files: ['**/*.{js,cjs,mjs}'],
    extends: [js.configs.recommended, tseslint.configs.disableTypeChecked],
    languageOptions: { globals: globals.node },
  },

  // Turn off any rules that would fight Prettier. Must stay last.
  eslintConfigPrettier,
]);
