// The service worker's cache of the app itself: built files and the shell
// page, nobody's data. Named for the build, so a new version replaces it.
import { version } from '$app/env';

export const APP_CACHE = `mila-${version}`;
