// Hybrid logical clocks: "<13-digit ms>-<4 hex counter>-<device>". They
// sort as strings in the order events happened, as far as any device can
// tell, and every device's are distinct. A device moves its clock past
// every HLC it sees, so its next edit always outranks what it has seen.
// The server's checks are in server/internal/syncer/hlc.go.

export const HLC_PATTERN = /^(\d{13})-([0-9a-f]{4})-([A-Za-z0-9_-]{1,32})$/;

export class Clock {
	ms = 0;
	counter = 0;

	constructor(readonly node: string) {}

	/** The HLC for a new local event. */
	tick(now = Date.now()): string {
		if (now > this.ms) {
			this.ms = now;
			this.counter = 0;
		} else {
			this.counter++;
			if (this.counter > 0xffff) {
				// 65,536 edits in one millisecond: borrow the next one.
				this.ms++;
				this.counter = 0;
			}
		}
		return this.toString();
	}

	/** Moves the clock past an HLC seen from elsewhere. */
	observe(hlc: string) {
		const m = HLC_PATTERN.exec(hlc);
		if (!m) return;
		const ms = Number(m[1]);
		const counter = parseInt(m[2], 16);
		if (ms > this.ms || (ms === this.ms && counter > this.counter)) {
			this.ms = ms;
			this.counter = counter;
		}
	}

	toString(): string {
		return `${String(this.ms).padStart(13, '0')}-${this.counter.toString(16).padStart(4, '0')}-${this.node}`;
	}
}
