import { readFileSync, readdirSync } from 'node:fs';

import { Ajv2020 } from 'ajv/dist/2020.js';

import { createRequire } from 'node:module';

import { DomainError } from '../domain/errors.js';
import type { Response } from '../domain/response.js';

const require = createRequire(import.meta.url);

const formats = require('ajv-formats');

const schemaDir = new URL('../../schemas/', import.meta.url);

// Seed contracts use required across conditional subschemas; enforce all other strict checks.
const ajv = new Ajv2020({
  strict: true,
  strictRequired: false,
  strictTypes: false,
  strictTuples: false,
  allErrors: true,
});

formats(ajv);

for (const file of readdirSync(schemaDir).filter((f) => f.endsWith('.schema.json'))) {
  ajv.addSchema(JSON.parse(readFileSync(new URL(file, schemaDir), 'utf8')));
}

export function packagedSchema(name: string): Record<string, unknown> {
  return JSON.parse(readFileSync(new URL(name + '.schema.json', schemaDir), 'utf8'));
}

export function validateResponse(value: Response): void {
  if (!ajv.validate('urn:teamcity-axi:response:1.0', value))
    throw new DomainError('INTERNAL_ERROR', 'Normalized output violated its public contract');
  if (
    value.command === 'run.view' &&
    value.status !== 'error' &&
    !ajv.validate('urn:teamcity-axi:run-view:1.0', value.data)
  )
    throw new DomainError('INTERNAL_ERROR', 'Run view violated its payload contract');
  if (
    value.command === 'run.list' &&
    value.status !== 'error' &&
    !ajv.validate('urn:teamcity-axi:run-list:1.0', value.data)
  )
    throw new DomainError('INTERNAL_ERROR', 'Run list violated its payload contract');

  for (const [command, schema] of [
    ['context.show', 'context-show'],
    ['doctor', 'doctor'],
    ['run.problems', 'run-problems'],
    ['run.tests', 'run-tests'],
    ['run.log', 'run-log'],
  ]) {
    if (
      value.command === command &&
      value.status !== 'error' &&
      !ajv.validate(`urn:teamcity-axi:${schema}:1.0`, value.data)
    )
      throw new DomainError('INTERNAL_ERROR', 'Normalized payload violated its public contract');
  }
}

export function validateConfig(name: 'user-config' | 'repository-config', value: unknown): void {
  if (!ajv.validate(`urn:teamcity-axi:${name}:1.0`, value))
    throw new DomainError('USAGE_ERROR', `Invalid ${name} configuration`, 2);
}
