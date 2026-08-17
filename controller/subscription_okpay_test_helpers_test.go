package controller

import (
	"crypto/md5"
	"fmt"
	"strings"
)

func resetOkpayRateCacheForTest() {
	okpayRateCacheMu.Lock()
	okpayRateCache = okpayRateCacheEntry{}
	okpayRateCacheMu.Unlock()
}

func okpaySignatureForTest(raw string, token string) string {
	sum := md5.Sum([]byte(raw + "&token=" + token))
	return strings.ToUpper(fmt.Sprintf("%x", sum))
}
