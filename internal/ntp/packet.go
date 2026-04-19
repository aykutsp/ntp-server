package ntp

import (
	"encoding/binary"
	"strings"
	"time"
)

const (
	packetLen      = 48
	ntpEpochOffset = 2208988800
)

type ResponseDefaults struct {
	ServeUnsynced    bool
	UnsyncedStratum  uint8
	LocalReferenceID [4]byte
	PrecisionExp     int8
	RootDelay        time.Duration
	RootDispersion   time.Duration
}

type SyncSnapshot struct {
	Synced         bool
	Stratum        uint8
	ReferenceID    [4]byte
	ReferenceTime  time.Time
	Offset         time.Duration
	RootDelay      time.Duration
	RootDispersion time.Duration
	Upstream       string
	LastError      string
}

func IsClientRequest(pkt []byte) bool {
	if len(pkt) < packetLen {
		return false
	}
	mode := pkt[0] & 0x7
	version := (pkt[0] >> 3) & 0x7
	return mode == 3 && version >= 3 && version <= 4
}

func BuildResponse(req []byte, receivedAt time.Time, transmitAt time.Time, sync SyncSnapshot, defaults ResponseDefaults) [packetLen]byte {
	var out [packetLen]byte

	version := ntpVersion(req)
	li := uint8(0)
	stratum := sync.Stratum
	referenceID := sync.ReferenceID
	referenceTime := sync.ReferenceTime
	rootDelay := sync.RootDelay
	rootDispersion := sync.RootDispersion

	if !sync.Synced {
		li = 3 // alarm condition
		stratum = defaults.UnsyncedStratum
		referenceID = defaults.LocalReferenceID
		referenceTime = transmitAt
		rootDelay = defaults.RootDelay
		rootDispersion = defaults.RootDispersion
	}

	out[0] = (li << 6) | (version << 3) | 4 // server mode
	out[1] = stratum
	out[2] = req[2] // echo poll interval
	out[3] = byte(defaults.PrecisionExp)

	binary.BigEndian.PutUint32(out[4:8], durationToShort(rootDelay))
	binary.BigEndian.PutUint32(out[8:12], durationToShort(rootDispersion))
	copy(out[12:16], referenceID[:])

	binary.BigEndian.PutUint64(out[16:24], ToTimestamp(referenceTime))
	copy(out[24:32], req[40:48]) // originate timestamp == client transmit
	binary.BigEndian.PutUint64(out[32:40], ToTimestamp(receivedAt))
	binary.BigEndian.PutUint64(out[40:48], ToTimestamp(transmitAt))

	return out
}

func BuildKissOfDeath(req []byte, receivedAt time.Time, transmitAt time.Time, code string) [packetLen]byte {
	var out [packetLen]byte
	version := ntpVersion(req)
	out[0] = (3 << 6) | (version << 3) | 4 // LI=alarm, mode=server
	out[1] = 0
	out[2] = req[2]
	out[3] = 0xEC

	ref := ReferenceID(code)
	copy(out[12:16], ref[:])
	copy(out[24:32], req[40:48])
	binary.BigEndian.PutUint64(out[32:40], ToTimestamp(receivedAt))
	binary.BigEndian.PutUint64(out[40:48], ToTimestamp(transmitAt))
	return out
}

func ntpVersion(req []byte) uint8 {
	if len(req) < 1 {
		return 4
	}
	v := (req[0] >> 3) & 0x7
	if v < 3 || v > 4 {
		return 4
	}
	return v
}

func ToTimestamp(t time.Time) uint64 {
	t = t.UTC()
	sec := uint64(t.Unix() + ntpEpochOffset)
	frac := uint64((float64(t.Nanosecond()) / 1e9) * (1 << 32))
	return (sec << 32) | frac
}

func FromTimestamp(ts uint64) time.Time {
	sec := int64(ts>>32) - ntpEpochOffset
	frac := ts & 0xffffffff
	nsec := (int64(frac) * 1e9) >> 32
	return time.Unix(sec, nsec).UTC()
}

func ReadTimestamp(pkt []byte, start int) time.Time {
	if len(pkt) < start+8 {
		return time.Time{}
	}
	return FromTimestamp(binary.BigEndian.Uint64(pkt[start : start+8]))
}

func durationToShort(d time.Duration) uint32 {
	if d < 0 {
		d = 0
	}
	sec := float64(d) / float64(time.Second)
	return uint32(sec * 65536.0)
}

func shortToDuration(v uint32) time.Duration {
	sec := float64(v) / 65536.0
	return time.Duration(sec * float64(time.Second))
}

func ReferenceID(ref string) [4]byte {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		ref = "LOCL"
	}
	var out [4]byte
	copy(out[:], []byte(ref))
	return out
}
