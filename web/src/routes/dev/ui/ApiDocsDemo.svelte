<script lang="ts">
	// The API reference's parts, loaded apart from the style guide so it
	// stays inside its script budget.
	import { groupByTag, type Spec } from '#lib/apidocs.js';
	import Operation from '#lib/components/apidocs/Operation.svelte';
	import OperationList from '#lib/components/apidocs/OperationList.svelte';
	import SchemaView from '#lib/components/apidocs/SchemaView.svelte';
	import TryIt from '#lib/components/apidocs/TryIt.svelte';

	// A slice of the API document, as the reference at /api/docs reads it.
	const apiSample: Spec = {
		info: { title: 'Mila API' },
		tags: [
			{ name: 'instance', description: 'The instance itself.' },
			{ name: 'labels', description: 'A label any task in the workspace can carry.' }
		],
		security: [{ session: [] }],
		paths: {
			'/instance': {
				get: {
					tags: ['instance'],
					operationId: 'demoGetInstance',
					security: [],
					description: "The instance's name and version.",
					responses: { '200': { description: 'OK' } }
				}
			},
			'/workspaces/{workspaceId}/labels': {
				get: {
					tags: ['labels'],
					operationId: 'demoListLabels',
					// Open to anyone here, so the guide shows the form, not "Sign in".
					security: [],
					summary: 'List labels',
					description: 'The labels you can see, a page at a time in ID order.',
					parameters: [
						{ name: 'workspaceId', in: 'path', required: true, schema: { type: 'string' } },
						{
							name: 'color',
							in: 'query',
							description: 'Only rows whose color is this.',
							schema: { type: 'string' }
						}
					],
					responses: {
						'200': {
							description: 'A page of labels.',
							content: {
								'application/json': { schema: { $ref: '#/components/schemas/LabelPage' } }
							}
						}
					}
				},
				post: {
					tags: ['labels'],
					operationId: 'demoCreateLabel',
					summary: 'Make a label',
					description: 'Makes a label through the same merge as sync.',
					parameters: [
						{ name: 'workspaceId', in: 'path', required: true, schema: { type: 'string' } }
					],
					requestBody: {
						required: true,
						content: {
							'application/json': { schema: { $ref: '#/components/schemas/LabelCreate' } }
						}
					},
					responses: { '201': { description: 'Made.' } }
				}
			}
		},
		components: {
			schemas: {
				Label: {
					description: 'A label any task in the workspace can carry.',
					type: 'object',
					required: ['id', 'name', 'color'],
					properties: {
						id: { type: 'string', readOnly: true },
						name: { type: 'string', minLength: 1, maxLength: 60 },
						color: { type: 'string', enum: ['red', 'blue', 'green'], default: 'blue' },
						deleted_at: { type: 'string', format: 'date-time', nullable: true }
					}
				},
				LabelCreate: {
					type: 'object',
					required: ['name'],
					properties: {
						name: { type: 'string', minLength: 1, maxLength: 60 },
						color: { type: 'string', enum: ['red', 'blue', 'green'], default: 'blue' }
					}
				},
				LabelPage: {
					type: 'object',
					required: ['items'],
					properties: {
						items: { type: 'array', items: { $ref: '#/components/schemas/Label' } },
						next: { type: 'string', description: 'Pass as after for the next page.' }
					}
				}
			}
		}
	};

	const groups = groupByTag(apiSample);
	const [instance, list, create] = groups.flatMap((g) => g.operations);
</script>

<h3>SchemaView</h3>
<SchemaView spec={apiSample} schema={{ $ref: '#/components/schemas/LabelPage' }} />

<h3>OperationList</h3>
<div class="nav-demo"><OperationList {groups} /></div>

<h3>Operation</h3>
<p class="note">Closed, then open with its parameters, body, responses and Try it.</p>
<div class="ops">
	<Operation spec={apiSample} op={instance} />
	<Operation spec={apiSample} op={create} open />
</div>

<h3>TryIt</h3>
<p class="note">
	A request with a query parameter: Send ticks itself when a value is typed, and can send one empty.
</p>
<TryIt spec={apiSample} op={list} />

<style>
	h3 {
		margin-top: var(--space-4);
		font-size: var(--text-md);
	}
	.note {
		color: var(--text-secondary);
		font-size: var(--text-sm);
	}
	.nav-demo {
		max-width: 280px;
	}
	.ops {
		display: grid;
		gap: var(--space-2);
	}
</style>
