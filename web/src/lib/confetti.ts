// Playful's one flourish: a short burst of confetti when a project's last
// open task is completed. Only where the theme turns it on (--celebrate,
// which Playful sets), and never when the OS asks for less motion. It is decoration, hidden from assistive technology
// and from the pointer, and gone in under two seconds.
import { reducedMotion } from './motion';
import { PALETTE } from './theme/palette';

const PIECES = 36;

/** Whether a finished project gets confetti here: the theme's choice, with motion allowed. */
export function wantsConfetti(): boolean {
	return (
		typeof document !== 'undefined' &&
		getComputedStyle(document.documentElement).getPropertyValue('--celebrate').trim() === '1' &&
		!reducedMotion()
	);
}

export function confetti() {
	if (!wantsConfetti()) return;
	const layer = document.createElement('div');
	layer.className = 'confetti';
	layer.setAttribute('aria-hidden', 'true');
	document.body.append(layer);
	const runs: Promise<unknown>[] = [];
	for (let i = 0; i < PIECES; i++) {
		const piece = document.createElement('span');
		piece.style.background = `var(--palette-${PALETTE[i % PALETTE.length]})`;
		layer.append(piece);
		// From the middle of the screen, up and out, then down with a spin.
		const angle = Math.random() * Math.PI - Math.PI;
		const reach = 120 + Math.random() * 220;
		const x = Math.cos(angle) * reach;
		const y = Math.sin(angle) * reach;
		const spin = (Math.random() - 0.5) * 1080;
		runs.push(
			piece.animate(
				[
					{ transform: 'translate(0, 0) rotate(0deg)', opacity: 1 },
					{
						transform: `translate(${x}px, ${y}px) rotate(${spin / 2}deg)`,
						opacity: 1,
						offset: 0.4
					},
					{ transform: `translate(${x * 1.2}px, ${y + 320}px) rotate(${spin}deg)`, opacity: 0 }
				],
				{ duration: 1200 + Math.random() * 400, easing: 'cubic-bezier(0.2, 0.6, 0.4, 1)' }
			).finished
		);
	}
	void Promise.allSettled(runs).then(() => layer.remove());
}
