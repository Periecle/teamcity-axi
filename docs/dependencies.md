# Dependency and license inventory

This snapshot comes from the committed npm lockfile. Runtime dependencies are
installed as separate packages; the wrapper does not bundle the official
TeamCity CLI or install it through lifecycle hooks. Development dependencies are
used for compilation, formatting and verification and are excluded by
`npm install --omit=dev`.

The project uses the [MIT license](../LICENSE), selected by its owner. The
v0.1.0 package is prepared for distribution; dependencies retain their own licenses.

Refresh this inventory when the lockfile changes. License names below are the
package metadata declarations, rather than a grant of rights for this project.
The native TeamCity CLI and server have their own JetBrains terms.

| Package                                                          | Pinned version | Declared license | Scope       |
| ---------------------------------------------------------------- | -------------- | ---------------- | ----------- |
| @cacheable/memory                                                | 2.2.0          | MIT              | development |
| @cacheable/utils                                                 | 2.5.0          | MIT              | development |
| @eslint-community/eslint-utils                                   | 4.10.1         | MIT              | development |
| @eslint-community/eslint-utils/node_modules/eslint-visitor-keys  | 3.4.3          | Apache-2.0       | development |
| @eslint-community/regexpp                                        | 4.12.2         | MIT              | development |
| @eslint/config-array                                             | 0.23.5         | Apache-2.0       | development |
| @eslint/config-helpers                                           | 0.7.0          | Apache-2.0       | development |
| @eslint/core                                                     | 1.2.1          | Apache-2.0       | development |
| @eslint/object-schema                                            | 3.0.5          | Apache-2.0       | development |
| @eslint/plugin-kit                                               | 0.7.3          | Apache-2.0       | development |
| @humanfs/core                                                    | 0.19.2         | Apache-2.0       | development |
| @humanfs/node                                                    | 0.16.8         | Apache-2.0       | development |
| @humanfs/types                                                   | 0.15.0         | Apache-2.0       | development |
| @humanwhocodes/module-importer                                   | 1.0.1          | Apache-2.0       | development |
| @humanwhocodes/retry                                             | 0.4.3          | Apache-2.0       | development |
| @keyv/bigmap                                                     | 1.3.1          | MIT              | development |
| @keyv/serialize                                                  | 1.1.1          | MIT              | development |
| @stylistic/eslint-plugin                                         | 5.10.0         | MIT              | development |
| @toon-format/toon                                                | 4.1.1          | MIT              | runtime     |
| @types/esrecurse                                                 | 4.3.1          | MIT              | development |
| @types/estree                                                    | 1.0.9          | MIT              | development |
| @types/json-schema                                               | 7.0.15         | MIT              | development |
| @types/node                                                      | 24.19.1        | MIT              | development |
| @typescript-eslint/parser                                        | 8.71.0         | MIT              | development |
| @typescript-eslint/project-service                               | 8.71.0         | MIT              | development |
| @typescript-eslint/scope-manager                                 | 8.71.0         | MIT              | development |
| @typescript-eslint/tsconfig-utils                                | 8.71.0         | MIT              | development |
| @typescript-eslint/types                                         | 8.71.0         | MIT              | development |
| @typescript-eslint/typescript-estree                             | 8.71.0         | MIT              | development |
| @typescript-eslint/visitor-keys                                  | 8.71.0         | MIT              | development |
| @typescript-eslint/visitor-keys/node_modules/eslint-visitor-keys | 5.0.1          | Apache-2.0       | development |
| acorn                                                            | 8.18.0         | MIT              | development |
| acorn-jsx                                                        | 5.3.2          | MIT              | development |
| ajv                                                              | 8.20.0         | MIT              | runtime     |
| ajv-formats                                                      | 3.0.1          | MIT              | runtime     |
| balanced-match                                                   | 4.0.4          | MIT              | development |
| base64-js                                                        | 1.5.1          | MIT              | development |
| brace-expansion                                                  | 5.0.12         | MIT              | development |
| cacheable                                                        | 2.5.0          | MIT              | development |
| cross-spawn                                                      | 7.0.6          | MIT              | development |
| debug                                                            | 4.4.3          | MIT              | development |
| deep-is                                                          | 0.1.4          | MIT              | development |
| escape-string-regexp                                             | 4.0.0          | MIT              | development |
| eslint                                                           | 10.11.0        | MIT              | development |
| eslint-scope                                                     | 9.1.2          | BSD-2-Clause     | development |
| eslint-visitor-keys                                              | 4.2.1          | Apache-2.0       | development |
| eslint/node_modules/ajv                                          | 6.15.0         | MIT              | development |
| eslint/node_modules/eslint-visitor-keys                          | 5.0.1          | Apache-2.0       | development |
| eslint/node_modules/espree                                       | 11.2.0         | BSD-2-Clause     | development |
| eslint/node_modules/json-schema-traverse                         | 0.4.1          | MIT              | development |
| espree                                                           | 10.4.0         | BSD-2-Clause     | development |
| esquery                                                          | 1.7.0          | BSD-3-Clause     | development |
| esrecurse                                                        | 4.3.0          | BSD-2-Clause     | development |
| estraverse                                                       | 5.3.0          | BSD-2-Clause     | development |
| esutils                                                          | 2.0.3          | BSD-2-Clause     | development |
| fast-deep-equal                                                  | 3.1.3          | MIT              | runtime     |
| fast-json-stable-stringify                                       | 2.1.0          | MIT              | development |
| fast-levenshtein                                                 | 2.0.6          | MIT              | development |
| fast-uri                                                         | 3.1.8          | BSD-3-Clause     | runtime     |
| fdir                                                             | 6.5.0          | MIT              | development |
| file-entry-cache                                                 | 11.1.5         | MIT              | development |
| find-up                                                          | 5.0.0          | MIT              | development |
| flat-cache                                                       | 6.1.23         | MIT              | development |
| flatted                                                          | 3.4.4          | ISC              | development |
| glob-parent                                                      | 6.0.2          | ISC              | development |
| hashery                                                          | 1.5.1          | MIT              | development |
| hookified                                                        | 1.15.1         | MIT              | development |
| ignore                                                           | 5.3.2          | MIT              | development |
| imurmurhash                                                      | 0.1.4          | MIT              | development |
| is-extglob                                                       | 2.1.1          | MIT              | development |
| is-glob                                                          | 4.0.3          | MIT              | development |
| isexe                                                            | 2.0.0          | ISC              | development |
| js-tiktoken                                                      | 1.0.21         | MIT              | development |
| json-schema-traverse                                             | 1.0.0          | MIT              | runtime     |
| json-stable-stringify-without-jsonify                            | 1.0.1          | MIT              | development |
| keyv                                                             | 5.6.0          | MIT              | development |
| levn                                                             | 0.4.1          | MIT              | development |
| locate-path                                                      | 6.0.0          | MIT              | development |
| minimatch                                                        | 10.2.6         | BlueOak-1.0.0    | development |
| ms                                                               | 2.1.3          | MIT              | development |
| natural-compare                                                  | 1.4.0          | MIT              | development |
| optionator                                                       | 0.9.4          | MIT              | development |
| p-limit                                                          | 3.1.0          | MIT              | development |
| p-locate                                                         | 5.0.0          | MIT              | development |
| path-exists                                                      | 4.0.0          | MIT              | development |
| path-key                                                         | 3.1.1          | MIT              | development |
| picomatch                                                        | 4.0.7          | MIT              | development |
| prelude-ls                                                       | 1.2.1          | MIT              | development |
| prettier                                                         | 3.9.9          | MIT              | development |
| punycode                                                         | 2.3.1          | MIT              | development |
| qified                                                           | 0.10.1         | MIT              | development |
| qified/node_modules/hookified                                    | 2.2.0          | MIT              | development |
| require-from-string                                              | 2.0.2          | MIT              | runtime     |
| semver                                                           | 7.8.5          | ISC              | development |
| shebang-command                                                  | 2.0.0          | MIT              | development |
| shebang-regex                                                    | 3.0.0          | MIT              | development |
| smol-toml                                                        | 1.9.0          | BSD-3-Clause     | runtime     |
| tinyglobby                                                       | 0.2.17         | MIT              | development |
| ts-api-utils                                                     | 2.5.0          | MIT              | development |
| type-check                                                       | 0.4.0          | MIT              | development |
| typescript                                                       | 6.0.3          | Apache-2.0       | development |
| undici-types                                                     | 7.24.6         | MIT              | development |
| uri-js                                                           | 4.4.1          | BSD-2-Clause     | development |
| which                                                            | 2.0.2          | ISC              | development |
| word-wrap                                                        | 1.2.5          | MIT              | development |
| yocto-queue                                                      | 0.1.0          | MIT              | development |
