package syncer

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// An HLC (hybrid logical clock) is written as a string that sorts in time
// order: milliseconds since the epoch (13 digits), a counter (4 hex
// digits) for events in the same millisecond, and the ID of the device
// that made it, which breaks ties between devices. Comparing two HLCs is
// comparing the strings. The client side lives in web/src/lib/sync/hlc.ts;
// shared/sync-vectors.json keeps the two in agreement.
var hlcPattern = regexp.MustCompile(`^(\d{13})-([0-9a-f]{4})-([A-Za-z0-9_-]{1,32})$`)

// maxClockSkew is how far ahead of the server's clock an HLC may be. A
// device with its clock set to next year would otherwise win every
// conflict until next year.
const maxClockSkew = time.Minute

// checkHLC validates an HLC's format and that it is not from the future.
func checkHLC(h string, now time.Time) error {
	m := hlcPattern.FindStringSubmatch(h)
	if m == nil {
		return fmt.Errorf("%q is not a valid HLC", h)
	}
	ms, _ := strconv.ParseInt(m[1], 10, 64)
	if time.UnixMilli(ms).After(now.Add(maxClockSkew)) {
		return fmt.Errorf("HLC %s is more than a minute ahead of the server; check this device's clock", h)
	}
	return nil
}

// hlcTime is when an HLC was made, or the zero time for none.
func hlcTime(h string) time.Time {
	if len(h) < 13 {
		return time.Time{}
	}
	ms, err := strconv.ParseInt(h[:13], 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}
