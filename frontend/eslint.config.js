import js from '@eslint/js';
import tseslint from 'typescript-eslint';
export default tseslint.config({ignores:['bindings/**','dist/**']},js.configs.recommended,...tseslint.configs.recommended);
