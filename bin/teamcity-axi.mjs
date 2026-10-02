#!/usr/bin/env node
// Keep the standalone version path independent of the application graph.
const args = process.argv.slice(2);
if (args.length === 1 && ['--version', '-v', '-V'].includes(args[0])) {
  process.stdout.write('0.1.0\n');
} else {
  const { main } = await import('../dist/cli/main.js');
  await main(args);
}
