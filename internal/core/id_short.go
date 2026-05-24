package core

// ShortID returns at most max bytes of id. It is intended for display names,
// log prefixes, and generated tags where callers should not panic on short IDs.
func ShortID(id string, max int) string {
	if max <= 0 || len(id) <= max {
		return id
	}
	return id[:max]
}
