// Shared ESLint flat config. Apps and packages extend it:
//
//   import anis from '@anis/config/eslint';
//   export default [...anis, { /* app-specific overrides */ }];
//
// Deliberately small. A large rule set in a scaffold is a large rule set
// somebody disables in month two.

import js from '@eslint/js';
import prettier from 'eslint-config-prettier';
import globals from 'globals';
import tseslint from 'typescript-eslint';

export default tseslint.config(
  {
    ignores: ['**/dist/**', '**/.turbo/**', '**/node_modules/**', '**/*.generated.ts'],
  },

  js.configs.recommended,
  ...tseslint.configs.recommended,

  {
    languageOptions: {
      ecmaVersion: 2023,
      sourceType: 'module',
      globals: { ...globals.browser, ...globals.es2021 },
    },
    rules: {
      // An unused parameter named `_thing` is documentation, not dead code.
      '@typescript-eslint/no-unused-vars': [
        'error',
        {
          argsIgnorePattern: '^_',
          varsIgnorePattern: '^_',
          caughtErrorsIgnorePattern: '^_',
        },
      ],

      // `import type` vs `import` is load-bearing under verbatimModuleSyntax:
      // a value import of a type-only module emits a real runtime import that
      // resolves to nothing.
      '@typescript-eslint/consistent-type-imports': [
        'error',
        { prefer: 'type-imports', fixStyle: 'inline-type-imports' },
      ],

      // `any` is a warning, not an error — it is sometimes the honest type at
      // an API boundary, and an error here just gets suppressed with a comment.
      '@typescript-eslint/no-explicit-any': 'warn',

      'no-console': ['warn', { allow: ['warn', 'error'] }],
      eqeqeq: ['error', 'always', { null: 'ignore' }],
    },
  },

  {
    files: ['**/*.test.ts', '**/*.test.tsx', '**/*.config.ts', '**/*.config.js'],
    rules: {
      'no-console': 'off',
      '@typescript-eslint/no-explicit-any': 'off',
    },
  },

  // Must stay last: switches off every stylistic rule Prettier owns.
  prettier,
);
