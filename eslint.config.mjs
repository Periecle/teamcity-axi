import stylistic from '@stylistic/eslint-plugin';
import tsParser from '@typescript-eslint/parser';

const declarations = ['const', 'let', 'var'];
const controlStatements = ['if', 'for', 'while', 'do', 'switch', 'try'];
const topLevelDefinition = { selector: 'Program > :not(ImportDeclaration)' };
const builtInImport = { selector: 'ImportDeclaration[source.value=/^node:/]' };
const otherImport = { selector: 'ImportDeclaration:not([source.value=/^node:/])' };
const relativeImport = { selector: 'ImportDeclaration[source.value=/^\\./]' };
const packageImport = {
  selector: 'ImportDeclaration:not([source.value=/^node:/]):not([source.value=/^\\./])',
};

export default [
  {
    files: ['src/**/*.ts'],
    languageOptions: { parser: tsParser },
    plugins: { '@stylistic': stylistic },
    rules: {
      '@stylistic/padding-line-between-statements': [
        'error',
        { blankLine: 'always', prev: 'import', next: '*' },
        { blankLine: 'any', prev: 'import', next: 'import' },
        { blankLine: 'always', prev: builtInImport, next: otherImport },
        { blankLine: 'always', prev: otherImport, next: builtInImport },
        { blankLine: 'always', prev: relativeImport, next: packageImport },
        { blankLine: 'always', prev: packageImport, next: relativeImport },
        { blankLine: 'always', prev: declarations, next: '*' },
        { blankLine: 'any', prev: declarations, next: declarations },
        { blankLine: 'always', prev: '*', next: ['return', 'throw'] },
        { blankLine: 'always', prev: 'block-like', next: '*' },
        { blankLine: 'always', prev: '*', next: 'block-like' },
        { blankLine: 'always', prev: controlStatements, next: '*' },
        { blankLine: 'always', prev: '*', next: controlStatements },
        { blankLine: 'always', prev: topLevelDefinition, next: topLevelDefinition },
        { blankLine: 'any', prev: 'function-overload', next: ['function', 'function-overload'] },
      ],
      '@stylistic/lines-between-class-members': [
        'error',
        {
          enforce: [
            { blankLine: 'always', prev: '*', next: 'method' },
            { blankLine: 'always', prev: 'method', next: '*' },
          ],
        },
        { exceptAfterOverload: true },
      ],
    },
  },
];
