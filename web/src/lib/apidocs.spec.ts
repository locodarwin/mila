import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import {
	blocks,
	exampleOf,
	flatten,
	groupByTag,
	operations,
	requestPath,
	resolveRef,
	typeOf,
	type Spec
} from './apidocs';

const spec: Spec = {
	info: { title: 'Test' },
	tags: [{ name: 'tasks', description: 'Tasks.' }, { name: 'empty' }],
	security: [{ session: [] }],
	paths: {
		'/instance': {
			get: { operationId: 'getInstance', tags: ['instance'], security: [], responses: {} }
		},
		'/workspaces/{workspaceId}/tasks': {
			get: {
				operationId: 'listTasks',
				tags: ['tasks'],
				parameters: [
					{ $ref: '#/components/parameters/WorkspaceId' } as never,
					{ name: 'project_id', in: 'query', schema: { type: 'string' } }
				],
				responses: {
					'200': {
						description: 'A page.',
						content: { 'application/json': { schema: { $ref: '#/components/schemas/Page' } } }
					},
					default: { $ref: '#/components/responses/Error' }
				}
			},
			post: {
				operationId: 'createTask',
				tags: ['tasks'],
				requestBody: {
					required: true,
					content: { 'application/json': { schema: { $ref: '#/components/schemas/TaskCreate' } } }
				},
				responses: {}
			}
		}
	},
	components: {
		parameters: {
			WorkspaceId: { name: 'workspaceId', in: 'path', required: true, schema: { type: 'string' } }
		},
		responses: {
			Error: {
				description: 'Wrong.',
				content: { 'application/json': { schema: { $ref: '#/components/schemas/Error' } } }
			}
		},
		schemas: {
			Error: { type: 'object', properties: { message: { type: 'string' } } },
			Task: {
				type: 'object',
				required: ['id', 'title'],
				properties: {
					id: { type: 'string', readOnly: true },
					title: { type: 'string', minLength: 1 },
					due_date: { type: 'string', format: 'date', nullable: true }
				}
			},
			Page: {
				type: 'object',
				properties: { items: { type: 'array', items: { $ref: '#/components/schemas/Task' } } }
			},
			TaskCreate: {
				type: 'object',
				required: ['title', 'priority', 'done'],
				properties: {
					title: { type: 'string', minLength: 1 },
					priority: { type: 'string', enum: ['none', 'high'], default: 'none' },
					done: { type: 'boolean' },
					notes: { type: 'string' }
				}
			},
			Patch: {
				type: 'object',
				minProperties: 1,
				properties: { notes: { type: 'string' }, title: { type: 'string', minLength: 1 } }
			},
			Change: {
				allOf: [
					{ $ref: '#/components/schemas/Task' },
					{ type: 'object', required: ['seq'], properties: { seq: { type: 'integer' } } }
				]
			}
		}
	}
};

describe('the API reference', () => {
	it('follows references', () => {
		expect(resolveRef(spec, { $ref: '#/components/schemas/Error' })).toEqual(
			spec.components!.schemas.Error
		);
		expect(() => resolveRef(spec, { $ref: '#/components/schemas/Nope' })).toThrow();
		const change = flatten(spec, { $ref: '#/components/schemas/Change' });
		expect(Object.keys(change.properties!)).toEqual(['id', 'title', 'due_date', 'seq']);
		expect(change.required).toEqual(['id', 'title', 'seq']);
	});

	it('lists operations with their parameters, body, responses and sign-in', () => {
		const ops = operations(spec);
		expect(ops.map((o) => `${o.method} ${o.id}`)).toEqual([
			'GET getInstance',
			'GET listTasks',
			'POST createTask'
		]);
		const list = ops[1];
		expect(list.parameters.map((p) => `${p.in}:${p.name}`)).toEqual([
			'path:workspaceId',
			'query:project_id'
		]);
		expect(list.responses.map((r) => r.status)).toEqual(['200', 'default']);
		expect(list.responses[1].description).toBe('Wrong.');
		expect(ops[0].auth).toBe(false);
		expect(list.auth).toBe(true);
		expect(ops[2].body?.required).toBe(true);
	});

	it('groups operations by tag, in the tags order, dropping empty tags', () => {
		expect(groupByTag(spec).map((g) => [g.tag, g.operations.length])).toEqual([
			['tasks', 2],
			['instance', 1]
		]);
	});

	it('makes example bodies from defaults, choices and types', () => {
		expect(exampleOf(spec, { $ref: '#/components/schemas/TaskCreate' })).toEqual({
			title: 'Text',
			priority: 'none',
			done: false
		});
		// Nothing required: the first field, so the body is not empty.
		expect(exampleOf(spec, { $ref: '#/components/schemas/Patch' })).toEqual({ notes: '' });
		expect(exampleOf(spec, { type: 'string', nullable: true })).toBeNull();
		expect(exampleOf(spec, { type: 'integer', minimum: 1 })).toBe(1);
	});

	it('names types', () => {
		expect(typeOf(spec, { $ref: '#/components/schemas/Task' })).toBe('Task');
		expect(typeOf(spec, { type: 'array', items: { $ref: '#/components/schemas/Task' } })).toBe(
			'array of Task'
		);
		expect(typeOf(spec, { type: 'string', format: 'date', nullable: true })).toBe(
			'string (date) or null'
		);
		expect(typeOf(spec, { type: 'string', enum: ['a', 'b'] })).toBe('"a" | "b"');
		expect(typeOf(spec, {})).toBe('any JSON');
		expect(
			typeOf(spec, { oneOf: [{ type: 'string' }, { type: 'array', items: { type: 'string' } }] })
		).toBe('string | array of string');
	});

	// The served spec: Try it starts every JSON field with a value the
	// server takes, never null.
	it('starts JSON fields of the real spec with a value of their shape', () => {
		const real = JSON.parse(
			readFileSync(
				new URL('../../../server/internal/httpapi/openapi.json', import.meta.url),
				'utf8'
			)
		) as Spec;
		const view = exampleOf(real, { $ref: '#/components/schemas/SavedViewCreate' }) as Record<
			string,
			unknown
		>;
		expect(view.config).toEqual({});
		const field = exampleOf(real, { $ref: '#/components/schemas/CustomFieldCreate' }) as Record<
			string,
			unknown
		>;
		expect(field.options ?? []).toEqual([]);
		const value = exampleOf(real, {
			$ref: '#/components/schemas/CustomFieldValueCreate'
		}) as Record<string, unknown>;
		expect(typeof value.value).toBe('string');
		const display = flatten(real, { $ref: '#/components/schemas/SavedViewCreate' }).properties!
			.display;
		expect(exampleOf(real, { ...display, nullable: false, default: undefined })).toEqual({});
	});

	it('splits descriptions into paragraphs and lists', () => {
		expect(blocks('One\nline.\n\n- a\n  more\n- b\n\nTwo.')).toEqual([
			{ kind: 'p', text: 'One line.' },
			{ kind: 'ul', items: ['a more', 'b'] },
			{ kind: 'p', text: 'Two.' }
		]);
	});

	it('fills in paths and queries', () => {
		const list = operations(spec)[1];
		expect(requestPath(list, { workspaceId: 'w 1', project_id: 'p' })).toEqual({
			path: '/workspaces/w%201/tasks?project_id=p',
			missing: []
		});
		expect(requestPath(list, {}).missing).toEqual(['workspaceId']);
		// An empty value sent on purpose: a filter's "none".
		expect(
			requestPath(list, { workspaceId: 'w', project_id: '' }, new Set(['project_id'])).path
		).toBe('/workspaces/w/tasks?project_id=');
		expect(requestPath(list, { workspaceId: 'w', project_id: 'p' }, new Set()).path).toBe(
			'/workspaces/w/tasks'
		);
	});

	it('reads the real document', () => {
		const real: Spec = JSON.parse(
			readFileSync(
				new URL('../../../server/internal/httpapi/openapi.json', import.meta.url),
				'utf8'
			)
		);
		const ops = operations(real);
		expect(new Set(ops.map((o) => o.id)).size).toBe(ops.length);
		expect(ops.length).toBeGreaterThan(100);
		const groups = groupByTag(real);
		expect(groups.map((g) => g.tag)).toContain('tasks');
		for (const op of ops) {
			for (const r of op.responses) if (r.schema) typeOf(real, r.schema);
			if (op.body?.schema && op.body.type === 'application/json') {
				expect(typeof exampleOf(real, op.body.schema), op.id).toBe('object');
			}
		}
		const create = ops.find((o) => o.id === 'createTask')!;
		expect(exampleOf(real, create.body!.schema)).toEqual({ title: 'Text' });
	});
});
