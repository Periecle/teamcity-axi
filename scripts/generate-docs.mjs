import { readFile, writeFile } from 'node:fs/promises';
import { format, resolveConfig } from 'prettier';
import { registry, help } from '../dist/cli/registry.js';
import { parse } from '../dist/cli/parser.js';

const path = new URL('../docs/commands.md', import.meta.url);
const version = JSON.parse(
  await readFile(new URL('../package.json', import.meta.url), 'utf8'),
).version;
const example = (descriptor) => {
  const args = descriptor.name.split('.');
  if (descriptor.positional === 'runId') args.push('482193');
  else if (descriptor.name === 'job.view') args.push('Payments_Build');
  else if (descriptor.name === 'agent.view') args.push('7');
  else if (descriptor.name === 'schema') args.push('run.view');
  if (['status', 'run.list', 'queue.list', 'agent.list'].includes(descriptor.name))
    args.push('--job=Payments_Build');
  if (descriptor.name === 'job.list') args.push('--project=Payments');
  if (!['schema', 'context.show'].includes(descriptor.name)) args.push('--server=work');
  args.push('--json');
  parse(args);
  return 'teamcity-axi ' + args.join(' ');
};

const sections = registry.map(
  (descriptor) =>
    `## ${descriptor.name}\n\n${descriptor.summary}.\n\n\`\`\`sh\n${example(descriptor)}\n\`\`\`\n\n\`\`\`text\n${help(descriptor).trimEnd()}\n\`\`\`\n`,
);
const content = await format(
  `# Command reference\n\nGenerated for teamcity-axi ${version} from the executable command registry.\nRun \`npm run docs:generate\` after changing commands; \`npm test\` rejects drift.\n\nUse a registered server alias. Example IDs are placeholders, not discovered resources.\nThese commands only read TeamCity. Status checks the local committed checkout;\nwatch checks the terminal outcome of one fixed execution. A normal red observation\nexits zero; \`--check\` fails its assertion with exit one.\n\n${sections.join('\n')}`,
  { ...(await resolveConfig(path.pathname)), parser: 'markdown' },
);
if (process.argv.includes('--check')) {
  if ((await readFile(path, 'utf8')) !== content)
    throw Error('Generated command documentation has drifted; run npm run docs:generate');
  console.log(`Verified help and parseable examples for ${registry.length} commands`);
} else {
  await writeFile(path, content);
  console.log(`Generated help and parseable examples for ${registry.length} commands`);
}
