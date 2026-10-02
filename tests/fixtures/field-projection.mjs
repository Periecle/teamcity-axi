// Minimal REST field selection for fixed evaluation requests. Applying this at
// the server keeps measured native output faithful to its selected fields.
export function selectFields(value, fields) {
  if (!fields) return value;

  const selections = [];
  let depth = 0,
    start = 0;

  for (let offset = 0; offset <= fields.length; offset++) {
    const character = fields[offset];

    if (character === '(') depth++;
    if (character === ')') depth--;
    if (depth < 0) throw new Error('Invalid fixture field selection');

    if (offset === fields.length || (character === ',' && depth === 0)) {
      selections.push(fields.slice(start, offset));
      start = offset + 1;
    }
  }

  if (depth !== 0) throw new Error('Unbalanced fixture field selection');
  if (selections.some((selection) => selection.startsWith('$')))
    throw new Error('Unsupported fixture field expansion');
  if (Array.isArray(value)) return value.map((item) => selectFields(item, fields));
  if (value === null || typeof value !== 'object') return value;

  const output = {};

  for (const selection of selections) {
    const opening = selection.indexOf('(');
    const name = opening < 0 ? selection : selection.slice(0, opening);

    if (!Object.hasOwn(value, name)) continue;

    output[name] =
      opening < 0 ? value[name] : selectFields(value[name], selection.slice(opening + 1, -1));
  }

  return output;
}
