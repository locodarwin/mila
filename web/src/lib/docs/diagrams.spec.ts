import { describe, expect, it } from 'vitest';
import { accessibleText, isDiagram, kindName, mistakeOf, sourceKind, STARTERS } from './diagrams';

describe('diagrams', () => {
	it('are code blocks in the language mermaid', () => {
		expect(isDiagram('mermaid')).toBe(true);
		expect(isDiagram(' Mermaid ')).toBe(true);
		expect(isDiagram('js')).toBe(false);
		expect(isDiagram(null)).toBe(false);
	});

	it('know their kind from the first line that says something', () => {
		expect(sourceKind('%% a comment\n\ngraph TD\n  A --> B')).toBe('flowchart');
		expect(sourceKind('sequenceDiagram\n  A->>B: hi')).toBe('sequence');
		expect(sourceKind('stateDiagram-v2\n  [*] --> A')).toBe('state');
		expect(sourceKind('erDiagram')).toBe('er');
		expect(sourceKind('architecture-beta')).toBe('architecture');
		expect(sourceKind('pie')).toBe('pie');
		expect(sourceKind('')).toBeUndefined();
		expect(kindName('sequence')).toBe('Sequence diagram');
		expect(kindName('something-new')).toBe('Diagram');
	});

	it('take their name and description from the source', () => {
		expect(
			accessibleText('flowchart LR\n  accTitle: Request flow\n  accDescr: How a request goes')
		).toEqual({ title: 'Request flow', description: 'How a request goes' });
		expect(
			accessibleText('flowchart LR\n  accDescr {\n    Two lines\n    of it\n  }\n  A --> B')
		).toEqual({ title: '', description: 'Two lines of it' });
		expect(accessibleText('timeline\n  title Release plan').title).toBe('Release plan');
	});

	it("say Mermaid's mistakes with their line", () => {
		expect(mistakeOf(new Error('Parse error on line 3:\n...'))).toEqual({
			message: 'Parse error on line 3:\n...',
			line: 3
		});
		expect(mistakeOf({ str: 'Lexical error' })).toEqual({ message: 'Lexical error', line: null });
	});

	it('start from examples that name themselves', () => {
		expect(new Set(STARTERS.map((s) => s.id)).size).toBe(STARTERS.length);
		for (const s of STARTERS) {
			expect(sourceKind(s.source), s.id).toBeTruthy();
			expect(accessibleText(s.source).title || s.id === 'mindmap', s.id).toBeTruthy();
		}
	});
});
