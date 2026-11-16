import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { DEFAULTS, PAIRS, pairId, xorId, type Entity } from './entities';

// The defaults a REST create fills in come from the server's registry
// (server/internal/syncer/entities.go), which a Go test holds to the
// migrations; the spec's JSON copy carries them here.
const spec = JSON.parse(
	readFileSync(new URL('../../../../server/internal/httpapi/openapi.json', import.meta.url), 'utf8')
);

type Schema = { properties: Record<string, { default?: unknown }> };

function typeName(entity: string): string {
	return entity
		.split('_')
		.map((part) => part[0].toUpperCase() + part.slice(1))
		.join('');
}

describe('DEFAULTS', () => {
	it('match the defaults of a REST create', () => {
		const schemas: Record<string, Schema> = spec.components.schemas;
		let compared = 0;
		for (const entity of Object.keys(DEFAULTS) as Entity[]) {
			// A document's rows are not offered by the REST API, which reads
			// and writes the document as Markdown, through the notes or body.
			if (entity === 'doc_update' || entity === 'page_update') continue;
			const row = schemas[typeName(entity)];
			expect(row, entity).toBeDefined();
			// Attachments are made only by an upload, so there is no create.
			const create = schemas[typeName(entity) + 'Create'] ?? row;
			for (const [field, value] of Object.entries(DEFAULTS[entity])) {
				expect(row.properties, `${entity}.${field}`).toHaveProperty(field);
				const schema = create.properties[field];
				if (schema && 'default' in schema) {
					expect(schema.default, `${entity}.${field}`).toEqual(value);
					compared++;
				}
			}
		}
		expect(compared).toBeGreaterThan(30);
	});
});

describe('pairId', () => {
	const task = '0190a4c2-6f1e-7b3a-9c4d-2e5f6a7b8c9d';
	const label = '4f8e2a1c-3b5d-4e6f-8a9b-0c1d2e3f4a5b';

	it("makes the server's IDs (server/internal/ids/ids_test.go)", () => {
		expect(pairId(task, label)).toBe('5ed720da-10d3-843a-a10e-001833259355');
		expect(xorId(task, label)).toBe('4e1e8ede-5443-3555-16d6-22424444c6c6');
	});

	it('depends on the order of the pair, and is empty without one', () => {
		expect(pairId(label, task)).not.toBe(pairId(task, label));
		expect(pairId(task, '')).toBe('');
	});

	it("names the server's pairs", () => {
		const go = readFileSync('../server/internal/syncer/dump.go', 'utf8');
		const block = go.slice(go.indexOf('var pairs = map'));
		const server = Object.fromEntries(
			[...block.slice(0, block.indexOf('\n}')).matchAll(/"(\w+)":\s*\{"(\w+)", "(\w+)"\}/g)].map(
				(m) => [m[1], [m[2], m[3]]]
			)
		);
		expect(PAIRS).toEqual(server);
	});
});
