// Drawing a diagram's source with Mermaid. Loaded only when a document holds
// a diagram, so neither the editor nor any page carries Mermaid until one is
// drawn. Mermaid draws in its neutral look; the colors are then the theme's,
// through the rules below, which name tokens rather than colors, so a
// diagram follows a change of theme or scheme without being drawn again.
import type { Mermaid } from 'mermaid';

/** The longest source drawn, in characters. */
export const SOURCE_LIMIT = 20_000;

// Every rule is scoped to the diagram's own SVG by Mermaid, and comes after
// its theme's, so these win.
const THEME_CSS = `
svg { background: transparent; }
/* Mermaid colors its words with rules more particular than these (\`.label text\`), so the
   theme's text colors win outright, and the few exceptions below do too. */
text, tspan, .label, .nodeLabel, .edgeLabel { fill: var(--diagram-text) !important; color: var(--diagram-text) !important; }
.grid .tick text { fill: var(--text-secondary) !important; }
.activeText0, .activeText1, .activeText2, .activeText3 { fill: var(--accent-contrast) !important; }
.nodeLabel, .edgeLabel, .label foreignObject div { color: var(--diagram-text); }
.node rect, .node circle, .node ellipse, .node polygon, .node path, .basic.label-container,
rect.actor, .labelBox, .classGroup rect, .er.entityBox, .entityBox, .stateGroup rect,
.statediagram-state rect, .statediagram-state .divider, .node .label-container {
  fill: var(--diagram-node-bg); stroke: var(--diagram-node-border);
}
.cluster rect, .statediagram-cluster rect, .composit, .er.attributeBoxOdd, .attributeBoxOdd {
  fill: var(--diagram-group-bg); stroke: var(--border);
}
.er.attributeBoxEven, .attributeBoxEven { fill: var(--diagram-node-bg); stroke: var(--border); }
.edgePath .path, .flowchart-link, .relation, .er.relationshipLine, .relationshipLine,
.transition, .messageLine0, .messageLine1, .loopLine, .actor-line, path.edge, .edge-pattern-solid,
.edge-pattern-dashed, .edge-pattern-dotted, line.divider {
  stroke: var(--diagram-line);
}
marker path, .arrowheadPath, .arrowMarkerPath, #arrowhead path, .marker, .marker.cross {
  fill: var(--diagram-line); stroke: var(--diagram-line);
}
.edgeLabel, .edgeLabel rect, .edgeLabel p, .labelBkg { background-color: var(--surface-0); fill: var(--surface-0); }
.note, rect.note { fill: var(--diagram-note-bg); stroke: var(--diagram-node-border); }
.activation0, .activation1, .activation2 { fill: var(--diagram-group-bg); stroke: var(--diagram-node-border); }
.state-start, .start-state, circle.state-start { fill: var(--diagram-text); stroke: var(--diagram-text); }
.state-end, .end-state-inner { fill: var(--diagram-text); stroke: var(--surface-0); }
.end-state-outer { fill: var(--surface-0); stroke: var(--diagram-text); }
.grid .tick line, .grid path { stroke: var(--border); }
rect.background, .relationshipLabelBox { fill: var(--surface-0); stroke: none; }
.today { stroke: var(--danger); }
.section0, .section2 { fill: var(--surface-2); }
.section1, .section3 { fill: var(--surface-1); }
.task0, .task1, .task2, .task3, .task { fill: var(--accent-soft); stroke: var(--accent); }
.done0, .done1, .done2, .done3 { fill: var(--surface-3); stroke: var(--border-strong); }
.active0, .active1, .active2, .active3 { fill: var(--accent); stroke: var(--accent); }
.crit0, .crit1, .crit2, .crit3 { fill: var(--palette-red-soft); stroke: var(--danger); }
.milestone { fill: var(--accent); stroke: var(--accent); }
.pieCircle { stroke: var(--surface-0); opacity: 1; }
.pieOuterCircle { stroke: var(--border); }
${[1, 2, 3, 4, 5, 6]
	.map(
		(n) =>
			// A legend's swatch has its color in a style of its own, and the
			// legends follow the group of slices.
			`.pieCircle:nth-of-type(6n+${n % 6}) { fill: var(--chart-series-${n}); }
.legend:nth-of-type(6n+${(n + 1) % 6}) rect { fill: var(--chart-series-${n}) !important; stroke: var(--chart-series-${n}) !important; }`
	)
	.join('\n')}
${[-1, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11]
	.map((i) => {
		// A timeline numbers its sections from -1, a mind map from 0.
		const hue = ['blue', 'green', 'purple', 'amber', 'teal', 'pink'][(i + 6) % 6];
		return `.section-${i} rect, .section-${i} path, .section-${i} circle, .section-${i} polygon, .node-bkg.section-${i} { fill: var(--palette-${hue}-soft); stroke: var(--palette-${hue}); }
.section-${i} line, .section-edge-${i}, .edge-depth-${i} { stroke: var(--palette-${hue}); }
`;
	})
	.join('\n')}
.section-root rect, .section-root path, .section-root circle, .mindmap-node.section-root circle {
  fill: var(--accent); stroke: var(--accent);
}
.section-root text, .section-root tspan, .section-root .nodeLabel { fill: var(--accent-contrast) !important; color: var(--accent-contrast) !important; }
.architecture-service rect, .node-bkg { stroke: var(--diagram-node-border); }
`;

let loaded: Promise<Mermaid> | null = null;
let fontFamily = '';

function load(): Promise<Mermaid> {
	loaded ??= import('mermaid').then((m) => m.default);
	return loaded;
}

/** Starts fetching Mermaid ahead of the first diagram, as one comes into view. */
export function preload() {
	void load().catch(() => {
		loaded = null;
	});
}

function configure(mermaid: Mermaid, font: string) {
	mermaid.initialize({
		startOnLoad: false,
		securityLevel: 'strict',
		theme: 'neutral',
		// The plain look: the newer one draws shadows and gradients in colors of its own.
		look: 'classic',
		themeCSS: THEME_CSS,
		fontFamily: font || 'sans-serif',
		htmlLabels: false,
		flowchart: { htmlLabels: false },
		maxTextSize: SOURCE_LIMIT,
		maxEdges: 500,
		suppressErrorRendering: true,
		// Only the icons Mermaid has itself: nothing is fetched to draw a diagram.
		architecture: {}
	});
	fontFamily = font;
}

export interface Drawn {
	svg: string;
	/** Mermaid's name for its kind, `flowchart` or `sequence`. */
	type: string;
}

let queue: Promise<unknown> = Promise.resolve();
let count = 0;

/**
 * Draws a source as SVG, one diagram at a time (Mermaid keeps state while it
 * draws). Rejects with Mermaid's message when the source has a mistake.
 */
export function draw(source: string, font: string): Promise<Drawn> {
	const run = async (): Promise<Drawn> => {
		if (source.length > SOURCE_LIMIT)
			throw new Error(
				`This diagram is longer than ${SOURCE_LIMIT.toLocaleString('en-US')} characters.`
			);
		const mermaid = await load();
		if (font !== fontFamily || !count) configure(mermaid, font);
		const id = `mila-diagram-${++count}`;
		try {
			const { svg, diagramType } = await mermaid.render(id, source);
			return { svg, type: diagramType };
		} finally {
			// Mermaid leaves its scratch element behind when a source fails.
			document.getElementById(id)?.remove();
			document.getElementById(`d${id}`)?.remove();
		}
	};
	const next = queue.then(run, run);
	queue = next.catch(() => undefined);
	return next;
}
