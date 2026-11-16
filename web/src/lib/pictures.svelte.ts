// When to try people's pictures again. An avatar whose picture failed to
// load (offline and not cached, say) shows initials until this round
// moves on: when the browser comes back online, and when the app
// reconnects and reloads people (session.refreshPeople).
class Pictures {
	round = $state(0);

	retry() {
		this.round++;
	}
}

export const pictures = new Pictures();

if (typeof window !== 'undefined') window.addEventListener('online', () => pictures.retry());
