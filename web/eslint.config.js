import js from '@eslint/js'
import pluginVue from 'eslint-plugin-vue'
import vueTsEslintConfig from '@vue/eslint-config-typescript'
import prettier from 'eslint-config-prettier'

export default [
  { ignores: ['dist/**', 'node_modules/**', '*.config.js', '*.config.ts'] },
  js.configs.recommended,
  ...pluginVue.configs['flat/recommended'],
  ...vueTsEslintConfig(),
  prettier,
  {
    rules: {
      // `catch (e: any)` throws away the type and invites `e.response.data.error` chains.
      // getApiError() exists for exactly that.
      '@typescript-eslint/no-explicit-any': 'error',
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_' }],
      // A swallowed error is why failed saves used to show nothing at all.
      'no-empty': ['error', { allowEmptyCatch: false }],
      'vue/multi-word-component-names': 'off',
      // Attribute ordering and line breaks are formatting, which Prettier owns.
      'vue/attributes-order': 'off',
      'vue/first-attribute-linebreak': 'off',
      'vue/max-attributes-per-line': 'off',
      'vue/singleline-html-element-content-newline': 'off',
      'vue/html-self-closing': 'off',
      // Optional props are optional; TypeScript already models `string | undefined`.
      'vue/require-default-prop': 'off',
    },
  },
]
