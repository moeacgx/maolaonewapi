package model

// walletQuotaValue lets legacy request paths keep using int while wallet and
// payment paths can pass int64 values on 32-bit builds.
type walletQuotaValue interface {
	~int | ~int64
}
