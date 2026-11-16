package syncer

import (
	"errors"
	"strings"
)

// The part of web/src/lib/sync/sortkey.ts a REST create needs: checking
// a key, and making one after the last. A key is an integer part, whose
// first character also says how long it is ('a' is two characters, 'b'
// three, 'Z' two below zero...), then an optional base-62 fraction with no
// trailing zero. shared/sortkey-vectors.json holds the two to the same
// answers.

const sortDigits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

var smallestInteger = "A" + strings.Repeat("0", 26)

var errSortKey = errors.New("is not a valid sort key")

func integerLength(head byte) (int, error) {
	switch {
	case head >= 'a' && head <= 'z':
		return int(head-'a') + 2, nil
	case head >= 'A' && head <= 'Z':
		return int('Z'-head) + 2, nil
	}
	return 0, errSortKey
}

func integerPart(key string) (string, error) {
	if key == "" {
		return "", errSortKey
	}
	n, err := integerLength(key[0])
	if err != nil || n > len(key) {
		return "", errSortKey
	}
	return key[:n], nil
}

// validateKey reports whether keyBetween in sortkey.ts would take the key.
func validateKey(key string) error {
	if key == smallestInteger {
		return errSortKey
	}
	ip, err := integerPart(key)
	if err != nil {
		return err
	}
	for i := 0; i < len(key); i++ {
		if strings.IndexByte(sortDigits, key[i]) < 0 {
			return errSortKey
		}
	}
	if fraction := key[len(ip):]; strings.HasSuffix(fraction, "0") {
		return errSortKey
	}
	return nil
}

// midpointAfter is sortkey.ts's midpoint(a, null).
func midpointAfter(a string) string {
	da := 0
	if a != "" {
		da = strings.IndexByte(sortDigits, a[0])
	}
	if len(sortDigits)-da > 1 {
		return string(sortDigits[(da+len(sortDigits)+1)/2])
	}
	return string(sortDigits[da]) + midpointAfter(a[min(1, len(a)):])
}

func incrementInteger(x string) (string, bool) {
	head, digits := x[0], []byte(x[1:])
	carry := true
	for i := len(digits) - 1; carry && i >= 0; i-- {
		d := strings.IndexByte(sortDigits, digits[i]) + 1
		if d == len(sortDigits) {
			digits[i] = '0'
		} else {
			digits[i] = sortDigits[d]
			carry = false
		}
	}
	if !carry {
		return string(head) + string(digits), true
	}
	if head == 'Z' {
		return "a0", true
	}
	if head == 'z' {
		return "", false
	}
	next := head + 1
	if next > 'a' {
		digits = append(digits, '0')
	} else {
		digits = digits[:len(digits)-1]
	}
	return string(next) + string(digits), true
}

// keyAfter is keyBetween(before, null): a key after before, or the first
// key of an empty list when before is "".
func keyAfter(before string) (string, error) {
	if before == "" {
		return "a0", nil
	}
	if err := validateKey(before); err != nil {
		return "", err
	}
	ip, _ := integerPart(before)
	if next, ok := incrementInteger(ip); ok {
		return next, nil
	}
	return ip + midpointAfter(before[len(ip):]), nil
}
