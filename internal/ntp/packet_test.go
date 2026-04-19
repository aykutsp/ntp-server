package ntp

import (
	"testing"
	"time"
)

func TestTimestampRoundtrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	ts := ToTimestamp(now)
	got := FromTimestamp(ts)

	delta := got.Sub(now)
	if delta < 0 {
		delta = -delta
	}
	if delta > time.Millisecond {
		t.Fatalf("timestamp roundtrip drift too large: %s", delta)
	}
}

func TestBuildResponse(t *testing.T) {
	var req [48]byte
	req[0] = (0 << 6) | (4 << 3) | 3 // client mode
	req[2] = 6
	req[3] = 0xEC
	reqTransmit := time.Now().UTC().Add(-5 * time.Millisecond)
	copy(req[40:48], u64Bytes(ToTimestamp(reqTransmit)))

	sync := SyncSnapshot{
		Synced:         true,
		Stratum:        2,
		ReferenceID:    ReferenceID("GPS"),
		ReferenceTime:  time.Now().UTC().Add(-2 * time.Second),
		Offset:         0,
		RootDelay:      2 * time.Millisecond,
		RootDispersion: 1 * time.Millisecond,
	}
	defaults := ResponseDefaults{
		ServeUnsynced:    true,
		UnsyncedStratum:  16,
		LocalReferenceID: ReferenceID("LOCL"),
		PrecisionExp:     -20,
	}

	resp := BuildResponse(req[:], time.Now().UTC(), time.Now().UTC(), sync, defaults)
	if mode := resp[0] & 0x7; mode != 4 {
		t.Fatalf("expected mode=4, got %d", mode)
	}
	if resp[1] != 2 {
		t.Fatalf("expected stratum 2, got %d", resp[1])
	}
}

func u64Bytes(v uint64) []byte {
	return []byte{
		byte(v >> 56), byte(v >> 48), byte(v >> 40), byte(v >> 32),
		byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v),
	}
}
