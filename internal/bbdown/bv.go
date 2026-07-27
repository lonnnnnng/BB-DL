package bbdown

import (
	"fmt"
	"strings"
)

const (
	xorCode  = int64(23442827791579)
	maskCode = int64((1 << 51) - 1)
	base     = int64(58)
	bvLen    = 9
)

var alphabet = []byte("FcwAPNKTMug3GV5Lj7EJnHpWsx4tb8haYeviqBz6rkCy12mUSDQX9RdoZf")
var reverseAlphabet = func() map[byte]int64 {
	m := make(map[byte]int64, len(alphabet))
	for i, b := range alphabet {
		m[b] = int64(i)
	}
	return m
}()

func EncodeBV(aid int64) (string, error) {
	if aid < 1 {
		return "", fmt.Errorf("av %d is smaller than 1", aid)
	}
	maxAid := maskCode + 1
	if aid >= maxAid {
		return "", fmt.Errorf("av %d is bigger than %d", aid, maxAid)
	}
	bvid := make([]byte, bvLen)
	tmp := (maxAid | aid) ^ xorCode
	for i := bvLen - 1; tmp != 0; i-- {
		bvid[i] = alphabet[tmp%base]
		tmp /= base
	}
	bvid[0], bvid[6] = bvid[6], bvid[0]
	bvid[1], bvid[4] = bvid[4], bvid[1]
	return "BV1" + string(bvid), nil
}

func DecodeBV(bv string) (int64, error) {
	bv = strings.TrimSpace(bv)
	lower := strings.ToLower(bv)
	switch {
	case strings.HasPrefix(lower, "bv1"):
		bv = bv[3:]
	case strings.HasPrefix(lower, "bv"):
		bv = bv[2:]
	}
	if len(bv) != bvLen {
		return 0, fmt.Errorf("bvid suffix %q must be 9 chars", bv)
	}
	buf := []byte(bv)
	buf[0], buf[6] = buf[6], buf[0]
	buf[1], buf[4] = buf[4], buf[1]
	var aid int64
	for _, b := range buf {
		v, ok := reverseAlphabet[b]
		if !ok {
			return 0, fmt.Errorf("invalid BV char: %q", b)
		}
		aid = aid*base + v
	}
	return (aid & maskCode) ^ xorCode, nil
}
