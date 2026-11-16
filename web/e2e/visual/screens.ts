// What the main screens' screenshots share with the setup that seeds them.
import { join } from 'node:path';

export const DATA = join(import.meta.dirname, '../../../tmp/visual');
export const SEED = join(DATA, 'seed.json');
export const STATE = join(DATA, 'state.json');
export const TODAY = new Date('2026-11-03T15:00:00Z');
export const DAY = 24 * 3600 * 1000;
