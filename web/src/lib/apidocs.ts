// What the API reference at /api/docs reads from the OpenAPI document the
// server serves (/api/v1/openapi.json, shared/openapi.yaml as JSON): its
// operations grouped by tag, schemas with their references followed, and
// an example body for each request.

export interface Schema {
	$ref?: string;
	type?: string;
	format?: string;
	enum?: unknown[];
	nullable?: boolean;
	readOnly?: boolean;
	default?: unknown;
	description?: string;
	properties?: Record<string, Schema>;
	required?: string[];
	items?: Schema;
	allOf?: Schema[];
	/** The value is one of these: a field that takes several JSON types. */
	oneOf?: Schema[];
	additionalProperties?: boolean | Schema;
	minLength?: number;
	maxLength?: number;
	minimum?: number;
	maximum?: number;
	minProperties?: number;
	pattern?: string;
}

export interface Parameter {
	$ref?: string;
	name: string;
	in: 'path' | 'query' | 'header' | 'cookie';
	required?: boolean;
	description?: string;
	schema?: Schema;
}

interface MediaTypes {
	content?: Record<string, { schema?: Schema }>;
}

interface RawOperation {
	tags?: string[];
	operationId: string;
	summary?: string;
	description?: string;
	security?: unknown[];
	parameters?: Parameter[];
	requestBody?: MediaTypes & { $ref?: string; required?: boolean };
	responses?: Record<string, MediaTypes & { $ref?: string; description?: string }>;
}

export interface Spec {
	info: { title: string; description?: string };
	tags?: { name: string; description?: string }[];
	paths: Record<string, Record<string, RawOperation>>;
	components?: Record<string, Record<string, unknown>>;
	security?: unknown[];
}

export interface Response {
	status: string;
	description: string;
	schema?: Schema;
	type?: string;
}

export interface Operation {
	id: string;
	method: string;
	path: string;
	summary: string;
	description: string;
	tags: string[];
	parameters: Parameter[];
	body?: { schema?: Schema; type: string; required: boolean };
	responses: Response[];
	/** Whether it needs someone signed in. */
	auth: boolean;
}

export interface Group {
	tag: string;
	description: string;
	operations: Operation[];
}

const METHODS = ['get', 'post', 'put', 'patch', 'delete'];

/** Follows a local "#/components/..." reference, or returns what it was given. */
export function resolveRef<T extends { $ref?: string }>(spec: Spec, value: T): T {
	let current: T = value;
	for (let hops = 0; current?.$ref && hops < 20; hops++) {
		const parts = current.$ref.replace(/^#\//, '').split('/');
		let at: unknown = spec;
		for (const p of parts) at = (at as Record<string, unknown> | undefined)?.[p];
		if (at === undefined) throw new Error(`No ${current.$ref} in the API document`);
		current = at as T;
	}
	return current;
}

/** The name a schema reference points at: "Task" for #/components/schemas/Task. */
export function refName(schema: Schema): string | undefined {
	return schema.$ref?.split('/').pop();
}

/** A schema with references followed and allOf merged into one object. */
export function flatten(spec: Spec, schema: Schema): Schema {
	const s = resolveRef(spec, schema);
	if (!s.allOf) return s;
	const out: Schema = { type: 'object', properties: {}, required: [] };
	for (const part of s.allOf) {
		const p = flatten(spec, part);
		Object.assign(out.properties!, p.properties ?? {});
		out.required!.push(...(p.required ?? []));
		out.description ??= p.description;
	}
	out.description = s.description ?? out.description;
	return out;
}

function firstContent(m: MediaTypes | undefined): { type: string; schema?: Schema } | undefined {
	const entries = Object.entries(m?.content ?? {});
	if (!entries.length) return undefined;
	const [type, { schema }] = entries.find(([t]) => t === 'application/json') ?? entries[0];
	return { type, schema };
}

/** Every operation in the document, in its order, with parameters resolved. */
export function operations(spec: Spec): Operation[] {
	const out: Operation[] = [];
	for (const [path, item] of Object.entries(spec.paths)) {
		for (const method of METHODS) {
			const op = item[method];
			if (!op) continue;
			const body = op.requestBody ? resolveRef(spec, op.requestBody) : undefined;
			const content = firstContent(body);
			out.push({
				id: op.operationId,
				method: method.toUpperCase(),
				path,
				summary: op.summary ?? '',
				description: op.description ?? '',
				tags: op.tags ?? [],
				parameters: (op.parameters ?? []).map((p) => resolveRef(spec, p)),
				body: content && { ...content, required: !!body?.required },
				responses: Object.entries(op.responses ?? {}).map(([status, r]) => {
					const resolved = resolveRef(spec, r);
					const c = firstContent(resolved);
					return {
						status,
						description: resolved.description ?? '',
						schema: c?.schema,
						type: c?.type
					};
				}),
				auth: (op.security ?? spec.security ?? []).length > 0
			});
		}
	}
	return out;
}

/** The operations under each tag, in the document's order of tags. */
export function groupByTag(spec: Spec): Group[] {
	const groups = new Map<string, Group>();
	for (const t of spec.tags ?? []) {
		groups.set(t.name, { tag: t.name, description: t.description ?? '', operations: [] });
	}
	for (const op of operations(spec)) {
		const tag = op.tags[0] ?? 'other';
		if (!groups.has(tag)) groups.set(tag, { tag, description: '', operations: [] });
		groups.get(tag)!.operations.push(op);
	}
	return [...groups.values()].filter((g) => g.operations.length > 0);
}

/**
 * A value that fits a schema, to start a request body from: required
 * fields (or, where nothing is required, the first one), each with its
 * default, its first choice, or a placeholder of its type.
 */
export function exampleOf(spec: Spec, schema: Schema | undefined, depth = 0): unknown {
	if (!schema || depth > 4) return null;
	const s = flatten(spec, schema);
	if (s.default !== undefined) return s.default;
	if (s.enum?.length) return s.enum[0];
	if (s.oneOf?.length) return exampleOf(spec, s.oneOf[0], depth + 1);
	if (s.properties) {
		const out: Record<string, unknown> = {};
		const writable = Object.entries(s.properties).filter(([, p]) => !resolveRef(spec, p).readOnly);
		const required = s.required ?? [];
		const picked = writable.filter(([name]) => required.includes(name));
		if (!picked.length && writable.length) picked.push(writable[0]);
		for (const [name, p] of picked) out[name] = exampleOf(spec, p, depth + 1);
		return out;
	}
	switch (s.type) {
		case 'array':
			return [];
		case 'boolean':
			return false;
		case 'integer':
		case 'number':
			return s.minimum ?? 0;
		case 'string':
			if (s.nullable) return null;
			if (s.format === 'date-time') return new Date().toISOString();
			if (s.format === 'date') return new Date().toISOString().slice(0, 10);
			return s.minLength ? 'Text' : '';
	}
	return s.type === 'object' ? {} : null;
}

/** A schema's type in a few words: "string (date-time) or null". */
export function typeOf(spec: Spec, schema: Schema): string {
	const name = refName(schema);
	const s = flatten(spec, schema);
	let t: string;
	if (s.type === 'array' && s.items) t = `array of ${typeOf(spec, s.items)}`;
	else if (name && (s.properties || s.allOf)) t = name;
	else if (s.enum) t = s.enum.map((v) => JSON.stringify(v)).join(' | ');
	else if (s.oneOf?.length) t = s.oneOf.map((o) => typeOf(spec, o)).join(' | ');
	else if (s.type) t = s.format ? `${s.type} (${s.format})` : s.type;
	else if (s.properties) t = 'object';
	else t = 'any JSON';
	return s.nullable ? `${t} or null` : t;
}

export type Block = { kind: 'p'; text: string } | { kind: 'ul'; items: string[] };

/**
 * Splits a description into paragraphs and lists: blank lines part
 * paragraphs, and lines starting "- " make a list.
 */
export function blocks(text: string): Block[] {
	const out: Block[] = [];
	for (const chunk of text.trim().split(/\n\s*\n/)) {
		const lines = chunk.split('\n');
		if (lines[0].trimStart().startsWith('- ')) {
			const items: string[] = [];
			for (const line of lines) {
				const l = line.trim();
				if (l.startsWith('- ')) items.push(l.slice(2));
				else if (items.length) items[items.length - 1] += ' ' + l;
			}
			out.push({ kind: 'ul', items });
		} else if (chunk.trim()) {
			out.push({ kind: 'p', text: lines.map((l) => l.trim()).join(' ') });
		}
	}
	return out;
}

/** The anchor an operation's entry has on the page. */
export function anchorOf(op: Operation): string {
	return 'op-' + op.id;
}

/**
 * A path with its parameters filled in, and the query from the rest: the
 * query parameters named in send, empty or not, or without send, those
 * with a value.
 */
export function requestPath(
	op: Operation,
	values: Record<string, string>,
	send?: Set<string>
): { path: string; missing: string[] } {
	const missing: string[] = [];
	const path = op.path.replace(/\{(\w+)\}/g, (_, name: string) => {
		const v = values[name]?.trim() ?? '';
		if (!v) missing.push(name);
		return encodeURIComponent(v);
	});
	const query = new URLSearchParams();
	for (const p of op.parameters) {
		const v = values[p.name];
		if (p.in !== 'query') continue;
		if (send ? send.has(p.name) : v !== undefined && v !== '') query.set(p.name, v ?? '');
	}
	const qs = query.toString();
	return { path: qs ? `${path}?${qs}` : path, missing };
}
