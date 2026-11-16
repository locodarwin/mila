// The one axe check every browser test uses: WCAG 2.2 AA, plus the
// best-practice rules about page structure that the WCAG tags leave out
// (one main, named and unique landmarks, everything inside one, an h1 and
// headings in order), which a screen reader user navigates by.
import AxeBuilder from '@axe-core/playwright';
import axeCore from 'axe-core';
import { expect, type Page } from '@playwright/test';

const WCAG = ['wcag2a', 'wcag2aa', 'wcag21aa', 'wcag22aa'];

const STRUCTURE = [
	'region',
	'landmark-one-main',
	'landmark-unique',
	'landmark-no-duplicate-main',
	'landmark-main-is-top-level',
	'landmark-complementary-is-top-level',
	'page-has-heading-one',
	'heading-order',
	'empty-heading',
	'aria-dialog-name',
	'skip-link',
	'tabindex',
	'presentation-role-conflict'
];

const RULES = [...axeCore.getRules(WCAG).map((r) => r.ruleId), ...STRUCTURE];

/**
 * Waits out animations and transitions: a panel still sliding in is half
 * transparent, which axe reads as low contrast. One that never ends (the
 * syncing dot's pulse) is not waited for.
 */
export async function settle(page: Page) {
	await page.waitForFunction(() =>
		document
			.getAnimations()
			.every((a) => a.playState !== 'running' || a.effect?.getTiming().iterations === Infinity)
	);
}

export interface AxeOptions {
	/**
	 * Leave out the structure rules, for a page that is a part of one
	 * rather than a whole (the style guide).
	 */
	structure?: boolean;
	/** Only these rules, for a narrower check (contrast while a slider moves). */
	rules?: string[];
}

/** Every violation on the page, as "rule: the elements", for a readable failure. */
export async function axeViolations(page: Page, options: AxeOptions = {}): Promise<string[]> {
	await settle(page);
	const rules =
		options.rules ??
		(options.structure === false ? axeCore.getRules(WCAG).map((r) => r.ruleId) : RULES);
	const results = await new AxeBuilder({ page }).withRules(rules).analyze();
	return results.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target).join(', ')}`);
}

/** Fails the test with the list of violations if axe finds any. */
export async function axe(page: Page, options: AxeOptions = {}) {
	expect(await axeViolations(page, options)).toEqual([]);
}
